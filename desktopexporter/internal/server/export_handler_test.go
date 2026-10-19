package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func TestExportPreviewBoundsAndMetricSelection(t *testing.T) {
	server, s, teardown := setupServerWithStore(t)
	defer teardown()
	data := pmetric.NewMetrics()
	rm := data.ResourceMetrics().AppendEmpty()
	rm.Resource().Attributes().PutStr("resource.owner", "preview-fixture")
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.Scope().SetName("preview-scope")
	metric := sm.Metrics().AppendEmpty()
	metric.SetName("preview-selection")
	points := metric.SetEmptyGauge().DataPoints()
	for i := range 3 {
		point := points.AppendEmpty()
		point.SetTimestamp(pcommon.Timestamp(i + 1))
		point.SetIntValue(9223372036854775807)
		point.Attributes().PutInt("series", int64(i/2))
	}
	other := sm.Metrics().AppendEmpty()
	other.SetName("other-metric")
	other.SetEmptyGauge().DataPoints().AppendEmpty().SetIntValue(1)
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(context.Background(), conn, data, s.FlushedIDs())
	}))
	var metricID, otherMetricID, seriesID, pointID string
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		if err := db.QueryRow("select id::varchar from metrics where name = 'other-metric'").Scan(&otherMetricID); err != nil {
			return err
		}
		return db.QueryRow(`select m.id::varchar, d.series_id::varchar, d.id::varchar
			from metrics m join metric_datapoints d on d.metric_id = m.id
			where m.name = 'preview-selection' and d.timestamp = 1`).Scan(&metricID, &seriesID, &pointID)
	}))
	url := server.URL + "/export/metrics/" + metricID
	response, err := http.Get(url)
	require.NoError(t, err)
	full, err := io.ReadAll(response.Body)
	require.NoError(t, response.Body.Close())
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, response.StatusCode)
	assert.Contains(t, string(full), `"asInt":"9223372036854775807"`)

	t.Run("head gives the exact full export size", func(t *testing.T) {
		response, err := http.Head(url)
		require.NoError(t, err)
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		assert.Empty(t, body)
		assert.Equal(t, strconv.Itoa(len(full)), response.Header.Get("Content-Length"))
	})
	t.Run("range preview is an exact bounded prefix", func(t *testing.T) {
		request, err := http.NewRequest(http.MethodGet, url, nil)
		require.NoError(t, err)
		request.Header.Set("Range", "bytes=0-63")
		response, err := http.DefaultClient.Do(request)
		require.NoError(t, err)
		defer response.Body.Close()
		body, err := io.ReadAll(response.Body)
		require.NoError(t, err)
		assert.Equal(t, http.StatusPartialContent, response.StatusCode)
		assert.Equal(t, full[:64], body)
		assert.Equal(t, "64", response.Header.Get("Content-Length"))
		assert.Equal(t, "bytes 0-63/"+strconv.Itoa(len(full)), response.Header.Get("Content-Range"))
	})
	for _, tc := range []struct {
		name, selector string
		count          int
	}{
		{"whole metric", "", 3},
		{"series", "?seriesRef=" + seriesID, 2},
		{"received datapoint", "?datapointRef=" + pointID, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			response, err := http.Get(url + tc.selector)
			require.NoError(t, err)
			defer response.Body.Close()
			raw, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, response.StatusCode, "%s", raw)
			decoded, err := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(raw)
			require.NoError(t, err)
			assert.Equal(t, tc.count, decoded.DataPointCount())
			rm := decoded.ResourceMetrics().At(0)
			owner, ok := rm.Resource().Attributes().Get("resource.owner")
			require.True(t, ok)
			assert.Equal(t, "preview-fixture", owner.Str())
			assert.Equal(t, "preview-scope", rm.ScopeMetrics().At(0).Scope().Name())
		})
	}
	for _, query := range []string{
		"?unknown=1", "?seriesRef=invalid", "?datapointRef=invalid",
		"?seriesRef=" + seriesID + "&seriesRef=" + seriesID,
		"?datapointRef=%ZZ", "?seriesRef=%", "?format=%ZZ",
		"?datapointRef=" + pointID + ";ignored=1",
		"?seriesRef=" + seriesID + "&datapointRef=%ZZ",
	} {
		t.Run("rejects "+query, func(t *testing.T) {
			response, err := http.Get(url + query)
			require.NoError(t, err)
			defer response.Body.Close()
			body, err := io.ReadAll(response.Body)
			require.NoError(t, err)
			assert.Equal(t, http.StatusBadRequest, response.StatusCode)
			assert.Empty(t, response.Header.Get("Content-Disposition"))
			assert.NotContains(t, string(body), "resourceMetrics")
		})
	}
	for _, query := range []string{"?spanId=%ZZ", "?spanId=0000000000000001;ignored=1"} {
		response, err := http.Get(server.URL + "/export/traces/00000000000000000000000000000001" + query)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		assert.Equal(t, http.StatusBadRequest, response.StatusCode)
		assert.Empty(t, response.Header.Get("Content-Disposition"))
	}
	for _, query := range []string{"?seriesRef=" + seriesID, "?datapointRef=" + pointID} {
		response, err := http.Get(server.URL + "/export/metrics/" + otherMetricID + query)
		require.NoError(t, err)
		require.NoError(t, response.Body.Close())
		assert.Equal(t, http.StatusNotFound, response.StatusCode)
	}
}

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
			for _, query := range []string{"", "?format=json"} {
				response, err := http.Get(server.URL + "/export/metrics/" + id + query)
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
