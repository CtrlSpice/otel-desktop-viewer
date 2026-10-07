package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"net/http"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func TestExportReconstructionFailuresAreNotAttachments(t *testing.T) {
	server, s, teardown := setupServerWithStore(t)
	defer teardown()
	data := pmetric.NewMetrics()
	metric := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	metric.SetName("export-errors")
	metric.SetEmptyGauge().DataPoints().AppendEmpty().SetDoubleValue(1)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(context.Background(), conn, data, s.FlushedIDs())
	}))
	var id string
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		return db.QueryRow("select id::varchar from metrics").Scan(&id)
	}))
	for _, tc := range []struct {
		name, sql string
		status    int
	}{
		{"unsupported", "update metrics set metric_type = 'Summary'", http.StatusUnprocessableEntity},
		{"malformed", "update metrics set metric_type = 'Gauge'; update metric_datapoints set value_type = 'Double', double_value = null, int_value = null", http.StatusInternalServerError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
				_, err := db.Exec(tc.sql)
				return err
			}))
			for _, format := range []string{"json", "protobuf"} {
				response, err := http.Get(server.URL + "/export/metrics/" + id + "?format=" + format)
				require.NoError(t, err)
				defer response.Body.Close()
				body, err := io.ReadAll(response.Body)
				require.NoError(t, err)
				assert.Equal(t, tc.status, response.StatusCode, "%s", body)
				assert.Empty(t, response.Header.Get("Content-Disposition"))
				assert.Contains(t, response.Header.Get("Content-Type"), "text/plain")
				assert.NotEmpty(t, body)
			}
		})
	}
}
