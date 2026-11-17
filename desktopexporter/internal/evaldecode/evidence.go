package evaldecode

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

const (
	ServiceName   = "otel-desktop-viewer"
	ScopeName     = "otel-desktop-viewer.evaldecode"
	EvidenceEvent = "viewer.eval.import.v1"
)

// Number paths form a sparse trie, not one repeated full pointer per number.
// This bounds metadata by source size even beneath long shared object keys.
// Object edges are unpadded base64url key segments prefixed with "k"; arrays
// address children by position and leaves are true. Encoding object keys keeps
// secret-key rules from masking type-only
// metadata. No source value is copied here. The hash detects processor damage.
func pathKey(s string) string { return "k" + base64.RawURLEncoding.EncodeToString([]byte(s)) }

func Encode(importID string, part IngestPart) (plog.Logs, error) {
	out := plog.NewLogs()
	if part.Format != "" && part.Format != "promptfoo-v3" {
		return out, fmt.Errorf("unsupported producer format")
	}
	if !validID(importID) {
		return out, fmt.Errorf("invalid import identity")
	}
	if len(part.Payload) > MaxRecordBytes {
		return out, fmt.Errorf("source part exceeds 4 MiB")
	}
	depth := 64
	if part.Row == -1 {
		depth = 66
	} // envelope/results surround bounded opaque metadata
	if err := validateJSONDepth(part.Payload, depth); err != nil {
		return out, err
	}
	d := json.NewDecoder(bytes.NewReader(part.Payload))
	d.UseNumber()
	var source any
	if err := d.Decode(&source); err != nil {
		return out, err
	}
	value, numbers := encodeSource(source)
	numericJSON, _ := json.Marshal(numbers)
	rl := out.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", ServiceName)
	sl := rl.ScopeLogs().AppendEmpty()
	sl.Scope().SetName(ScopeName)
	sl.Scope().SetVersion("1")
	l := sl.LogRecords().AppendEmpty()
	l.SetEventName(EvidenceEvent)
	// Historical observations have no inferred event time, interval or IDs.
	// The caller records its actual conversion/forwarding time as observed time.
	b := l.Body().SetEmptyMap()
	b.PutStr("version", "1")
	b.PutStr("producer", "promptfoo-v3")
	b.PutStr("importID", importID)
	b.PutStr("envelope", strconv.Itoa(part.Envelope))
	b.PutStr("ordinal", strconv.FormatInt(part.Row, 10))
	b.PutStr("count", strconv.FormatInt(part.Count, 10))
	b.PutStr("numberPathsHash", fmt.Sprintf("%x", sha256.Sum256(numericJSON)))
	if err := b.PutEmpty("numberPaths").FromRaw(numbers); err != nil {
		return out, err
	}
	if err := b.PutEmpty("source").FromRaw(value); err != nil {
		return out, err
	}
	return out, nil
}

func encodeSource(v any) (any, any) {
	switch x := v.(type) {
	case json.Number:
		return x.String(), true
	case map[string]any:
		out, paths := map[string]any{}, map[string]any{}
		for k, value := range x {
			child, numbers := encodeSource(value)
			out[k] = child
			if numbers != nil {
				paths[pathKey(k)] = numbers
			}
		}
		if len(paths) == 0 {
			return out, nil
		}
		return out, paths
	case []any:
		out, paths := make([]any, len(x)), make([]any, len(x))
		found := false
		for i, value := range x {
			child, numbers := encodeSource(value)
			out[i] = child
			if numbers != nil {
				paths[i] = numbers
				found = true
			}
		}
		if !found {
			return out, nil
		}
		return out, paths
	default:
		return v, nil
	}
}

func validID(id string) bool { return len(id) == 64 && strings.Trim(id, "0123456789abcdef") == "" }

// ImportID can associate damaged evidence with its transfer so that it prevents
// publication. It does not make the rest of that record trustworthy.
func ImportID(log plog.LogRecord) string {
	if log.Body().Type() != pcommon.ValueTypeMap {
		return ""
	}
	v, ok := log.Body().Map().Get("importID")
	if !ok || v.Type() != pcommon.ValueTypeStr || !validID(v.Str()) {
		return ""
	}
	return v.Str()
}

func Decode(log plog.LogRecord) (IngestPart, error) {
	var part IngestPart
	if log.EventName() != EvidenceEvent || ImportID(log) == "" || !log.TraceID().IsEmpty() || !log.SpanID().IsEmpty() {
		return part, fmt.Errorf("invalid evaluation observation identity")
	}
	b := log.Body().Map()
	text := func(key string) string {
		v, ok := b.Get(key)
		if !ok || v.Type() != pcommon.ValueTypeStr {
			return ""
		}
		return v.Str()
	}
	if b.Len() != 9 || text("version") != "1" || text("producer") != "promptfoo-v3" {
		return part, fmt.Errorf("unsupported evidence encoding")
	}
	number := func(key string) (int64, error) {
		s := text(key)
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil || strconv.FormatInt(n, 10) != s {
			return 0, fmt.Errorf("invalid %s", key)
		}
		return n, nil
	}
	env, err := number("envelope")
	if err != nil {
		return part, err
	}
	part.Row, err = number("ordinal")
	if err != nil {
		return part, err
	}
	part.Count, err = number("count")
	if err != nil {
		return part, err
	}
	if env < -1 || env >= 1000 || part.Row < -2 || part.Count < 0 {
		return part, fmt.Errorf("invalid observation coordinates")
	}
	part.Envelope = int(env)
	manifest := env == -1 && part.Row == -2
	if !manifest && (env < 0 || part.Row < -1) || manifest && (part.Count < 1 || part.Count > 1000) || part.Row >= 0 && part.Count != 0 {
		return part, fmt.Errorf("invalid observation kind")
	}
	numbers, ok := b.Get("numberPaths")
	if !ok {
		return part, fmt.Errorf("missing number paths")
	}
	paths := numbers.AsRaw()
	if err := validateNumberPaths(paths, 0); err != nil {
		return part, err
	}
	numericJSON, err := json.Marshal(paths)
	if err != nil || fmt.Sprintf("%x", sha256.Sum256(numericJSON)) != text("numberPathsHash") {
		return part, fmt.Errorf("number paths changed in processing")
	}
	source, ok := b.Get("source")
	if !ok {
		return part, fmt.Errorf("missing source")
	}
	restored, err := restoreSource(source.AsRaw(), paths, 0)
	if err != nil {
		return part, err
	}
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(restored); err != nil {
		return part, err
	}
	part.Payload = bytes.TrimSuffix(encoded.Bytes(), []byte{'\n'})
	// Canonical JSON escaping can expand source strings. The upload limit stays
	// 4 MiB; reconstruction permits the bounded worst-case escaping expansion.
	if len(part.Payload) > 6*MaxRecordBytes {
		return part, fmt.Errorf("processed source part exceeds reconstruction bound")
	}
	depth := 64
	if part.Row == -1 {
		depth = 66
	}
	if err := validateJSONDepth(part.Payload, depth); err != nil {
		return part, err
	}
	if _, ok := restored.(map[string]any); !ok {
		return part, fmt.Errorf("source must be an object")
	}
	part.Format = "promptfoo-v3"
	part.Bytes = int64(len(part.Payload))
	if part.Row == -1 {
		if _, err := Detect(part.Payload); err != nil {
			return part, err
		}
	}
	return part, nil
}

func restoreSource(v, paths any, depth int) (any, error) {
	if depth > 66 {
		return nil, fmt.Errorf("source nesting exceeds envelope bound")
	}
	if marker, ok := paths.(bool); ok {
		if !marker {
			return nil, fmt.Errorf("invalid number marker")
		}
		s, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("numeric source is not a string")
		}
		if s == "****" {
			return s, nil
		}
		if !json.Valid([]byte(s)) || len(s) == 0 || (s[0] != '-' && (s[0] < '0' || s[0] > '9')) || strings.TrimSpace(s) != s {
			return nil, fmt.Errorf("processed numeric lexeme is invalid")
		}
		return json.Number(s), nil
	}
	// A secret-key match can replace a whole source subtree. Nothing beneath
	// that deliberate redaction may be reconstructed from type metadata.
	if s, ok := v.(string); ok && s == "****" {
		return s, nil
	}
	switch x := v.(type) {
	case map[string]any:
		var children map[string]any
		if paths != nil {
			var ok bool
			children, ok = paths.(map[string]any)
			if !ok || len(children) == 0 {
				return nil, fmt.Errorf("invalid object number paths")
			}
		}
		used := 0
		for key, value := range x {
			if !utf8.ValidString(key) {
				return nil, fmt.Errorf("invalid UTF-8 source key")
			}
			p, present := children[pathKey(key)]
			if present {
				used++
			}
			y, err := restoreSource(value, p, depth+1)
			if err != nil {
				return nil, err
			}
			x[key] = y
		}
		if used != len(children) {
			return nil, fmt.Errorf("numeric path does not address retained source")
		}
	case []any:
		var children []any
		if paths != nil {
			var ok bool
			children, ok = paths.([]any)
			if !ok || len(children) != len(x) {
				return nil, fmt.Errorf("invalid array number paths")
			}
		}
		for i, value := range x {
			var p any
			if children != nil {
				p = children[i]
			}
			y, err := restoreSource(value, p, depth+1)
			if err != nil {
				return nil, err
			}
			x[i] = y
		}
	case string:
		if !utf8.ValidString(x) {
			return nil, fmt.Errorf("invalid UTF-8 source string")
		}
		if paths != nil {
			return nil, fmt.Errorf("numeric path does not address retained source")
		}
	case nil, bool:
		if paths != nil {
			return nil, fmt.Errorf("numeric path does not address retained source")
		}
	default:
		return nil, fmt.Errorf("unexpected source scalar type")
	}
	return v, nil
}

func validateNumberPaths(paths any, depth int) error {
	if depth > 66 {
		return fmt.Errorf("number paths exceed nesting bound")
	}
	switch p := paths.(type) {
	case nil:
		return nil
	case bool:
		if p {
			return nil
		}
	case map[string]any:
		if len(p) == 0 {
			break
		}
		for key, child := range p {
			if !strings.HasPrefix(key, "k") || child == nil {
				return fmt.Errorf("invalid number path edge")
			}
			decoded, err := base64.RawURLEncoding.DecodeString(key[1:])
			if err != nil || !utf8.Valid(decoded) || pathKey(string(decoded)) != key {
				return fmt.Errorf("invalid number path key")
			}
			if err := validateNumberPaths(child, depth+1); err != nil {
				return err
			}
		}
		return nil
	case []any:
		found := false
		for _, child := range p {
			if child != nil {
				found = true
			}
			if err := validateNumberPaths(child, depth+1); err != nil {
				return err
			}
		}
		if found {
			return nil
		}
	}
	return fmt.Errorf("invalid number path metadata")
}
