package ingest_test

import (
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowbridge"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/apache/arrow-go/v18/arrow/array"
	"github.com/stretchr/testify/require"
)

// A failed second Arrow insert must roll back an earlier successful insert,
// then permit a fresh batch on the same dedicated connection.
func TestArrowFailureRollsBackEarlierTablesAndAllowsRetry(t *testing.T) {
	s, ctx := storetest.New(t)
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		_, err := db.ExecContext(ctx, `create table flush_first(id integer primary key);
			create table flush_second(id integer primary key)`)
		return err
	}))
	batch := func(duplicate bool) error {
		return s.WithConn(func(conn driver.Conn) error {
			return ingest.InTransaction(ctx, conn, func() error {
				for _, table := range []string{"flush_first", "flush_second"} {
					builder := array.NewRecordBuilder(arrowbridge.Allocator(), arrow.NewSchema([]arrow.Field{{Name: "id", Type: arrow.PrimitiveTypes.Int32}}, nil))
					builder.Field(0).(*array.Int32Builder).Append(7)
					if duplicate && table == "flush_second" {
						builder.Field(0).(*array.Int32Builder).Append(7)
					}
					record := builder.NewRecordBatch()
					builder.Release()
					err := arrowbridge.Insert(ctx, conn, "insert into "+table+" select * from appended_data", record)
					record.Release()
					if err != nil {
						return err
					}
				}
				return nil
			})
		})
	}
	counts := func() []int {
		var first, second int
		require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
			return db.QueryRowContext(ctx, `select (select count(*) from flush_first),
				(select count(*) from flush_second)`).Scan(&first, &second)
		}))
		return []int{first, second}
	}
	require.ErrorContains(t, batch(true), "Constraint Error")
	require.Equal(t, []int{0, 0}, counts())
	require.NoError(t, batch(false))
	require.Equal(t, []int{1, 1}, counts())
}
