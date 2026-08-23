package attributes_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/attributes"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

type match struct {
	Name           string   `json:"name"`
	AttributeScope string   `json:"attributeScope"`
	Type           string   `json:"type"`
	MatchCount     int      `json:"matchCount"`
	SampleValues   []string `json:"sampleValues"`
}

func setup(t *testing.T) (*store.Store, context.Context) {
	t.Helper()
	s, ctx := storetest.New(t)

	td := ptrace.NewTraces()
	rs := td.ResourceSpans().AppendEmpty()
	rs.Resource().Attributes().PutStr("service.name", "checkout-api")
	ss := rs.ScopeSpans().AppendEmpty()
	ss.Scope().SetName("otelhttp")
	// More routes than maxSampleValues, so a missing bound is visible: an
	// unbounded sample list would return all six.
	for i, route := range []string{
		"/checkout", "/checkout/confirm", "/health",
		"/cart", "/orders", "/orders/history",
	} {
		sp := ss.Spans().AppendEmpty()
		sp.SetTraceID([16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16})
		sp.SetSpanID([8]byte{byte(i + 1), 2, 3, 4, 5, 6, 7, 8})
		sp.Attributes().PutStr("http.route", route)
		sp.Attributes().PutInt("http.status_code", int64(200+i))
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, td, s.FlushedIDs())
	}))

	// A log from the same service, so a cross-signal match is possible.
	ld := plog.NewLogs()
	rl := ld.ResourceLogs().AppendEmpty()
	rl.Resource().Attributes().PutStr("service.name", "checkout-api")
	sl := rl.ScopeLogs().AppendEmpty()
	lr := sl.LogRecords().AppendEmpty()
	lr.SetTimestamp(pcommon.Timestamp(1_700_000_000_000_000_000))
	lr.Body().SetStr("done")
	lr.Attributes().PutStr("log.origin", "checkout-handler")
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return logs.Ingest(ctx, conn, ld, s.FlushedIDs())
	}))

	return s, ctx
}

func search(t *testing.T, s *store.Store, ctx context.Context, term string) []match {
	t.Helper()
	var out []match
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		raw, err := attributes.Search(ctx, db, term)
		if err != nil {
			return err
		}
		return json.Unmarshal(raw, &out)
	}))
	return out
}

// The question the feature exists to answer: a value seen in the UI, traced
// back to the keys that hold it -- across signals, in one call.
func TestSearchFindsKeysByValue(t *testing.T) {
	t.Parallel()
	s, ctx := setup(t)
	got := search(t, s, ctx, "checkout")

	byKey := map[string]match{}
	for _, m := range got {
		byKey[m.Name+"/"+m.AttributeScope] = m
	}

	// A resource attribute (shared by traces and logs), a span attribute, and a
	// log attribute -- three scopes, two signals, one query.
	assert.Contains(t, byKey, "service.name/resource")
	assert.Contains(t, byKey, "http.route/span")
	assert.Contains(t, byKey, "log.origin/log")

	// http.route has two matching values (/checkout and /checkout/confirm) but
	// three distinct values overall, so the count must reflect the match, not
	// the key's cardinality.
	assert.Equal(t, 2, byKey["http.route/span"].MatchCount)
	assert.ElementsMatch(t,
		[]string{"/checkout", "/checkout/confirm"},
		byKey["http.route/span"].SampleValues)

	// /health does not match, so it must not be offered as an example.
	assert.NotContains(t, byKey["http.route/span"].SampleValues, "/health")
}

func TestSearchMatchesKeyNamesToo(t *testing.T) {
	t.Parallel()
	s, ctx := setup(t)
	// "status" appears in no value, only in a key name.
	got := search(t, s, ctx, "status")
	require.Len(t, got, 1)
	assert.Equal(t, "http.status_code", got[0].Name)
}

func TestSearchIsCaseInsensitive(t *testing.T) {
	t.Parallel()
	s, ctx := setup(t)
	assert.ElementsMatch(t, search(t, s, ctx, "CHECKOUT"), search(t, s, ctx, "checkout"))
}

func TestSearchEmptyAndNoMatch(t *testing.T) {
	t.Parallel()
	s, ctx := setup(t)
	assert.Empty(t, search(t, s, ctx, ""), "an empty term is not a request for everything")
	assert.Empty(t, search(t, s, ctx, "   "))
	assert.Empty(t, search(t, s, ctx, "no-such-value-anywhere"))
}

// A literal % in the box must be a literal, not "match everything". Without
// escaping, typing % returns the entire dictionary, which looks like a bug and
// is a slow one.
func TestSearchEscapesLikeWildcards(t *testing.T) {
	t.Parallel()
	s, ctx := setup(t)
	assert.Empty(t, search(t, s, ctx, "%"), "%% must be a literal, not a wildcard")
	assert.Empty(t, search(t, s, ctx, "_"))
	assert.NotEmpty(t, search(t, s, ctx, "checkout"), "control: real terms still match")
}

// Samples are bounded, or a broad term returns the dictionary through the back
// door.
func TestSearchBoundsSampleValues(t *testing.T) {
	t.Parallel()
	s, ctx := setup(t)
	got := search(t, s, ctx, "/")

	var routes match
	for _, m := range got {
		if m.Name == "http.route" {
			routes = m
		}
	}
	require.NotZero(t, routes.MatchCount, "http.route should match")

	// The guard: more values match than are sampled, so the cap is doing work
	// rather than being satisfied by a small fixture.
	assert.Greater(t, routes.MatchCount, 3,
		"fixture must have more matches than the sample cap, or this proves nothing")
	assert.Len(t, routes.SampleValues, 3, "sample list must be capped")
}
