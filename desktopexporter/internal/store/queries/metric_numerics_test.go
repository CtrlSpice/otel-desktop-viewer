package queries_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store"
	"github.com/CtrlSpice/otel-desktop-viewer/desktopexporter/internal/store/metrics"
	"github.com/stretchr/testify/require"
)

func numericID(n int) string { return fmt.Sprintf("00000000-0000-0000-0000-%012d", n) }

func numericMetricDB(t *testing.T, kind string, seriesCount int) (*sql.DB, []string) {
	t.Helper()
	db := freshDB(t)
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{
		`insert into metric_records(record_id,id,resource_id,resource_payload_id,scope_id,name,metric_type,aggregation_temporality,datapoints,resource,scope)
		 values ('` + numericID(4) + `','` + numericID(3) + `','` + numericID(1) + `','` + numericID(1) + `','` + numericID(2) + `','numerics','` + kind + `',1,[],
		 {'id': '` + numericID(1) + `'::uuid, 'payload_id': '` + numericID(1) + `'::uuid, 'schema_url': '', 'attributes': '{}'::json::variant, 'dropped_attributes_count': 0},
		 {'id': '` + numericID(2) + `'::uuid, 'name': '', 'version': '', 'schema_url': '', 'attributes': '{}'::json::variant, 'dropped_attributes_count': 0})`,
	} {
		_, err := db.Exec(stmt)
		require.NoError(t, err)
	}
	if kind == "Sum" {
		_, err := db.Exec("update metric_records set aggregation_temporality=2")
		require.NoError(t, err)
	}
	var ids []string
	// Preserve ascending series-ID order in the cancellation-sensitive fold
	// fixture while using the production deterministic identity expression.
	_, err := db.Exec(`create temp table numeric_series as select row_number() over(order by id)-1 as ordinal, *
		from (select range::varchar as key, md5(?::varchar||':'||range::varchar)::uuid as id from range(?))`, numericID(3), seriesCount)
	require.NoError(t, err)
	for i := range seriesCount {
		var id string
		err := db.QueryRow(`select id::varchar from numeric_series where ordinal=?`, i).Scan(&id)
		require.NoError(t, err)
		ids = append(ids, id)
	}
	return db, ids
}

func numericPoint(t *testing.T, db *sql.DB, id, series int, timestamp uint64, value float64, integer *int64) {
	t.Helper()
	var double any = value
	if integer != nil {
		double = nil
	}
	var key string
	require.NoError(t, db.QueryRow(`select key from numeric_series where ordinal=?`, series).Scan(&key))
	_, err := db.Exec(`update metric_records set datapoints=list_append(datapoints,
		struct_pack(id:=$1::uuid,timestamp:=$2::ubigint,attributes:={'series': $3::varchar}::variant,
		attribute_key:=$3::varchar,double_value:=$4::double,int_value:=$5::bigint,value_type:=$6::varchar,
		count:=1::ubigint,sum:=$7::double,explicit_bounds:=[0,1]::double[],bucket_counts:=[0,1,0]::ubigint[],
		scale:=0,zero_count:=0::ubigint,zero_threshold:=0::double,positive_bucket_offset:=0,
		positive_bucket_counts:=[1]::ubigint[],negative_bucket_offset:=0,negative_bucket_counts:=[]::ubigint[])),
		point_time_min=least(point_time_min,$2::ubigint),point_time_max=greatest(point_time_max,$2::ubigint)`,
		numericID(1000+id), timestamp, key,
		double, integer, map[bool]string{true: "Int", false: "Double"}[integer != nil], value)
	require.NoError(t, err)
}

func numericView(t *testing.T, db *sql.DB, selected []string, end uint64, views int64, aggregate bool, histogram bool) map[string]any {
	t.Helper()
	var raw json.RawMessage
	var err error
	var target int64
	var quantiles []float64
	if histogram {
		target, quantiles = 1, []float64{0.5}
	}
	if aggregate {
		raw, err = metrics.GetMetricAggregateView(context.Background(), db, numericID(3), store.BoundedTimeRange(0, end), target, nil, quantiles, 0, views, selected, "")
	} else {
		raw, err = metrics.GetMetricView(context.Background(), db, numericID(3), store.BoundedTimeRange(0, end), target, nil, quantiles, 0, views, 0, selected, "", nil, 0)
	}
	require.NoError(t, err)
	var result map[string]any
	require.NoError(t, json.Unmarshal(raw, &result))
	return result
}

// Reinsert the identical stored rows (including IDs) in a different physical
// order. Changing ingest UUIDs instead would change the timestamp-tie contract.
func reorderNumericPoints(t *testing.T, db *sql.DB, reverse bool) {
	t.Helper()
	order := "d.id"
	if reverse {
		order = "d.id desc"
	}
	_, err := db.Exec(`update metric_records set datapoints=(select list(d order by ` + order + `)
		from (select unnest(datapoints) as d))`)
	require.NoError(t, err)
}

func TestMetricNumericsOrderedScalarFolds(t *testing.T) {
	for _, pool := range []bool{false, true} {
		t.Run(fmt.Sprintf("cross-series=%v", pool), func(t *testing.T) {
			n := 1
			if pool {
				n = 4
			}
			db, ids := numericMetricDB(t, "Sum", n)
			for i, value := range []float64{1e16, 1, -1e16, 1} {
				series, timestamp := 0, uint64(100)
				if pool {
					series = i
				}
				// Tied timestamps make the datapoint ID tie-break observable.
				numericPoint(t, db, i, series, timestamp, value, nil)
				if pool {
					numericPoint(t, db, 10+i, series, 200, 0, nil)
				}
			}
			var baseline map[string]any
			for _, reverse := range []bool{false, true} {
				reorderNumericPoints(t, db, reverse)
				for _, threads := range []int{1, 4} {
					_, err := db.Exec(fmt.Sprintf("set threads=%d", threads))
					require.NoError(t, err)
					full := numericView(t, db, ids, 999999, 1, false, false)
					only := numericView(t, db, ids, 999999, 1, true, false)
					require.Equal(t, full["scalarAggregate"], only["scalarAggregate"])
					if baseline != nil {
						require.Equal(t, baseline, full, "physical order=%v threads=%d", reverse, threads)
					}
					baseline = full
					pools := full["scalarAggregate"].(map[string]any)
					for _, key := range []string{"all", "selected"} {
						buckets := pools[key].([]any)
						require.Len(t, buckets, 1)
						bucket := buckets[0].(map[string]any)
						require.Equal(t, float64(1), bucket["sum"])
						count := float64(4)
						if pool {
							count = 8
							require.Equal(t, float64(-1000), bucket["rate"])
						}
						require.Equal(t, 1/count, bucket["avg"])
					}
					if !pool {
						series := full["timeseries"].([]any)[0].(map[string]any)
						require.Equal(t, float64(1), series["stats"].(map[string]any)["sum"])
						require.Equal(t, 0.25, series["stats"].(map[string]any)["avg"])
						bucket := series["views"].([]any)[0].(map[string]any)
						require.Equal(t, float64(1), bucket["sum"])
						require.Equal(t, 0.25, bucket["avg"])
					}
				}
			}
		})
	}
}

func TestMetricNumericsOrderedRates(t *testing.T) {
	for _, separateBuckets := range []bool{false, true} {
		db, ids := numericMetricDB(t, "Sum", 1)
		for i, value := range []int64{0, 10000000000000000, 10000000000000001, 1, 2} {
			timestamp := uint64(i * 100)
			if separateBuckets {
				timestamp = uint64(i) * 1e9
			}
			numericPoint(t, db, i, 0, timestamp, 0, &value)
		}
		for _, reverse := range []bool{false, true} {
			reorderNumericPoints(t, db, reverse)
			for _, threads := range []int{1, 4} {
				_, err := db.Exec(fmt.Sprintf("set threads=%d", threads))
				require.NoError(t, err)
				end, views := uint64(999999), int64(1)
				if separateBuckets {
					end, views = 4e9, 5
				}
				full := numericView(t, db, ids, end, views, false, false)
				series := full["timeseries"].([]any)[0].(map[string]any)
				want := 1000.0 // chronological deltas add to 1, divided by 1ms
				if separateBuckets {
					want = 0.25 // [1e16, 1, -1e16, 1] in bucket timestamp order
				} else {
					require.Equal(t, want, series["views"].([]any)[0].(map[string]any)["rate"])
				}
				require.Equal(t, want, series["rateStats"].(map[string]any)["avg"])
				only := numericView(t, db, ids, end, views, true, false)
				require.Equal(t, full["scalarAggregate"], only["scalarAggregate"])
			}
		}
	}
}

func TestMetricNumericsNegativeDeltaProjection(t *testing.T) {
	db, ids := numericMetricDB(t, "Sum", 1)
	for i, value := range []int64{0, -9007199254740994} {
		numericPoint(t, db, i, 0, uint64(i)*1e9, 0, &value)
	}
	full := numericView(t, db, ids, 1e9, 2, false, false)
	series := full["timeseries"].([]any)[0].(map[string]any)
	latest := series["datapoints"].([]any)[0].(map[string]any)
	require.Equal(t, "-9007199254740994", latest["delta"])
	require.Equal(t, "-9007199254740994", latest["intValue"])
	require.Equal(t, false, latest["isReset"])
	views := series["views"].([]any)
	got := views[len(views)-1].(map[string]any)["rate"].(float64)
	require.Equal(t, math.Float64bits(-9007199254740994), math.Float64bits(got))
}

func TestMetricNumericsOrderedHistogramSums(t *testing.T) {
	for _, kind := range []string{"Histogram", "ExponentialHistogram"} {
		t.Run(kind, func(t *testing.T) {
			db, ids := numericMetricDB(t, kind, 4)
			for i, value := range []float64{1e16, 1, -1e16, 1} {
				numericPoint(t, db, i, 0, 100, value, nil)
			}
			for i, value := range []float64{1e16, -1e16, 1} {
				numericPoint(t, db, 10+i, i+1, 100, value, nil)
			}
			// Histogram rows have no scalar arms.
			_, err := db.Exec("update metric_records set datapoints=list_transform(datapoints,lambda d:struct_update(d,double_value:=null,value_type:=null))")
			require.NoError(t, err)
			var baseline map[string]any
			for _, reverse := range []bool{false, true} {
				reorderNumericPoints(t, db, reverse)
				for _, threads := range []int{1, 4} {
					_, err := db.Exec(fmt.Sprintf("set threads=%d", threads))
					require.NoError(t, err)
					full := numericView(t, db, ids, 999999, 0, false, true)
					only := numericView(t, db, ids, 999999, 0, true, true)
					require.Equal(t, full["aggregate"], only["aggregate"])
					if baseline != nil {
						require.Equal(t, baseline, full)
					}
					baseline = full
					aggregate := full["aggregate"].([]any)
					require.Len(t, aggregate, 1)
					require.Equal(t, float64(1), aggregate[0].(map[string]any)["sum"])
					for _, raw := range full["timeseries"].([]any) {
						series := raw.(map[string]any)
						if series["seriesRef"] == ids[0] {
							points := series["datapoints"].([]any)
							require.Len(t, points, 1)
							require.Equal(t, float64(1), points[0].(map[string]any)["sum"])
						}
					}
				}
			}
		})
	}
}

func TestMetricNumericsHistogramVectorMerges(t *testing.T) {
	for _, kind := range []string{"Histogram", "ExponentialHistogram"} {
		for _, tc := range []struct {
			name               string
			vectors            [3]string
			first, second, all []string
		}{
			{"unequal lengths and NULL slots", [3]string{"[1,NULL,3]", "[4]", "[NULL,2]"}, []string{"5", "0", "3"}, []string{"", "2"}, []string{"5", "2", "3"}},
			{"empty vectors", [3]string{"[]", "[]", "[]"}, []string{}, []string{}, []string{}},
			{"absent vectors", [3]string{"NULL", "NULL", "NULL"}, []string{}, nil, []string{}},
			{"exact totals above uint64", [3]string{"[18446744073709551615,1]", "[1]", "[2,3]"}, []string{"18446744073709551616", "1"}, []string{"2", "3"}, []string{"18446744073709551618", "4"}},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				db, ids := numericMetricDB(t, kind, 2)
				for i, vector := range tc.vectors {
					series := 0
					if i == 2 {
						series = 1
					}
					numericPoint(t, db, i, series, 100, 0, nil)
					column := "bucket_counts"
					bounds := ""
					if kind == "ExponentialHistogram" {
						column, bounds = "positive_bucket_counts", ", explicit_bounds:=null"
					}
					_, err := db.Exec("update metric_records set datapoints=list_transform(datapoints,lambda d:case when d.id=?::uuid then struct_update(d,double_value:=null,value_type:=null,"+column+":="+vector+"::ubigint[]"+bounds+") else d end)", numericID(1000+i))
					require.NoError(t, err)
				}
				raw, err := metrics.GetMetricView(t.Context(), db, numericID(3), store.BoundedTimeRange(0, 1000), 1, nil, nil, 0, 0, 0, nil, "", nil, 0)
				require.NoError(t, err)
				var result struct {
					Timeseries []struct {
						SeriesRef  string `json:"seriesRef"`
						Datapoints []struct {
							Counts   []string `json:"bucketCounts"`
							Positive []string `json:"positiveBucketCounts"`
						} `json:"datapoints"`
					} `json:"timeseries"`
					Aggregate []struct {
						Counts   []json.Number `json:"bucketCounts"`
						Positive []json.Number `json:"positiveBucketCounts"`
					} `json:"aggregate"`
				}
				require.NoError(t, json.Unmarshal(raw, &result))
				require.Len(t, result.Timeseries, 2)
				for _, series := range result.Timeseries {
					require.Len(t, series.Datapoints, 1)
					got := series.Datapoints[0].Counts
					if kind == "ExponentialHistogram" {
						got = series.Datapoints[0].Positive
					}
					want := tc.second
					if series.SeriesRef == ids[0] {
						want = tc.first
					}
					require.Equal(t, want, got)
				}
				require.Len(t, result.Aggregate, 1)
				got := result.Aggregate[0].Counts
				if kind == "ExponentialHistogram" {
					got = result.Aggregate[0].Positive
				}
				want := make([]json.Number, len(tc.all))
				for i, value := range tc.all {
					want[i] = json.Number(value)
				}
				require.Equal(t, want, got)
			})
		}
	}
}
