package arrowingest

import (
	"encoding/binary"

	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/google/uuid"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// AppendSpan writes the parent's scalar columns and shared context. The nested
// writer appends the event and link lists before submitting this batch.
func (a *Appender) AppendSpan(s ptrace.Span, attrs []byte, service string) error {
	a.b.Field(0).(*array.StringBuilder).Append(uuid.UUID(s.TraceID()).String())
	a.b.Field(1).(*array.StringBuilder).Append(s.TraceState().AsRaw())
	id := s.SpanID()
	a.b.Field(2).(*array.Uint64Builder).Append(binary.BigEndian.Uint64(id[:]))
	parent := s.ParentSpanID()
	if parent.IsEmpty() {
		a.b.Field(3).AppendNull()
	} else {
		a.b.Field(3).(*array.Uint64Builder).Append(binary.BigEndian.Uint64(parent[:]))
	}
	a.b.Field(4).(*array.Uint32Builder).Append(uint32(s.Flags()))
	a.b.Field(5).(*array.StringBuilder).Append(s.Name())
	a.b.Field(6).(*array.Int32Builder).Append(int32(s.Kind()))
	a.b.Field(7).(*array.Uint64Builder).Append(uint64(s.StartTimestamp()))
	a.b.Field(8).(*array.Uint64Builder).Append(uint64(s.EndTimestamp()))
	a.appendContext()
	a.b.Field(11).(*array.BinaryBuilder).Append(attrs)
	a.b.Field(12).(*array.Uint32Builder).Append(s.DroppedAttributesCount())
	a.b.Field(13).(*array.Uint32Builder).Append(s.DroppedEventsCount())
	a.b.Field(14).(*array.Uint32Builder).Append(s.DroppedLinksCount())
	a.b.Field(15).(*array.Int32Builder).Append(int32(s.Status().Code()))
	a.b.Field(16).(*array.StringBuilder).Append(s.Status().Message())
	a.b.Field(17).(*array.StringBuilder).Append(service)
	a.n++
	return nil
}
