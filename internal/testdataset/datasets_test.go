package testdataset

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/plog"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func datasetFiles(t *testing.T, dataset, signal string) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "testdata", "otlp", dataset, signal+"*.json"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	return files
}

func readRequest(t *testing.T, file string) []byte {
	t.Helper()
	raw, err := os.ReadFile(file)
	require.NoError(t, err)
	var formatted bytes.Buffer
	require.NoError(t, json.Indent(&formatted, bytes.TrimSpace(raw), "", "  "))
	assert.Equal(t, append(formatted.Bytes(), '\n'), raw, "format %s with two-space JSON indentation", file)
	return raw
}

type datasetManifest struct {
	Requests     []struct{ Signal, File string }       `json:"requests"`
	StoredCounts struct{ Spans, Logs, Datapoints int } `json:"storedCounts"`
}

func manifests(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("..", "..", "testdata", "otlp", "*", "manifest.json"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	return files
}

func TestOTLPDatasets(t *testing.T) {
	for _, filename := range manifests(t) {
		t.Run(filepath.Base(filepath.Dir(filename)), func(t *testing.T) {
			var manifest datasetManifest
			require.NoError(t, json.Unmarshal(readRequest(t, filename), &manifest))
			var spans, logs, datapoints int
			for _, request := range manifest.Requests {
				file := filepath.Join(filepath.Dir(filename), request.File)
				switch request.Signal {
				case "traces":
					data, err := (&ptrace.JSONUnmarshaler{}).UnmarshalTraces(readRequest(t, file))
					require.NoError(t, err, file)
					spans += data.SpanCount()
				case "logs":
					data, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs(readRequest(t, file))
					require.NoError(t, err, file)
					logs += data.LogRecordCount()
				case "metrics":
					data, err := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(readRequest(t, file))
					require.NoError(t, err, file)
					datapoints += data.DataPointCount()
				default:
					t.Fatalf("unknown signal %q", request.Signal)
				}
			}
			assert.Equal(t, manifest.StoredCounts.Spans, spans)
			assert.Equal(t, manifest.StoredCounts.Logs, logs)
			assert.Equal(t, manifest.StoredCounts.Datapoints, datapoints)
		})
	}
}

func TestDatasetManifests(t *testing.T) {
	for _, filename := range manifests(t) {
		dataset := filepath.Base(filepath.Dir(filename))
		t.Run(dataset, func(t *testing.T) {
			var manifest datasetManifest
			require.NoError(t, json.Unmarshal(readRequest(t, filename), &manifest))
			listed := map[string]bool{}
			for _, request := range manifest.Requests {
				require.Contains(t, []string{"traces", "logs", "metrics"}, request.Signal)
				require.Equal(t, filepath.Base(request.File), request.File)
				require.False(t, listed[request.File], "duplicate request")
				listed[request.File] = true
				assert.FileExists(t, filepath.Join(filepath.Dir(filename), request.File))
			}
			for _, signal := range []string{"traces", "logs", "metrics"} {
				for _, file := range datasetFiles(t, dataset, signal) {
					assert.True(t, listed[filepath.Base(file)], "request missing from manifest")
				}
			}
			assert.Positive(t, manifest.StoredCounts.Spans)
			assert.Positive(t, manifest.StoredCounts.Logs)
			assert.Positive(t, manifest.StoredCounts.Datapoints)
		})
	}
}

func TestCheckoutAssociations(t *testing.T) {
	type identity struct {
		trace pcommon.TraceID
		span  pcommon.SpanID
	}
	spans := map[identity]bool{}
	parents := map[identity]identity{}
	var errors int
	for _, file := range datasetFiles(t, "checkout", "traces") {
		data, err := (&ptrace.JSONUnmarshaler{}).UnmarshalTraces(readRequest(t, file))
		require.NoError(t, err)
		for _, r := range data.ResourceSpans().All() {
			for _, s := range r.ScopeSpans().All() {
				for _, span := range s.Spans().All() {
					id := identity{span.TraceID(), span.SpanID()}
					require.False(t, spans[id], "duplicate span")
					spans[id] = true
					if !span.ParentSpanID().IsEmpty() {
						parents[id] = identity{span.TraceID(), span.ParentSpanID()}
					}
					if span.Status().Code() == ptrace.StatusCodeError {
						errors++
					}
					assert.GreaterOrEqual(t, span.EndTimestamp(), span.StartTimestamp())
				}
			}
		}
	}
	assert.Equal(t, 3, errors)
	for _, parent := range parents {
		assert.True(t, spans[parent], "missing parent")
	}
	for _, file := range datasetFiles(t, "checkout", "logs") {
		data, err := (&plog.JSONUnmarshaler{}).UnmarshalLogs(readRequest(t, file))
		require.NoError(t, err)
		for _, r := range data.ResourceLogs().All() {
			for _, s := range r.ScopeLogs().All() {
				for _, log := range s.LogRecords().All() {
					assert.True(t, spans[identity{log.TraceID(), log.SpanID()}], "log association")
				}
			}
		}
	}
	var exemplarCount int
	for _, file := range datasetFiles(t, "checkout", "metrics") {
		data, err := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(readRequest(t, file))
		require.NoError(t, err)
		for _, r := range data.ResourceMetrics().All() {
			for _, s := range r.ScopeMetrics().All() {
				for _, metric := range s.Metrics().All() {
					if metric.Type() != pmetric.MetricTypeHistogram {
						continue
					}
					for _, point := range metric.Histogram().DataPoints().All() {
						var total uint64
						for _, count := range point.BucketCounts().All() {
							total += count
						}
						assert.Equal(t, point.Count(), total)
						assert.Equal(t, point.ExplicitBounds().Len()+1, point.BucketCounts().Len())
						for _, exemplar := range point.Exemplars().All() {
							exemplarCount++
							assert.True(t, spans[identity{exemplar.TraceID(), exemplar.SpanID()}], "exemplar association")
						}
					}
				}
			}
		}
	}
	assert.Equal(t, 1, exemplarCount)
}

func TestSmallMetricCoverageAndBucketCounts(t *testing.T) {
	seen := map[pmetric.MetricType]bool{}
	for _, file := range datasetFiles(t, "small", "metrics") {
		data, err := (&pmetric.JSONUnmarshaler{}).UnmarshalMetrics(readRequest(t, file))
		require.NoError(t, err)
		for _, resource := range data.ResourceMetrics().All() {
			for _, scope := range resource.ScopeMetrics().All() {
				for _, metric := range scope.Metrics().All() {
					seen[metric.Type()] = true
					switch metric.Type() {
					case pmetric.MetricTypeHistogram:
						for _, point := range metric.Histogram().DataPoints().All() {
							var count uint64
							for _, value := range point.BucketCounts().All() {
								count += value
							}
							assert.Equal(t, point.Count(), count)
							assert.Equal(t, point.ExplicitBounds().Len()+1, point.BucketCounts().Len())
							assert.GreaterOrEqual(t, point.Timestamp(), point.StartTimestamp())
						}
					case pmetric.MetricTypeExponentialHistogram:
						for _, point := range metric.ExponentialHistogram().DataPoints().All() {
							count := point.ZeroCount()
							for _, value := range point.Positive().BucketCounts().All() {
								count += value
							}
							for _, value := range point.Negative().BucketCounts().All() {
								count += value
							}
							assert.Equal(t, point.Count(), count)
							assert.GreaterOrEqual(t, point.Timestamp(), point.StartTimestamp())
						}
					}
				}
			}
		}
	}
	assert.Equal(t, map[pmetric.MetricType]bool{
		pmetric.MetricTypeGauge: true, pmetric.MetricTypeSum: true,
		pmetric.MetricTypeHistogram: true, pmetric.MetricTypeExponentialHistogram: true,
	}, seen)
}
