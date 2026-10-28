package queries_test

import (
	"encoding/json"
	"math"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/util"
	"github.com/CtrlSpice/otel-desktop-viewer/internal/discovery"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
)

func TestDiscoveryNativeServiceLabels(t *testing.T) {
	db := freshDB(t)
	for _, tc := range []struct {
		name    string
		value   any
		missing bool
	}{
		{"missing", nil, true}, {"empty", nil, false}, {"empty string", "", false},
		{"escaped string", "quote'\"\\\n<>&\u2028\u2029", false},
		{"int64", int64(math.MaxInt64), false}, {"bool", false, false},
		{"bytes", []byte{0, 1, 255}, false}, {"negative zero", math.Copysign(0, -1), false},
		{"exponent", 1e21, false}, {"nan", math.NaN(), false}, {"infinity", math.Inf(1), false},
		{"negative infinity", math.Inf(-1), false},
		{"array", []any{int64(math.MaxInt64), "quoted", true}, false},
		{"map", map[string]any{"a": []any{nil, []byte{0, 255}}}, false},
		{"nonfinite container", []any{math.Inf(1)}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			attributes := pcommon.NewMap()
			value := pcommon.NewValueEmpty()
			require.NoError(t, value.FromRaw(tc.value))
			if !tc.missing {
				value.CopyTo(attributes.PutEmpty("service.name"))
				value.CopyTo(attributes.PutEmpty("service.namespace"))
			}
			encoded, err := ingest.AttributeBytes(attributes)
			require.NoError(t, err)
			label := value.AsString()
			_, err = db.Exec(`insert into logs(id, timestamp, service_name, attributes, resource, scope)
				values (uuid(), 100, ?, '{}'::json::variant,
				struct_pack(id := uuid(), payload_id := uuid(), schema_url := '',
					attributes := variant_bytes_to_variant(?::blob), dropped_attributes_count := 0)::otel_resource,
				struct_pack(id := uuid(), name := '', version := '', schema_url := '',
					attributes := '{}'::json::variant, dropped_attributes_count := 0)::otel_scope)`, label, encoded)
			require.NoError(t, err)
			var raw string
			require.NoError(t, db.QueryRow("select entry::varchar from ("+discovery.Services(discovery.Query{})+")").Scan(&raw))
			var service struct {
				Name      string `json:"serviceName"`
				Namespace string `json:"serviceNamespace"`
				Identity  string `json:"serviceIdentity"`
				Count     int    `json:"logCount"`
			}
			require.NoError(t, json.Unmarshal([]byte(raw), &service))
			require.Equal(t, label, service.Name)
			require.Equal(t, label, service.Namespace)
			require.Equal(t, 1, service.Count)
			identity := `[null,{"kind":"string","value":""}]`
			if !tc.missing {
				wire, err := util.EncodeValue(value)
				require.NoError(t, err)
				identity = "[" + string(wire) + "," + string(wire) + "]"
			}
			require.JSONEq(t, identity, service.Identity)

			// Exercise the actual shared namespace predicate, including absent and
			// received empty values, through record discovery and service summaries.
			records, err := discovery.Records("logs", "resource")
			require.NoError(t, err)
			for _, namespace := range []string{label, label + "not a match"} {
				query := discovery.Query{Namespace: &namespace}
				var count int
				require.NoError(t, db.QueryRow("select count(*) from ("+records+") s where "+discovery.Predicate(query)).Scan(&count))
				want := 0
				if namespace == label {
					want = 1
				}
				require.Equal(t, want, count)
				require.NoError(t, db.QueryRow("select count(*) from ("+discovery.Services(query)+")").Scan(&count))
				require.Equal(t, want, count)
			}
			_, err = db.Exec("delete from logs")
			require.NoError(t, err)
		})
	}
}
