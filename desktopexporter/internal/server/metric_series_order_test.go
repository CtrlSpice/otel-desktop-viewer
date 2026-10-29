package server

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMetricSeriesOrderParams(t *testing.T) {
	const id = "00000000-0000-0000-0000-000000000001"
	for _, key := range []string{"", "__proto__", "a\"[0].雪"} {
		raw, err := json.Marshal(map[string]any{"metricRef": id, "startTime": "0", "endTime": "18446744073709551615", "seriesRefs": []string{id, id}, "key": key, "direction": "desc"})
		require.NoError(t, err)
		normalized, err := normalizeParams("getMetricSeriesOrder", raw)
		require.NoError(t, err)
		params, err := parseMetricSeriesOrderParams(normalized)
		require.NoError(t, err)
		require.Equal(t, key, params.key)
		require.Equal(t, []string{id, id}, params.seriesRefs)
		require.Equal(t, uint64(0), *params.timeRange.Start)
		require.Equal(t, ^uint64(0), *params.timeRange.End)
		require.True(t, params.descending)
	}
	for _, raw := range []string{
		`[]`, `["` + id + `",null,null,null,"key","asc"]`, `["` + id + `",null,null,["bad"],"key","asc"]`,
		`["` + id + `",null,null,[1],"key","asc"]`, `["` + id + `",null,null,[],null,"asc"]`,
		`["` + id + `","-1",null,[],"key","asc"]`,
		`["` + id + `",null,null,[],"key","desc;drop table metrics"]`,
	} {
		_, err := parseMetricSeriesOrderParams(json.RawMessage(raw))
		require.Error(t, err, raw)
	}
}
