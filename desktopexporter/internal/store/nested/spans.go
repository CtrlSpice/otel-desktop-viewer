package nested

import (
	"context"
	"database/sql/driver"
	"encoding/binary"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowingest"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type SpanAppender struct{ *arrowingest.Appender }

func NewSpanAppender(ctx context.Context, c driver.Conn) *SpanAppender {
	return &SpanAppender{spanAppender(ctx, c)}
}
func (a *SpanAppender) AppendSpan(s ptrace.Span, attrs []byte, service string) error {
	if err := a.Appender.AppendSpan(s, attrs, service); err != nil {
		return err
	}
	a.Builder().Field(18).(*array.ListBuilder).Append(true)
	a.Builder().Field(19).(*array.ListBuilder).Append(true)
	return nil
}
func (a *SpanAppender) AppendEvent(e ptrace.SpanEvent, id uuid.UUID, _ duckdb.UUID, _ uint64, ordinal int, attrs []byte) error {
	b := a.Builder().Field(18).(*array.ListBuilder).ValueBuilder().(*array.StructBuilder)
	b.Append(true)
	b.FieldBuilder(0).(*array.StringBuilder).Append(id.String())
	b.FieldBuilder(1).(*array.StringBuilder).Append(e.Name())
	b.FieldBuilder(2).(*array.Uint64Builder).Append(uint64(e.Timestamp()))
	b.FieldBuilder(3).(*array.BinaryBuilder).Append(attrs)
	b.FieldBuilder(4).(*array.Uint32Builder).Append(e.DroppedAttributesCount())
	b.FieldBuilder(5).(*array.Uint32Builder).Append(uint32(ordinal))
	return nil
}
func (a *SpanAppender) AppendLink(l ptrace.SpanLink, id uuid.UUID, _ duckdb.UUID, _ uint64, ordinal int, attrs []byte) error {
	b := a.Builder().Field(19).(*array.ListBuilder).ValueBuilder().(*array.StructBuilder)
	b.Append(true)
	b.FieldBuilder(0).(*array.StringBuilder).Append(id.String())
	if tid := l.TraceID(); tid.IsEmpty() {
		b.FieldBuilder(1).AppendNull()
	} else {
		b.FieldBuilder(1).(*array.StringBuilder).Append(uuid.UUID(tid).String())
	}
	if sid := l.SpanID(); sid.IsEmpty() {
		b.FieldBuilder(2).AppendNull()
	} else {
		b.FieldBuilder(2).(*array.Uint64Builder).Append(binary.BigEndian.Uint64(sid[:]))
	}
	b.FieldBuilder(3).(*array.StringBuilder).Append(l.TraceState().AsRaw())
	b.FieldBuilder(4).(*array.BinaryBuilder).Append(attrs)
	b.FieldBuilder(5).(*array.Uint32Builder).Append(l.DroppedAttributesCount())
	b.FieldBuilder(6).(*array.Uint32Builder).Append(uint32(l.Flags()))
	b.FieldBuilder(7).(*array.Uint32Builder).Append(uint32(ordinal))
	return nil
}
