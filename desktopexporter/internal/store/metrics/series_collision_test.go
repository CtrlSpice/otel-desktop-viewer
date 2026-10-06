package metrics

import (
	"testing"

	"github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/require"
)

func TestAddSeriesRowRejectsSameBatchContentIDCollision(t *testing.T) {
	id := duckdb.UUID{1}
	rows := map[duckdb.UUID]seriesRow{}
	require.NoError(t, addSeriesRow(rows, seriesRow{
		id: id, stream: duckdb.UUID{2}, attrs: []duckdb.UUID{{3}},
	}))
	require.NoError(t, addSeriesRow(rows, seriesRow{
		id: id, stream: duckdb.UUID{2}, attrs: []duckdb.UUID{{3}},
	}))

	err := addSeriesRow(rows, seriesRow{
		id: id, stream: duckdb.UUID{4}, attrs: []duckdb.UUID{{5}},
	})
	require.ErrorContains(t, err, "metric series content ID collision")
}
