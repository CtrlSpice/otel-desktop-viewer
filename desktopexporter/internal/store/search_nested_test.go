package store

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"math"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/attributes"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

// The same selected discovery addresses must work on a small live table and on
// checkpointed native storage. Dots, brackets and numeric keys are never parsed.
func TestNestedSearchDiscoveryAndReopen(t *testing.T) {
	ctx := t.Context()
	file := filepath.Join(t.TempDir(), "nested-search.duckdb")
	s, err := NewStore(ctx, file, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		if s != nil {
			_ = s.Close()
		}
	})
	data := plog.NewLogs()
	resource := data.ResourceLogs().AppendEmpty()
	resource.Resource().Attributes().PutEmptyMap("nested").PutStr("owner", "resource")
	scope := resource.ScopeLogs().AppendEmpty()
	scope.Scope().Attributes().PutEmptyMap("nested").PutStr("owner", "scope")
	record := scope.LogRecords().AppendEmpty()
	record.SetEventName("name")
	require.NoError(t, record.Body().FromRaw([]any{map[string]any{"0": int64(9007199254740993)}}))
	require.NoError(t, record.Attributes().FromRaw(map[string]any{
		"name": "attribute name",
		"nested": map[string]any{
			"owner": "log", "id": int64(9007199254740993), "id.text": "literal dot",
			"empty": nil, "blank": "", "bytes": []byte{0, 255},
			"zero": math.Copysign(0, -1), "nan": math.NaN(),
			"array": []any{map[string]any{"0": "numeric key", "": "empty key", "[0]": "bracket key"}, int64(math.MaxInt64), nil},
			"map":   map[string]any{}, "list": []any{},
		},
		"nested.id": "literal root", "": map[string]any{"": "double empty"},
		"'); drop table logs; --": map[string]any{"雪": "bound key"},
	}))
	// A second row catches accidental missing-value and owner matches.
	scope.LogRecords().AppendEmpty().Attributes().PutEmptyMap("nested").PutStr("id", "9007199254740993")
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return logs.Ingest(ctx, conn, data) }))
	type definition struct {
		Owner string `json:"owner"`
		Path  []any  `json:"path"`
		Type  string `json:"type"`
	}
	check := func() {
		t.Helper()
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			for _, tc := range []struct {
				owner, kind, value string
				path               []any
				want               int
			}{
				{"log", "string", "name", []any{"eventName"}, 1},
				{"log", "int64", "9007199254740993", []any{"body", 0, "0"}, 1},
				{"log", "string", "attribute name", []any{"attributes", "name"}, 1},
				{"log", "int64", "9007199254740993", []any{"attributes", "nested", "id"}, 1},
				{"log", "int64", "9007199254740992", []any{"attributes", "nested", "id"}, 0},
				{"log", "string", "literal root", []any{"attributes", "nested.id"}, 1},
				{"log", "string", "literal dot", []any{"attributes", "nested", "id.text"}, 1},
				{"log", "string", "numeric key", []any{"attributes", "nested", "array", 0, "0"}, 1},
				{"log", "string", "empty key", []any{"attributes", "nested", "array", 0, ""}, 1},
				{"log", "string", "bracket key", []any{"attributes", "nested", "array", 0, "[0]"}, 1},
				{"log", "int64", "9223372036854775807", []any{"attributes", "nested", "array", 1}, 1},
				{"log", "empty", "null", []any{"attributes", "nested", "array", 2}, 1},
				{"log", "empty", "null", []any{"attributes", "nested", "array", 3}, 0},
				{"log", "string", "numeric key", []any{"attributes", "nested", "array", "0", "0"}, 0},
				{"log", "string", "numeric key", []any{"attributes", "nested", "array", 0, 0}, 0},
				{"log", "string", "double empty", []any{"attributes", "", ""}, 1},
				{"log", "string", "bound key", []any{"attributes", "'); drop table logs; --", "雪"}, 1},
				{"log", "empty", "null", []any{"attributes", "nested", "empty"}, 1},
				{"log", "empty", "null", []any{"attributes", "nested", "missing"}, 0},
				{"log", "string", "", []any{"attributes", "nested", "blank"}, 1},
				{"log", "bytes", "AP8=", []any{"attributes", "nested", "bytes"}, 1},
				{"log", "double", "0x8000000000000000", []any{"attributes", "nested", "zero"}, 1},
				{"log", "double", "0x7ff8000000000000", []any{"attributes", "nested", "nan"}, 1},
				{"log", "map", "{}", []any{"attributes", "nested", "map"}, 1},
				{"resource", "string", "resource", []any{"attributes", "nested", "owner"}, 2},
				{"scope", "string", "scope", []any{"attributes", "nested", "owner"}, 2},
				{"log", "string", "resource", []any{"attributes", "nested", "owner"}, 0},
			} {
				criteria := search.QueryNode{Type: "condition", Query: &search.Query{
					Field: &search.FieldDefinition{Owner: tc.owner, Path: tc.path, Type: tc.kind}, FieldOperator: "=", Value: tc.value,
				}}
				raw, err := logs.Search(ctx, db, TimeRange{}, criteria)
				require.NoError(t, err, "%s %v", tc.owner, tc.path)
				var results []json.RawMessage
				require.NoError(t, json.Unmarshal(raw, &results))
				require.Len(t, results, tc.want, "%s %v", tc.owner, tc.path)
			}
			raw, err := logs.GetLogAttributeDefinitions(ctx, db)
			require.NoError(t, err)
			var definitions []definition
			require.NoError(t, json.Unmarshal(raw, &definitions))
			want := []any{"attributes", "nested", "array", float64(0), "0"}
			require.Contains(t, definitions, definition{Owner: "log", Path: want, Type: "string"})
			require.Contains(t, definitions, definition{Owner: "log", Path: []any{"body", float64(0), "0"}, Type: "int64"})
			raw, err = attributes.SearchScoped(ctx, db, attributes.SearchOptions{Term: "numeric key", Signal: "logs", Owner: "log", Path: want, Kind: "string"})
			require.NoError(t, err)
			var matches []definition
			require.NoError(t, json.Unmarshal(raw, &matches))
			require.Contains(t, matches, definition{Owner: "log", Path: want, Type: "string"})
			return nil
		}))
	}
	check()
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error { _, err := db.ExecContext(ctx, "checkpoint"); return err }))
	require.NoError(t, s.Close())
	s, err = NewStore(ctx, file, nil)
	require.NoError(t, err)
	check()
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var segments int
		err := db.QueryRowContext(ctx, `select count(*) from pragma_storage_info('logs')
			where column_name = 'attributes' and segment_type = 'BIGINT'`).Scan(&segments)
		require.NoError(t, err)
		require.Positive(t, segments, "checkpoint must exercise physically shredded attribute paths")
		return nil
	}))
}

func TestNestedSearchRetainsChildOwnership(t *testing.T) {
	ctx := t.Context()
	s, err := NewStore(ctx, "", nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	for seed := byte(1); seed <= 2; seed++ {
		put := func(attrs pcommon.Map, owner string) {
			attrs.PutEmptyMap("nested").PutStr("owner", owner+strconv.Itoa(int(seed)))
		}
		traces := integrityTraces(seed)
		resource := traces.ResourceSpans().At(0)
		scope := resource.ScopeSpans().At(0)
		span := scope.Spans().At(0)
		// OTLP span IDs are unique within a trace, not across traces.
		span.SetSpanID([8]byte{1})
		put(resource.Resource().Attributes(), "resource")
		put(scope.Scope().Attributes(), "scope")
		put(span.Attributes(), "span")
		put(span.Events().At(0).Attributes(), "event")
		put(span.Links().At(0).Attributes(), "link")
		require.NoError(t, s.WithConn(func(conn driver.Conn) error { return spans.Ingest(ctx, conn, traces) }))
		data := integrityMetrics(seed)
		metricResource := data.ResourceMetrics().At(0)
		metricScope := metricResource.ScopeMetrics().At(0)
		metric := metricScope.Metrics().At(0)
		metric.SetName("metric" + strconv.Itoa(int(seed)))
		put(metricResource.Resource().Attributes(), "resource")
		put(metricScope.Scope().Attributes(), "scope")
		put(metric.Metadata(), "metric")
		point := metric.Gauge().DataPoints().At(0)
		put(point.Attributes(), "datapoint")
		put(point.Exemplars().At(0).FilteredAttributes(), "exemplar")
		require.NoError(t, s.WithConn(func(conn driver.Conn) error { return metrics.Ingest(ctx, conn, data, s.FlushedIDs()) }))
	}
	condition := func(owner, value string) search.QueryNode {
		root := "attributes"
		if owner == "metric" {
			root = "metadata"
		}
		return search.QueryNode{Type: "condition", Query: &search.Query{
			Field:         &search.FieldDefinition{Owner: owner, Path: []any{root, "nested", "owner"}, Type: "string"},
			FieldOperator: "=", Value: value,
		}}
	}
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		for _, signal := range []struct {
			name   string
			owners []string
			run    func(any) (json.RawMessage, error)
		}{
			{"traces", []string{"resource", "scope", "span", "event", "link"}, func(query any) (json.RawMessage, error) {
				return spans.SearchTraceSummaries(ctx, db, TimeRange{}, query)
			}},
			{"metrics", []string{"resource", "scope", "metric", "datapoint", "exemplar"}, func(query any) (json.RawMessage, error) { return metrics.SearchSummaries(ctx, db, TimeRange{}, query) }},
		} {
			for _, owner := range signal.owners {
				query := condition(owner, owner+"1")
				raw, err := signal.run(query)
				require.NoError(t, err, "%s %s", signal.name, owner)
				var result []json.RawMessage
				require.NoError(t, json.Unmarshal(raw, &result))
				require.Len(t, result, 1, "%s %s", signal.name, owner)
				// Combining that child with the other parent's resource must fail.
				group := search.QueryNode{Type: "group", Group: &search.QueryGroup{LogicalOperator: "AND", Children: []search.QueryNode{query, condition("resource", "resource2")}}}
				raw, err = signal.run(group)
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(raw, &result))
				require.Empty(t, result, "%s %s crossed owners", signal.name, owner)
			}
		}
		return nil
	}))
}
