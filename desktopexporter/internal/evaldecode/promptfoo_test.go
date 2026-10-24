package evaldecode

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseIngestPreservesProducerContextAndExactValues(t *testing.T) {
	row := `{"id":"one","value":9007199254740993,"decimal":1.2300e9999,"zero":-0,"null":null,"false":false,"unknown":{"雪":"😀"}}`
	promptfoo := `{"evalId":"run","config":{"unknown":true},"results":{"version":3,"results":[` + row + `],"stats":{"x":18446744073709551615}},"future":null}`
	second := `{"evalId":"run-2","config":{},"results":{"version":3,"results":[{}]}}`
	var parts []IngestPart
	err := ParseIngest(t.Context(), strings.NewReader(promptfoo+"\n"+second+"\n"), func(p IngestPart) error { parts = append(parts, p); return nil })
	require.NoError(t, err)
	require.Len(t, parts, 4)
	require.Equal(t, row, string(parts[0].Payload))
	require.Equal(t, "promptfoo-v3", parts[1].Format)
	require.Equal(t, "promptfoo-v3", parts[3].Format)
	require.Contains(t, string(parts[1].Payload), `"x":18446744073709551615`)
	require.True(t, json.Valid(parts[1].Payload))
	require.Contains(t, string(parts[1].Payload), `"results":[]`)
	require.Equal(t, int64(len(promptfoo)), parts[1].Bytes)
}

func TestParseIngestRejectsAmbiguityAndInvalidData(t *testing.T) {
	for _, input := range []string{
		`{"id":"bare-result","success":true}`,
		`{"evalId":"run","config":{},"results":{"version":2,"results":[]}}`,
		`{"evalId":"run","config":{},"results":{"version":3,"results":[null]}}`,
		`{"evalId":"run","config":{},"results":{"version":3,"results":[{"x":"\ud800"}]}}`,
		`{"evalId":"run","config":{},"results":{"version":3,"results":[{"x":1,"\u0078":2}]}}`,
		`{"evalId":"run","config":{},"resourceLogs":[],"results":{"version":3,"results":[]}}`,
		`{"manifest":{"models":[]},"pilot":{"results":{"results":[]}},"results":{"version":3,"results":[]}}`,
		`{"evalId":"run","config":{},"results":{"version":3,"results":[]}} {}`,
		`{"evalId":"run","config":{},"results":{"version":3,"results":[]},"late":`,
		"{\"evalId\":\"\xff\",\"config\":{},\"results\":{\"version\":3,\"results\":[]}}",
	} {
		t.Run(input[:min(len(input), 70)], func(t *testing.T) {
			require.Error(t, ParseIngest(t.Context(), strings.NewReader(input), func(IngestPart) error { return nil }))
		})
	}
	large := `{"evalId":"run","config":{},"results":{"version":3,"results":[{"answer":"` + strings.Repeat("a", MaxRecordBytes) + `"}]}}`
	require.ErrorContains(t, ParseIngest(t.Context(), strings.NewReader(large), func(IngestPart) error { return nil }), "4 MiB")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	require.ErrorIs(t, ParseIngest(ctx, strings.NewReader(`{}`), func(IngestPart) error { t.Fatal("emitted after cancel"); return nil }), context.Canceled)
}
