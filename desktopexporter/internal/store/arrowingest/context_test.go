package arrowingest_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowingest"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/duckdb/duckdb-go/v2"
	"github.com/stretchr/testify/require"
)

func TestSharedContextAcrossFlush(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("duckdb", "")
	require.NoError(t, err)
	defer db.Close()
	_, err = db.Exec(`create table received(resource struct(id varchar, payload_id varchar, schema_url varchar, attributes blob, dropped_attributes_count uinteger), scope struct(id varchar, name varchar, version varchar, schema_url varchar, attributes blob, dropped_attributes_count uinteger))`)
	require.NoError(t, err)
	conn, err := db.Conn(ctx)
	require.NoError(t, err)
	defer conn.Close()
	require.NoError(t, conn.Raw(func(raw any) error {
		var fields []arrow.Field
		for _, f := range arrowingest.TableFields("logs") {
			if f.Name == "resource" || f.Name == "scope" {
				fields = append(fields, f)
			}
		}
		a := arrowingest.NewCustomAppender(ctx, raw.(driver.Conn), fields, "insert into received select * from appended_data")
		defer a.Discard()
		set := func(value byte) {
			// Equal canonical IDs with different received bytes must not share a slot.
			a.SetContext(map[string]any{"id": duckdb.UUID{}, "payload_id": duckdb.UUID{}, "schema_url": "r", "attributes": []byte{value}, "dropped_attributes_count": uint32(7)},
				map[string]any{"id": duckdb.UUID{}, "name": "scope", "version": "v", "schema_url": "s", "attributes": []byte{value + 10}, "dropped_attributes_count": uint32(8)})
		}
		set(1)
		a.FinishParent()
		set(2)
		a.FinishParent()
		set(1)
		a.FinishParent()
		require.NoError(t, a.Flush())
		// Flush consumed the dictionary, but the active context still applies.
		a.FinishParent()
		set(2)
		a.FinishParent()
		require.NoError(t, a.Flush())
		a.FinishParent()
		return a.Close()
	}))
	rows, err := conn.QueryContext(ctx, `select resource.attributes, scope.attributes, resource.dropped_attributes_count, scope.dropped_attributes_count, scope.name from received`)
	require.NoError(t, err)
	defer rows.Close()
	var values []byte
	for rows.Next() {
		var r, s []byte
		var rd, sd uint32
		var name string
		require.NoError(t, rows.Scan(&r, &s, &rd, &sd, &name))
		require.Equal(t, []byte{r[0] + 10}, s)
		require.Equal(t, uint32(7), rd)
		require.Equal(t, uint32(8), sd)
		require.Equal(t, "scope", name)
		values = append(values, r[0])
	}
	require.NoError(t, rows.Err())
	require.Equal(t, []byte{1, 2, 1, 1, 2, 2}, values)
}
