package evals

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode/utf8"
)

const MaxRecordBytes = 4 << 20

// IngestPart is a bounded staging record. Row -1 contains the envelope context,
// with its result array replaced by []. Other rows retain producer order.
type IngestPart struct {
	Envelope int             `json:"envelope"`
	Row      int64           `json:"row"`
	Payload  json.RawMessage `json:"payload"`
	Format   string          `json:"format,omitempty"`
	Count    int64           `json:"count,omitempty"`
	Bytes    int64           `json:"bytes,omitempty"`
}

type envelopeReader struct {
	r        *bufio.Reader
	ctx      context.Context
	envelope int
	count    int64
	bytes    int64
	emit     func(IngestPart) error
}

func (p *envelopeReader) space() (bool, error) {
	newline := false
	for {
		if err := p.ctx.Err(); err != nil {
			return newline, err
		}
		b, err := p.r.Peek(1)
		if err != nil {
			return newline, err
		}
		if !bytes.ContainsRune([]byte(" \t\r\n"), rune(b[0])) {
			return newline, nil
		}
		newline = newline || b[0] == '\n'
		_, _ = p.r.ReadByte()
	}
}

func (p *envelopeReader) expect(want byte) error {
	if _, err := p.space(); err != nil {
		return err
	}
	b, err := p.r.ReadByte()
	if err != nil {
		return err
	}
	if b != want {
		return fmt.Errorf("expected %q, received %q", want, b)
	}
	return nil
}

// raw bounds memory before consuming a value, including a huge individual token.
// Syntax and Unicode validation run on the bounded value before staging it.
func (p *envelopeReader) raw() ([]byte, error) {
	if _, err := p.space(); err != nil {
		return nil, err
	}
	var out []byte
	depth, quoted, escaped := 0, false, false
	for {
		if len(out)%65536 == 0 {
			if err := p.ctx.Err(); err != nil {
				return nil, err
			}
		}
		b, err := p.r.Peek(1)
		if err == io.EOF && len(out) > 0 {
			break
		}
		if err != nil {
			return nil, err
		}
		c := b[0]
		if len(out) > 0 && !quoted && depth == 0 && strings.ContainsRune(" \t\r\n,}]", rune(c)) {
			break
		}
		if len(out) == MaxRecordBytes {
			return nil, fmt.Errorf("an individual result or metadata value exceeds 4 MiB")
		}
		_, _ = p.r.ReadByte()
		out = append(out, c)
		if quoted {
			if escaped {
				escaped = false
			} else if c == '\\' {
				escaped = true
			} else if c == '"' {
				quoted = false
				if depth == 0 {
					break
				}
			}
		} else {
			switch c {
			case '"':
				quoted = true
			case '{', '[':
				depth++
			case '}', ']':
				depth--
				if depth == 0 {
					goto done
				}
			}
		}
	}
done:
	if err := validateIngestJSON(out); err != nil {
		return nil, err
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, out); err != nil {
		return nil, err
	}
	return compact.Bytes(), nil
}

func validateIngestJSON(raw []byte) error {
	if !utf8.Valid(raw) || !json.Valid(raw) {
		return fmt.Errorf("invalid UTF-8 JSON value")
	}
	for i := 0; i < len(raw); i++ {
		if raw[i] != '\\' {
			continue
		}
		if raw[i+1] != 'u' {
			i++
			continue
		}
		u, _ := strconv.ParseUint(string(raw[i+2:i+6]), 16, 16)
		i += 5
		if u >= 0xd800 && u <= 0xdbff {
			if len(raw)-i-1 < 6 || raw[i+1] != '\\' || raw[i+2] != 'u' {
				return fmt.Errorf("unpaired Unicode surrogate")
			}
			low, _ := strconv.ParseUint(string(raw[i+3:i+7]), 16, 16)
			if low < 0xdc00 || low > 0xdfff {
				return fmt.Errorf("unpaired Unicode surrogate")
			}
			i += 6
		} else if u >= 0xdc00 && u <= 0xdfff {
			return fmt.Errorf("unpaired Unicode surrogate")
		}
	}
	// Reject duplicate decoded keys instead of silently choosing one meaning.
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	var visit func(int) error
	visit = func(depth int) error {
		if depth > 64 {
			return fmt.Errorf("JSON nesting exceeds 64 levels")
		}
		t, err := d.Token()
		if err != nil {
			return err
		}
		delim, ok := t.(json.Delim)
		if !ok {
			return nil
		}
		seen := map[string]bool{}
		for d.More() {
			if delim == '{' {
				k, err := d.Token()
				if err != nil {
					return err
				}
				key := k.(string)
				if seen[key] {
					return fmt.Errorf("duplicate JSON key %q", key)
				}
				seen[key] = true
			}
			if err := visit(depth + 1); err != nil {
				return err
			}
		}
		_, err = d.Token()
		return err
	}
	return visit(0)
}

func (p *envelopeReader) rows() error {
	if err := p.expect('['); err != nil {
		return err
	}
	if _, err := p.space(); err != nil {
		return err
	}
	if b, _ := p.r.Peek(1); b[0] == ']' {
		_, _ = p.r.ReadByte()
		return nil
	}
	for {
		raw, err := p.raw()
		if err != nil {
			return err
		}
		if raw[0] != '{' {
			return fmt.Errorf("result row must be an object")
		}
		if err := p.emit(IngestPart{Envelope: p.envelope, Row: p.count, Payload: raw}); err != nil {
			return err
		}
		p.count++
		p.bytes += int64(len(raw))
		if p.count > 1 {
			p.bytes++
		}
		if _, err := p.space(); err != nil {
			return err
		}
		b, _ := p.r.ReadByte()
		if b == ']' {
			return nil
		}
		if b != ',' {
			return fmt.Errorf("expected result separator")
		}
	}
}

func (p *envelopeReader) object(path string) ([]byte, error) {
	if err := p.expect('{'); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	out.WriteByte('{')
	seen := map[string]bool{}
	if _, err := p.space(); err != nil {
		return nil, err
	}
	if b, _ := p.r.Peek(1); b[0] == '}' {
		_, _ = p.r.ReadByte()
		return []byte("{}"), nil
	}
	for {
		keyRaw, err := p.raw()
		if err != nil {
			return nil, err
		}
		var key string
		if err := json.Unmarshal(keyRaw, &key); err != nil {
			return nil, fmt.Errorf("object key must be text")
		}
		if seen[key] {
			return nil, fmt.Errorf("duplicate JSON key %q", key)
		}
		seen[key] = true
		if err := p.expect(':'); err != nil {
			return nil, err
		}
		child := path + "/" + strings.ReplaceAll(strings.ReplaceAll(key, "~", "~0"), "/", "~1")
		var value []byte
		switch child {
		case "/results/results":
			err = p.rows()
			value = []byte("[]")
		case "/results":
			value, err = p.object(child)
		default:
			value, err = p.raw()
		}
		if err != nil {
			return nil, err
		}
		if out.Len()+len(keyRaw)+len(value)+3 > MaxRecordBytes {
			return nil, fmt.Errorf("envelope metadata exceeds 4 MiB")
		}
		if out.Len() > 1 {
			out.WriteByte(',')
		}
		out.Write(keyRaw)
		out.WriteByte(':')
		out.Write(value)
		if _, err := p.space(); err != nil {
			return nil, err
		}
		b, _ := p.r.ReadByte()
		if b == '}' {
			out.WriteByte('}')
			return out.Bytes(), nil
		}
		if b != ',' {
			return nil, fmt.Errorf("expected object separator")
		}
	}
}

func envelopeFormat(raw []byte) (string, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return "", err
	}
	for _, key := range []string{"resourceSpans", "resourceLogs", "resourceMetrics", "resourceProfiles"} {
		if _, ok := root[key]; ok {
			return "", fmt.Errorf("eval envelope cannot contain OTLP wrappers")
		}
	}
	var id string
	var config map[string]json.RawMessage
	if json.Unmarshal(root["evalId"], &id) != nil || id == "" || json.Unmarshal(root["config"], &config) != nil || config == nil {
		return "", fmt.Errorf("Promptfoo requires evalId, config and results in the full export; bare rows lack run context")
	}
	summaryRaw := root["results"]
	var summary map[string]json.RawMessage
	if json.Unmarshal(summaryRaw, &summary) != nil || summary == nil || string(summary["results"]) != "[]" {
		return "", fmt.Errorf("envelope requires results.results array")
	}
	version := string(summary["version"])
	if version != "3" {
		return "", fmt.Errorf("only results.version 3 is supported")
	}
	return "promptfoo-v3", nil
}

// ParseIngest stages complete independent producer envelopes, separated by LF.
// No database writes occur here. The caller must discard staging on any error.
func ParseIngest(ctx context.Context, input io.Reader, emit func(IngestPart) error) error {
	p := envelopeReader{r: bufio.NewReader(input), ctx: ctx, emit: emit}
	if prefix, _ := p.r.Peek(3); bytes.Equal(prefix, []byte{0xef, 0xbb, 0xbf}) {
		_, _ = p.r.Discard(3)
	}
	for {
		newline, err := p.space()
		if err == io.EOF && p.envelope > 0 {
			return nil
		}
		if err != nil {
			return err
		}
		if p.envelope > 0 && !newline {
			return fmt.Errorf("JSONL envelopes must be separated by a newline")
		}
		p.count = 0
		p.bytes = 0
		if p.envelope >= 1000 {
			return fmt.Errorf("a file may contain at most 1000 independent envelopes")
		}
		raw, err := p.object("")
		if err != nil {
			return err
		}
		format, err := envelopeFormat(raw)
		if err != nil {
			return err
		}
		if err := emit(IngestPart{Envelope: p.envelope, Row: -1, Payload: raw, Format: format, Count: p.count, Bytes: p.bytes + int64(len(raw))}); err != nil {
			return err
		}
		p.envelope++
	}
}
