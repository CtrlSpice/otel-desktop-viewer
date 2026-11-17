// Package evaldecode converts producer files into historical observations.
// It has no database or network access. Callers must validate the complete input
// before forwarding any observation to the Collector.
package evaldecode

import (
	"context"
	"encoding/json"
	"io"
)

const MaxRecordBytes = 4 << 20

// IngestPart is a bounded staging record. Row -1 contains envelope context with
// results replaced by []; nonnegative rows retain producer order. The transport
// uses envelope -1 / row -2 exclusively for its file completion manifest.
type IngestPart struct {
	Envelope int             `json:"envelope"`
	Row      int64           `json:"row"`
	Payload  json.RawMessage `json:"payload"`
	Format   string          `json:"format,omitempty"`
	Count    int64           `json:"count,omitempty"`
	Bytes    int64           `json:"bytes,omitempty"`
}

// ParseIngest emits bounded staging parts, not persistent evidence. The caller
// must discard every emitted part if any later input fails validation. The only
// supported streaming layout is currently Promptfoo; Detect verifies each full
// envelope's structure and version before accepting it.
func ParseIngest(ctx context.Context, input io.Reader, emit func(IngestPart) error) error {
	return parsePromptfoo(ctx, input, emit)
}
