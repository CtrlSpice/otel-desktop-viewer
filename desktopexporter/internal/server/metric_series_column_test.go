package server

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMetricSeriesColumnParams(t *testing.T) {
	const id = "00000000-0000-0000-0000-000000000001"
	for _, key := range []string{"", "__proto__", "a\"[0].雪"} {
		raw, err := json.Marshal(map[string]any{"metricRef": id, "seriesRefs": []string{id, id}, "key": key})
		require.NoError(t, err)
		normalized, err := normalizeParams("getMetricSeriesColumn", raw)
		require.NoError(t, err)
		params, err := parseMetricSeriesColumnParams(normalized)
		require.NoError(t, err)
		require.Equal(t, key, params.key)
		require.Equal(t, []string{id, id}, params.seriesRefs)
	}
	for _, raw := range []string{
		`[]`, `["` + id + `",null,"key"]`, `["` + id + `",["bad"],"key"]`,
		`["` + id + `",[1],"key"]`, `["` + id + `",[],null]`, `["bad",[],"key"]`,
	} {
		_, err := parseMetricSeriesColumnParams(json.RawMessage(raw))
		require.Error(t, err, raw)
	}
}
