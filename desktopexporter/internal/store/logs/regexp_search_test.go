package logs_test

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

// A regex search must execute, not die in the SQL parser.
//
// It died in the SQL parser from the day the operator shipped: the condition
// builder emitted `x REGEXP y`, and DuckDB's grammar has no infix REGEXP --
// its regex operators are ~ and !~. No test executed a REGEXP condition
// against a real database, so the operator was advertised, highlighted, and
// broken. This test is the one that was missing, and it covers the negation
// the same way.
func TestRegexpSearchExecutes(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "svc")
	sl := rl.ScopeLogs().AppendEmpty()
	for _, body := range []string{"checkout failed hard", "payment ok"} {
		lr := sl.LogRecords().AppendEmpty()
		lr.SetTimestamp(pcommon.Timestamp(time.Now().UnixNano()))
		lr.Body().SetStr(body)
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, ld, s.FlushedIDs())
	}))

	run := func(operator, pattern string) []any {
		tree := &search.QueryNode{
			Type: "condition",
			Query: &search.Query{
				Field:         &search.FieldDefinition{Name: "body", SearchScope: "field", Type: "string"},
				FieldOperator: operator,
				Value:         pattern,
			},
		}
		raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return logs.Search(ctx, db, store.BoundedTimeRange(0, time.Now().UnixNano()+int64(time.Hour)), tree)
		})
		require.NoError(t, err, "%s must execute", operator)
		var out []any
		require.NoError(t, json.Unmarshal(raw, &out))
		return out
	}

	// ~ is a full match, so the pattern spans the whole body.
	assert.Len(t, run("REGEXP", "checkout.*"), 1)
	assert.Len(t, run("NOT REGEXP", "checkout.*"), 1)
	assert.Len(t, run("REGEXP", ".*"), 2)
}
