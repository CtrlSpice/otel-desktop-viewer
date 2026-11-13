package store

import (
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/stretchr/testify/require"
)

func TestCommitNotificationSurvivesPartialIngestFailure(t *testing.T) {
	s, ctx, teardown := setupStore(t)
	defer teardown()
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec("CREATE TABLE commit_records (id INTEGER CHECK (id > 0))")
		return err
	}))
	before, changed, _ := s.CommitSnapshot()
	var rejected ingest.Rejected
	err := s.WithConn(func(conn driver.Conn) error {
		var err error
		rejected, err = ingest.BisectingWrite(ctx, 4, nil, func(lo, hi int) error {
			// The first attempt rolls back on the invalid third row. Bisection
			// commits the first half before this infrastructure fault ends ingest.
			if lo == 3 {
				return ingest.ErrNotRowFault
			}
			return ingest.InTransaction(ctx, conn, func() error {
				values := []int64{1, 2, -1, 4}
				for _, value := range values[lo:hi] {
					_, err := conn.(driver.ExecerContext).ExecContext(ctx, "INSERT INTO commit_records VALUES (?)", []driver.NamedValue{{Ordinal: 1, Value: value}})
					if err != nil {
						return err
					}
				}
				return nil
			})
		})
		return err
	})
	require.ErrorIs(t, err, ingest.ErrNotRowFault)
	require.Equal(t, 1, rejected.Count())
	after, _, _ := s.CommitSnapshot()
	require.Equal(t, before+1, after, "only the successful sub-batch should notify")
	select {
	case <-changed:
	default:
		t.Fatal("successful partial ingest did not wake subscribers")
	}
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var count int
		err := db.QueryRow("SELECT count(*) FROM commit_records").Scan(&count)
		require.Equal(t, 2, count)
		return err
	}))
}
