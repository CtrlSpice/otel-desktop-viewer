package logs

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/searchindex"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/google/uuid"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

// resourceServiceName extracts the service.name resource attribute as a
// plain string, returning "" when not present. Mirrors the same helper
// in spans / metrics; kept private here so logs doesn't grow a cross-
// package dependency just for one attribute lookup.
func resourceServiceName(attrs pcommon.Map) string {
	if v, ok := util.LastValue(attrs, "service.name"); ok {
		return v.AsString()
	}
	return ""
}

// Ingest ingests log records from pdata into the logs table.
// The caller must hold any required lock on the connection.
//
// Each log carries its body, attributes, Resource and Scope in one append.
// Ingest is IngestReport for callers with nowhere to put the report. Records
// the store refused are still skipped rather than failing the batch; this form
// just cannot say how many there were.
func Ingest(ctx context.Context, conn driver.Conn, logs plog.Logs) error {
	_, err := IngestReport(ctx, conn, logs)
	return err
}

// IngestReport ingests a batch and reports the records it could not write. A
// non-empty Rejected is not a failure: the batch landed, minus those rows.
func IngestReport(ctx context.Context, conn driver.Conn, logs plog.Logs) (_ ingest.Rejected, err error) {
	defer func() { err = ingest.InterruptedContextError(ctx, err) }()

	if err := ctx.Err(); err != nil {
		return ingest.Rejected{}, err
	}

	return ingest.BisectingWrite(ctx, logs.LogRecordCount(), nil, func(lo, hi int) error {
		return ingest.InTransaction(ctx, conn, func() error {
			return appendPass(ctx, conn, logs,
				func(ordinal int) bool { return ordinal >= lo && ordinal < hi })
		})
	})
}

// appendPass writes the records keep selects, by ordinal. Skipped records
// still advance logCur, which is consumed by position.
func appendPass(
	ctx context.Context,
	conn driver.Conn,
	logs plog.Logs,
	keep func(ordinal int) bool,
) (err error) {
	tables := []string{"logs"}
	appenders, err := arrowingest.NewAppenders(ctx, conn, tables)
	if err != nil {
		return fmt.Errorf("Ingest: %w: %w", ErrLogsStoreInternal, err)
	}
	var catalogue searchindex.Batch
	defer func() {
		if err != nil {
			for _, appender := range appenders {
				appender.Discard()
			}
			return
		}
		err = errors.Join(err, arrowingest.CloseAppenders(appenders, tables))
		if err == nil {
			if indexErr := catalogue.Write(ctx, conn); indexErr != nil {
				err = fmt.Errorf("%w: %w", ingest.ErrNotRowFault, indexErr)
			}
		}
	}()

	logCur := 0
	for _, resourceLogs := range logs.ResourceLogs().All() {
		resource := resourceLogs.Resource()
		resourceInput, err := ingest.ResourceInput(resource, resourceLogs.SchemaUrl())
		if err != nil {
			return err
		}
		serviceName := resourceServiceName(resource.Attributes())

		for _, scopeLogs := range resourceLogs.ScopeLogs().All() {
			scopeInput, err := ingest.ScopeInput(scopeLogs.Scope(), scopeLogs.SchemaUrl())
			if err != nil {
				return err
			}

			appenders["logs"].SetContext(resourceInput, scopeInput)
			for _, log := range scopeLogs.LogRecords().All() {
				if err := ctx.Err(); err != nil {
					return err
				}

				if !keep(logCur) {
					logCur++
					continue
				}

				var traceUUID *duckdb.UUID
				if tid := log.TraceID(); !tid.IsEmpty() {
					u := duckdb.UUID(tid)
					traceUUID = &u
				}
				var spanID driver.Value
				if sid := log.SpanID(); !sid.IsEmpty() {
					spanID = util.SpanIDUint64(sid)
				}

				body, err := ingest.ValueBytes(log.Body())
				if err != nil {
					return fmt.Errorf("Ingest: %w: %w", ErrLogsStoreInternal, err)
				}
				attributes, err := ingest.AttributeBytes(log.Attributes())
				if err != nil {
					return err
				}
				logCur++

				id := uuid.New()
				err = appenders["logs"].AppendLog(log, id, body, attributes, serviceName)
				if err != nil {
					return fmt.Errorf("Ingest: %w: %w", ErrLogsStoreInternal, err)
				}
				resourceRef, scopeRef, err := catalogue.Contexts(resourceInput, scopeInput)
				if err != nil {
					return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
				}
				o := searchindex.Object{Ref: "log:log:" + id.String(), Signal: "log", Owner: "log", RootKey: id.String(), RecordKey: id.String(), ResourceRef: resourceRef, ScopeRef: scopeRef}
				if err := o.AddNative("attributes", attributes); err != nil {
					return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
				}
				if err := o.AddNative("body", body); err != nil {
					return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
				}
				var traceWireID, spanWireID any
				if traceUUID != nil {
					traceWireID = log.TraceID().String()
				}
				if spanID != nil {
					spanWireID = log.SpanID().String()
				}
				if err := o.AddStrings(map[string]any{"traceID": traceWireID, "spanID": spanWireID, "severityText": log.SeverityText(), "severityNumber": fmt.Sprint(int32(log.SeverityNumber())), "eventName": log.EventName()}); err != nil {
					return fmt.Errorf("%w: %w", ingest.ErrNotRowFault, err)
				}
				catalogue.Add(o)

			}
		}
	}

	return nil
}
