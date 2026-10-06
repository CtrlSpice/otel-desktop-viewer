package metrics_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/pmetric/pmetricotlp"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	otelmetric "go.opentelemetry.io/otel/metric"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"google.golang.org/grpc"
)

// collector stores metrics decoded by pdata's OTLP gRPC server.
type collector struct {
	pmetricotlp.UnimplementedGRPCServer
	batches []pmetric.Metrics
}

func (c *collector) Export(_ context.Context, req pmetricotlp.ExportRequest) (pmetricotlp.ExportResponse, error) {
	md := pmetric.NewMetrics()
	req.Metrics().CopyTo(md)
	c.batches = append(c.batches, md)
	return pmetricotlp.NewExportResponse(), nil
}

// TestCumulativeMergeAgainstTheOtelSDK verifies cumulative merging against
// running totals produced by the OpenTelemetry Go SDK. Only observations from
// the second collection contribute to the expected window delta.
func TestCumulativeMergeAgainstTheOtelSDK(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	sink := &collector{}
	srv := grpc.NewServer()
	pmetricotlp.RegisterGRPCServer(srv, sink)
	go func() { _ = srv.Serve(lis) }()
	defer srv.Stop()

	exp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(lis.Addr().String()),
		otlpmetricgrpc.WithInsecure(),
		// Explicit, though it is also the default: this test is about cumulative.
		otlpmetricgrpc.WithTemporalitySelector(func(sdkmetric.InstrumentKind) metricdata.Temporality {
			return metricdata.CumulativeTemporality
		}),
	)
	require.NoError(t, err)

	reader := sdkmetric.NewPeriodicReader(exp, sdkmetric.WithInterval(time.Hour))
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))

	bounds := []float64{1, 5, 10}
	hist, err := provider.Meter("sdk-cumulative-test").Float64Histogram(
		"request.duration",
		otelmetric.WithExplicitBucketBoundaries(bounds...),
	)
	require.NoError(t, err)
	attrs := otelmetric.WithAttributes(attribute.String("route", "/checkout"))

	// The first collection establishes the cumulative baseline.
	firstCycle := []float64{0.5, 0.7, 2.0}
	for _, v := range firstCycle {
		hist.Record(ctx, v, attrs)
	}
	require.NoError(t, provider.ForceFlush(ctx))

	// The second collection covers every explicit bucket and the overflow bucket.
	secondCycle := []float64{0.5, 3.0, 7.0, 7.5, 50.0}
	for _, v := range secondCycle {
		hist.Record(ctx, v, attrs)
	}
	require.NoError(t, provider.ForceFlush(ctx))
	require.NoError(t, provider.Shutdown(ctx))

	// Shutdown may emit an unchanged cumulative batch after the forced collections.
	require.GreaterOrEqual(t, len(sink.batches), 2, "one batch per collection")

	s, storeCtx := storetest.New(t)
	for _, md := range sink.batches {
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(storeCtx, conn, md, s.FlushedIDs())
		}))
	}

	summaries := searchMetricsAll(t, s, storeCtx)
	require.Len(t, summaries, 1)
	require.Equal(t, "Cumulative", summaries[0]["aggregationTemporality"],
		"the SDK must have sent cumulative, or this test proves nothing")

	// One bucket over everything, so the merge is last-minus-first across both
	// collections.
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricView(storeCtx, db, summaries[0]["metricRef"].(string), store.TimeRange{},
			1, nil, nil, 0, 0, 0, nil, "", nil, 0)

	})
	require.NoError(t, err)
	var got map[string]any
	require.NoError(t, json.Unmarshal(raw, &got))
	dps := got["timeseries"].([]any)[0].(map[string]any)["datapoints"].([]any)
	require.Len(t, dps, 1, "one bucket is one merged datapoint")
	merged := dps[0].(map[string]any)

	// Compute the expected delta independently from the recorded values.
	wantCounts := make([]uint64, len(bounds)+1)
	wantSum := 0.0
	for _, v := range secondCycle {
		wantSum += v
		i := len(bounds)
		for j, b := range bounds {
			if v <= b {
				i = j
				break
			}
		}
		wantCounts[i]++
	}

	assert.Equal(t, uint64(len(secondCycle)), metricWireUint64(t, merged["count"]),
		"the merge must recover exactly the second collection's observation count")
	assert.InDelta(t, wantSum, merged["sum"], 1e-9,
		"and its sum: the first collection is the baseline and subtracts away")

	gotCounts := merged["bucketCounts"].([]any)
	require.Len(t, gotCounts, len(wantCounts))
	for i, want := range wantCounts {
		assert.Equalf(t, want, metricWireUint64(t, gotCounts[i]), "bucket %d", i)
	}
}
