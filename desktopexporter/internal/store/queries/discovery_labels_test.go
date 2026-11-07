package queries_test

import (
	"database/sql/driver"
	"encoding/json"
	"math"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/logs"
	"github.com/CtrlSpice/otel-desktop-viewer/internal/discovery"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
)

func TestDiscoveryNativeServiceLabels(t *testing.T) {
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
			db := freshDB(t)
			data := plog.NewLogs()
			resource := data.ResourceLogs().AppendEmpty()
			attributes := resource.Resource().Attributes()
			value := pcommon.NewValueEmpty()
			require.NoError(t, value.FromRaw(tc.value))
			if !tc.missing {
				value.CopyTo(attributes.PutEmpty("service.name"))
				value.CopyTo(attributes.PutEmpty("service.namespace"))
			}
			label := value.AsString()
			resource.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().SetTimestamp(100)
			conn, err := db.Conn(t.Context())
			require.NoError(t, err)
			require.NoError(t, conn.Raw(func(raw any) error { return logs.Ingest(t.Context(), raw.(driver.Conn), data) }))
			require.NoError(t, conn.Close())
			var raw string
			var builder discovery.Builder
			statement := builder.Services(discovery.Query{})
			require.NoError(t, db.QueryRow("select entry::varchar from ("+statement+")", builder.Args()...).Scan(&raw))
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
			require.Regexp(t, `^[0-9a-f]{64}$`, service.Identity)
			var matched int
			builder = discovery.Builder{}
			statement = builder.Services(discovery.Query{ServiceIdentity: &service.Identity})
			require.NoError(t, db.QueryRow("select count(*) from ("+statement+")", builder.Args()...).Scan(&matched))
			require.Equal(t, 1, matched)

			// Exercise the actual shared namespace predicate, including absent and
			// received empty values, through record discovery and service summaries.
			builder = discovery.Builder{}
			records, err := builder.Records("logs", "resource")
			require.NoError(t, err)
			for _, namespace := range []string{label, label + "not a match"} {
				query := discovery.Query{Namespace: &namespace}
				var count int
				builder = discovery.Builder{}
				statement = "select count(*) from (" + records + ") s where " + builder.Predicate(query)
				require.NoError(t, db.QueryRow(statement, builder.Args()...).Scan(&count))
				want := 0
				if namespace == label {
					want = 1
				}
				require.Equal(t, want, count)
				builder = discovery.Builder{}
				statement = builder.Services(query)
				require.NoError(t, db.QueryRow("select count(*) from ("+statement+")", builder.Args()...).Scan(&count))
				require.Equal(t, want, count)
			}
		})
	}
}
