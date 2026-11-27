package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

// Decode repeated wire entries rather than constructing a Go map, which would
// remove duplicate keys before ingestion ever sees them.
func decodedOwnerAttributes(t *testing.T, raw string) pcommon.Map {
	t.Helper()
	logs, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs([]byte(
		`{"resourceLogs":[{"resource":{"attributes":` + raw + `}}]}`))
	require.NoError(t, err)
	return logs.ResourceLogs().At(0).Resource().Attributes()
}

func runAttributesCLIAt(t *testing.T, endpoint string, at time.Time, args ...string) string {
	t.Helper()
	return mustRunCLIAt(t, at, append(append([]string{"attributes"}, args...), "--endpoint", endpoint)...)
}

func TestAttributesThroughProductionServer(t *testing.T) {
	t.Parallel()
	endpoint, te, le, me := startAttributeIntegration(t)
	ctx := context.Background()

	// Each subtest occupies its own disjoint 1-hour window so read-only queries
	// share one Collector + DuckDB instance without cross-test interference.
	t.Run("double bits and int64 fidelity", func(t *testing.T) {
		at := time.Unix(10000, 0)
		bits := []uint64{
			0x0000000000000000, // positive zero
			0x8000000000000000, // negative zero
			0x0000000000000001, // smallest positive subnormal
			0x7fefffffffffffff, // largest finite double
			0xffefffffffffffff, // smallest finite double
			0x7ff0000000000000, // positive infinity
			0xfff0000000000000, // negative infinity
			0x7ff8000000000001, // NaN with payload
		}
		traces := ptrace.NewTraces()
		resource := traces.ResourceSpans().AppendEmpty()
		resource.Resource().Attributes().PutStr("resource.only", "excluded")
		spans := resource.ScopeSpans().AppendEmpty().Spans()
		for i, value := range bits {
			span := spans.AppendEmpty()
			span.SetTraceID(pcommon.TraceID{14: 1, 15: byte(i + 1)})
			span.SetSpanID(pcommon.SpanID{7: 1})
			span.SetStartTimestamp(8000_000_000_000)
			span.SetEndTimestamp(8000_000_000_001)
			span.Attributes().PutDouble("number", math.Float64frombits(value))
			span.Attributes().PutInt("integer", math.MaxInt64)
		}
		require.NoError(t, te.ConsumeTraces(ctx, traces))

		var keys attributeKeysResult
		require.NoError(t, json.Unmarshal([]byte(runAttributesCLIAt(t, endpoint, at, "keys", "--json")), &keys))
		assert.Equal(t, []attributeKey{
			{Key: "integer", Kind: "int64", FoundOn: []attributeLocation{{Signal: "traces", OwnerType: "span"}}},
			{Key: "number", Kind: "double", FoundOn: []attributeLocation{{Signal: "traces", OwnerType: "span"}}},
			{Key: "resource.only", Kind: "string", FoundOn: []attributeLocation{{Signal: "traces", OwnerType: "resource"}}},
		}, keys.Keys)
		assert.False(t, keys.Truncated)

		var values attributeValuesResult
		require.NoError(t, json.Unmarshal([]byte(runAttributesCLIAt(t, endpoint, at, "values", "number", "--json", "--limit", "8")), &values))
		require.Len(t, values.Values, len(bits))
		assert.False(t, values.Truncated)
		var receivedBits []uint64
		for _, value := range values.Values {
			assert.Equal(t, uint64(1), value.Count)
			assert.Equal(t, uint64(len(bits)), value.Denominator)
			assert.Equal(t, 0.125, value.RelativeFrequency)
			assert.Equal(t, []attributeLocation{{Signal: "traces", OwnerType: "span"}}, value.FoundOn)
			var tagged struct {
				Kind  string          `json:"kind"`
				Value json.RawMessage `json:"value"`
			}
			require.NoError(t, json.Unmarshal(value.Value, &tagged))
			assert.Equal(t, "double", tagged.Kind)
			if len(tagged.Value) > 0 && tagged.Value[0] == '"' {
				var hex string
				require.NoError(t, json.Unmarshal(tagged.Value, &hex))
				var exact uint64
				_, err := fmt.Sscanf(hex, "0x%016x", &exact)
				require.NoError(t, err)
				receivedBits = append(receivedBits, exact)
			} else {
				var number float64
				require.NoError(t, json.Unmarshal(tagged.Value, &number))
				receivedBits = append(receivedBits, math.Float64bits(number))
			}
		}
		bits[len(bits)-1] = 0x7ff8000000000000 // NaNs have one storage/identity representation.
		assert.ElementsMatch(t, bits, receivedBits)

		var limited attributeValuesResult
		require.NoError(t, json.Unmarshal([]byte(runAttributesCLIAt(t, endpoint, at, "values", "number", "--json", "--limit", "1")), &limited))
		require.Len(t, limited.Values, 1)
		assert.True(t, limited.Truncated)
		assert.Equal(t, uint64(8), limited.Values[0].Denominator)

		var integer attributeValuesResult
		require.NoError(t, json.Unmarshal([]byte(runAttributesCLIAt(t, endpoint, at, "values", "integer", "--json")), &integer))
		require.Len(t, integer.Values, 1)
		assert.JSONEq(t, `{"kind":"int64","value":"9223372036854775807"}`, string(integer.Values[0].Value))
		assert.Equal(t, uint64(8), integer.Values[0].Count)
		assert.Equal(t, 1.0, integer.Values[0].RelativeFrequency)
		assert.Contains(t, runAttributesCLIAt(t, endpoint, at, "values", "integer"), "100.00%")
		assert.Contains(t, runAttributesCLIAt(t, endpoint, at, "values", "integer"), "9223372036854775807")
	})

	t.Run("resource and scope frequencies across signals", func(t *testing.T) {
		at := time.Unix(20000, 0)
		traces, logs, metrics := ptrace.NewTraces(), plog.NewLogs(), pmetric.NewMetrics()
		// West records share one resource and scope. East uses a different owner.
		// Missing-key, out-of-window and other-service records must not change the
		// denominator. The timestamp and histogram count deliberately differ.
		groups := []struct {
			value     string
			service   string
			count     int
			timestamp pcommon.Timestamp
		}{
			{"west", "checkout", 100, 18000_000_000_000},
			{"east", "checkout", 1, 18000_000_000_000},
			{"", "checkout", 3, 18000_000_000_000},
			{"west", "checkout", 7, 20000_000_000_001},
			{"west", "other", 5, 18000_000_000_000},
		}
		for groupIndex, group := range groups {
			rs := traces.ResourceSpans().AppendEmpty()
			rl := logs.ResourceLogs().AppendEmpty()
			rm := metrics.ResourceMetrics().AppendEmpty()
			ss, sl, sm := rs.ScopeSpans().AppendEmpty(), rl.ScopeLogs().AppendEmpty(), rm.ScopeMetrics().AppendEmpty()
			for _, resource := range []pcommon.Resource{rs.Resource(), rl.Resource(), rm.Resource()} {
				resource.Attributes().PutStr("service.name", group.service)
				if group.value != "" {
					resource.Attributes().PutStr("region", group.value)
				}
			}
			for _, scope := range []pcommon.InstrumentationScope{ss.Scope(), sl.Scope(), sm.Scope()} {
				scope.SetName("test-scope")
				if group.value != "" {
					scope.Attributes().PutStr("region", group.value)
				}
			}
			metric := sm.Metrics().AppendEmpty()
			metric.SetName("requests")
			histogram := metric.SetEmptyHistogram()
			histogram.SetAggregationTemporality(pmetric.AggregationTemporalityCumulative)
			for i := 0; i < group.count; i++ {
				span := ss.Spans().AppendEmpty()
				span.SetTraceID(pcommon.TraceID{13: 2, 14: byte(groupIndex + 1), 15: byte(i + 1)})
				span.SetSpanID(pcommon.SpanID{7: 1})
				span.SetStartTimestamp(group.timestamp)
				span.SetEndTimestamp(group.timestamp + 1)
				log := sl.LogRecords().AppendEmpty()
				// No trace/span correlation. Alternate received and observed fallback.
				if i%2 == 0 {
					log.SetTimestamp(group.timestamp)
				} else {
					log.SetObservedTimestamp(group.timestamp)
				}
				dp := histogram.DataPoints().AppendEmpty()
				dp.SetTimestamp(group.timestamp)
				dp.SetStartTimestamp(1)
				dp.SetCount(9999)
				if group.value != "" {
					for _, attributes := range []pcommon.Map{span.Attributes(), log.Attributes(), dp.Attributes()} {
						attributes.PutStr("region", group.value)
						attributes.PutInt("exact", math.MaxInt64)
					}
				}
			}
		}
		require.NoError(t, te.ConsumeTraces(ctx, traces))
		require.NoError(t, le.ConsumeLogs(ctx, logs))
		require.NoError(t, me.ConsumeMetrics(ctx, metrics))

		for _, pair := range []attributeLocation{
			{Signal: "traces", OwnerType: "span"}, {Signal: "logs", OwnerType: "log"}, {Signal: "metrics", OwnerType: "datapoint"},
			{Signal: "traces", OwnerType: "resource"}, {Signal: "logs", OwnerType: "resource"}, {Signal: "metrics", OwnerType: "resource"},
			{Signal: "traces", OwnerType: "scope"}, {Signal: "logs", OwnerType: "scope"}, {Signal: "metrics", OwnerType: "scope"},
		} {
			t.Run(pair.Signal+"/"+pair.OwnerType, func(t *testing.T) {
				flags := []string{"--signal", pair.Signal, "--owner-type", pair.OwnerType, "--service", "checkout", "--json"}
				var result attributeValuesResult
				require.NoError(t, json.Unmarshal([]byte(runAttributesCLIAt(t, endpoint, at, append([]string{"values", "region", "--limit", "2"}, flags...)...)), &result))
				require.Len(t, result.Values, 2)
				assert.False(t, result.Truncated)
				assert.Equal(t, uint64(100), result.Values[0].Count)
				assert.Equal(t, uint64(1), result.Values[1].Count)
				assert.JSONEq(t, `{"kind":"string","value":"west"}`, string(result.Values[0].Value))
				assert.JSONEq(t, `{"kind":"string","value":"east"}`, string(result.Values[1].Value))
				for _, value := range result.Values {
					assert.Equal(t, uint64(101), value.Denominator)
					assert.Equal(t, float64(value.Count)/101, value.RelativeFrequency)
					assert.Equal(t, []attributeLocation{pair}, value.FoundOn)
				}
				var limited attributeValuesResult
				require.NoError(t, json.Unmarshal([]byte(runAttributesCLIAt(t, endpoint, at, append([]string{"values", "region", "--limit", "1"}, flags...)...)), &limited))
				require.Len(t, limited.Values, 1)
				assert.True(t, limited.Truncated)
				assert.Equal(t, uint64(101), limited.Values[0].Denominator)
				var keys attributeKeysResult
				require.NoError(t, json.Unmarshal([]byte(runAttributesCLIAt(t, endpoint, at, append([]string{"keys"}, flags...)...)), &keys))
				assert.Contains(t, keys.Keys, attributeKey{Key: "region", Kind: "string", FoundOn: []attributeLocation{pair}})
				// Scope is shared across services, but counts still follow filtered records.
				var other attributeValuesResult
				require.NoError(t, json.Unmarshal([]byte(runAttributesCLIAt(t, endpoint, at, "values", "region", "--signal", pair.Signal, "--owner-type", pair.OwnerType, "--service", "other", "--json")), &other))
				require.Len(t, other.Values, 1)
				assert.Equal(t, uint64(5), other.Values[0].Count)
				assert.Equal(t, uint64(5), other.Values[0].Denominator)
			})
		}
	})

	t.Run("child owners follow their records", func(t *testing.T) {
		at := time.Unix(30000, 0)
		traces, metrics := ptrace.NewTraces(), pmetric.NewMetrics()
		parents := []struct {
			service   string
			timestamp pcommon.Timestamp
		}{
			{"checkout", 26400_000_000_000},
			{"checkout", 30000_000_000_000},
			{"checkout", 30000_000_000_001},
			{"other", 28000_000_000_000},
			{"checkout", 26399_999_999_999},
		}
		for i, parent := range parents {
			rs := traces.ResourceSpans().AppendEmpty()
			rs.Resource().Attributes().PutStr("service.name", parent.service)
			span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
			span.SetTraceID(pcommon.TraceID{14: 3, 15: byte(i + 1)})
			span.SetSpanID(pcommon.SpanID{7: 42})
			span.SetStartTimestamp(parent.timestamp)
			span.SetEndTimestamp(parent.timestamp + 1)
			span.Attributes().PutStr("direct.only", "exclude")
			rm := metrics.ResourceMetrics().AppendEmpty()
			rm.Resource().Attributes().PutStr("service.name", parent.service)
			metric := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
			metric.SetName("requests")
			metric.Metadata().PutStr("phase", "alpha")
			dp := metric.SetEmptyGauge().DataPoints().AppendEmpty()
			dp.SetTimestamp(parent.timestamp)
			dp.SetIntValue(1)
			dp.Attributes().PutStr("direct.only", "exclude")
			// Duplicate child values on a parent count that parent once. The second
			// eligible parent also carries beta, so shares can add to more than 100%.
			for j, value := range []string{"alpha", "alpha", "beta"} {
				if j == 2 && i != 1 {
					continue
				}
				event := span.Events().AppendEmpty()
				event.SetName("phase")
				event.SetTimestamp(parent.timestamp)
				event.Attributes().PutStr("phase", value)
				link := span.Links().AppendEmpty()
				link.SetTraceID(pcommon.TraceID{15: 4})
				link.SetSpanID(pcommon.SpanID{7: 42})
				link.Attributes().PutStr("phase", value)
				exemplar := dp.Exemplars().AppendEmpty()
				exemplar.SetTimestamp(parent.timestamp)
				exemplar.SetIntValue(int64(j))
				// No correlation IDs: metric_datapoint_id supplies ownership.
				exemplar.FilteredAttributes().PutStr("phase", value)
			}
		}
		require.NoError(t, te.ConsumeTraces(ctx, traces))
		require.NoError(t, me.ConsumeMetrics(ctx, metrics))

		for _, pair := range []attributeLocation{
			{Signal: "traces", OwnerType: "event"},
			{Signal: "traces", OwnerType: "link"},
			{Signal: "metrics", OwnerType: "exemplar"},
			{Signal: "metrics", OwnerType: "metadata"},
		} {
			t.Run(pair.Signal+"/"+pair.OwnerType, func(t *testing.T) {
				flags := []string{"--signal", pair.Signal, "--owner-type", pair.OwnerType, "--service", "checkout", "--json"}
				var keys attributeKeysResult
				require.NoError(t, json.Unmarshal([]byte(runAttributesCLIAt(t, endpoint, at, append([]string{"keys"}, flags...)...)), &keys))
				assert.Equal(t, []attributeKey{{Key: "phase", Kind: "string", FoundOn: []attributeLocation{pair}}}, keys.Keys)
				var values attributeValuesResult
				require.NoError(t, json.Unmarshal([]byte(runAttributesCLIAt(t, endpoint, at, append([]string{"values", "phase"}, flags...)...)), &values))
				expectedLen := 2
				if pair.OwnerType == "metadata" {
					expectedLen = 1
				}
				require.Len(t, values.Values, expectedLen)
				assert.False(t, values.Truncated)
				assert.JSONEq(t, `{"kind":"string","value":"alpha"}`, string(values.Values[0].Value))
				assert.Equal(t, uint64(2), values.Values[0].Count)
				assert.Equal(t, uint64(2), values.Values[0].Denominator)
				assert.Equal(t, 1.0, values.Values[0].RelativeFrequency)
				assert.Equal(t, []attributeLocation{pair}, values.Values[0].FoundOn)
				if expectedLen == 2 {
					assert.JSONEq(t, `{"kind":"string","value":"beta"}`, string(values.Values[1].Value))
					assert.Equal(t, uint64(1), values.Values[1].Count)
					assert.Equal(t, uint64(2), values.Values[1].Denominator)
					assert.Equal(t, 0.5, values.Values[1].RelativeFrequency)
				}
				var limited attributeValuesResult
				require.NoError(t, json.Unmarshal([]byte(runAttributesCLIAt(t, endpoint, at, append([]string{"values", "phase", "--limit", "1"}, flags...)...)), &limited))
				assert.Equal(t, expectedLen == 2, limited.Truncated)
				require.Len(t, limited.Values, 1)
				assert.Equal(t, uint64(2), limited.Values[0].Denominator)
			})
		}
	})

	t.Run("kinds survive every owner path", func(t *testing.T) {
		at := time.Unix(40000, 0)
		const ts = pcommon.Timestamp(38000_000_000_000)
		traces, logs, metrics := ptrace.NewTraces(), plog.NewLogs(), pmetric.NewMetrics()
		rs, rl, rm := traces.ResourceSpans().AppendEmpty(), logs.ResourceLogs().AppendEmpty(), metrics.ResourceMetrics().AppendEmpty()
		ss, sl, sm := rs.ScopeSpans().AppendEmpty(), rl.ScopeLogs().AppendEmpty(), rm.ScopeMetrics().AppendEmpty()
		span := ss.Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{14: 4, 15: 1})
		span.SetSpanID(pcommon.SpanID{7: 1})
		span.SetStartTimestamp(ts)
		span.SetEndTimestamp(ts + 1)
		log := sl.LogRecords().AppendEmpty()
		log.SetObservedTimestamp(ts)
		metric := sm.Metrics().AppendEmpty()
		metric.SetName("typed")
		dp := metric.SetEmptyGauge().DataPoints().AppendEmpty()
		dp.SetTimestamp(ts)
		dp.SetIntValue(1)
		event := span.Events().AppendEmpty()
		event.SetTimestamp(ts)
		exemplar := dp.Exemplars().AppendEmpty()
		exemplar.SetTimestamp(ts)
		owners := []struct {
			location   attributeLocation
			attributes pcommon.Map
		}{
			{attributeLocation{"traces", "span"}, span.Attributes()},
			{attributeLocation{"traces", "event"}, event.Attributes()},
			{attributeLocation{"traces", "link"}, span.Links().AppendEmpty().Attributes()},
			{attributeLocation{"traces", "resource"}, rs.Resource().Attributes()},
			{attributeLocation{"traces", "scope"}, ss.Scope().Attributes()},
			{attributeLocation{"logs", "log"}, log.Attributes()},
			{attributeLocation{"logs", "resource"}, rl.Resource().Attributes()},
			{attributeLocation{"logs", "scope"}, sl.Scope().Attributes()},
			{attributeLocation{"metrics", "datapoint"}, dp.Attributes()},
			{attributeLocation{"metrics", "exemplar"}, exemplar.FilteredAttributes()},
			{attributeLocation{"metrics", "metadata"}, metric.Metadata()},
			{attributeLocation{"metrics", "resource"}, rm.Resource().Attributes()},
			{attributeLocation{"metrics", "scope"}, sm.Scope().Attributes()},
		}
		for _, owner := range owners {
			a := owner.attributes
			a.PutStr("location", owner.location.Signal+"/"+owner.location.OwnerType)
			a.PutStr("string", "9007199254740993")
			a.PutInt("int64", math.MaxInt64)
			a.PutDouble("double", math.Copysign(0, -1))
			a.PutDouble("infinity", math.Inf(1))
			a.PutBool("bool", true)
			a.PutEmpty("empty")
			a.PutEmptyBytes("bytes").FromRaw([]byte{0, 1, 255})
			array := a.PutEmptySlice("array")
			array.AppendEmpty().SetInt(math.MinInt64)
			array.AppendEmpty().SetDouble(math.SmallestNonzeroFloat64)
			a.PutEmptyMap("map").PutInt("nested", 9007199254740993)
		}
		require.NoError(t, te.ConsumeTraces(ctx, traces))
		require.NoError(t, le.ConsumeLogs(ctx, logs))
		require.NoError(t, me.ConsumeMetrics(ctx, metrics))

		expected := map[string]string{
			"string":   `{"kind":"string","value":"9007199254740993"}`,
			"int64":    `{"kind":"int64","value":"9223372036854775807"}`,
			"double":   `{"kind":"double","value":"0x8000000000000000"}`,
			"infinity": `{"kind":"double","value":"0x7ff0000000000000"}`,
			"bool":     `{"kind":"bool","value":true}`,
			"empty":    `{"kind":"empty","value":null}`,
			"bytes":    `{"kind":"bytes","value":"AAH/"}`,
			"array":    `{"kind":"array","value":[{"kind":"int64","value":"-9223372036854775808"},{"kind":"double","value":5e-324}]}`,
			"map":      `{"kind":"map","value":[{"key":"nested","value":{"kind":"int64","value":"9007199254740993"}}]}`,
		}
		for _, filter := range []struct {
			args      []string
			records   uint64
			locations int
		}{
			{nil, 3, 13},
			{[]string{"--signal", "logs"}, 1, 3},
			{[]string{"--owner-type", "resource"}, 3, 3},
		} {
			var shared, distinct attributeValuesResult
			run := func(key string, result *attributeValuesResult) {
				args := append([]string{"values", key, "--json"}, filter.args...)
				require.NoError(t, json.Unmarshal([]byte(runAttributesCLIAt(t, endpoint, at, args...)), result))
			}
			run("int64", &shared)
			require.Len(t, shared.Values, 1)
			assert.Equal(t, filter.records, shared.Values[0].Count)
			assert.Equal(t, filter.records, shared.Values[0].Denominator)
			assert.Len(t, shared.Values[0].FoundOn, filter.locations)
			run("location", &distinct)
			require.Len(t, distinct.Values, filter.locations)
			for _, v := range distinct.Values {
				require.Len(t, v.FoundOn, 1)
				assert.JSONEq(t, fmt.Sprintf(`{"kind":"string","value":%q}`, v.FoundOn[0].Signal+"/"+v.FoundOn[0].OwnerType), string(v.Value))
				assert.Equal(t, uint64(1), v.Count)
				assert.Equal(t, filter.records, v.Denominator)
			}
			var keys attributeKeysResult
			require.NoError(t, json.Unmarshal([]byte(runAttributesCLIAt(t, endpoint, at, append([]string{"keys", "--json"}, filter.args...)...)), &keys))
			var locations []attributeLocation
			for _, k := range keys.Keys {
				if k.Key == "int64" && k.Kind == "int64" {
					locations = append(locations, k.FoundOn...)
				}
			}
			assert.ElementsMatch(t, shared.Values[0].FoundOn, locations)
		}
		table := runAttributesCLIAt(t, endpoint, at, "values", "int64", "--signal", "logs")
		assert.Contains(t, table, "foundOn")
		assert.Contains(t, table, "resource")
		for _, owner := range owners {
			pair := owner.location
			t.Run(pair.Signal+"/"+pair.OwnerType, func(t *testing.T) {
				for key, want := range expected {
					var result attributeValuesResult
					require.NoError(t, json.Unmarshal([]byte(runAttributesCLIAt(t, endpoint, at, "values", key, "--signal", pair.Signal, "--owner-type", pair.OwnerType, "--json")), &result))
					require.Len(t, result.Values, 1, key)
					value := result.Values[0]
					assert.JSONEq(t, want, string(value.Value), key)
					assert.Equal(t, uint64(1), value.Count)
					assert.Equal(t, uint64(1), value.Denominator)
					assert.Equal(t, []attributeLocation{pair}, value.FoundOn)
				}
				var location attributeValuesResult
				require.NoError(t, json.Unmarshal([]byte(runAttributesCLIAt(t, endpoint, at, "values", "location", "--signal", pair.Signal, "--owner-type", pair.OwnerType, "--json")), &location))
				require.Len(t, location.Values, 1)
				assert.JSONEq(t, fmt.Sprintf(`{"kind":"string","value":%q}`, pair.Signal+"/"+pair.OwnerType), string(location.Values[0].Value))
			})
		}
	})

	t.Run("last attribute selection is local to every owner", func(t *testing.T) {
		at := time.Unix(50000, 0)
		const ts = pcommon.Timestamp(48000_000_000_000)
		traces, logs, metrics := ptrace.NewTraces(), plog.NewLogs(), pmetric.NewMetrics()
		rs, rl, rm := traces.ResourceSpans().AppendEmpty(), logs.ResourceLogs().AppendEmpty(), metrics.ResourceMetrics().AppendEmpty()
		ss, sl, sm := rs.ScopeSpans().AppendEmpty(), rl.ScopeLogs().AppendEmpty(), rm.ScopeMetrics().AppendEmpty()
		span := ss.Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{14: 5, 15: 1})
		span.SetSpanID(pcommon.SpanID{7: 1})
		span.SetStartTimestamp(ts)
		span.SetEndTimestamp(ts + 1)
		event := span.Events().AppendEmpty()
		event.SetTimestamp(ts)
		link := span.Links().AppendEmpty()
		log := sl.LogRecords().AppendEmpty()
		log.SetTimestamp(ts)
		metric := sm.Metrics().AppendEmpty()
		metric.SetName("usage")
		dp := metric.SetEmptyGauge().DataPoints().AppendEmpty()
		dp.SetTimestamp(ts)
		dp.SetIntValue(1)
		exemplar := dp.Exemplars().AppendEmpty()
		exemplar.SetTimestamp(ts)
		exemplar.SetIntValue(1)
		owners := []struct {
			location attributeLocation
			attrs    pcommon.Map
		}{
			{attributeLocation{"traces", "resource"}, rs.Resource().Attributes()},
			{attributeLocation{"traces", "scope"}, ss.Scope().Attributes()},
			{attributeLocation{"traces", "span"}, span.Attributes()},
			{attributeLocation{"traces", "event"}, event.Attributes()},
			{attributeLocation{"traces", "link"}, link.Attributes()},
			{attributeLocation{"logs", "resource"}, rl.Resource().Attributes()},
			{attributeLocation{"logs", "scope"}, sl.Scope().Attributes()},
			{attributeLocation{"logs", "log"}, log.Attributes()},
			{attributeLocation{"metrics", "resource"}, rm.Resource().Attributes()},
			{attributeLocation{"metrics", "scope"}, sm.Scope().Attributes()},
			{attributeLocation{"metrics", "metadata"}, metric.Metadata()},
			{attributeLocation{"metrics", "datapoint"}, dp.Attributes()},
			{attributeLocation{"metrics", "exemplar"}, exemplar.FilteredAttributes()},
		}
		for i, owner := range owners {
			decodedOwnerAttributes(t, fmt.Sprintf(`[
{"key":"shared","value":{"stringValue":"discard"}},
{"key":"shared","value":{"intValue":"%d"}},
{"key":"Shared","value":{"boolValue":true}}]`, i+1)).CopyTo(owner.attrs)
		}
		// Another span is a separate owner even within the same Resource and Scope.
		other := ss.Spans().AppendEmpty()
		other.SetTraceID(pcommon.TraceID{14: 5, 15: 2})
		other.SetSpanID(pcommon.SpanID{7: 1})
		other.SetStartTimestamp(ts)
		other.SetEndTimestamp(ts + 1)
		decodedOwnerAttributes(t, `[{"key":"shared","value":{"intValue":"99"}},
{"key":"shared","value":{"intValue":"100"}}]`).CopyTo(other.Attributes())
		decodedBody, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs([]byte(`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"kvlistValue":{"values":[
{"key":"x","value":{"stringValue":"old"}},
{"key":"x","value":{"intValue":"9223372036854775807"}},
{"key":"array","value":{"arrayValue":{"values":[{"intValue":"2"},{"intValue":"2"}]}}}
]}}}]}]}]}`))
		require.NoError(t, err)
		decodedBody.ResourceLogs().At(0).ScopeLogs().At(0).LogRecords().At(0).Body().CopyTo(log.Body())
		require.NoError(t, te.ConsumeTraces(ctx, traces))
		require.NoError(t, le.ConsumeLogs(ctx, logs))
		require.NoError(t, me.ConsumeMetrics(ctx, metrics))

		for i, owner := range owners {
			var result attributeValuesResult
			pair := owner.location
			output := runAttributesCLIAt(t, endpoint, at, "values", "shared", "--signal", pair.Signal, "--owner-type", pair.OwnerType, "--json")
			require.NoError(t, json.Unmarshal([]byte(output), &result))
			want := []string{fmt.Sprintf(`{"kind":"int64","value":"%d"}`, i+1)}
			if pair.OwnerType == "span" {
				want = append(want, `{"kind":"int64","value":"100"}`)
			}
			require.Len(t, result.Values, len(want), pair)
			var actual []string
			for _, value := range result.Values {
				actual = append(actual, string(value.Value))
			}
			assert.ElementsMatch(t, want, actual, pair)
			assert.Equal(t, 3, owner.attrs.Len(), "incoming collections remain untouched")
		}

		_, refs, err := requestQuery(ctx, http.DefaultClient, endpoint, fmt.Sprintf("select id::varchar from logs where timestamp = %d", uint64(ts)), 1)
		require.NoError(t, err)
		require.Len(t, refs.Rows, 1)
		ref, ok := refs.Rows[0][0].(string)
		require.True(t, ok)
		for _, query := range []string{"", "?format=json"} {
			response, err := http.Get(endpoint + "/export/logs/" + ref + query)
			require.NoError(t, err)
			require.Equal(t, http.StatusOK, response.StatusCode)
			wire, err := io.ReadAll(response.Body)
			require.NoError(t, response.Body.Close())
			require.NoError(t, err)
			exported, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs(wire)
			require.NoError(t, err)
			resource := exported.ResourceLogs().At(0)
			scope := resource.ScopeLogs().At(0)
			record := scope.LogRecords().At(0)
			for i, attrs := range []pcommon.Map{resource.Resource().Attributes(), scope.Scope().Attributes(), record.Attributes()} {
				require.Equal(t, 2, attrs.Len())
				value, found := attrs.Get("shared")
				require.True(t, found)
				assert.Equal(t, int64(i+6), value.Int())
			}
			require.Equal(t, 2, record.Body().Map().Len())
			winner, found := record.Body().Map().Get("x")
			require.True(t, found)
			assert.Equal(t, int64(9223372036854775807), winner.Int())
			array, found := record.Body().Map().Get("array")
			require.True(t, found)
			assert.Equal(t, 2, array.Slice().Len(), "array repetition survives export")
		}
	})

	t.Run("service labels follow selected resource attributes", func(t *testing.T) {
		at := time.Unix(60000, 0)
		const ts = pcommon.Timestamp(58000_000_000_000)
		traces, logs, metrics := ptrace.NewTraces(), plog.NewLogs(), pmetric.NewMetrics()
		rs, rl, rm := traces.ResourceSpans().AppendEmpty(), logs.ResourceLogs().AppendEmpty(), metrics.ResourceMetrics().AppendEmpty()
		attrs := decodedOwnerAttributes(t, `[
{"key":"service.name","value":{"stringValue":"discard"}},
{"key":"service.name","value":{"intValue":"42"}},
{"key":"service.namespace","value":{"stringValue":"discard"}},
{"key":"service.namespace","value":{"stringValue":"shop"}}]`)
		for _, resource := range []pcommon.Resource{rs.Resource(), rl.Resource(), rm.Resource()} {
			attrs.CopyTo(resource.Attributes())
		}
		span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
		span.SetTraceID(pcommon.TraceID{14: 6, 15: 1})
		span.SetSpanID(pcommon.SpanID{7: 1})
		span.SetStartTimestamp(ts)
		span.SetEndTimestamp(ts + 1)
		rl.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().SetTimestamp(ts)
		metric := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
		metric.SetName("usage")
		dp := metric.SetEmptyGauge().DataPoints().AppendEmpty()
		dp.SetTimestamp(ts)
		dp.SetIntValue(1)
		healthy := logs.ResourceLogs().AppendEmpty()
		healthy.Resource().Attributes().PutStr("service.name", "healthy")
		healthy.ScopeLogs().AppendEmpty().LogRecords().AppendEmpty().SetTimestamp(ts)
		require.NoError(t, te.ConsumeTraces(ctx, traces))
		require.NoError(t, le.ConsumeLogs(ctx, logs))
		require.NoError(t, me.ConsumeMetrics(ctx, metrics))

		result := servicesResultAt(t, endpoint, at)
		assertServiceSummaries(t, []serviceSummary{
			{ServiceName: "healthy", ServiceNameKind: "string", ServiceNamespaceKind: "string", LogCount: 1, LogSeverityCounts: logSeverityCounts{Other: 1}, LastSeen: fmt.Sprint(uint64(ts))},
			{ServiceName: "42", ServiceNamespace: "shop", ServiceNameKind: "int64", ServiceNamespaceKind: "string", TraceCount: 1, SpanCount: 1, LogCount: 1, LogSeverityCounts: logSeverityCounts{Other: 1}, MetricCount: 1, DataPointCount: 1, LastSeen: fmt.Sprint(uint64(ts))},
		}, result.Services)
		assert.Equal(t, result.Services[1:], servicesResultAt(t, endpoint, at, "--service", "42").Services)
		assert.Empty(t, servicesResultAt(t, endpoint, at, "--service", "discard").Services)
	})

	t.Run("log time precedence and unsigned bounds", func(t *testing.T) {
		at := time.Unix(70000, 0)
		logs := plog.NewLogs()
		for i, times := range [][2]uint64{
			{66400_000_000_000, 0}, {70000_000_000_000, 0}, {0, 68000_000_000_000},
			{1, 68000_000_000_000}, {70000_000_000_001, 68000_000_000_000},
			{0, 0}, {math.MaxUint64, 0},
		} {
			rl := logs.ResourceLogs().AppendEmpty()
			// Exercise the existing resource-derived service-name projection.
			rl.Resource().Attributes().PutInt("service.name", 42)
			sl := rl.ScopeLogs().AppendEmpty()
			log := sl.LogRecords().AppendEmpty()
			log.SetTimestamp(pcommon.Timestamp(times[0]))
			log.SetObservedTimestamp(pcommon.Timestamp(times[1]))
			for _, attributes := range []pcommon.Map{rl.Resource().Attributes(), sl.Scope().Attributes(), log.Attributes()} {
				attributes.PutInt("row", int64(i))
			}
		}
		require.NoError(t, le.ConsumeLogs(ctx, logs))
		for _, owner := range []string{"log", "resource", "scope"} {
			var result attributeValuesResult
			require.NoError(t, json.Unmarshal([]byte(runAttributesCLIAt(t, endpoint, at, "values", "row", "--signal", "logs", "--owner-type", owner, "--service", "42", "--json")), &result))
			require.Len(t, result.Values, 3)
			for i, value := range result.Values {
				assert.JSONEq(t, fmt.Sprintf(`{"kind":"int64","value":"%d"}`, i), string(value.Value))
				assert.Equal(t, uint64(3), value.Denominator)
			}
			var upper attributeValuesResult
			require.NoError(t, json.Unmarshal([]byte(runAttributesCLIAt(t, endpoint, at, "values", "row", "--signal", "logs", "--owner-type", owner,
				"--start", "2554-07-21T23:34:33.709551615Z", "--end", "2554-07-21T23:34:33.709551615Z", "--json")), &upper))
			require.Len(t, upper.Values, 1)
			assert.JSONEq(t, `{"kind":"int64","value":"6"}`, string(upper.Values[0].Value))
		}
	})

	t.Run("events and exemplars use their own time", func(t *testing.T) {
		at := time.Unix(80000, 0)
		traces, metrics := ptrace.NewTraces(), pmetric.NewMetrics()
		type child struct {
			timestamp pcommon.Timestamp
			value     string
		}
		fixtures := []struct {
			parentTime pcommon.Timestamp
			service    string
			children   []child
		}{
			{1, "checkout", []child{{76400_000_000_000, "alpha"}, {78000_000_000_000, "alpha"}, {80000_000_000_001, "outside"}}},
			{78000_000_000_000, "checkout", []child{{76399_999_999_999, "outside"}, {80000_000_000_001, "outside"}}},
			{80000_000_000_001, "checkout", []child{{80000_000_000_000, "beta"}}},
			{78000_000_000_000, "other", []child{{78000_000_000_000, "other-service"}}},
			{78000_000_000_000, "checkout", []child{{0, "zero"}}},
			{78000_000_000_000, "checkout", []child{{math.MaxUint64, "unsigned"}}},
		}
		for i, fixture := range fixtures {
			rs := traces.ResourceSpans().AppendEmpty()
			rs.Resource().Attributes().PutStr("service.name", fixture.service)
			span := rs.ScopeSpans().AppendEmpty().Spans().AppendEmpty()
			span.SetTraceID(pcommon.TraceID{14: 8, 15: byte(i + 1)})
			span.SetSpanID(pcommon.SpanID{7: 42})
			span.SetStartTimestamp(fixture.parentTime)
			span.SetEndTimestamp(fixture.parentTime + 1)
			rm := metrics.ResourceMetrics().AppendEmpty()
			rm.Resource().Attributes().PutStr("service.name", fixture.service)
			metric := rm.ScopeMetrics().AppendEmpty().Metrics().AppendEmpty()
			metric.SetName("child-time")
			dp := metric.SetEmptyGauge().DataPoints().AppendEmpty()
			dp.SetTimestamp(fixture.parentTime)
			dp.SetIntValue(1)
			for _, child := range fixture.children {
				event := span.Events().AppendEmpty()
				event.SetTimestamp(child.timestamp)
				event.Attributes().PutStr("phase", child.value)
				event.Attributes().PutBool("only."+child.value, true)
				exemplar := dp.Exemplars().AppendEmpty()
				exemplar.SetTimestamp(child.timestamp)
				exemplar.SetIntValue(1)
				exemplar.FilteredAttributes().PutStr("phase", child.value)
				exemplar.FilteredAttributes().PutBool("only."+child.value, true)
			}
		}
		require.NoError(t, te.ConsumeTraces(ctx, traces))
		require.NoError(t, me.ConsumeMetrics(ctx, metrics))

		for _, pair := range []attributeLocation{{"traces", "event"}, {"metrics", "exemplar"}} {
			t.Run(pair.Signal+"/"+pair.OwnerType, func(t *testing.T) {
				flags := []string{"--signal", pair.Signal, "--owner-type", pair.OwnerType, "--service", "checkout", "--json"}
				var values attributeValuesResult
				require.NoError(t, json.Unmarshal([]byte(runAttributesCLIAt(t, endpoint, at, append([]string{"values", "phase"}, flags...)...)), &values))
				require.Len(t, values.Values, 2)
				for i, want := range []string{"alpha", "beta"} {
					assert.JSONEq(t, fmt.Sprintf(`{"kind":"string","value":%q}`, want), string(values.Values[i].Value))
					assert.Equal(t, uint64(1), values.Values[i].Count)
					assert.Equal(t, uint64(2), values.Values[i].Denominator)
					assert.Equal(t, 0.5, values.Values[i].RelativeFrequency)
					assert.Equal(t, []attributeLocation{pair}, values.Values[i].FoundOn)
				}
				var keys attributeKeysResult
				require.NoError(t, json.Unmarshal([]byte(runAttributesCLIAt(t, endpoint, at, append([]string{"keys"}, flags...)...)), &keys))
				assert.Equal(t, []attributeKey{
					{Key: "only.alpha", Kind: "bool", FoundOn: []attributeLocation{pair}},
					{Key: "only.beta", Kind: "bool", FoundOn: []attributeLocation{pair}},
					{Key: "phase", Kind: "string", FoundOn: []attributeLocation{pair}},
				}, keys.Keys)
				var limited attributeValuesResult
				require.NoError(t, json.Unmarshal([]byte(runAttributesCLIAt(t, endpoint, at, append([]string{"values", "phase", "--limit", "1"}, flags...)...)), &limited))
				require.Len(t, limited.Values, 1)
				assert.True(t, limited.Truncated)
				assert.Equal(t, uint64(2), limited.Values[0].Denominator)
				for _, boundary := range []struct{ time, value string }{
					{"1970-01-01T00:00:00Z", "zero"},
					{"2554-07-21T23:34:33.709551615Z", "unsigned"},
				} {
					var result attributeValuesResult
					args := append([]string{"values", "phase", "--start", boundary.time, "--end", boundary.time}, flags...)
					require.NoError(t, json.Unmarshal([]byte(runAttributesCLIAt(t, endpoint, at, args...)), &result))
					require.Len(t, result.Values, 1)
					assert.JSONEq(t, fmt.Sprintf(`{"kind":"string","value":%q}`, boundary.value), string(result.Values[0].Value))
					assert.Equal(t, uint64(1), result.Values[0].Denominator)
				}
			})
		}
	})
}
