# Metric charts

`getMetricView` returns chart data for one Metric and time window. DuckDB reduces
the data before encoding JSON or Arrow, so response size follows the requested
chart resolution rather than the number of stored datapoints.

The running viewer's [OpenRPC description](http://localhost:8000/openrpc.json)
lists JSON-RPC parameters and result schemas. The link uses the default UI port.

Exact-data methods serve a separate purpose: `getMetric` returns identity and
the series catalogue; `getMetricSeries` returns retained datapoints for one
series and time window. Store method `GetMetricOTLP` reconstructs retained
records as [OTLP JSON](telemetry-files.md), without chart projections.

## Gauge and Sum

M4 reduction keeps each time bucket's first, last, minimum and maximum
datapoints, dropping duplicate selections. Points retain their received
timestamps and database-local references. Counts and statistics cover the full
filtered window, including points omitted from the chart.

Cumulative Sums derive interval values from consecutive datapoints in a series.
A monotonic decrease is a reset; non-monotonic Sums keep the signed difference.
Rates divide the derived value by elapsed seconds. Integer calculations stay
exact within the supported integer ranges; floating-point operations use
DuckDB `DOUBLE`. Chart coordinates do not replace received measurements.

### Gauge gaps

Raw Gauge lines break when one or more M4 buckets between samples are empty.
Detection runs per series on the original samples; a received zero occupies its
bucket. Occupancy is checked before assigning breaks to chronological neighbours.

During a daylight-saving fall-back, an earlier sample in a repeated local bucket
still occupies it. During forward clock changes, fully skipped local intervals
are excluded; partly skipped intervals remain eligible. Local labels resolving
to the same UTC bucket count once.

Both points beside a gap survive reduction, alongside the usual M4 and exemplar
selections. The following point carries `breakBefore?: true`; otherwise JSON
omits the flag and Arrow uses null. Detection depends on the time window and M4
resolution. Disabled reduction and a single whole-window bucket do not detect
gaps. Aggregate views keep their own bucket-based behaviour.

The renderer starts a new path after a gap. It clips the line to leave an opening
at least one line-stroke wide without changing timestamps or coordinates.
Real points retain 6px circular hover/click targets, including their gap-facing
sides, and remain keyboard-selectable. A series supplies no hover value or click
target farther inside its gap. Pointer checks use drawing coordinates without
rounding fractional milliseconds to an endpoint.

Sparklines reuse the main chart's samples and gap flags. Each preview bucket
keeps at most two finite extrema with their real timestamps. A 96px preview
requests 48 equal-duration buckets across the effective window, for at most 96
points. Breaks survive numeric filtering, but previews add no extra boundary
points, so small outages can disappear. With main-chart reduction disabled,
previews simplify the unreduced samples without detecting gaps.

## Histograms

Histogram and ExponentialHistogram points merge within each time bucket:

- Delta counts are added.
- Cumulative counts use differences between consecutive readings. The first
  reading in the window establishes the baseline and supplies no interval.
  One reading cannot supply an interval; unchanged readings produce zero activity
  even with nonzero received counts. Charts explain this empty state, while
  Details and downloads retain the received counts and buckets.
- A negative cumulative difference is a reset.
- Optional `sum`, `min` and `max` retain the difference between absent and zero.

ExponentialHistogram merges align scale, offset and zero threshold before adding
bucket vectors. Signed 128-bit intermediates preserve all uint64 inputs and
totals within that range. Overflow fails the query instead of wrapping or clipping.

Quantiles are computed from merged buckets for the requested probabilities.
They are display values, not received OTel fields.

## Time buckets

Buckets align to absolute time, so small window changes do not move existing
boundaries. The response reports:

- `window.requested`: the caller's nullable bounds.
- `window.effective`: supplied bounds, with missing bounds filled from the
  filtered data extent.

An effective bound stays null when empty data cannot supply it. Follow-up
aggregate requests use the effective bounds.

## Series and aggregates

Each series has a database-local `seriesRef`. Filters use `seriesRefs`,
`selectedSeriesRefs` and `datapointSeriesRefs`. Series filters choose returned
series; datapoint-series filters choose which of those include point payloads.
Series metadata, statistics, chart views and sparklines remain available when
points are omitted.

Cross-series views have two pools: `All` contains every series in the Metric;
`Selected` contains checked series. `getMetricAggregateView` returns aggregate
data without resending per-series data. Histogram aggregation narrows the merge
to selected series. Scalar aggregation keeps both pools.

## Exemplars

Each chart datapoint includes at most five exemplars. Each reduced bucket keeps
at most two extra datapoints to carry exemplars. `exemplarCount` reports when a
point has more exemplars than the response includes. Mixed integer and double
values have deterministic ordering; empty and non-finite values sort after
finite values.

## Numeric precision

Browser responses use native Arrow columns. Signed int64 measurements and
unsigned uint64 counts become frontend `bigint` values; doubles retain their
IEEE-754 representation. JSON-RPC uses decimal strings for exact integers,
JSON numbers for ordinary finite doubles, and IEEE-754 bit text for negative
zero and non-finite doubles.

Gauge and Sum view datapoints contain `intValue` and `doubleValue`. The inactive
value is null; an empty point has both null. Derived rates, quantiles and chart
coordinates use IEEE-754 numbers and are labelled and typed as computed values.

At rate and histogram-quantile arithmetic boundaries, signed 128-bit integers
convert to binary64 with round-to-nearest, ties-to-even, matching JavaScript
`Number(BigInt(value))`. Exact deltas and histogram counts stay integers until
that boundary.

Floating-point sums and averages use a stable addition order: timestamp then
datapoint ID within a series, series ID when combining series, and bucket
timestamp for rate statistics. They use ordinary `DOUBLE` arithmetic and its
rounding, without compensated or decimal summation.

## Frontend

The frontend uses the server's aggregates and handles axes, coordinates,
colours, visible series, histogram/heatmap layout, selections, URLs and saved
preferences. Legend changes request new aggregate data without fetching the
full Metric view again.
