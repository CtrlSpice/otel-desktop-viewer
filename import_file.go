package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
)

const importRequestBytes int64 = 20 * 1024 * 1024

var importResourceKeys = []string{"resourceSpans", "resourceLogs", "resourceMetrics"}
var importScopeKeys = []string{"scopeSpans", "scopeLogs", "scopeMetrics"}
var importRecordKeys = []string{"spans", "logRecords", "metrics"}
var importSignals = []string{"traces", "logs", "metrics"}

type importRange struct {
	start, end  int64
	collections []*importCollection
}

type importCollection struct {
	start, end int64
	items      []*importRange
	splittable bool
}

type importPlan struct {
	resources [3][]*importRange
	issues    []string
}

type importFrame struct {
	object       bool
	role, key    string
	expectingKey bool
	signal       int
	node         *importRange
	collection   *importCollection
}

// The decoder validates syntax once; telemetry tokens are never encoded again.
func scanImportFile(ctx context.Context, source io.ReaderAt, size int64) (importPlan, error) {
	var plan importPlan
	base := int64(0)
	var prefix [3]byte
	if n, _ := source.ReadAt(prefix[:], 0); n == 3 && bytes.Equal(prefix[:], []byte{0xef, 0xbb, 0xbf}) {
		base = 3
	}
	decoder := json.NewDecoder(io.NewSectionReader(source, base, size-base))
	decoder.UseNumber()
	var frames []*importFrame
	wrappers := 0
	lastRootEnd := base
	for {
		if err := ctx.Err(); err != nil {
			return plan, err
		}
		before := decoder.InputOffset() + base
		token, err := decoder.Token()
		if err == io.EOF {
			if len(frames) != 0 {
				return plan, fmt.Errorf("invalid JSON: incomplete input near byte %d", size+1)
			}
			break
		}
		if err != nil {
			return plan, fmt.Errorf("invalid JSON near byte %d: %w", decoder.InputOffset()+base+1, err)
		}
		end := decoder.InputOffset() + base
		start, err := importTokenStart(source, before, end)
		if err != nil {
			return plan, err
		}
		var parent *importFrame
		if len(frames) > 0 {
			parent = frames[len(frames)-1]
		}
		if parent != nil && parent.object && parent.expectingKey {
			if key, ok := token.(string); ok {
				parent.key = key
				parent.expectingKey = false
				continue
			}
		}
		delim, isDelim := token.(json.Delim)
		if isDelim && (delim == '}' || delim == ']') {
			closed := frames[len(frames)-1]
			frames = frames[:len(frames)-1]
			if closed.node != nil && closed.role != "metricData" {
				closed.node.end = end
			}
			if closed.collection != nil {
				closed.collection.end = end
			}
			if len(frames) == 0 {
				lastRootEnd = end
			}
			continue
		}
		object, array := isDelim && delim == '{', isDelim && delim == '['
		frame := &importFrame{object: object, expectingKey: object, role: "opaque", signal: -1}
		if parent == nil {
			if !object {
				return plan, fmt.Errorf("OTLP input must contain JSON objects (byte %d)", start+1)
			}
			if lastRootEnd > base {
				var separator [256]byte
				hasNewline := false
				for position := lastRootEnd; position < start && !hasNewline; {
					if err := ctx.Err(); err != nil {
						return plan, err
					}
					n := min(int64(len(separator)), start-position)
					if _, err := source.ReadAt(separator[:n], position); err != nil {
						return plan, err
					}
					hasNewline = bytes.Contains(separator[:n], []byte{'\n'})
					position += n
				}
				if !hasNewline {
					return plan, fmt.Errorf("JSONL objects must be separated by a newline (byte %d)", start+1)
				}
			}
			frame.role = "root"
		} else {
			frame.signal = parent.signal
			parent.expectingKey = parent.object
			switch {
			case parent.role == "root":
				wrappers++
				for i, key := range importResourceKeys {
					if parent.key == key {
						frame.signal = i
					}
				}
				if parent.key == "resourceProfiles" {
					plan.issues = append(plan.issues, fmt.Sprintf("Profiles support coming soon (byte %d)", start+1))
				} else if frame.signal < 0 {
					plan.issues = append(plan.issues, fmt.Sprintf("Invalid OTLP wrapper: %s (byte %d)", parent.key, start+1))
				} else if !array {
					plan.issues = append(plan.issues, fmt.Sprintf("%s must be an array (byte %d)", parent.key, start+1))
				} else {
					frame.role = "resources"
				}
			case parent.role == "resources" && frame.signal >= 0:
				if object {
					frame.role = "resource"
					frame.node = &importRange{start: start}
					plan.resources[frame.signal] = append(plan.resources[frame.signal], frame.node)
				} else {
					plan.issues = append(plan.issues, fmt.Sprintf("%s must contain resource objects (byte %d)", importResourceKeys[frame.signal], start+1))
				}
			case !parent.object && parent.collection != nil:
				if object {
					frame.role = "leaf"
					if parent.role == "scopes" {
						frame.role = "scope"
					}
					if parent.role == "records" && frame.signal == 2 {
						frame.role = "metric"
					}
					frame.node = &importRange{start: start}
					parent.collection.items = append(parent.collection.items, frame.node)
				} else {
					parent.collection.splittable = false
				}
			case parent.node != nil && frame.signal >= 0:
				if array && parent.role == "resource" && parent.key == importScopeKeys[frame.signal] {
					frame.role = "scopes"
				}
				if array && parent.role == "scope" && parent.key == importRecordKeys[frame.signal] {
					frame.role = "records"
				}
				if array && parent.role == "metricData" && parent.key == "dataPoints" {
					frame.role = "points"
				}
				if object && parent.role == "metric" {
					switch parent.key {
					case "gauge", "sum", "histogram", "exponentialHistogram", "summary":
						frame.role = "metricData"
						frame.node = parent.node
					}
				}
				if frame.role == "scopes" || frame.role == "records" || frame.role == "points" {
					frame.collection = &importCollection{start: start, splittable: true}
					parent.node.collections = append(parent.node.collections, frame.collection)
				}
			}
		}
		if object || array {
			frames = append(frames, frame)
		}
	}
	if wrappers == 0 {
		plan.issues = append(plan.issues, "No OTLP resource wrappers found (byte 1)")
	}
	return plan, nil
}

func importTokenStart(source io.ReaderAt, start, end int64) (int64, error) {
	var buffer [256]byte
	for start < end {
		n := min(int64(len(buffer)), end-start)
		if _, err := source.ReadAt(buffer[:n], start); err != nil {
			return 0, err
		}
		for _, b := range buffer[:n] {
			switch b {
			case ' ', '\t', '\r', '\n', ',', ':':
				start++
			default:
				return start, nil
			}
		}
	}
	return start, nil
}

type importPiece struct {
	parts []importRange
	size  int64
}

// Only known collection arrays may be replaced. Everything around them is copied.
func splitImportRange(ctx context.Context, node *importRange, budget int64, emit func(importPiece) error, issue func(string)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if node.end-node.start <= budget {
		return emit(importPiece{parts: []importRange{{start: node.start, end: node.end}}, size: node.end - node.start})
	}
	if len(node.collections) != 1 {
		issue(fmt.Sprintf("An individual OTLP record or its metadata exceeds the 20 MiB request limit (byte %d)", node.start+1))
		return nil
	}
	list := node.collections[0]
	if !list.splittable {
		issue(fmt.Sprintf("Invalid OTLP collection cannot be split into requests (byte %d)", list.start+1))
		return nil
	}
	before, after := importRange{start: node.start, end: list.start + 1}, importRange{start: list.end - 1, end: node.end}
	innerBudget := budget - (before.end - before.start) - (after.end - after.start)
	if innerBudget <= 0 || len(list.items) == 0 {
		issue(fmt.Sprintf("OTLP resource or scope metadata exceeds the 20 MiB request limit (byte %d)", node.start+1))
		return nil
	}
	var pending importPiece
	flush := func() error {
		if len(pending.parts) == 0 {
			return nil
		}
		piece := importPiece{parts: append([]importRange{before}, pending.parts...), size: pending.size + before.end - before.start + after.end - after.start}
		piece.parts = append(piece.parts, after)
		pending = importPiece{}
		return emit(piece)
	}
	for _, item := range list.items {
		if err := splitImportRange(ctx, item, innerBudget, func(piece importPiece) error {
			separator := int64(0)
			if len(pending.parts) > 0 {
				separator = 1
			}
			if pending.size+separator+piece.size > innerBudget {
				if err := flush(); err != nil {
					return err
				}
				separator = 0
			}
			if separator > 0 {
				pending.parts = append(pending.parts, importRange{start: -1, end: 0})
			}
			pending.parts = append(pending.parts, piece.parts...)
			pending.size += separator + piece.size
			return nil
		}, issue); err != nil {
			return err
		}
	}
	return flush()
}

func batchImportFile(ctx context.Context, source io.ReaderAt, signal int, resources []*importRange, maximum int64, send func(io.Reader, int64) error, issue func(string)) error {
	opening := `{"` + importResourceKeys[signal] + `":[`
	closing := `]}`
	budget := maximum - int64(len(opening)+len(closing))
	var pending importPiece
	flush := func() error {
		if len(pending.parts) == 0 {
			return nil
		}
		readers := []io.Reader{bytes.NewBufferString(opening)}
		for _, part := range pending.parts {
			if part.start < 0 {
				readers = append(readers, bytes.NewBufferString(","))
			} else {
				readers = append(readers, io.NewSectionReader(source, part.start, part.end-part.start))
			}
		}
		readers = append(readers, bytes.NewBufferString(closing))
		size := pending.size + int64(len(opening)+len(closing))
		pending = importPiece{}
		return send(io.MultiReader(readers...), size)
	}
	for _, resource := range resources {
		if err := splitImportRange(ctx, resource, budget, func(piece importPiece) error {
			separator := int64(0)
			if len(pending.parts) > 0 {
				separator = 1
			}
			if pending.size+separator+piece.size > budget {
				if err := flush(); err != nil {
					return err
				}
				separator = 0
			}
			if separator > 0 {
				pending.parts = append(pending.parts, importRange{start: -1})
			}
			pending.parts = append(pending.parts, piece.parts...)
			pending.size += separator + piece.size
			return nil
		}, issue); err != nil {
			return err
		}
	}
	return flush()
}
