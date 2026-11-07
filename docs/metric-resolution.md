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

### Gauge gaps

Raw Gauge lines detect empty M4 buckets separately for each series, using the
original samples before reduction. One or more empty buckets between samples
creates a break. A received zero still occupies its bucket.

Occupancy is checked across the series before assigning breaks to chronological
neighbours. During a daylight-saving fall-back, an earlier sample in a repeated
local bucket still counts as occupying it; moving back into that hour does not
itself create a break.

Both neighbouring real datapoints are retained alongside the usual M4 and
exemplar selections. The following chart point carries `breakBefore?: true`;
the property is omitted otherwise. JSON omits absent flags, and Arrow uses null.
Sparklines detect gaps independently on their own coarser grid, using original
samples before numeric filtering and min/max reduction. A short outage visible
in the main chart can disappear in this resolution-limited overview. Empty or
non-finite readings occupy their buckets but cannot be drawn; a detected break
is carried to the next finite sample, retaining the finite samples on both sides.

The renderer starts a new path at the following point. Openings have a minimum
width equal to the current line stroke, applied by clipping the drawing without
changing sample timestamps or coordinates. Real points have circular 6px hover
and click targets, including the gap-facing side of a boundary point. Farther
inside a gap, that series supplies no hover value or click target. Pointer checks
use drawing coordinates so fractional-millisecond positions are not rounded to
an endpoint. Boundary samples also remain keyboard-selectable.

Detection uses the requested M4 resolution, so changing the time window or
bucket target can change which gaps qualify. Disabled main-chart reduction and a
single whole-window M4 bucket do not detect main-chart gaps; sparkline detection
still follows its own grid. These breaks apply to raw Gauge lines
and their sparklines; aggregate views retain their bucket-based semantics.

## Histograms

Histogram and ExponentialHistogram datapoints are merged within each time
bucket.

- Delta counts are added.
- Cumulative counts use the difference between consecutive readings.
- The first cumulative reading in the requested window establishes the baseline;
  it is not an interval observation. One reading cannot supply an interval.
  Repeated unchanged readings produce zero interval activity even when their
  recorded cumulative counts are nonzero. Charts explain this empty state;
  received counts and buckets remain available in Details and exact downloads.
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

Gauge and Sum view datapoints include both `intValue` and `doubleValue`.
The inactive value is JSON `null`; an empty datapoint has both values null.

Derived rates, quantiles, and chart coordinates use IEEE-754 numbers. They are
labelled and typed as computed display values.

At rate and histogram-quantile arithmetic boundaries, signed 128-bit integers
convert to binary64 with round-to-nearest, ties-to-even, matching JavaScript
`Number(BigInt(value))`. Exact deltas and histogram counts retain their integer
representation until that boundary.

Floating-point sums and averages use a stable addition order: received timestamp,
then stored datapoint ID within each series; series ID when folding series; and
bucket timestamp for rate statistics. These remain ordinary `DOUBLE` sums and
averages, with their usual rounding, rather than compensated or decimal sums.

## Frontend responsibility

The frontend does not recompute metric aggregates. It handles presentation:

- chart coordinates and axes;
- legend selection and colour assignment;
- histogram tabs and heatmap layout;
- selected datapoint and series state;
- URL and local preference state.

Changing the legend selection requests a new aggregate envelope. It does not
refetch the full Metric view.
