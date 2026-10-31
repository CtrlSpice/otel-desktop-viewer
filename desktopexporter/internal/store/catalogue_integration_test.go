package store_test

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/attributes"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func ftsText(value string) search.QueryNode {
	return search.QueryNode{Type: "text", Query: &search.Query{FieldOperator: "CONTAINS", Value: value}}
}
func ftsAnd(values ...string) *search.QueryNode {
	children := make([]search.QueryNode, len(values))
	for i, v := range values {
		children[i] = ftsText(v)
	}
	return &search.QueryNode{Type: "group", Group: &search.QueryGroup{LogicalOperator: "AND", Children: children}}
}
func ftsTraces() ptrace.Traces {
	traces := ptrace.NewTraces()
	ss := traces.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty()
	for i := byte(1); i <= 2; i++ {
		s := ss.Spans().AppendEmpty()
		s.SetTraceID(pcommon.TraceID{15: i})
		s.SetSpanID(pcommon.SpanID{7: 1})
		s.SetName("rootword")
		s.SetStartTimestamp(pcommon.Timestamp(i * 10))
		s.SetEndTimestamp(pcommon.Timestamp(i*10 + 5))
		if i == 2 {
			s.SetSpanID(pcommon.SpanID{0: 255, 1: 255, 2: 255, 3: 255, 4: 255, 5: 255, 6: 255, 7: 255})
		}
		if i == 1 {
			for _, word := range []string{"alpha", "beta"} {
				e := s.Events().AppendEmpty()
				e.SetName(word)
				e.Attributes().PutStr("exception.type", "example.Exception")
			}
		}
	}
	return traces
}
func ftsMetric(description string, words ...string) pmetric.Metrics {
	m := pmetric.NewMetrics()
	metric := m.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	metric.SetName("latency")
	metric.SetDescription(description)
	metric.Metadata().PutStr("search.tag", "metaword")
	g := metric.SetEmptyGauge()
	for _, word := range words {
		d := g.DataPoints().AppendEmpty()
		d.SetIntValue(1)
		d.Attributes().PutStr("label", word)
		if word == "alpha" {
			for _, x := range []string{"eone", "etwo"} {
				e := d.Exemplars().AppendEmpty()
				e.SetIntValue(1)
				e.FilteredAttributes().PutStr("exlabel", x)
			}
		}
	}
	return m
}
func ftsSummaryCount(t *testing.T, s *store.Store, signal string, node *search.QueryNode) int {
	t.Helper()
	var result []json.RawMessage
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var raw json.RawMessage
		var err error
		switch signal {
		case "span":
			raw, err = spans.SearchTraceSummaries(t.Context(), db, store.TimeRange{}, node)
		case "log":
			raw, err = logs.Search(t.Context(), db, store.TimeRange{}, node)
		case "metric":
			raw, err = metrics.SearchSummaries(t.Context(), db, store.TimeRange{}, node)
		}
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, &result)
	}))
	return len(result)
}
func assertFTSIntegrity(t *testing.T, s *store.Store) {
	t.Helper()
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var orphans int
		require.NoError(t, db.QueryRow(`select count(*) from search_owners d where
		(d.signal='span' and not exists(select 1 from spans s where d.root_key=replace(s.trace_id::varchar,'-','')||':'||span_id_wire(s.span_id)))
		or (d.signal='span' and d.owner='event' and d.record_key not in(select id::varchar from events))
		or (d.signal='span' and d.owner='link' and d.record_key not in(select id::varchar from links))
		or (d.signal='log' and d.root_key not in(select id::varchar from logs))
		or (d.signal='metric' and d.root_key not in(select id::varchar from metrics))
		or (d.signal='metric' and d.owner='datapoint' and d.record_key not in(select id::varchar from metric_datapoints))
		or (d.signal='metric' and d.owner='exemplar' and d.record_key not in(select id::varchar from exemplars))`).Scan(&orphans))
		require.Zero(t, orphans)
		require.NoError(t, db.QueryRow(`select count(*) from search_postings l where not exists(select 1 from search_owners o where o.id=l.owner_id) or not exists(select 1 from search_paths p where p.id=l.path_id) or not exists(select 1 from search_values v where v.id=l.value_id)`).Scan(&orphans))
		require.Zero(t, orphans)
		return nil
	}))
}

func TestCatalogueProductionBoundariesRollbackRetryAndReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "catalogue.duckdb")
	s, err := store.NewStore(t.Context(), path, nil)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	traces := ftsTraces()
	require.NoError(t, s.WithConn(func(c driver.Conn) error { return spans.Ingest(t.Context(), c, traces) }))
	require.Equal(t, 1, ftsSummaryCount(t, s, "span", ftsAnd("ffffffffffffffff")), "full unsigned span IDs remain searchable")
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		raw, err := spans.SearchTraceSummaries(t.Context(), db, store.TimeRange{}, ftsAnd("rootword"))
		if err != nil {
			return err
		}
		var ordered []struct {
			TraceID string `json:"traceID"`
		}
		if err := json.Unmarshal(raw, &ordered); err != nil {
			return err
		}
		require.Len(t, ordered, 2)
		require.Equal(t, "00000000000000000000000000000002", ordered[0].TraceID)
		require.Equal(t, "00000000000000000000000000000001", ordered[1].TraceID)
		return nil
	}))
	require.Equal(t, 1, ftsSummaryCount(t, s, "span", ftsAnd("alpha")))
	require.Zero(t, ftsSummaryCount(t, s, "span", ftsAnd("alpha", "beta")), "two different events cannot satisfy free-text AND")
	require.Equal(t, 1, ftsSummaryCount(t, s, "span", ftsAnd("rootword", "alpha")))
	require.Zero(t, ftsSummaryCount(t, s, "span", ftsAnd("alph")), "untargeted text is token search, not substring search")
	selected := &search.QueryNode{Type: "condition", Query: &search.Query{Field: &search.FieldDefinition{Owner: "event", Path: []any{"name"}}, FieldOperator: "CONTAINS", Value: "alph"}}
	require.Equal(t, 1, ftsSummaryCount(t, s, "span", selected), "selected native CONTAINS keeps substring semantics")
	mixed := ftsAnd("alpha")
	mixed.Group.Children = append(mixed.Group.Children, search.QueryNode{Type: "condition", Query: &search.Query{Field: &search.FieldDefinition{Owner: "event", Path: []any{"name"}}, FieldOperator: "=", Value: "beta"}})
	require.Zero(t, ftsSummaryCount(t, s, "span", mixed), "native child predicates cannot pool a text hit from another sibling")
	// A derived-index failure must roll back native rows and be retriable, not
	// misclassified as a bad received row that bisection silently skips.
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec(`alter table search_paths rename to unavailable_search_paths`)
		return err
	}))
	more := ptrace.NewTraces()
	p := more.ResourceSpans().AppendEmpty().ScopeSpans().AppendEmpty().Spans().AppendEmpty()
	p.SetTraceID(pcommon.TraceID{15: 3})
	p.SetSpanID(pcommon.SpanID{7: 1})
	p.SetName("retryword")
	require.Error(t, s.WithConn(func(c driver.Conn) error { return spans.Ingest(t.Context(), c, more) }))
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec(`alter table unavailable_search_paths rename to search_paths`)
		return err
	}))
	require.Zero(t, ftsSummaryCount(t, s, "span", ftsAnd("retryword")))
	require.NoError(t, s.WithConn(func(c driver.Conn) error { return spans.Ingest(t.Context(), c, more) }))
	require.Equal(t, 1, ftsSummaryCount(t, s, "span", ftsAnd("retryword")))
	assertFTSIntegrity(t, s)
	// Reopen a genuinely ingested current-schema capture. Native rows and the
	// derived catalogue must remain in sync without rebuild.
	require.NoError(t, s.Close())
	s, err = store.NewStore(t.Context(), path, nil)
	require.NoError(t, err)
	require.Equal(t, 1, ftsSummaryCount(t, s, "span", ftsAnd("alpha")))
	assertFTSIntegrity(t, s)
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		return spans.DeleteSpansByTraceIDs(t.Context(), db, []any{"00000000000000000000000000000001"})
	}))
	require.Zero(t, ftsSummaryCount(t, s, "span", ftsAnd("alpha")))
	assertFTSIntegrity(t, s)
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error { return spans.Clear(t.Context(), db) }))
	assertFTSIntegrity(t, s)
}

func TestCatalogueMetricContextUpdatesAndRetention(t *testing.T) {
	s, err := store.NewStore(t.Context(), "", nil)
	require.NoError(t, err)
	defer s.Close()
	ingestMetric := func(m pmetric.Metrics) error {
		return s.WithConn(func(c driver.Conn) error { return metrics.Ingest(t.Context(), c, m) })
	}
	require.NoError(t, ingestMetric(ftsMetric("oldcontext", "alpha", "beta")))
	require.Equal(t, 1, ftsSummaryCount(t, s, "metric", ftsAnd("metaword")))
	require.Zero(t, ftsSummaryCount(t, s, "metric", ftsAnd("alpha", "beta")))
	require.Zero(t, ftsSummaryCount(t, s, "metric", ftsAnd("eone", "etwo")))
	require.Equal(t, 1, ftsSummaryCount(t, s, "metric", ftsAnd("alpha", "eone")))
	require.NoError(t, ingestMetric(ftsMetric("newcontext", "gamma")))
	require.Equal(t, 1, ftsSummaryCount(t, s, "metric", ftsAnd("oldcontext", "alpha")))
	require.Zero(t, ftsSummaryCount(t, s, "metric", ftsAnd("newcontext", "alpha")), "a received descriptor cannot borrow another record's point")
	require.Equal(t, 1, ftsSummaryCount(t, s, "metric", ftsAnd("newcontext", "gamma")))
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec(`alter table search_paths rename to unavailable_search_paths`)
		return err
	}))
	require.Error(t, ingestMetric(ftsMetric("rolledbackcontext", "delta")))
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec(`alter table unavailable_search_paths rename to search_paths`)
		return err
	}))
	require.Zero(t, ftsSummaryCount(t, s, "metric", ftsAnd("rolledbackcontext")))
	require.Equal(t, 1, ftsSummaryCount(t, s, "metric", ftsAnd("newcontext", "gamma")))
	assertFTSIntegrity(t, s)
	l := plog.NewLogs()
	l.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().Body().SetStr("logword")
	require.NoError(t, s.WithConn(func(c driver.Conn) error { return logs.Ingest(t.Context(), c, l) }))
	require.Equal(t, 1, ftsSummaryCount(t, s, "log", ftsAnd("logword")))
	require.NoError(t, s.EnforceRetention(t.Context(), 1))
	assertFTSIntegrity(t, s)
	require.Zero(t, ftsSummaryCount(t, s, "metric", ftsAnd("oldcontext")))
	require.NoError(t, ingestMetric(ftsMetric("afterretention", "fresh")))
	var id string
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error { return db.QueryRow(`select id::varchar from metrics limit 1`).Scan(&id) }))
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error { return metrics.DeleteMetric(t.Context(), db, strings.ToUpper(id)) }))
	assertFTSIntegrity(t, s)
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error { return logs.Clear(t.Context(), db) }))
	assertFTSIntegrity(t, s)
}

func TestCatalogueNativePreparationDoesNotSerializePayloads(t *testing.T) {
	s, err := store.NewStore(t.Context(), "", nil)
	require.NoError(t, err)
	defer s.Close()
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec(`create or replace macro otel_value_json(v) as error('payload serializer must not run during catalogue ingestion')`)
		return err
	}))
	data := ftsTraces()
	for i := 0; i < 2; i++ {
		attrs := data.ResourceSpans().At(0).ScopeSpans().At(0).Spans().At(i).Attributes()
		attrs.PutInt("exact", 9223372036854775807)
		attrs.PutStr("same", "9223372036854775807")
		attrs.PutDouble("zero", 0)
		if i == 1 {
			attrs.PutDouble("zero", math.Copysign(0, -1))
		}
		attrs.PutEmptyBytes("bytes").FromRaw([]byte{0, 255})
		nested := attrs.PutEmptyMap("nested")
		nested.PutStr("", "literal")
		nested.PutEmptySlice("array").AppendEmpty()
	}
	require.NoError(t, s.WithConn(func(c driver.Conn) error { return spans.Ingest(t.Context(), c, data) }))
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var count int
		require.NoError(t, db.QueryRow(`select count(distinct v.id) from search_values v join search_postings l on l.value_id=v.id join search_paths p on p.id=l.path_id where p.key='zero'`).Scan(&count))
		require.Equal(t, 2, count, "signed zeros retain separate typed identities")
		require.NoError(t, db.QueryRow(`select count(*) from search_values where value='9223372036854775807'`).Scan(&count))
		require.Equal(t, 2, count, "exact integer and string are distinct values")
		require.NoError(t, db.QueryRow(`select count(*) from search_owners where owner='resource'`).Scan(&count))
		require.Equal(t, 1, count, "shared context is not copied into every child")
		require.NoError(t, db.QueryRow(`select count(*) from information_schema.tables where table_name='search_values'`).Scan(&count))
		require.Equal(t, 1, count, "distinct values have a shared typed dictionary")
		require.NoError(t, db.QueryRow(`select count(*) from (select owner_id,path_id from search_postings group by all having count(*)>1)`).Scan(&count))
		require.Zero(t, count, "accepted native owners have one value per typed path")
		require.NoError(t, db.QueryRow(`select count(*) from search_values where kind in('map','array') and value<>''`).Scan(&count))
		require.Zero(t, count, "containers retain fingerprints, never payload text")
		return nil
	}))
}

func TestCatalogueMetricReceivedDescriptorsPreserveExactChanges(t *testing.T) {
	s, err := store.NewStore(t.Context(), "", nil)
	require.NoError(t, err)
	defer s.Close()
	ingestMetric := func(m pmetric.Metrics) error {
		return s.WithConn(func(c driver.Conn) error { return metrics.Ingest(t.Context(), c, m) })
	}
	makeMetric := func(value float64) pmetric.Metrics {
		m := ftsMetric("samecontext")
		m.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).Metadata().PutDouble("zero", value)
		return m
	}
	require.NoError(t, ingestMetric(makeMetric(0)))
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec(`alter table search_paths rename to unavailable_search_paths`)
		return err
	}))
	require.Error(t, ingestMetric(makeMetric(0)), "each received record needs its own catalogue owner")
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec(`alter table unavailable_search_paths rename to search_paths`)
		return err
	}))
	require.NoError(t, ingestMetric(makeMetric(math.Copysign(0, -1))))
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var values string
		err := db.QueryRow(`select string_agg(v.value, ',' order by v.value) from search_owners o join search_postings l on l.owner_id=o.id join search_paths p on p.id=l.path_id join search_values v on v.id=l.value_id where o.owner='metric' and p.key='zero' and p.kind='double'`).Scan(&values)
		require.NoError(t, err)
		require.Equal(t, "-0,0", values, "both received descriptors retain their float bits")
		return nil
	}))
}

func TestCatalogueNewOwnerCTERuntimeFailureRollsBackEveryWrite(t *testing.T) {
	s, err := store.NewStore(t.Context(), "", nil)
	require.NoError(t, err)
	defer s.Close()
	data := plog.NewLogs()
	data.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().Body().SetStr("runtime rollback word")
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec(`alter table search_postings rename to held_search_postings;
		create table search_postings(owner_id bigint check(owner_id<0),path_id bigint,value_id bigint)`)
		return err
	}))
	err = s.WithConn(func(c driver.Conn) error { return logs.Ingest(t.Context(), c, data) })
	require.ErrorContains(t, err, "CHECK constraint failed")
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		for _, table := range []string{"logs", "search_owners", "search_paths", "search_values", "search_postings"} {
			var count int
			require.NoError(t, db.QueryRow(`select count(*) from `+table).Scan(&count))
			require.Zero(t, count, table)
		}
		return nil
	}))
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec(`drop table search_postings;alter table held_search_postings rename to search_postings`)
		return err
	}))
	for i := 0; i < 2; i++ {
		require.NoError(t, s.WithConn(func(c driver.Conn) error { return logs.Ingest(t.Context(), c, data) }))
	}
	require.Equal(t, 2, ftsSummaryCount(t, s, "log", ftsAnd("rollback")))
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var count int
		require.NoError(t, db.QueryRow(`select count(*) from (select owner_id,path_id,value_id from search_postings group by all having count(*)>1)`).Scan(&count))
		require.Zero(t, count, "existing shared contexts must not receive duplicate links")
		require.NoError(t, db.QueryRow(`select count(*) from search_owners where signal=''`).Scan(&count))
		require.Equal(t, 2, count, "one shared Resource and Scope across accepted transactions")
		return nil
	}))
}

func TestCatalogueNewOwnerPathExcludesRejectedDuplicateSpans(t *testing.T) {
	s, err := store.NewStore(t.Context(), "", nil)
	require.NoError(t, err)
	defer s.Close()
	data := ftsTraces()
	ss := data.ResourceSpans().At(0).ScopeSpans().At(0)
	ss.Spans().At(0).CopyTo(ss.Spans().AppendEmpty())
	ss.Spans().At(2).SetName("nevercatalogued")
	require.NoError(t, s.WithConn(func(c driver.Conn) error {
		rejected, err := spans.IngestReport(t.Context(), c, data)
		require.Equal(t, 1, rejected.Count())
		return err
	}))
	require.Zero(t, ftsSummaryCount(t, s, "span", ftsAnd("nevercatalogued")))
	require.Equal(t, 2, ftsSummaryCount(t, s, "span", ftsAnd("rootword")))
	require.NoError(t, s.WithConn(func(c driver.Conn) error {
		rejected, err := spans.IngestReport(t.Context(), c, data)
		require.Equal(t, 3, rejected.Count())
		return err
	}))
	require.Zero(t, ftsSummaryCount(t, s, "span", ftsAnd("nevercatalogued")))
	assertFTSIntegrity(t, s)
}

func TestCatalogueContainerDiscoverySamplesNativeExactTypes(t *testing.T) {
	s, err := store.NewStore(t.Context(), "", nil)
	require.NoError(t, err)
	defer s.Close()
	data := plog.NewLogs()
	sl := data.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty()
	var expected []json.RawMessage
	for _, zero := range []float64{0, math.Copysign(0, -1)} {
		record := sl.LogRecords().AppendEmpty()
		nested := record.Attributes().PutEmptyMap("nested")
		nested.PutInt("exact", 9223372036854775807)
		nested.PutDouble("zero", zero)
		nested.PutEmptyBytes("bytes").FromRaw([]byte{0, 255})
		nested.PutEmptySlice("array").AppendEmpty()
		value, ok := record.Attributes().Get("nested")
		require.True(t, ok)
		raw, err := util.EncodeValue(value)
		require.NoError(t, err)
		expected = append(expected, raw)
	}
	require.NoError(t, s.WithConn(func(c driver.Conn) error { return logs.Ingest(t.Context(), c, data) }))
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		raw, err := attributes.SearchScoped(t.Context(), db, attributes.SearchOptions{Signal: "logs", Owner: "log", Path: []any{"attributes", "nested"}, Kind: "map"})
		require.NoError(t, err)
		var matches []struct {
			Count   int               `json:"matchCount"`
			Samples []json.RawMessage `json:"sampleValues"`
		}
		require.NoError(t, json.Unmarshal(raw, &matches))
		require.Len(t, matches, 1)
		require.Equal(t, 2, matches[0].Count, "container identity includes signed zero and exact types")
		require.ElementsMatch(t, expected, matches[0].Samples)
		return nil
	}))
}

func TestUnscopedDiscoveryAndFiltersKeepOwnerBoundaries(t *testing.T) {
	s, err := store.NewStore(t.Context(), "", nil)
	require.NoError(t, err)
	defer s.Close()
	td := ftsTraces()
	rs := td.ResourceSpans().At(0)
	rs.Resource().Attributes().PutStr("shared", "context")
	p := rs.ScopeSpans().At(0).Spans().At(0)
	p.Events().At(0).Attributes().PutStr("needle", "alpha")
	p.Events().At(1).Attributes().PutStr("other", "beta")
	p.Attributes().PutInt("needle", 7)
	ld := plog.NewLogs()
	ld.ResourceLogs().AppendEmpty().ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().Attributes().PutStr("needle", "logvalue")
	require.NoError(t, s.WithConn(func(c driver.Conn) error { return spans.Ingest(t.Context(), c, td) }))
	require.NoError(t, s.WithConn(func(c driver.Conn) error { return logs.Ingest(t.Context(), c, ld) }))
	condition := func(key, value string) search.QueryNode {
		return search.QueryNode{Type: "condition", Query: &search.Query{Field: &search.FieldDefinition{Path: []any{"attributes", key}, Type: "string"}, FieldOperator: "=", Value: value}}
	}
	alpha := condition("needle", "alpha")
	require.Equal(t, 1, ftsSummaryCount(t, s, "span", &alpha))
	missing := condition("missing", "alpha")
	require.Zero(t, ftsSummaryCount(t, s, "span", &missing))
	group := &search.QueryNode{Type: "group", Group: &search.QueryGroup{LogicalOperator: "AND", Children: []search.QueryNode{alpha, condition("other", "beta")}}}
	require.Zero(t, ftsSummaryCount(t, s, "span", group), "different sibling events cannot satisfy one expression")
	group.Group.Children[1] = condition("shared", "context")
	require.Equal(t, 1, ftsSummaryCount(t, s, "span", group), "the candidate can use its resource context")
	group.Group.LogicalOperator = "OR"
	group.Group.Children = []search.QueryNode{alpha, condition("other", "beta")}
	require.Equal(t, 1, ftsSummaryCount(t, s, "span", group))
	for _, operator := range []string{"=", "!=", "NOT CONTAINS", "NOT IN"} {
		q := condition("absent", "alpha")
		q.Query.FieldOperator = operator
		if operator == "NOT IN" {
			q.Query.Value = `["alpha"]`
		}
		require.Zero(t, ftsSummaryCount(t, s, "span", &q), operator)
	}
	typed := condition("needle", "7")
	require.Zero(t, ftsSummaryCount(t, s, "span", &typed), "integer is not a string")
	typed.Query.Field.Type = "int64"
	require.Equal(t, 1, ftsSummaryCount(t, s, "span", &typed))
	for _, legacy := range []bool{false, true} {
		left, right := condition("needle", "alpha"), condition("other", "beta")
		for _, q := range []*search.Query{left.Query, right.Query} {
			if legacy {
				q.Field.Name = q.Field.Path[1].(string)
				q.Field.Path = nil
				q.Field.SearchScope, q.Field.AttributeScope = "attribute", "event"
			} else {
				q.Field.Owner = "event"
			}
		}
		group.Group.LogicalOperator, group.Group.Children = "AND", []search.QueryNode{left, right}
		require.Zero(t, ftsSummaryCount(t, s, "span", group), "explicit event filters cannot pool siblings")
		group.Group.Children[1] = condition("shared", "context")
		require.Equal(t, 1, ftsSummaryCount(t, s, "span", group))
	}
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		decode := func(options attributes.SearchOptions) []struct {
			Owner   string            `json:"owner"`
			Kind    string            `json:"type"`
			Samples []json.RawMessage `json:"sampleValues"`
		} {
			raw, err := attributes.SearchScoped(t.Context(), db, options)
			require.NoError(t, err)
			var got []struct {
				Owner   string            `json:"owner"`
				Kind    string            `json:"type"`
				Samples []json.RawMessage `json:"sampleValues"`
			}
			require.NoError(t, json.Unmarshal(raw, &got))
			return got
		}
		all := decode(attributes.SearchOptions{Path: []any{"attributes", "needle"}})
		require.Len(t, all, 3, "omitted signal/owner includes event, span and log without merging types")
		for _, m := range all {
			require.NotEmpty(t, m.Owner)
			require.NotEmpty(t, m.Kind)
			require.NotEmpty(t, m.Samples)
		}
		scoped := decode(attributes.SearchOptions{Signal: "traces", Path: []any{"attributes", "needle"}, Term: "alpha"})
		require.Len(t, scoped, 1)
		require.Equal(t, "event", scoped[0].Owner)
		explicit := decode(attributes.SearchOptions{Signal: "traces", Owner: "span", Path: []any{"attributes", "needle"}, Term: "alpha"})
		require.Empty(t, explicit, "explicit owner never silently widens")
		return nil
	}))
}
