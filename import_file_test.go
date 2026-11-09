package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func importTestBatches(t *testing.T, input string, maximum int64) ([3][]string, []string) {
	t.Helper()
	source := strings.NewReader(input)
	plan, err := scanImportFile(t.Context(), source, int64(len(input)))
	require.NoError(t, err)
	issues := plan.issues
	var batches [3][]string
	for kind, resources := range plan.resources {
		err := batchImportFile(t.Context(), source, kind, resources, maximum, func(reader io.Reader, size int64) error {
			body, err := io.ReadAll(reader)
			require.NoError(t, err)
			require.Equal(t, int64(len(body)), size)
			require.LessOrEqual(t, size, maximum)
			require.True(t, json.Valid(body), string(body))
			batches[kind] = append(batches[kind], string(body))
			return nil
		}, func(issue string) { issues = append(issues, issue) })
		require.NoError(t, err)
	}
	return batches, issues
}

func TestImportScanPreservesBytesAndMixedJSONL(t *testing.T) {
	resource := `{"resource":{"attributes":[{"key":"precise","value":{"intValue":18446744073709551615}}]},"scopeSpans":[{"scope":{"name":"x"},"spans":[{"startTimeUnixNano":18446744073709551615,"attributes":[{"key":"n","value":{"doubleValue":-0}},{"key":"i","value":{"intValue":-9223372036854775808}},{"key":"text","value":{"stringValue":"\\u1234"}}]}]}]}`
	input := "\xef\xbb\xbf" + `{"resourceSpans":[` + resource + `],"resourceLogs":[{}],"resourceMetrics":[{}]}` + "\n" + `{"resourceSpans":[{}]}`
	batches, issues := importTestBatches(t, input, importRequestBytes)
	require.Empty(t, issues)
	require.Equal(t, []string{`{"resourceSpans":[` + resource + `,{}]}`}, batches[0])
	require.Equal(t, []string{`{"resourceLogs":[{}]}`}, batches[1])
	require.Equal(t, []string{`{"resourceMetrics":[{}]}`}, batches[2])
}

func TestImportSplitMetadataAndMetricAssociation(t *testing.T) {
	point := `{"asInt":"9223372036854775807","timeUnixNano":"18446744073709551615"}`
	input := `{"resourceMetrics":[{"resource":{"attributes":[]},"scopeMetrics":[{"scope":{"name":"scope"},"metrics":[{"name":"metric","unit":"s","gauge":{"dataPoints":[` + point + `,` + point + `,` + point + `]},"metadata":[{"key":"z","value":{"doubleValue":-0}}]}],"schemaUrl":"scope-schema"}],"schemaUrl":"resource-schema"}]}`
	batches, issues := importTestBatches(t, input, int64(len(input)-len(point)-1))
	require.Empty(t, issues)
	require.Len(t, batches[2], 2)
	count := 0
	for _, batch := range batches[2] {
		for _, metadata := range []string{`"resource":{"attributes":[]}`, `"name":"scope"`, `"name":"metric","unit":"s"`, `"doubleValue":-0`, `"schemaUrl":"scope-schema"`, `"schemaUrl":"resource-schema"`} {
			require.Contains(t, batch, metadata)
		}
		count += strings.Count(batch, point)
	}
	require.Equal(t, 3, count)
}

func TestImportTwentyMiBBatchingAndOversizedRecord(t *testing.T) {
	record := `{"body":{"stringValue":"` + strings.Repeat("a", 11*1024*1024) + `"}}`
	input := `{"resourceLogs":[{"scopeLogs":[{"logRecords":[` + record + `,` + record + `]}]}]}`
	batches, issues := importTestBatches(t, input, importRequestBytes)
	require.Empty(t, issues)
	require.Len(t, batches[1], 2)
	tooBig := `{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"stringValue":"` + strings.Repeat("b", int(importRequestBytes)) + `"}},{}]}]}]}`
	batches, issues = importTestBatches(t, tooBig, importRequestBytes)
	require.Len(t, batches[1], 1)
	require.Len(t, issues, 1)
	require.Contains(t, issues[0], "individual OTLP record")
}

func TestImportMalformedCollectionsNeverOmittedWhenSplitting(t *testing.T) {
	input := `{"resourceSpans":[{"scopeSpans":[{"spans":[null,{"name":"` + strings.Repeat("a", 100) + `"}]}]}]}`
	batches, issues := importTestBatches(t, input, int64(len(input)-1))
	require.Empty(t, batches[0])
	require.Len(t, issues, 1)
	require.Contains(t, issues[0], "Invalid OTLP collection")
}

func TestImportScanSyntaxAndCancellation(t *testing.T) {
	for _, input := range []string{`{"resourceSpans":[{}]}` + "\n" + `{"resourceLogs":[`, `{"resourceSpans":[{}]}{"resourceSpans":[{}]}`, `[]`, `{"resourceSpans":[{}],}`} {
		_, err := scanImportFile(t.Context(), strings.NewReader(input), int64(len(input)))
		require.Error(t, err, input)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err := scanImportFile(ctx, bytes.NewReader(nil), 0)
	require.ErrorIs(t, err, context.Canceled)
}

func TestImportWrapperIssuesAndEmptyFiles(t *testing.T) {
	batches, issues := importTestBatches(t, `{"resourceProfiles":[],"wrong":[],"resourceSpans":null,"resourceLogs":[true,{}]}`, importRequestBytes)
	require.Len(t, issues, 4)
	require.Contains(t, issues[0], "coming soon")
	require.Equal(t, []string{`{"resourceLogs":[{}]}`}, batches[1])
	_, issues = importTestBatches(t, "", importRequestBytes)
	require.Equal(t, []string{"No OTLP resource wrappers found (byte 1)"}, issues)
}

func TestImportSplitsResourcesScopesAndRecordsAtExactBoundaries(t *testing.T) {
	for _, key := range []string{"gauge", "sum", "histogram", "exponentialHistogram", "summary"} {
		t.Run(key, func(t *testing.T) {
			point := `{"asDouble":-0.000e+10,"count":18446744073709551615}`
			metric := `{"name":"` + key + `","` + key + `":{"dataPoints":[` + point + `,` + point + `]}}`
			input := `{"resourceMetrics":[{"scopeMetrics":[{"metrics":[` + metric + `]}]}]}`
			batches, issues := importTestBatches(t, input, int64(len(input)-len(point)-1))
			require.Empty(t, issues)
			require.Len(t, batches[2], 2)
			for _, batch := range batches[2] {
				require.Contains(t, batch, point)
				require.Contains(t, batch, `"name":"`+key+`"`)
			}
		})
	}
	resource := `{"resource":{"attributes":[]},"scopeSpans":[{"scope":{"name":"first"},"spans":[{"name":"one"}]},{"scope":{"name":"second"},"spans":[{"name":"two"}]}]}`
	input := `{"resourceSpans":[` + resource + `,` + resource + `]}`
	maximum := int64(len(`{"resourceSpans":[` + resource + `]}`))
	batches, issues := importTestBatches(t, input, maximum)
	require.Empty(t, issues)
	require.Equal(t, []string{`{"resourceSpans":[` + resource + `]}`, `{"resourceSpans":[` + resource + `]}`}, batches[0])
	batches, issues = importTestBatches(t, input, maximum-1)
	require.Empty(t, issues)
	require.Len(t, batches[0], 4)
	for i, batch := range batches[0] {
		require.Contains(t, batch, `"resource":{"attributes":[]}`)
		if i%2 == 0 {
			require.Contains(t, batch, `"name":"first"`)
			require.NotContains(t, batch, `"name":"second"`)
		} else {
			require.Contains(t, batch, `"name":"second"`)
			require.NotContains(t, batch, `"name":"first"`)
		}
	}
}
