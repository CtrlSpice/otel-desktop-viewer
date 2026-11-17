package spans

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/nested"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/searchindex"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// resourceServiceName extracts the service.name resource attribute as a
// plain string, returning "" when not present. This is the same logic
// used by the metrics package to denormalize service onto metrics,
// kept private here so spans doesn't grow a metrics dependency.
func resourceServiceName(attrs pcommon.Map) string {
	if v, ok := util.LastValue(attrs, "service.name"); ok {
		return v.AsString()
	}
	return ""
}

// Ingest is IngestReport for callers with nowhere to put the report. Refused
// rows are still skipped rather than failing the batch.
func Ingest(ctx context.Context, conn driver.Conn, traces ptrace.Traces) error {
	_, err := IngestReport(ctx, conn, traces)
	return err
}

// IngestReport ingests trace spans into the spans, events and links
// tables, reporting the spans it could not write. A non-empty Rejected is not a
// failure: the batch landed without them. The caller must hold any required
// lock on the connection.
//
// Check span identities before appending self-contained payloads. Bisection
// isolates bad rows without separating owners from their attributes.
func IngestReport(ctx context.Context, conn driver.Conn, traces ptrace.Traces) (_ ingest.Rejected, err error) {
	defer func() { err = ingest.InterruptedContextError(ctx, err) }()

	if err := ctx.Err(); err != nil {
		return ingest.Rejected{}, err
	}

	// Span identities in walk order, so dedupe below can speak in the same
	// ordinals the append pass and bisection do.
	var spanKeys []spanKey

	for _, resourceSpan := range traces.ResourceSpans().All() {
		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			for _, span := range scopeSpan.Spans().All() {
				spanKeys = append(spanKeys, spanKey{
					trace: duckdb.UUID(span.TraceID()),
					span:  util.SpanIDUint64(span.SpanID()),
				})
			}
		}
	}

	// Skip ids the store already holds before writing rather than after
	// failing: bisection would find them, but one transaction at a time.
	skip, err := skipAlreadyStored(ctx, conn, spanKeys)
	if err != nil {
		return ingest.Rejected{}, err
	}
	for ordinal, key := range spanKeys {
		if key.span == 0 {
			skip[ordinal] = ErrInvalidSpanID
		}
	}

	// Pass 2: append, retrying in halves so a bad row costs only itself. The
	// skipped ordinals go in as pre-rejected rather than being tallied on
	// afterwards, so every rejection is built in one place and in walk order.
	rejected, err := appendSpansBisecting(ctx, conn, traces, skip)
	if err != nil {
		return rejected, err
	}

	// Recorded after the appends commit, never inside them: a rejection
	// describes a batch that succeeded, and rolling it back with a failed
	// attempt would lose the only trace that anything was refused.
	if err := recordSpanRejections(ctx, conn, rejected, spanKeys); err != nil {
		// The telemetry landed. Failing the batch because we could not write
		// a note about it would turn a report into an outage.
		return rejected, nil
	}
	return rejected, nil
}

// appendPass writes the spans keep selects, by ordinal rather than id so two
// occurrences of one id stay separable.
func appendPass(
	ctx context.Context,
	conn driver.Conn,
	traces ptrace.Traces,
	keep func(ordinal int) bool,
) (err error) {
	a := nested.NewSpanAppender(ctx, conn)
	var catalogue searchindex.Batch
	defer func() {
		if err != nil {
			a.Discard()
			return
		}
		err = errors.Join(err, a.Close())
		if err == nil {
			if indexErr := catalogue.Write(ctx, conn); indexErr != nil {
				err = fmt.Errorf("%w: %w", ingest.ErrNotRowFault, indexErr)
			}
		}
	}()

	var spanCur int
	for _, resourceSpan := range traces.ResourceSpans().All() {
		resource := resourceSpan.Resource()
		resourceInput, err := ingest.ResourceInput(resource, resourceSpan.SchemaUrl())
		if err != nil {
			return err
		}
		serviceName := resourceServiceName(resource.Attributes())

		for _, scopeSpan := range resourceSpan.ScopeSpans().All() {
			scopeInput, err := ingest.ScopeInput(scopeSpan.Scope(), scopeSpan.SchemaUrl())
			if err != nil {
				return err
			}

			a.SetContext(resourceInput, scopeInput)
			for _, span := range scopeSpan.Spans().All() {
				if err := ctx.Err(); err != nil {
					return err
				}

				if !keep(spanCur) {
					spanCur++
					continue
				}

				traceUUID := duckdb.UUID(span.TraceID())

				spanID := util.SpanIDUint64(span.SpanID())
				rootKey := fmt.Sprintf("%s:%016x", span.TraceID().String(), spanID)

				var parentSpanID driver.Value
				if pid := span.ParentSpanID(); !pid.IsEmpty() {
					parentSpanID = util.SpanIDUint64(pid)
				}

				attributes, err := ingest.AttributeBytes(span.Attributes())
				if err != nil {
					return err
				}
				spanCur++

				err = a.AppendSpan(span, attributes, serviceName)
				if err != nil {
					return fmt.Errorf("Ingest: %w: %w", ErrSpansStoreInternal, err)
				}
				resourceRef, scopeRef, err := catalogue.Contexts(resourceInput, scopeInput)
				if err != nil {
					return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
				}
				parent := searchindex.Object{Ref: "span:span:" + rootKey, Signal: "span", Owner: "span", RootKey: rootKey, RecordKey: rootKey, ResourceRef: resourceRef, ScopeRef: scopeRef}
				if err := parent.AddNative("attributes", attributes); err != nil {
					return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
				}
				kind, status := span.Kind().String(), span.Status().Code().String()
				if int32(span.Kind()) < 0 || int32(span.Kind()) > 5 {
					kind = fmt.Sprintf("Unknown (%d)", span.Kind())
				}
				if int32(span.Status().Code()) < 0 || int32(span.Status().Code()) > 2 {
					status = fmt.Sprintf("Unknown (%d)", span.Status().Code())
				}
				var parentWireID any
				if parentSpanID != nil {
					parentWireID = span.ParentSpanID().String()
				}
				if err := parent.AddStrings(map[string]any{"traceID": span.TraceID().String(), "spanID": span.SpanID().String(), "parentSpanID": parentWireID, "name": span.Name(), "statusMessage": span.Status().Message(), "traceState": span.TraceState().AsRaw(), "kind": kind, "status": status}); err != nil {
					return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
				}
				catalogue.Add(parent)

				for ordinal, event := range span.Events().All() {
					eventID := uuid.New()
					attributes, err := ingest.AttributeBytes(event.Attributes())
					if err != nil {
						return err
					}
					err = a.AppendEvent(event, eventID, traceUUID, spanID, ordinal, attributes)
					if err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrSpansStoreInternal, err)
					}
					child := searchindex.Object{Ref: "span:event:" + eventID.String(), Signal: "span", Owner: "event", RootKey: rootKey, RecordKey: eventID.String(), ParentRef: parent.Ref}
					if err := child.AddNative("attributes", attributes); err != nil {
						return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
					}
					if err := child.AddStrings(map[string]any{"name": event.Name()}); err != nil {
						return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
					}
					catalogue.Add(child)
				}

				for ordinal, link := range span.Links().All() {
					linkID := uuid.New()
					var linkTraceUUID driver.Value
					if tid := link.TraceID(); !tid.IsEmpty() {
						linkTraceUUID = duckdb.UUID(tid)
					}
					var linkSpanID driver.Value
					if sid := link.SpanID(); !sid.IsEmpty() {
						linkSpanID = util.SpanIDUint64(sid)
					}

					attributes, err := ingest.AttributeBytes(link.Attributes())
					if err != nil {
						return err
					}
					err = a.AppendLink(link, linkID, traceUUID, spanID, ordinal, attributes)
					if err != nil {
						return fmt.Errorf("Ingest: %w: %w", ErrSpansStoreInternal, err)
					}
					child := searchindex.Object{Ref: "span:link:" + linkID.String(), Signal: "span", Owner: "link", RootKey: rootKey, RecordKey: linkID.String(), ParentRef: parent.Ref}
					if err := child.AddNative("attributes", attributes); err != nil {
						return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
					}
					var traceWireID, spanWireID any
					if linkTraceUUID != nil {
						traceWireID = link.TraceID().String()
					}
					if linkSpanID != nil {
						spanWireID = link.SpanID().String()
					}
					if err := child.AddStrings(map[string]any{"traceID": traceWireID, "spanID": spanWireID, "traceState": link.TraceState().AsRaw()}); err != nil {
						return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
					}
					catalogue.Add(child)
				}
			}
		}
	}

	return nil
}
