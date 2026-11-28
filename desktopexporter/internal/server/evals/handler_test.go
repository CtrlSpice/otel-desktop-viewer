//go:build evals

package evals

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/evaldecode"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	storeevals "github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/evals"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	querysearch "github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/open-telemetry/opentelemetry-collector-contrib/processor/redactionprocessor"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/confmap"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/plog/plogotlp"
	"go.opentelemetry.io/collector/processor"
	"go.opentelemetry.io/collector/receiver"
	"go.opentelemetry.io/collector/receiver/otlpreceiver"
	"go.yaml.in/yaml/v3"
)

type evalTestServer struct {
	*Handler
	mux *http.ServeMux
}

// Exercise the real HTTP receiver and pinned upstream redactor using the shipped
// policy. The final consumer is the same retained log ingest used by the desktop
// exporter; no upload path receives a store-writing test hook.
func startEvalReceiver(t *testing.T, s *evalTestServer) {
	t.Helper()
	sink, err := consumer.NewLogs(func(ctx context.Context, data plog.Logs) error {
		return s.store.WithConn(func(conn driver.Conn) error { return logs.Ingest(ctx, conn, data) })
	})
	require.NoError(t, err)
	policy, err := os.ReadFile("../../../../internal/redaction/policy.yaml")
	require.NoError(t, err)
	var raw map[string]any
	require.NoError(t, yaml.Unmarshal(policy, &raw))
	factory := redactionprocessor.NewFactory()
	cfg := factory.CreateDefaultConfig()
	require.NoError(t, confmap.NewFromStringMap(raw["processors"].(map[string]any)["redaction"].(map[string]any)).Unmarshal(cfg))
	p, err := factory.CreateLogs(t.Context(), processor.Settings{ID: component.MustNewID("redaction"), TelemetrySettings: componenttest.NewNopTelemetrySettings()}, cfg, sink)
	require.NoError(t, err)
	require.NoError(t, p.Start(t.Context(), componenttest.NewNopHost()))
	t.Cleanup(func() { require.NoError(t, p.Shutdown(context.Background())) })
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	endpoint := listener.Addr().String()
	require.NoError(t, listener.Close())
	rf := otlpreceiver.NewFactory()
	rc := rf.CreateDefaultConfig()
	require.NoError(t, confmap.NewFromStringMap(map[string]any{"protocols": map[string]any{"http": map[string]any{"endpoint": endpoint}}}).Unmarshal(rc))
	r, err := rf.CreateLogs(t.Context(), receiver.Settings{ID: component.MustNewID("otlp"), TelemetrySettings: componenttest.NewNopTelemetrySettings()}, rc, p)
	require.NoError(t, err)
	require.NoError(t, r.Start(t.Context(), componenttest.NewNopHost()))
	t.Cleanup(func() { require.NoError(t, r.Shutdown(context.Background())) })
	s.receiverURL = "http://" + endpoint + "/v1/logs"
}

type testStoreHandle struct {
	store *store.Store
}

func setupEvalServer(t *testing.T) (*testStoreHandle, *evalTestServer) {
	t.Helper()
	st, _ := storetest.New(t)
	h := New(st, "127.0.0.1", 0)
	t.Cleanup(h.Close)
	mux := http.NewServeMux()
	h.Register(mux, http.NotFoundHandler())
	s := &evalTestServer{Handler: h, mux: mux}
	startEvalReceiver(t, s)
	return &testStoreHandle{store: st}, s
}

func TestCollectorEvalRedactionFidelityAndCacheRebuild(t *testing.T) {
	h, s := setupEvalServer(t)
	input, err := os.ReadFile("../../evaldecode/testdata/promptfoo-v3.json")
	require.NoError(t, err)
	r := httptest.NewRecorder()
	s.mux.ServeHTTP(r, httptest.NewRequest("POST", "/api/evals/ingest?name=synthetic.json", bytes.NewReader(input)))
	require.Equal(t, 200, r.Code, r.Body.String())
	var reply struct {
		IDs []string `json:"ids"`
	}
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &reply))
	id := reply.IDs[0]
	read := func() storeevals.RecordPage {
		var page storeevals.RecordPage
		require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
			var err error
			page, err = storeevals.Records(t.Context(), db, id, 0, 100)
			return err
		}))
		return page
	}
	page := read()
	require.Len(t, page.Rows, 1)
	for _, text := range []string{`"large":9007199254740993`, `"negativeZero":-0`, `"decimal":123.0`, `"stringNumber":"9007199254740993"`, `"null":null`, `"success":false`, `"secret":"****"`, `"password":"****"`} {
		require.Contains(t, string(page.Rows[0]), text)
	}
	require.Contains(t, string(page.Context), `"1e9999"`)
	require.True(t, json.Valid(page.Context))
	require.NotContains(t, string(page.Context), "synthetic-secret")
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		for _, q := range []string{`select count(*) from logs where body::varchar like '%synthetic-secret%' or body::varchar like '%synthetic-nested-secret%'`, `select count(*) from eval_records where payload::varchar like '%synthetic-nested-secret%'`, `select count(*) from eval_datasets where context::varchar like '%synthetic-secret%'`} {
			var n int
			require.NoError(t, db.QueryRow(q).Scan(&n))
			require.Zero(t, n)
		}
		var n int
		require.NoError(t, db.QueryRow(`select count(*) from spans`).Scan(&n))
		require.Zero(t, n)
		require.NoError(t, db.QueryRow(`select count(*) from logs where trace_id is not null or span_id is not null or timestamp<>0`).Scan(&n))
		require.Zero(t, n)
		return nil
	}))
	// Exercise the existing single-log OTLP serializer, then decode the retained
	// body again. This is an internal fidelity contract, not an Evals archive API.
	var refs []string
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		rows, err := db.Query(`select id::varchar from logs`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var ref string
			if err := rows.Scan(&ref); err != nil {
				return err
			}
			refs = append(refs, ref)
		}
		return rows.Err()
	}))
	for _, ref := range refs {
		var raw json.RawMessage
		require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error { var err error; raw, err = logs.GetLogOTLP(t.Context(), db, ref); return err }))
		data, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs(raw)
		require.NoError(t, err)
		part, err := evaldecode.Decode(data.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0))
		require.NoError(t, err)
		if part.Row == 0 {
			require.JSONEq(t, string(page.Rows[0]), string(part.Payload))
			require.Contains(t, string(part.Payload), `"decimal":1.2300e+02`)
		}
	}
	// Destroy every disposable projection/tracking table; retained logs alone
	// recreate the same source identities, values and experiment indexes.
	require.NoError(t, h.store.WithDBWrite(func(db *sql.DB) error {
		for _, table := range []string{"eval_experiment_search", "eval_experiment_rows", "eval_experiments", "eval_experiment_indexes", "eval_records", "eval_datasets", "eval_parts", "eval_imports", "eval_transfers", "eval_evidence"} {
			if _, err := db.Exec(`delete from ` + table); err != nil {
				return err
			}
		}
		return nil
	}))
	require.NoError(t, s.reconcileEvalEvidence(t.Context()))
	require.Equal(t, page, read())
	// The size-based retention path also invalidates all derived copies, rather
	// than leaving an apparently complete Evals dataset after its logs expire.
	require.NoError(t, h.store.EnforceRetention(t.Context(), 1))
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		_, err := storeevals.Records(t.Context(), db, id, 0, 100)
		require.ErrorIs(t, err, storeevals.ErrEvalNotFound)
		return nil
	}))
	// Retention/ordinary log deletion cannot leave source or search copies alive.
	require.NoError(t, h.store.WithDBWrite(func(db *sql.DB) error { return logs.Clear(t.Context(), db) }))
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		for _, table := range []string{"eval_records", "eval_experiment_search", "eval_experiment_rows", "eval_experiments"} {
			var n int
			require.NoError(t, db.QueryRow(`select count(*) from `+table).Scan(&n))
			require.Zero(t, n)
		}
		_, err := storeevals.Records(t.Context(), db, id, 0, 100)
		require.ErrorIs(t, err, storeevals.ErrEvalNotFound)
		return nil
	}))
}

func TestEvalManifestCoverageDuplicatesConflictsAndDeletionGeneration(t *testing.T) {
	h, s := setupEvalServer(t)
	input := `{"evalId":"synthetic","config":{},"results":{"version":3,"results":[{"value":-0}]}}`
	var parts []evaldecode.IngestPart
	require.NoError(t, evaldecode.ParseIngest(t.Context(), strings.NewReader(input), func(p evaldecode.IngestPart) error { parts = append(parts, p); return nil }))
	id := strings.Repeat("d", 64)
	manifest := evaldecode.IngestPart{Envelope: -1, Row: -2, Count: 1, Payload: json.RawMessage(`{"name":"synthetic"}`)}
	require.NoError(t, s.forwardEvalPart(t.Context(), id, manifest))
	require.NoError(t, s.reconcileEvalEvidence(t.Context()))
	complete, _, err := s.evalImportState(t.Context(), id)
	require.NoError(t, err)
	require.False(t, complete)
	for range 2 {
		for _, part := range parts {
			require.NoError(t, s.forwardEvalPart(t.Context(), id, part))
		}
	}
	require.NoError(t, s.reconcileEvalEvidence(t.Context()))
	complete, _, err = s.evalImportState(t.Context(), id)
	require.NoError(t, err)
	require.True(t, complete)
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		var n int
		require.NoError(t, db.QueryRow(`select count(*) from eval_records`).Scan(&n))
		require.Equal(t, 1, n)
		return nil
	}))
	conflict := parts[0]
	conflict.Payload = json.RawMessage(`{"value":"conflicting-value"}`)
	require.NoError(t, s.forwardEvalPart(t.Context(), id, conflict))
	require.NoError(t, s.reconcileEvalEvidence(t.Context()))
	complete, reason, err := s.evalImportState(t.Context(), id)
	require.NoError(t, err)
	require.False(t, complete)
	require.Contains(t, reason, "Conflicting")
	var ref string
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		return db.QueryRow(`select id::varchar from logs where body::varchar like '%conflicting-value%'`).Scan(&ref)
	}))
	require.NoError(t, h.store.WithDBWrite(func(db *sql.DB) error { return logs.DeleteLogsByRefs(t.Context(), db, []any{ref}) }))
	require.NoError(t, s.reconcileEvalEvidence(t.Context()))
	complete, _, err = s.evalImportState(t.Context(), id)
	require.NoError(t, err)
	require.True(t, complete)
	// The existing deliberate dataset tombstone produces a fresh generation on
	// explicit reimport. Rebuilding an index never removes the tombstone.
	post := func() *httptest.ResponseRecorder {
		r := httptest.NewRecorder()
		s.mux.ServeHTTP(r, httptest.NewRequest("POST", "/api/evals/ingest", strings.NewReader(input)))
		return r
	}
	r := post()
	require.Equal(t, 200, r.Code, r.Body.String())
	var first, second struct {
		IDs []string `json:"ids"`
	}
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &first))
	require.NoError(t, h.store.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec(`update eval_datasets set deleted=true where id=?`, first.IDs[0])
		return err
	}))
	r = post()
	require.Equal(t, 200, r.Code, r.Body.String())
	require.NoError(t, json.Unmarshal(r.Body.Bytes(), &second))
	require.NotEqual(t, first.IDs, second.IDs)
}

func TestEvalAcceptedIsNotPersistedCancellationConcurrentRetryAndLateArrival(t *testing.T) {
	h, s := setupEvalServer(t)
	realURL := s.receiverURL
	var mu sync.Mutex
	var accepted [][]byte
	manifestAccepted := make(chan struct{})
	ack := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read", 500)
			return
		}
		mu.Lock()
		accepted = append(accepted, body)
		count := len(accepted)
		mu.Unlock()
		w.Header().Set("Content-Type", "application/x-protobuf")
		w.WriteHeader(200)
		if count == 3 {
			close(manifestAccepted)
		}
	}))
	defer ack.Close()
	s.receiverURL = ack.URL
	input := `{"evalId":"late","config":{},"results":{"version":3,"results":[{"value":9007199254740993}]}}`
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		r := httptest.NewRecorder()
		s.mux.ServeHTTP(r, httptest.NewRequest("POST", "/api/evals/ingest", strings.NewReader(input)).WithContext(ctx))
		done <- r
	}()
	select {
	case <-manifestAccepted:
	case <-time.After(10 * time.Second):
		t.Fatal("submission did not reach manifest")
	}
	r := httptest.NewRecorder()
	s.mux.ServeHTTP(r, httptest.NewRequest("POST", "/api/evals/ingest", strings.NewReader(input)))
	require.Equal(t, 409, r.Code, r.Body.String())
	select {
	case r := <-done:
		t.Fatalf("receiver acceptance completed import: %d %s", r.Code, r.Body.String())
	default:
	}
	cancel()
	select {
	case r := <-done:
		require.NotEqual(t, 200, r.Code)
	case <-time.After(10 * time.Second):
		t.Fatal("cancellation did not stop wait")
	}
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		for _, table := range []string{"logs", "eval_records", "eval_datasets"} {
			var n int
			require.NoError(t, db.QueryRow(`select count(*) from `+table).Scan(&n))
			require.Zero(t, n)
		}
		return nil
	}))
	mu.Lock()
	queued := append([][]byte(nil), accepted...)
	mu.Unlock()
	for _, body := range queued {
		response, err := http.Post(realURL, "application/x-protobuf", bytes.NewReader(body))
		require.NoError(t, err)
		require.Equal(t, 200, response.StatusCode)
		require.NoError(t, response.Body.Close())
	}
	require.NoError(t, s.reconcileEvalEvidence(t.Context()))
	s.receiverURL = realURL
	r = httptest.NewRecorder()
	s.mux.ServeHTTP(r, httptest.NewRequest("POST", "/api/evals/ingest", strings.NewReader(input)))
	require.Equal(t, 200, r.Code, r.Body.String())
	require.Contains(t, r.Body.String(), `"created":false`)
}

func TestEvalForwardingRejectsPartialAndMalformedAcknowledgements(t *testing.T) {
	partial := plogotlp.NewExportResponse()
	partial.PartialSuccess().SetRejectedLogRecords(1)
	partial.PartialSuccess().SetErrorMessage("synthetic rejection")
	partialBytes, err := partial.MarshalProto()
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		status int
		body   []byte
	}{
		{"partial", 200, partialBytes},
		{"malformed", 200, []byte("invalid protobuf")},
		{"unavailable", 503, nil},
		{"redirect", 307, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/x-protobuf")
				w.Header().Set("Location", "/unexpected")
				w.WriteHeader(tc.status)
				_, _ = w.Write(tc.body)
			}))
			defer receiver.Close()
			s := &Handler{receiverURL: receiver.URL, receiverClient: newReceiverClient()}
			t.Cleanup(s.receiverClient.CloseIdleConnections)
			err := s.forwardEvalPart(t.Context(), strings.Repeat("a", 64), evaldecode.IngestPart{Payload: json.RawMessage(`{"value":1}`)})
			require.Error(t, err)
		})
	}
}

func TestEvalExperimentSummarySearchAndScopedDetail(t *testing.T) {
	h, s := setupEvalServer(t)
	input := `{"evalId":"experiment","config":{"description":"Experiment 2"},"results":{"version":3,"timestamp":"2026-10-21T10:00:00Z","results":[
{"provider":{"id":"a"},"promptId":"p","success":true,"future":9007199254740993,"id":"firstonly","score":0.95,"response":{"output":"exact phrase"}},
{"provider":{"id":"b"},"promptId":"p","success":true,"answer":"OTHER RUN"},
{"provider":{"id":"a"},"promptId":"p","gradingResult":{"pass":false,"reason":"Missing idempotency key"},"id":"secondonly","score":0.25,"success":false},
{"provider":{"id":"a"},"promptId":"q","failureReason":2},
{"provider":{"id":"a"},"promptId":"p","success":true},
{"id":"missing-provider-and-prompt"}
]}}`
	r := httptest.NewRecorder()
	s.mux.ServeHTTP(r, httptest.NewRequest("POST", "/api/evals/ingest", strings.NewReader(input)))
	require.Equal(t, 200, r.Code, r.Body.String())
	search := func(path string) storeevals.ExperimentSearchPage {
		r := httptest.NewRecorder()
		s.mux.ServeHTTP(r, httptest.NewRequest("GET", path, nil))
		require.Equal(t, 200, r.Code, r.Body.String())
		var page storeevals.ExperimentSearchPage
		require.NoError(t, json.Unmarshal(r.Body.Bytes(), &page))
		return page
	}
	condition := func(field, op, value string) querysearch.QueryNode {
		scope := "field"
		if field == "" {
			scope = "global"
		}
		return querysearch.QueryNode{Type: "condition", Query: &querysearch.Query{Field: &querysearch.FieldDefinition{Name: field, SearchScope: scope}, FieldOperator: op, Value: value}}
	}
	group := func(op string, children ...querysearch.QueryNode) querysearch.QueryNode {
		return querysearch.QueryNode{Type: "group", Group: &querysearch.QueryGroup{LogicalOperator: op, Children: children}}
	}
	criteriaURL := func(node querysearch.QueryNode) string {
		raw, err := json.Marshal(node)
		require.NoError(t, err)
		return "/api/evals/experiments?criteria=" + url.QueryEscape(string(raw))
	}
	metadataURL := criteriaURL(condition("", "CONTAINS", "Experiment 2"))
	page := search("/api/evals/experiments?sort=name&limit=1")
	require.Equal(t, int64(4), page.Total)
	require.True(t, page.HasMore)
	require.Len(t, page.Experiments, 1)
	require.Zero(t, page.Indexing.Remaining)
	filtered := search(metadataURL + "&outcome=error%3AFail")
	require.Equal(t, int64(1), filtered.Total)
	require.Len(t, filtered.Experiments, 1)
	experiment := filtered.Experiments[0]
	require.Equal(t, int64(3), experiment.ResultCount)
	require.Nil(t, experiment.PlannedCount)
	require.Equal(t, "Unknown", experiment.State)
	require.Equal(t, []storeevals.OutcomeCount{{Key: "success:Pass", Label: "Pass", Tone: "success", Count: 2}, {Key: "error:Fail", Label: "Fail", Tone: "error", Count: 1}}, experiment.Outcomes)
	// Facets count whole experiments, before the outcome filter, not rows in this page.
	require.Equal(t, int64(2), filtered.Facets[0].Count)
	// Counts and facets survive an empty page, whether caused by the cursor or
	// by an outcome filter. No placeholder experiment escapes the left join.
	beyond := search(metadataURL + "&outcome=error%3AFail&offset=100")
	require.Equal(t, filtered.Total, beyond.Total)
	require.Equal(t, filtered.Facets, beyond.Facets)
	require.Empty(t, beyond.Experiments)
	require.False(t, beyond.HasMore)
	noOutcome := search(metadataURL + "&outcome=warning%3APartial")
	require.Zero(t, noOutcome.Total)
	require.Equal(t, filtered.Facets, noOutcome.Facets)
	require.Empty(t, noOutcome.Experiments)
	var ordinals []int64
	after := int64(-1)
	for {
		var detail storeevals.ExperimentSourcePage
		require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
			var err error
			detail, err = storeevals.GetExperimentPage(t.Context(), db, experiment.ID, after, 1)
			return err
		}))
		require.Equal(t, experiment, detail.Summary)
		require.NotContains(t, detail.Rows[0].JSON, "OTHER RUN")
		ordinals = append(ordinals, detail.Rows[0].Ordinal)
		if after == -1 {
			require.Contains(t, detail.Rows[0].JSON, "9007199254740993")
		}
		if !detail.HasMore {
			break
		}
		after = detail.NextAfter
	}
	require.Equal(t, []int64{0, 2, 4}, ordinals)
	r = httptest.NewRecorder()
	s.mux.ServeHTTP(r, httptest.NewRequest("GET", "/api/evals/experiments/"+experiment.ID+"/rows?limit=1", nil))
	require.Equal(t, 200, r.Code)
	require.Contains(t, r.Body.String(), "9007199254740993")
	// Selection remains retrievable when it is absent from a different list filter.
	require.Zero(t, search(criteriaURL(condition("", "CONTAINS", "not-present"))).Total)
	require.Equal(t, int64(1), search(criteriaURL(condition("", "CONTAINS", "idempotency key"))).Total)
	for _, tc := range []struct {
		name string
		node querysearch.QueryNode
		want int64
	}{
		{"different results cannot satisfy AND", group("AND", condition("", "CONTAINS", "firstonly"), condition("", "CONTAINS", "secondonly")), 0},
		{"same result can satisfy AND", group("AND", condition("", "CONTAINS", "secondonly"), condition("", "CONTAINS", "idempotency")), 1},
		{"parent metadata and result", group("AND", condition("evaluation", "=", "Experiment 2"), condition("result.id", "=", "secondonly")), 1},
		{"nested Boolean groups", group("AND", group("OR", condition("result.id", "=", "firstonly"), condition("result.id", "=", "secondonly")), condition("result.score", "<", "0.5")), 1},
		{"typed values on different results", group("AND", condition("result.id", "=", "firstonly"), condition("result.score", "<", "0.5")), 0},
		{"numeric list", condition("result.score", "IN", `["0.25", "0.5"]`), 1},
		{"boolean", condition("result.success", "=", "false"), 1},
		{"boolean list", condition("result.success", "IN", `["false"]`), 1},
		{"ordinal numeric comparison", condition("result.ordinal", ">", "4"), 1},
		{"missing result field", condition("result.id", "IS NULL", ""), 3},
		{"phrase", condition("result.output", "CONTAINS", "exact phrase"), 1},
		{"case follows shared search", condition("result.output", "CONTAINS", "EXACT PHRASE"), 0},
		{"regex", condition("result.provider", "REGEXP", "[ab]"), 3},
		{"set", condition("result.prompt", "IN", `["p"]`), 2},
		{"negative predicate stays on row", group("AND", condition("result.id", "=", "firstonly"), condition("result.text", "NOT CONTAINS", "secondonly")), 1},
		{"SQL-looking value is literal", condition("name", "=", "' OR true --"), 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := search(criteriaURL(tc.node))
			require.Equal(t, tc.want, got.Total)
			if tc.name == "nested Boolean groups" {
				require.Equal(t, experiment, got.Experiments[0])
			}
		})
	}
	for _, invalid := range []string{
		"/api/evals/experiments?q=old-matcher",
		"/api/evals/experiments?criteria=%7B",
		"/api/evals/experiments?criteria=null",
		criteriaURL(condition("unknown", "=", "x")),
		criteriaURL(condition("result.ordinal", "=", "9223372036854775808")),
		criteriaURL(condition("result.ordinal", ">", "1.5")),
		criteriaURL(condition("result.score", "CONTAINS", "1")),
		criteriaURL(condition("result.success", "=", "yes")),
		criteriaURL(group("XOR", condition("name", "=", "x"))),
	} {
		r := httptest.NewRecorder()
		s.mux.ServeHTTP(r, httptest.NewRequest("GET", invalid, nil))
		require.Equal(t, 400, r.Code, r.Body.String())
	}
	r = httptest.NewRecorder()
	s.mux.ServeHTTP(r, httptest.NewRequest("GET", "/api/evals/experiments/"+experiment.ID+"/rows", nil))
	require.Equal(t, 200, r.Code)
}

type evalUploadReader struct {
	io.Reader
	check func()
}

func (r *evalUploadReader) Read(p []byte) (int, error) {
	if r.check != nil {
		check := r.check
		r.check = nil
		check()
	}
	return r.Reader.Read(p)
}

func TestEvalIngestionHandlesLargeNativePayload(t *testing.T) {
	h, s := setupEvalServer(t)
	row := `{"answer":"` + strings.Repeat("x", 3<<20) + `","exact":9007199254740993}`
	input := `{"evalId":"large","config":{},"results":{"version":3,"results":[` + strings.TrimSuffix(strings.Repeat(row+",", 6), ",") + `]}}`
	reader := &evalUploadReader{Reader: strings.NewReader(input), check: func() {
		// A producer can be slow: neither readers nor writes wait on upload.
		require.NoError(t, h.store.WithDBWrite(func(db *sql.DB) error {
			var count int
			require.NoError(t, db.QueryRow(`select count(*) from eval_imports`).Scan(&count))
			require.Zero(t, count)
			return nil
		}))
	}}
	response := httptest.NewRecorder()
	s.mux.ServeHTTP(response, httptest.NewRequest("POST", "/api/evals/ingest", reader))
	require.Equal(t, 200, response.Code, response.Body.String())
	var imported struct {
		IDs []string `json:"ids"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &imported))
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		page, err := storeevals.Records(t.Context(), db, imported.IDs[0], 0, 100)
		require.NoError(t, err)
		require.Len(t, page.Rows, 1)
		require.Equal(t, int64(6), page.Total)
		require.True(t, page.HasMore)
		var count int
		require.NoError(t, db.QueryRow(`select count(*) from eval_records`).Scan(&count))
		require.Equal(t, 6, count)
		return nil
	}))
}

func TestEvalIngestionAtomicBatchesRetryAndQuery(t *testing.T) {
	h, s := setupEvalServer(t)
	row := `{"id":"source-id","future":9007199254740993,"value":-0,"missingIsNotNull":null,"unicode":"雪😀"}`
	input := `{"evalId":"source-run","config":{},"results":{"version":3,"results":[` + strings.TrimSuffix(strings.Repeat(row+",", 130), ",") + `]}}`
	post := func(body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		s.mux.ServeHTTP(response, httptest.NewRequest("POST", "/api/evals/ingest?name=received.jsonl", strings.NewReader(body)))
		return response
	}
	bad := post(input + "\n{\"late\":")
	require.Equal(t, 400, bad.Code)
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		var count int
		require.NoError(t, db.QueryRow(`select count(*) from eval_imports`).Scan(&count))
		require.Zero(t, count)
		return nil
	}))
	response := post(input)
	require.Equal(t, 200, response.Code, response.Body.String())
	var imported struct {
		IDs      []string `json:"ids"`
		Created  bool     `json:"created"`
		Complete bool     `json:"complete"`
	}
	require.NoError(t, json.Unmarshal(response.Body.Bytes(), &imported))
	require.True(t, imported.Created)
	require.True(t, imported.Complete)
	require.Len(t, imported.IDs, 1)
	id := imported.IDs[0]
	var importID string
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		return db.QueryRow(`select import_id from eval_datasets where id = ?`, id).Scan(&importID)
	}))
	// Explicit reads can reconcile a completed transfer repeatedly.
	require.NoError(t, s.reconcileEvalEvidence(t.Context()))
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		page, err := storeevals.Records(t.Context(), db, id, 64, 2)
		require.NoError(t, err)
		require.Equal(t, int64(130), page.Total)
		require.Len(t, page.Rows, 2)
		require.JSONEq(t, row, string(page.Rows[0]))
		require.Contains(t, string(page.Rows[0]), `9007199254740993`)
		require.Contains(t, string(page.Rows[0]), `"value":-0`)
		require.True(t, page.HasMore)
		list, err := storeevals.SearchExperiments(t.Context(), db, storeevals.ExperimentSearch{Sort: "started", Direction: "desc", Limit: 10})
		require.NoError(t, err)
		require.Equal(t, int64(130), list.Total)
		return nil
	}))
	response = post(input)
	require.Equal(t, 200, response.Code)
	require.Contains(t, response.Body.String(), `"created":false`)
	// Simulate a prior process interrupted before publication; retained evidence
	// rebuilds the hidden cache without submitting the file a second time.
	require.NoError(t, h.store.WithDBWrite(func(db *sql.DB) error {
		if _, err := db.Exec(`update eval_imports set complete = false`); err != nil {
			return err
		}
		_, err := db.Exec(`update eval_transfers set dirty=true`)
		return err
	}))
	require.NoError(t, h.store.WithDBRead(func(db *sql.DB) error {
		list, err := storeevals.SearchExperiments(t.Context(), db, storeevals.ExperimentSearch{Sort: "started", Direction: "desc", Limit: 10})
		require.NoError(t, err)
		require.Empty(t, list.Experiments)
		return nil
	}))
	response = post(input)
	require.Equal(t, 200, response.Code, response.Body.String())
	require.Contains(t, response.Body.String(), `"created":false`)
}

func TestEvalForwardingBypassesEnvironmentProxy(t *testing.T) {
	// ProxyFromEnvironment caches process environment on its first invocation.
	if os.Getenv("VIEWER_EVAL_PROXY_TEST_CHILD") != "1" {
		executable, err := os.Executable()
		require.NoError(t, err)
		cmd := exec.CommandContext(t.Context(), executable, "-test.run=^TestEvalForwardingBypassesEnvironmentProxy$")
		cmd.Env = append(os.Environ(), "VIEWER_EVAL_PROXY_TEST_CHILD=1")
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, string(output))
		return
	}
	var proxyCalls, dials atomic.Int32
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxyCalls.Add(1)
		http.Error(w, "unexpected pre-redaction proxy traffic", 502)
	}))
	defer proxy.Close()
	for _, key := range []string{"HTTP_PROXY", "http_proxy", "HTTPS_PROXY", "https_proxy"} {
		t.Setenv(key, proxy.URL)
	}
	for _, key := range []string{"NO_PROXY", "no_proxy", "REQUEST_METHOD"} {
		t.Setenv(key, "")
	}
	const authority = "192.168.1.20:4318"
	request, err := http.NewRequest(http.MethodPost, "http://"+authority+"/v1/logs", nil)
	require.NoError(t, err)
	selected, err := http.ProxyFromEnvironment(request)
	require.NoError(t, err)
	require.NotNil(t, selected)
	require.Equal(t, proxy.URL, selected.String())
	type received struct {
		path, host, contentType string
		body                    []byte
		err                     error
	}
	arrivals := make(chan received, 2)
	receiver := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		arrivals <- received{r.URL.Path, r.Host, r.Header.Get("Content-Type"), body, err}
		w.Header().Set("Content-Type", "application/x-protobuf")
	}))
	defer receiver.Close()
	s := New(nil, "192.168.1.20", 4318)
	t.Cleanup(s.Close)
	transport := s.receiverClient.Transport.(*http.Transport)
	// Map only the intended receiver to our local listener. A proxy dial remains
	// a real proxy dial, so accidental proxy selection cannot pass unnoticed.
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		if address == authority {
			dials.Add(1)
			address = receiver.Listener.Addr().String()
		}
		return (&net.Dialer{}).DialContext(ctx, network, address)
	}
	for range 2 {
		require.NoError(t, s.forwardEvalPart(t.Context(), strings.Repeat("a", 64), evaldecode.IngestPart{Payload: json.RawMessage(`{"password":"synthetic-before-redaction"}`)}))
		got := <-arrivals
		require.NoError(t, got.err)
		require.Equal(t, "/v1/logs", got.path)
		require.Equal(t, authority, got.host)
		require.Equal(t, "application/x-protobuf", got.contentType)
		exported := plogotlp.NewExportRequest()
		require.NoError(t, exported.UnmarshalProto(got.body))
		source, ok := exported.Logs().ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0).Body().Map().Get("source")
		require.True(t, ok)
		password, ok := source.Map().Get("password")
		require.True(t, ok)
		require.Equal(t, "synthetic-before-redaction", password.Str())
	}
	require.Zero(t, proxyCalls.Load())
	require.Equal(t, int32(1), dials.Load(), "reuse the internal connection pool across parts")
}
