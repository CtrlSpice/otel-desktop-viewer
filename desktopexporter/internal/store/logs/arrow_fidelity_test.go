package logs_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"math"
	"path/filepath"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

func TestArrowPreservesReceivedFloatBitsAcrossSharedContexts(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "float-bits.duckdb")
	s, err := store.NewStore(ctx, path, nil)
	require.NoError(t, err)
	defer func() { _ = s.Close() }()
	data := plog.NewLogs()
	bits := []uint64{0x7ff8000000000042, 0xfff8000000000013, 0x8000000000000000}
	for i, bit := range bits {
		r := data.ResourceLogs().AppendEmpty()
		r.Resource().Attributes().PutDouble("bits", math.Float64frombits(bit))
		sc := r.ScopeLogs().AppendEmpty()
		sc.Scope().Attributes().PutDouble("bits", math.Float64frombits(bit))
		log := sc.LogRecords().AppendEmpty()
		log.SetTimestamp(pcommon.Timestamp(i + 1))
		log.Body().SetDouble(math.Float64frombits(bit))
		log.Attributes().PutDouble("bits", math.Float64frombits(bit))
	}
	require.NoError(t, s.WithConn(func(conn driver.Conn) error { return logs.Ingest(ctx, conn, data) }))
	verify := func() {
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			for i, bit := range bits {
				var body, attr, resource, scope float64
				err := db.QueryRow(`select body::double,attributes['bits']::double,resource.attributes['bits']::double,scope.attributes['bits']::double from logs where timestamp=?`, i+1).Scan(&body, &attr, &resource, &scope)
				if err != nil {
					return err
				}
				for _, value := range []float64{body, attr, resource, scope} {
					require.Equal(t, bit, math.Float64bits(value))
				}
			}
			return nil
		}))
	}
	verify()
	require.NoError(t, s.Close())
	s, err = store.NewStore(ctx, path, nil)
	require.NoError(t, err)
	verify()
}
