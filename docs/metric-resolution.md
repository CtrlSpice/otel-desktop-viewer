# Metric resolution

`getMetricView` returns chart data for one Metric and time window. DuckDB
reduces the data before JSON encoding, so response size follows the requested
chart resolution rather than the number of stored datapoints.

The exact-data methods have a different contract:

- `getMetric` returns the stored Metric identity and its series catalogue.
- `getMetricSeries` returns retained received datapoints for one series and
  time window.
- `GetMetricOTLP` reconstructs OTLP JSON inside the store.

These methods do not return chart projections.

## Scalar series

Gauge and Sum charts use M4 reduction. Each absolute-time bucket can retain the
first, last, minimum, and maximum datapoint. Duplicate selections collapse to
one row.

Each retained point keeps its received timestamp and database-local datapoint
reference. The response also reports counts and statistics computed from the
full filtered window, not only the retained chart points.

Cumulative Sum views derive interval values from consecutive datapoints in one
series. A monotonic decrease is treated as a reset. Non-monotonic Sums keep the
signed difference. Rates divide the derived value by the elapsed seconds.

Integer calculations use exact integer arithmetic where the stored values and
result fit the supported domain. Values that require floating-point arithmetic
use `DOUBLE`. Chart coordinates are display values and do not replace received
measurements.

## Histograms

Histogram and ExponentialHistogram datapoints are merged within each time
bucket.

- Delta counts are added.
- Cumulative counts use the difference between consecutive readings.
- A negative cumulative difference is treated as a reset.
- Optional `sum`, `min`, and `max` preserve absence. Zero remains distinct from
  absence.

ExponentialHistogram merges align scale, offset, and zero threshold before
adding bucket vectors. Signed 128-bit intermediates preserve all uint64 inputs
and derived totals within the supported range. Overflow fails the query rather
than wrapping or clipping.

Quantiles are calculated from merged buckets for the requested probabilities.
They are display projections, not received OTel fields.

## Time buckets

Bucket boundaries are aligned to absolute time. Small changes to the requested
window therefore do not move existing bucket boundaries.

`getMetricView` reports both windows:

- `window.requested` contains the nullable bounds supplied by the caller.
- `window.effective` keeps concrete requested bounds and fills missing bounds
  from the filtered data extent.

An effective bound remains null when an empty result cannot supply it. Follow-up
aggregate requests use the effective bounds from the detail response.

## Series and aggregates

Each series has a database-local `seriesRef`. Public filters use `seriesRefs`,
`selectedSeriesRefs`, and `datapointSeriesRefs`.

Series filters control which series are returned. Datapoint-series filters
control which returned series include datapoint payloads. Series metadata,
statistics, chart views, and sparklines remain available when datapoints are
omitted.

Cross-series views use separate pools:

- `All` includes every series in the Metric.
- `Selected` includes the checked series.

`getMetricAggregateView` returns the aggregate envelope without resending the
per-series payload. Histogram aggregation narrows the merge to selected series.
Scalar aggregation keeps both pools so `All` retains its full meaning.

## Exemplars

The chart response limits exemplar payload size in two places:

- each datapoint includes at most 5 exemplars;
- each reduced bucket keeps at most 2 extra datapoints as exemplar carriers.

`exemplarCount` is present when a datapoint has more exemplars than the response
includes. Mixed integer and double exemplar values retain deterministic ordering.
Empty and non-finite values sort after finite values.

## Numeric transport

Received signed int64 measurements and unsigned uint64 counts cross JSON as
decimal strings. The frontend decodes them to `bigint`. Ordinary finite doubles
use JSON numbers. Negative zero and non-finite doubles use exact IEEE-754 bit
text and are decoded once by the frontend service.

Derived rates, quantiles, and chart coordinates use IEEE-754 numbers. They are
labelled and typed as computed display values.

## Frontend responsibility

The frontend does not recompute metric aggregates. It handles presentation:

- chart coordinates and axes;
- legend selection and colour assignment;
- histogram tabs and heatmap layout;
- selected datapoint and series state;
- URL and local preference state.

Changing the legend selection requests a new aggregate envelope. It does not
refetch the full Metric view.
