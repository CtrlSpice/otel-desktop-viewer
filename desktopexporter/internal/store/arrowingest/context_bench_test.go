package arrowingest

import (
	"fmt"
	"testing"

	"github.com/duckdb/duckdb-go/v2"
)

func BenchmarkSharedContext(b *testing.B) {
	for _, size := range []int{64, 65536, 1048576} {
		b.Run(fmt.Sprintf("%dB", size), func(b *testing.B) {
			a := &Appender{}
			attrs := make([]byte, size)
			a.SetContext(map[string]any{"id": duckdb.UUID{}, "payload_id": duckdb.UUID{}, "schema_url": "", "attributes": attrs, "dropped_attributes_count": uint32(0)}, map[string]any{"id": duckdb.UUID{}, "name": "", "version": "", "schema_url": "", "attributes": attrs, "dropped_attributes_count": uint32(0)})
			defer a.resourceBuilder.Release()
			defer a.scopeBuilder.Release()
			defer a.resourceIndices.Release()
			defer a.scopeIndices.Release()
			a.appendContext()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				a.appendContext()
			}
		})
	}
}
