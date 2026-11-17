package server

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMetricSeriesContextOnlyParameter(t *testing.T) {
	base := `["00000000-0000-0000-0000-000000000001","00000000-0000-0000-0000-000000000002",null,null`
	for _, fixture := range []struct {
		suffix   string
		included bool
	}{
		{"]", true}, {",null]", true}, {",true]", true}, {",false]", false},
	} {
		params, err := parseGetMetricSeriesParams(json.RawMessage(base + fixture.suffix))
		require.NoError(t, err)
		require.Equal(t, fixture.included, params.includeDatapoints)
	}
	_, err := parseGetMetricSeriesParams(json.RawMessage(base + `,"false"]`))
	require.Error(t, err)
}
