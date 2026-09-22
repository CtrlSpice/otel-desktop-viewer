package metrics_test

import (
	"bytes"
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/storetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.uber.org/zap"
)

var (
	metricHex16   = regexp.MustCompile(`^[0-9a-f]{16}$`)
	metricHex32   = regexp.MustCompile(`^[0-9a-f]{32}$`)
	metricDecimal = regexp.MustCompile(`^-?[0-9]+$`)
)

func TestGetMetricOTLP(t *testing.T) {
	s, ctx := storetest.New(t)
	input := otlpMetricFixture()
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, input, s.FlushedIDs())
	}))

	ids := metricIngestIDs(t, s, ctx)
	for _, name := range []string{"otlp.gauge", "otlp.sum", "otlp.histogram", "otlp.exponential"} {
		t.Run(name, func(t *testing.T) {
			raw := getMetricOTLP(t, s, ctx, ids[name])
			require.NoError(t, validateMetricOTLP(raw))
			decoded, err := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(raw)
			require.NoError(t, err)
			require.Equal(t, 1, decoded.ResourceMetrics().Len())
			rm := decoded.ResourceMetrics().At(0)
			assert.Equal(t, "https://example.test/resource", rm.SchemaUrl())
			assert.Equal(t, uint32(7), rm.Resource().DroppedAttributesCount())
			assert.Equal(t, "fixture", mustMapValue(t, rm.Resource().Attributes(), "resource.owner").Str())
			require.Equal(t, 1, rm.ScopeMetrics().Len())
			sm := rm.ScopeMetrics().At(0)
			assert.Equal(t, "https://example.test/scope", sm.SchemaUrl())
			assert.Equal(t, "metric-fixture", sm.Scope().Name())
			assert.Equal(t, uint32(8), sm.Scope().DroppedAttributesCount())
			require.Equal(t, 1, sm.Metrics().Len())
			got := sm.Metrics().At(0)
			assert.Equal(t, name, got.Name())
			assert.Equal(t, "description "+name, got.Description())
			assert.Equal(t, int64(math.MinInt64), mustMapValue(t, got.Metadata(), "metadata.minimum").Int())
		})
	}

	gaugeRaw := getMetricOTLP(t, s, ctx, ids["otlp.gauge"])
	gaugeText := string(gaugeRaw)
	assert.Contains(t, gaugeText, `"asInt":"-9223372036854775808"`)
	assert.Contains(t, gaugeText, `"asDouble":-0.0`)
	assert.Contains(t, gaugeText, `"timeUnixNano":"18446744073709551615"`)
	assert.Contains(t, gaugeText, `"traceId":"11111111111111112222222222222222"`)
	assert.Contains(t, gaugeText, `"spanId":"8000000000000000"`)
	assert.Contains(t, gaugeText, `"bytesValue":"+/8="`)
	assert.Contains(t, gaugeText, `"key":"MiXeD_snake\n\"é"`)
	assert.NotContains(t, gaugeText, `:null`)
	gauge, err := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(gaugeRaw)
	require.NoError(t, err)
	gaugePoints := gauge.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).Gauge().DataPoints()
	require.Equal(t, 3, gaugePoints.Len())
	pointsByTime := map[pcommon.Timestamp]pmetric.NumberDataPoint{}
	for _, point := range gaugePoints.All() {
		pointsByTime[point.Timestamp()] = point
	}
	intPoint := pointsByTime[pcommon.Timestamp(math.MaxUint64)]
	assert.Equal(t, pmetric.NumberDataPointValueTypeInt, intPoint.ValueType())
	assert.Equal(t, int64(math.MinInt64), intPoint.IntValue())
	assert.Equal(t, pmetric.NumberDataPointValueTypeDouble, pointsByTime[2].ValueType())
	assert.True(t, math.Signbit(pointsByTime[2].DoubleValue()))
	assert.Equal(t, pmetric.NumberDataPointValueTypeEmpty, pointsByTime[3].ValueType())
	require.Equal(t, 3, intPoint.Exemplars().Len())
	assert.Equal(t, pmetric.ExemplarValueTypeInt, intPoint.Exemplars().At(0).ValueType())
	assert.Equal(t, pmetric.ExemplarValueTypeDouble, intPoint.Exemplars().At(1).ValueType())
	assert.Equal(t, pmetric.ExemplarValueTypeEmpty, intPoint.Exemplars().At(2).ValueType())

	sumRaw := getMetricOTLP(t, s, ctx, ids["otlp.sum"])
	assert.Contains(t, string(sumRaw), `"aggregationTemporality":99`)
	assert.Contains(t, string(sumRaw), `"isMonotonic":false`)
	sum, err := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(sumRaw)
	require.NoError(t, err)
	assert.Equal(t, pmetric.AggregationTemporality(99), sum.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).Sum().AggregationTemporality())

	histRaw := getMetricOTLP(t, s, ctx, ids["otlp.histogram"])
	histText := string(histRaw)
	assert.Contains(t, histText, `"count":"18446744073709551615"`)
	assert.Contains(t, histText, `"bucketCounts":["18446744073709551615","0"]`)
	assert.Contains(t, histText, `"explicitBounds":[0.5]`)
	assert.NotContains(t, histText, `"sum"`)
	assert.Contains(t, histText, `"min":-0.0`)
	assert.Contains(t, histText, `"max":"Infinity"`)
	hist, err := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(histRaw)
	require.NoError(t, err)
	hdp := hist.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).Histogram().DataPoints().At(0)
	assert.False(t, hdp.HasSum())
	assert.True(t, hdp.HasMin())
	assert.True(t, math.Signbit(hdp.Min()))
	assert.True(t, math.IsInf(hdp.Max(), 1))
	assert.Equal(t, []uint64{math.MaxUint64, 0}, hdp.BucketCounts().AsRaw())

	expRaw := getMetricOTLP(t, s, ctx, ids["otlp.exponential"])
	expText := string(expRaw)
	assert.Contains(t, expText, `"zeroCount":"18446744073709551615"`)
	assert.Contains(t, expText, `"positive":{"offset":-7,"bucketCounts":["0","18446744073709551615"]}`)
	assert.Contains(t, expText, `"negative":{"offset":9,"bucketCounts":[]}`)
	assert.Contains(t, expText, `"zeroThreshold":-0.0`)
	exp, err := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(expRaw)
	require.NoError(t, err)
	edp := exp.ResourceMetrics().At(0).ScopeMetrics().At(0).Metrics().At(0).ExponentialHistogram().DataPoints().At(0)
	assert.Equal(t, int32(-7), edp.Positive().Offset())
	assert.Equal(t, int32(9), edp.Negative().Offset())
	assert.Equal(t, 0, edp.Negative().BucketCounts().Len())
	assert.True(t, math.Signbit(edp.ZeroThreshold()))

	for name, mutated := range map[string]string{
		"wrong casing":   strings.Replace(gaugeText, `"timeUnixNano":`, `"timeUnixNANO":`, 1),
		"enum name":      strings.Replace(string(sumRaw), `"aggregationTemporality":99`, `"aggregationTemporality":"CUMULATIVE"`, 1),
		"base64 ID":      strings.Replace(gaugeText, `"traceId":"11111111111111112222222222222222"`, `"traceId":"EREREREREREiIiIiIiIiIg=="`, 1),
		"unquoted int64": strings.Replace(gaugeText, `"timeUnixNano":"18446744073709551615"`, `"timeUnixNano":18446744073709551615`, 1),
	} {
		t.Run("rejects "+name, func(t *testing.T) {
			require.Error(t, validateMetricOTLP([]byte(mutated)))
		})
	}
}

func TestGetMetricOTLPOccurrenceOwnershipAndEmptyValues(t *testing.T) {
	s, ctx := storetest.New(t)
	for i, owner := range []string{"first", "second"} {
		batch := pmetric.NewMetrics()
		rm := batch.ResourceMetrics().AppendEmpty()
		rm.SetSchemaUrl("resource-" + owner)
		rm.Resource().Attributes().PutStr("service.name", "same-service")
		rm.Resource().Attributes().PutStr("owner", owner)
		rm.Resource().SetDroppedAttributesCount(uint32(i + 1))
		sm := rm.ScopeMetrics().AppendEmpty()
		sm.SetSchemaUrl("scope-" + owner)
		sm.Scope().SetName("same-scope")
		sm.Scope().SetDroppedAttributesCount(uint32(i + 3))
		metric := sm.Metrics().AppendEmpty()
		metric.SetName("same-stream")
		metric.SetDescription(owner)
		metric.Metadata().PutStr("metadata.owner", owner)
		dp := metric.SetEmptyGauge().DataPoints().AppendEmpty()
		dp.SetTimestamp(pcommon.Timestamp(100 + i))
		dp.SetIntValue(int64(i))
		dp.Attributes().PutStr("datapoint.owner", owner)
		dp.Exemplars().AppendEmpty().FilteredAttributes().PutStr("exemplar.owner", owner)
		require.NoError(t, s.WithConn(func(conn driver.Conn) error {
			return metrics.Ingest(ctx, conn, batch, s.FlushedIDs())
		}))
	}

	var ids map[string]string = map[string]string{}
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		rows, err := db.QueryContext(ctx, `select description, id::varchar from metric_ingests`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var owner, id string
			if err := rows.Scan(&owner, &id); err != nil {
				return err
			}
			ids[owner] = id
		}
		return rows.Err()
	}))
	for _, owner := range []string{"first", "second"} {
		raw := getMetricOTLP(t, s, ctx, ids[owner])
		text := string(raw)
		assert.Contains(t, text, `"description":"`+owner+`"`)
		assert.Contains(t, text, `"schemaUrl":"resource-`+owner+`"`)
		assert.Contains(t, text, `"schemaUrl":"scope-`+owner+`"`)
		assert.Contains(t, text, `"stringValue":"`+owner+`"`)
		other := map[string]string{"first": "second", "second": "first"}[owner]
		assert.NotContains(t, text, other)
		repeated := getMetricOTLP(t, s, ctx, ids[owner])
		assert.Equal(t, raw, repeated)
	}

	empty := pmetric.NewMetrics()
	em := empty.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	em.SetName("empty-occurrence")
	em.SetEmptyGauge()
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, empty, s.FlushedIDs())
	}))
	emptyID := metricIngestIDs(t, s, ctx)["empty-occurrence"]
	emptyRaw := getMetricOTLP(t, s, ctx, emptyID)
	assert.Contains(t, string(emptyRaw), `"dataPoints":[]`)
	assert.Contains(t, string(emptyRaw), `"attributes":[]`)
	assert.Contains(t, string(emptyRaw), `"metadata":[]`)
	_, err := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(emptyRaw)
	require.NoError(t, err)
}

func TestGetMetricOTLPErrors(t *testing.T) {
	s, ctx := storetest.New(t)
	for _, id := range []string{"00000000-0000-0000-0000-000000000000", "not-a-uuid"} {
		_, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
			return metrics.GetMetricOTLP(ctx, db, id)
		})
		assert.ErrorIs(t, err, metrics.ErrMetricIngestIDNotFound)
	}

	summary := pmetric.NewMetrics()
	m := summary.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
	m.SetName("unsupported-summary")
	m.SetEmptySummary()
	require.NoError(t, s.WithConn(func(conn driver.Conn) error {
		return metrics.Ingest(ctx, conn, summary, s.FlushedIDs())
	}))
	summaryID := metricIngestIDs(t, s, ctx)["unsupported-summary"]
	_, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricOTLP(ctx, db, summaryID)
	})
	assert.ErrorIs(t, err, metrics.ErrUnsupportedMetricType)

	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		_, err := db.ExecContext(ctx, `
			insert into metric_streams
				(id, name, unit, metric_type, aggregation_temporality, is_monotonic, scope_name, scope_version, service_name)
			select uuid(), name, unit, 'FutureMetric', aggregation_temporality, is_monotonic, scope_name, scope_version, service_name
			from metric_streams where name = 'unsupported-summary';
			update metric_ingests set stream_id = (
				select id from metric_streams where metric_type = 'FutureMetric'
			) where id = ?::uuid`, summaryID)
		return err
	}))
	_, err = readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricOTLP(ctx, db, summaryID)
	})
	assert.ErrorIs(t, err, metrics.ErrUnsupportedMetricType)
}

func TestGetMetricOTLPRejectsStoredNullAndInvalidOneof(t *testing.T) {
	for name, mutation := range map[string]string{
		"SQL null":      `update datapoints set timestamp = null`,
		"invalid oneof": `update datapoints set value_type = 'Int', int_value = null`,
		"unknown oneof": `update datapoints set value_type = 'FutureValue'`,
	} {
		t.Run(name, func(t *testing.T) {
			s, ctx := storetest.New(t)
			data := pmetric.NewMetrics()
			m := data.ResourceMetrics().AppendEmpty().ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
			m.SetName("guard")
			dp := m.SetEmptyGauge().DataPoints().AppendEmpty()
			dp.SetTimestamp(1)
			dp.SetIntValue(1)
			require.NoError(t, s.WithConn(func(conn driver.Conn) error {
				return metrics.Ingest(ctx, conn, data, s.FlushedIDs())
			}))
			id := metricIngestIDs(t, s, ctx)["guard"]
			err := s.WithDBRead(func(db *sql.DB) error {
				if _, err := db.ExecContext(ctx, mutation); err != nil {
					return err
				}
				_, err := metrics.GetMetricOTLP(ctx, db, id)
				return err
			})
			assert.ErrorIs(t, err, metrics.ErrMetricsStoreInternal)
		})
	}
}

func otlpMetricFixture() pmetric.Metrics {
	data := pmetric.NewMetrics()
	rm := data.ResourceMetrics().AppendEmpty()
	rm.SetSchemaUrl("https://example.test/resource")
	rm.Resource().SetDroppedAttributesCount(7)
	rm.Resource().Attributes().PutStr("service.name", "metric-service")
	rm.Resource().Attributes().PutStr("resource.owner", "fixture")
	rm.Resource().Attributes().PutEmptyBytes("bytes").FromRaw([]byte{0xfb, 0xff})
	sm := rm.ScopeMetrics().AppendEmpty()
	sm.SetSchemaUrl("https://example.test/scope")
	sm.Scope().SetName("metric-fixture")
	sm.Scope().SetVersion("1.0")
	sm.Scope().SetDroppedAttributesCount(8)
	sm.Scope().Attributes().PutBool("scope.enabled", false)

	newMetric := func(name string) pmetric.Metric {
		metric := sm.Metrics().AppendEmpty()
		metric.SetName(name)
		metric.SetDescription("description " + name)
		metric.SetUnit("1")
		metric.Metadata().PutInt("metadata.minimum", math.MinInt64)
		nested := metric.Metadata().PutEmptyMap("metadata.nested")
		nested.PutEmptySlice("values").AppendEmpty()
		return metric
	}

	gauge := newMetric("otlp.gauge").SetEmptyGauge()
	intPoint := gauge.DataPoints().AppendEmpty()
	intPoint.SetStartTimestamp(1)
	intPoint.SetTimestamp(pcommon.Timestamp(math.MaxUint64))
	intPoint.SetFlags(pmetric.DataPointFlags(0x101))
	intPoint.SetIntValue(math.MinInt64)
	intPoint.Attributes().PutStr("MiXeD_snake\n\"é", "unchanged")
	intPoint.Attributes().PutEmpty("empty")
	intExemplar := intPoint.Exemplars().AppendEmpty()
	intExemplar.SetTimestamp(10)
	intExemplar.SetIntValue(math.MaxInt64)
	intExemplar.SetTraceID(mustDecodeTraceIDMetrics("11111111111111112222222222222222"))
	intExemplar.SetSpanID(mustDecodeSpanIDMetrics("8000000000000000"))
	intExemplar.FilteredAttributes().PutStr("exemplar.owner", "int")
	doubleExemplar := intPoint.Exemplars().AppendEmpty()
	doubleExemplar.SetTimestamp(11)
	doubleExemplar.SetDoubleValue(math.Inf(-1))
	intPoint.Exemplars().AppendEmpty().SetTimestamp(12)
	doublePoint := gauge.DataPoints().AppendEmpty()
	doublePoint.SetTimestamp(2)
	doublePoint.SetDoubleValue(math.Copysign(0, -1))
	emptyPoint := gauge.DataPoints().AppendEmpty()
	emptyPoint.SetTimestamp(3)

	sum := newMetric("otlp.sum").SetEmptySum()
	sum.SetAggregationTemporality(pmetric.AggregationTemporality(99))
	sum.SetIsMonotonic(false)
	sumPoint := sum.DataPoints().AppendEmpty()
	sumPoint.SetTimestamp(4)
	sumPoint.SetIntValue(math.MaxInt64)

	hist := newMetric("otlp.histogram").SetEmptyHistogram()
	hist.SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	histPoint := hist.DataPoints().AppendEmpty()
	histPoint.SetTimestamp(5)
	histPoint.SetCount(math.MaxUint64)
	histPoint.SetMin(math.Copysign(0, -1))
	histPoint.SetMax(math.Inf(1))
	histPoint.BucketCounts().FromRaw([]uint64{math.MaxUint64, 0})
	histPoint.ExplicitBounds().FromRaw([]float64{0.5})

	exp := newMetric("otlp.exponential").SetEmptyExponentialHistogram()
	exp.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
	expPoint := exp.DataPoints().AppendEmpty()
	expPoint.SetTimestamp(6)
	expPoint.SetCount(math.MaxUint64)
	expPoint.SetSum(0)
	expPoint.SetMin(math.Inf(-1))
	expPoint.SetMax(math.NaN())
	expPoint.SetScale(-3)
	expPoint.SetZeroCount(math.MaxUint64)
	expPoint.SetZeroThreshold(math.Copysign(0, -1))
	expPoint.Positive().SetOffset(-7)
	expPoint.Positive().BucketCounts().FromRaw([]uint64{0, math.MaxUint64})
	expPoint.Negative().SetOffset(9)
	expPoint.Negative().BucketCounts().FromRaw([]uint64{})
	return data
}

func metricIngestIDs(t *testing.T, s *store.Store, ctx context.Context) map[string]string {
	t.Helper()
	ids := map[string]string{}
	require.NoError(t, s.WithDBRead(func(db *sql.DB) error {
		rows, err := db.QueryContext(ctx, `select ms.name, mi.id::varchar
			from metric_ingests mi join metric_streams ms on ms.id = mi.stream_id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name, id string
			if err := rows.Scan(&name, &id); err != nil {
				return err
			}
			ids[name] = id
		}
		return rows.Err()
	}))
	return ids
}

func getMetricOTLP(t testing.TB, s *store.Store, ctx context.Context, id string) json.RawMessage {
	t.Helper()
	raw, err := readStore(s, func(db *sql.DB) (json.RawMessage, error) {
		return metrics.GetMetricOTLP(ctx, db, id)
	})
	require.NoError(t, err)
	return raw
}

func mustMapValue(t *testing.T, values pcommon.Map, key string) pcommon.Value {
	t.Helper()
	value, ok := values.Get(key)
	require.True(t, ok, "missing value %q", key)
	return value
}

func validateMetricOTLP(document []byte) error {
	if err := rejectMetricDuplicateJSONKeys(document); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	return validateMetricOTLPValue(value, "$")
}

func validateMetricOTLPValue(value any, path string) error {
	allowed := map[string]bool{
		"resourceMetrics": true, "resource": true, "scopeMetrics": true, "schemaUrl": true,
		"scope": true, "metrics": true, "name": true, "version": true, "description": true, "unit": true,
		"attributes": true, "metadata": true, "droppedAttributesCount": true, "key": true, "value": true,
		"gauge": true, "sum": true, "histogram": true, "exponentialHistogram": true, "dataPoints": true,
		"startTimeUnixNano": true, "timeUnixNano": true, "flags": true, "asInt": true, "asDouble": true,
		"exemplars": true, "filteredAttributes": true, "traceId": true, "spanId": true,
		"aggregationTemporality": true, "isMonotonic": true, "count": true, "min": true, "max": true,
		"bucketCounts": true, "explicitBounds": true, "scale": true, "zeroCount": true, "zeroThreshold": true,
		"positive": true, "negative": true, "offset": true, "stringValue": true, "boolValue": true,
		"intValue": true, "doubleValue": true, "arrayValue": true, "kvlistValue": true, "bytesValue": true, "values": true,
	}
	switch value := value.(type) {
	case nil:
		return fmt.Errorf("%s: serializer emitted null", path)
	case []any:
		for i, child := range value {
			if err := validateMetricOTLPValue(child, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	case map[string]any:
		oneofCount := 0
		for _, field := range []string{"asInt", "asDouble"} {
			if _, ok := value[field]; ok {
				oneofCount++
			}
		}
		if oneofCount > 1 {
			return fmt.Errorf("%s: multiple number alternatives", path)
		}
		for key, child := range value {
			if !allowed[key] {
				return fmt.Errorf("%s: unknown or incorrectly cased schema key %q", path, key)
			}
			switch key {
			case "traceId":
				if text, ok := child.(string); !ok || !metricHex32.MatchString(text) {
					return fmt.Errorf("%s.%s: not fixed-width trace hex", path, key)
				}
			case "spanId":
				if text, ok := child.(string); !ok || !metricHex16.MatchString(text) {
					return fmt.Errorf("%s.%s: not fixed-width span hex", path, key)
				}
			case "startTimeUnixNano", "timeUnixNano", "asInt", "intValue", "count", "zeroCount":
				if text, ok := child.(string); !ok || !metricDecimal.MatchString(text) {
					return fmt.Errorf("%s.%s: 64-bit integer is not decimal text", path, key)
				}
			case "aggregationTemporality", "flags", "scale", "offset", "droppedAttributesCount":
				if _, ok := child.(json.Number); !ok {
					return fmt.Errorf("%s.%s: 32-bit field is not numeric", path, key)
				}
			case "bytesValue":
				text, ok := child.(string)
				if !ok {
					return fmt.Errorf("%s.%s: bytes is not text", path, key)
				}
				if _, err := base64.StdEncoding.Strict().DecodeString(text); err != nil {
					return fmt.Errorf("%s.%s: invalid padded base64: %w", path, key, err)
				}
			case "asDouble", "doubleValue", "sum", "min", "max", "zeroThreshold":
				if key == "sum" {
					if _, object := child.(map[string]any); object {
						break
					}
				}
				if text, ok := child.(string); ok {
					if text != "NaN" && text != "Infinity" && text != "-Infinity" {
						return fmt.Errorf("%s.%s: invalid non-finite double", path, key)
					}
				} else if _, ok := child.(json.Number); !ok {
					return fmt.Errorf("%s.%s: finite double is not numeric", path, key)
				}
			}
			if err := validateMetricOTLPValue(child, path+"."+key); err != nil {
				return err
			}
		}
	}
	return nil
}

func rejectMetricDuplicateJSONKeys(document []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(document))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delim {
		case '{':
			seen := map[string]bool{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key := keyToken.(string)
				if seen[key] {
					return fmt.Errorf("duplicate JSON key %q", key)
				}
				seen[key] = true
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		}
		return nil
	}
	return walk()
}

func BenchmarkGetMetricOTLP(b *testing.B) {
	for _, points := range []int{8, 512} {
		b.Run(fmt.Sprintf("datapoints-%d-buckets-64-exemplars-4", points), func(b *testing.B) {
			ctx := context.Background()
			s, err := store.NewStore(ctx, "", zap.NewNop())
			require.NoError(b, err)
			b.Cleanup(func() { s.Close() })
			data := pmetric.NewMetrics()
			rm := data.ResourceMetrics().AppendEmpty()
			rm.Resource().Attributes().PutStr("service.name", "benchmark")
			sm := rm.ScopeMetrics().AppendEmpty()
			sm.Scope().SetName("benchmark")
			metric := sm.Metrics().AppendEmpty()
			metric.SetName("benchmark.metric")
			hist := metric.SetEmptyExponentialHistogram()
			hist.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
			for i := 0; i < points; i++ {
				dp := hist.DataPoints().AppendEmpty()
				dp.SetTimestamp(pcommon.Timestamp(i + 1))
				dp.SetCount(64)
				dp.SetSum(float64(i))
				dp.SetScale(3)
				dp.Attributes().PutStr("series", fmt.Sprintf("series-%02d", i%16))
				dp.Positive().SetOffset(-32)
				counts := make([]uint64, 64)
				for j := range counts {
					counts[j] = uint64(i + j)
				}
				dp.Positive().BucketCounts().FromRaw(counts)
				for j := 0; j < 4; j++ {
					ex := dp.Exemplars().AppendEmpty()
					ex.SetTimestamp(pcommon.Timestamp(i*10 + j))
					ex.SetIntValue(int64(i*10 + j))
					ex.FilteredAttributes().PutStr("source", fmt.Sprintf("source-%d", j))
				}
			}
			require.NoError(b, s.WithConn(func(conn driver.Conn) error {
				return metrics.Ingest(ctx, conn, data, s.FlushedIDs())
			}))
			var id string
			require.NoError(b, s.WithDBRead(func(db *sql.DB) error {
				return db.QueryRowContext(ctx, `select id::varchar from metric_ingests`).Scan(&id)
			}))
			raw := getMetricOTLP(b, s, ctx, id)
			for _, threads := range []int{1, 4} {
				b.Run(fmt.Sprintf("threads-%d", threads), func(b *testing.B) {
					require.NoError(b, s.WithDBRead(func(db *sql.DB) error {
						_, err := db.ExecContext(ctx, fmt.Sprintf("set threads=%d", threads))
						return err
					}))
					b.ResetTimer()
					for i := 0; i < b.N; i++ {
						_ = getMetricOTLP(b, s, ctx, id)
					}
					b.ReportMetric(float64(len(raw)), "output-bytes")
				})
			}
		})
	}
}
