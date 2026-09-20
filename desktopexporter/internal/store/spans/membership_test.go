package spans_test

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/search"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/spans"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpanMembershipExecutionAndExistentialSemantics(t *testing.T) {
	t.Parallel()
	s, ctx := storetest.New(t)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return spans.Ingest(ctx, conn, createTestTracePdata(), s.FlushedIDs())
	}))
	const traceID = "00000000000000000000000000000099"

	searchSpans := func(name, operator, value string) json.RawMessage {
		t.Helper()
		query := &search.QueryNode{Type: "condition", Query: &search.Query{
			Field:         &search.FieldDefinition{Name: name, SearchScope: "field"},
			FieldOperator: operator,
			Value:         value,
		}}
		var raw json.RawMessage
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			var err error
			raw, err = spans.SearchSpans(ctx, db, traceID, query)
			return err
		}))
		return raw
	}
	matchedNames := func(raw json.RawMessage) []string {
		t.Helper()
		var trace struct {
			Spans []struct {
				Matched  bool `json:"matched"`
				SpanData struct {
					Name string `json:"name"`
				} `json:"spanData"`
			} `json:"spans"`
		}
		require.NoError(t, json.Unmarshal(raw, &trace))
		var names []string
		for _, span := range trace.Spans {
			if span.Matched {
				names = append(names, span.SpanData.Name)
			}
		}
		return names
	}

	t.Run("duration list", func(t *testing.T) {
		raw := searchSpans("duration", "IN", `["1s"]`)
		assert.Equal(t, []string{"root-operation"}, matchedNames(raw))
	})

	t.Run("nullable NOT IN excludes SQL null", func(t *testing.T) {
		raw := searchSpans("parentSpanID", "NOT IN", `["not-a-span"]`)
		assert.Len(t, matchedNames(raw), 8, "the root has a SQL-null parent and must not match")
	})

	t.Run("multi-event NOT IN stays existential", func(t *testing.T) {
		raw := searchSpans("event.name", "NOT IN", `["root-event-1"]`)
		assert.ElementsMatch(t, []string{"root-operation", "child-operation"}, matchedNames(raw), "another event on the same span satisfies NOT IN")
	})

	t.Run("wire ID list malformed value matches nothing", func(t *testing.T) {
		raw := searchSpans("spanID", "IN", `["not-a-span"]`)
		assert.Empty(t, matchedNames(raw))
	})

	t.Run("received numeric enum lists", func(t *testing.T) {
		assert.Equal(t, []string{"root-operation"}, matchedNames(searchSpans("kindCode", "IN", `["2"]`)))
		assert.Equal(t, []string{
			"child-operation",
			"great-grandchild-operation",
			"orphaned-grandchild-operation",
		}, matchedNames(searchSpans("statusCodeValue", "IN", `["2"]`)))
	})

	t.Run("all mapper modes execute with NOT IN", func(t *testing.T) {
		for _, tc := range []struct {
			name  string
			value string
		}{
			{"name", `["absent"]`},
			{"flags", `["99"]`},
			{"duration", `["99ns"]`},
			{"spanID", `["ffffffffffffffff"]`},
		} {
			assert.NotEmpty(t, searchSpans(tc.name, "NOT IN", tc.value), tc.name)
		}
	})
}
