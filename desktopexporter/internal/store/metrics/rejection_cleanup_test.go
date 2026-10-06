package metrics_test

import (
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/ingest"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"
)

func TestSummaryRejectionPreservesAcceptedEmptyMetrics(t *testing.T) {
	for _, summaryFirst := range []bool{true, false} {
		name := "summary last"
		if summaryFirst {
			name = "summary first"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s, ctx := storetest.New(t)
			batch := pmetric.NewMetrics()
			appendSummary := func() {
				rm := batch.ResourceMetrics().AppendEmpty()
				rm.Resource().Attributes().PutStr("rejected.resource", "summary")
				sm := rm.ScopeMetrics().AppendEmpty()
				sm.Scope().Attributes().PutBool("rejected.scope", true)
				metric := sm.Metrics().AppendEmpty()
				metric.SetName("unsupported-summary")
				metric.Metadata().PutBool("rejected.metadata", true)
				metric.SetEmptySummary().DataPoints().AppendEmpty().SetCount(1)
			}
			if summaryFirst {
				appendSummary()
			}
			rm := batch.ResourceMetrics().AppendEmpty()
			rm.SetSchemaUrl("resource-schema")
			rm.Resource().Attributes().PutStr("service.name", "accepted")
			sm := rm.ScopeMetrics().AppendEmpty()
			sm.SetSchemaUrl("scope-schema")
			sm.Scope().SetName("sdk")
			for _, metricType := range []pmetric.MetricType{
				pmetric.MetricTypeGauge, pmetric.MetricTypeSum,
				pmetric.MetricTypeHistogram, pmetric.MetricTypeExponentialHistogram,
			} {
				metric := sm.Metrics().AppendEmpty()
				metric.SetName(metricType.String())
				switch metricType {
				case pmetric.MetricTypeGauge:
					metric.SetEmptyGauge()
				case pmetric.MetricTypeSum:
					metric.SetEmptySum()
				case pmetric.MetricTypeHistogram:
					metric.SetEmptyHistogram()
				case pmetric.MetricTypeExponentialHistogram:
					metric.SetEmptyExponentialHistogram()
				}
			}
			repeated := sm.Metrics().AppendEmpty()
			repeated.SetName("Gauge")
			repeated.SetDescription("latest")
			repeated.SetEmptyGauge()
			if !summaryFirst {
				appendSummary()
			}

			var rejected ingest.Rejected
			require.NoError(t, s.WithConn(func(conn driver.Conn) error {
				var err error
				rejected, err = metrics.IngestReport(ctx, conn, batch, s.FlushedIDs())
				return err
			}))
			require.Equal(t, 1, rejected.Count())
			require.ErrorIs(t, rejected.Reason(), metrics.ErrUnsupportedMetricType)
			ids := metricMetricIDs(t, s, ctx)
			require.Len(t, ids, 4, "every accepted empty Metric must survive cleanup")
			require.NotContains(t, ids, "unsupported-summary")
			for name, id := range ids {
				raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
					return metrics.GetMetric(ctx, db, id)
				})
				require.NoError(t, err)
				var exact map[string]any
				require.NoError(t, json.Unmarshal(raw, &exact))
				assert.Equal(t, name, exact["name"])
				assert.Empty(t, exact["series"])
				if name == "Gauge" {
					assert.Equal(t, "latest", exact["description"])
				}
				otlp := getMetricOTLP(t, s, ctx, id)
				decoded, err := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(otlp)
				require.NoError(t, err)
				require.Equal(t, 1, decoded.MetricCount())
				assert.Zero(t, decoded.DataPointCount())
			}
			require.Equal(t, 0, countRows(t, s, ctx, `select count(*) from metric_datapoints`))
			require.Equal(t, 0, countRows(t, s, ctx, `select count(*) from metric_series`))
			require.Equal(t, 1, countRows(t, s, ctx, `select count(*) from resources`))
			require.Equal(t, 1, countRows(t, s, ctx, `select count(*) from scopes`))
			require.Equal(t, 0, countRows(t, s, ctx, `select count(*) from attributes where key like 'rejected.%'`))
		})
	}
}
