package ingest_test

import (
	"database/sql"
	"database/sql/driver"
	"math"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/arrowingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/apache/arrow-go/v18/arrow"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestInlineOwnersTypedRoundTrip(t *testing.T) {
	s, _ := storetest.New(t)
	resource := pcommon.NewResource()
	resource.SetDroppedAttributesCount(17)
	resource.Attributes().PutInt("large", math.MaxInt64)
	resource.Attributes().PutDouble("negative-zero", math.Copysign(0, -1))
	resource.Attributes().PutEmptyBytes("bytes").FromRaw([]byte{0, 255, 1})
	resource.Attributes().PutEmptyMap("nested").PutStr("a.b", "literal key")
	scope := pcommon.NewInstrumentationScope()
	scope.SetName("typed-owner")
	scope.SetVersion("1")
	scope.SetDroppedAttributesCount(23)
	scope.Attributes().PutBool("enabled", true)
	r, err := ingest.ResourceInput(resource, "resource-schema")
	require.NoError(t, err)
	sc, err := ingest.ScopeInput(scope, "scope-schema")
	require.NoError(t, err)
	// Mutating pdata after transport creation must not mutate the appended row.
	resource.Attributes().Clear()
	scope.Attributes().Clear()
	require.NoError(t, s.WithDBWrite(func(db *sql.DB) error {
		_, err := db.Exec(`create table inline_owner_test(resource otel_resource, scope otel_scope)`)
		return err
	}))
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		var fields []arrow.Field
		for _, field := range arrowingest.TableFields("logs") {
			if field.Name == "resource" || field.Name == "scope" {
				fields = append(fields, field)
			}
		}
		a := arrowingest.NewCustomAppender(t.Context(), conn, fields,
			ingest.OwnedAppendQuery("inline_owner_test", []string{"resource.attributes", "scope.attributes"}))
		a.SetContext(r, sc)
		a.FinishParent()
		return a.Close()
	}))
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		var large int64
		var zero float64
		var bytes []byte
		var nested, resourceSchema, scopeName, scopeVersion, scopeSchema string
		var droppedResource, droppedScope uint32
		var enabled bool
		err := db.QueryRow(`select resource.attributes.large::bigint,
			resource.attributes['negative-zero']::double, resource.attributes.bytes::blob,
			resource.attributes.nested['a.b']::varchar, resource.schema_url,
			resource.dropped_attributes_count, scope.name, scope.version, scope.schema_url,
			scope.dropped_attributes_count, scope.attributes.enabled::boolean
			from inline_owner_test`).Scan(&large, &zero, &bytes, &nested, &resourceSchema,
			&droppedResource, &scopeName, &scopeVersion, &scopeSchema, &droppedScope, &enabled)
		if err != nil {
			return err
		}
		require.Equal(t, int64(math.MaxInt64), large)
		require.Equal(t, uint64(1)<<63, math.Float64bits(zero))
		require.Equal(t, []byte{0, 255, 1}, bytes)
		require.Equal(t, "literal key", nested)
		require.Equal(t, "resource-schema", resourceSchema)
		require.Equal(t, uint32(17), droppedResource)
		require.Equal(t, "typed-owner", scopeName)
		require.Equal(t, "1", scopeVersion)
		require.Equal(t, "scope-schema", scopeSchema)
		require.Equal(t, uint32(23), droppedScope)
		require.True(t, enabled)
		return nil
	}))
}
