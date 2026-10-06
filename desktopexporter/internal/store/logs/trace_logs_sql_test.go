package logs

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetTraceLogSummariesSQLBindsTraceID(t *testing.T) {
	t.Parallel()
	hostile := `00000000-0000-0000-0000-000000000099' or true --`

	query, args, err := getTraceLogSummariesSQL(hostile)
	require.NoError(t, err)
	require.Equal(t, []any{hostile}, args)
	require.NotContains(t, query, hostile)
	require.Contains(t, query, "where l.trace_id = ?::uuid")
	require.NotContains(t, query, " limit ")
	require.NotContains(t, query, "attrs_json")
}
