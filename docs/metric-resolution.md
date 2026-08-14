# Server-side metric resolution

Status: proposal. Nothing here is implemented.

## The problem

`getMetric` returns every datapoint in the window. Measured on a corpus of the
2026 season to date (13 traces, 50,758 spans, 2,855,384 datapoints):

| metric type | datapoints | streams |
|---|---|---|
| Gauge | 2,584,185 | 18 |
| ExponentialHistogram | 200,791 | 1 |
| Histogram | 56,473 | 4 |
| Sum | 13,935 | 5 |

One `getMetric` call against the ExponentialHistogram stream takes **278.8 ms**
and returned **44 MB** in an earlier run. `EXPLAIN ANALYZE` puts 183 ms in a
projection and 122 ms in a scan, both over 200,791 rows — the work is
proportional to the answer, so this is not the per-row-versus-per-group mistake
found elsewhere in the store. The response simply contains everything.

The client then throws most of it away:

```ts
const CHART_POINTS_PER_SERIES = 2000        // metric-view-context.svelte.ts
points = downsampleLTTB(points, opts.downsampleTo)   // chart-projection.ts
```

So 200,791 datapoints are serialised, transferred and parsed to draw lines that
render at most 2,000 points each.

## Two different problems

Reduction is not one problem. Scalar series and histograms fail differently and
want different answers.

### Gauge and Sum: M4, not LTTB

The client uses LTTB (Steinarsson 2013), which picks real samples and preserves
visual shape. It is lossy: a spike survives only if its triangle is large
enough.

M4 — minimum, maximum, first and last per time bucket — has a stronger
property. For a chart of a given width, **the line drawn from M4 output is
identical to the line drawn from every point**, because the extremes of each
pixel column are always present. Nothing that would have been visible is
dropped.

It is also expressible in SQL, which LTTB is not: LTTB needs a running
"previously selected point" and triangle areas, which is why it lives in
TypeScript.

Each emitted point must carry **its own timestamp**, and the result must be
ordered by time rather than by value. For a monotonic series that is naturally
sorted; for a counter reset it is what puts the drop in the right place.

Across temporality and monotonicity:

- **Cumulative + monotonic** — `min` is `first` and `max` is `last` within a
  bucket, so M4 degenerates to two distinct points. No loss.
- **Counter reset mid-bucket** — `(…100, 0, 5…)` yields min 0 beside max 100,
  so the drop is always visible. This is a case LTTB can miss.
- **Cumulative + non-monotonic** (UpDownCounter) — ordinary envelope.
- **Delta** — values are per-interval and plotted raw, so the line is preserved.
  Summing *displayed* deltas is not the window total; see stats below.

Two edge cases the reduction has to handle explicitly:

- **NaN elects itself.** DuckDB orders NaN above infinity — verified:
  `max()` over `(10.0, NaN, 99.0)` returns NaN, and `arg_max` elects that row.
  So a single NaN sample would become its bucket's representative and displace a
  real maximum. Filter `isfinite(double_value)` in the M4 CTE.
- **`FLAG_NO_RECORDED_VALUE`** is never filtered anywhere today; such points
  chart as 0 via the `?? 0` fallback in chart-projection.ts. The server-side
  reduction is the right place to finally exclude them.
- **Multiple resets in one bucket** collapse to at most one visible reset, and
  the increments between hidden pairs are lost from the rate view. Bounded and
  honest — LTTB has the same class of problem — but it should be stated rather
  than discovered.

### Histogram and ExponentialHistogram: merge, do not sample

These are not scalar series. The UI renders a heatmap, quantiles over time, or a
single distribution, and it currently does not downsample them at all — which is
why the largest stream in the corpus is the exponential histogram.

The reduction is a **merge within a time bucket**, and it is exact — but the
merge is not the same operation for both temporalities, and an earlier draft of
this document got that wrong.

- **Delta**: add bucket counts. Each datapoint covers its own interval, so
  summing is correct.
- **Cumulative**: **last minus first**, with a reset clamp. Each datapoint is a
  running total, so adding them multiply-counts everything.

The client already knows this and is the reference implementation:
`mergeHistogramSliceCumulative` (histogram-aggregation.ts:338-355) sorts by
timestamp and subtracts, and `subtractHistogramSlices` returns the later slice
unchanged when subtraction would go negative, which is how a counter reset is
absorbed. A server implementation has to branch on temporality exactly as
`mergeSliceGroup` does, and port those fallback rules rather than reinvent them.

Min and max need the same care: for a cumulative bucket you cannot subtract
minima, so they are derived from the merged counts (`withBucketDerivedMinMax`)
or returned null.

For ExponentialHistogram, "rescale to the coarsest scale" is necessary but not
sufficient. Offset alignment and `zero_threshold` folding are equally
load-bearing: downscale everyone to the minimum scale, left-pad to the minimum
offset, sum element-wise, then fold buckets at or below the zero threshold into
`zeroCount`. Negative buckets travel symmetrically.

The machinery for this already exists server-side — macros
`downscale_exp_buckets`, `fold_below_cutoff`, `pad_left_to_offset` and
`sum_bucket_vectors` (ddl/macros/24-27). They are referenced by no query, but
they are **not untested**: `queries/schema_test.go:295-830` covers each one,
including odd and negative offsets. What is untested is their *composition* into
a merge.

Their arithmetic has been checked against the spec and against the TypeScript:
399 downscale combinations (offsets -9..9 x lengths 1..7 x levels 1..3) with no
mismatch, and 200 randomized multi-stream merges matching
`mergeExpHistogramStreams` exactly on scale, threshold, zero count, both offsets
and both arrays.

**But they are not the right implementation for the hot path.**
`downscale_exp_buckets` is O(buckets^2) — for each output bucket it re-zips and
re-filters the whole input list, and the planner does not hoist the `list_zip`
out of the lambda. Measured at 2,000 rows:

	buckets/row    time
	20             0.015 s
	40             0.055 s
	80             0.200 s
	160            0.717 s

Extrapolated to the 200,791-datapoint stream that is ~43 s for a single
downscale pass, against ~0.6 s for the same work expressed relationally
(`unnest` -> `group by floor((offset+p)/2^k)` -> `list(c order by k)`).

So: build the merge relationally, keep the cheap scalar macros, and treat 24/26/
27 as the **verified oracle** that a differential test checks the relational
query against. The relational form needs a `generate_series` left join to
gap-fill, since `list(c order by k)` is only dense when every coarse bucket in
range receives input.

Three pieces do not exist yet and are needed either way: the zero cutoff
`floor(log2(T) * 2^scale) - 1` (currently an inline expression in
`histogram-merge.ts:129-137`, and the single most dangerous line to leave
uncodified), bucket-derived min/max, and a `diff_bucket_vectors` for the
cumulative path.

Two defects to fix before any of this ships:

- **`fold_below_cutoff` contains a subquery** (25_...sql:21-31), which macro 24's
  own comment warns against. It binds today but hard-fails inside a lambda:
  `subqueries in lambda expressions are not supported`. A subquery-free rewrite
  is byte-identical across 169 offset/cutoff pairs.
- **An empty bucket array returns its offset un-rescaled**, in both the SQL
  (24_...sql:30-31) and the TypeScript (histogram-merge.ts:40-42) — verified:
  `downscale_exp_buckets([], -7, 5)` returns offset -7 where a non-empty array
  returns -1. That un-rescaled offset then wins `min(...)` and pads the result
  with zeros out to it. Harmless numerically, unbounded in memory: a stale
  high-scale offset can materialise a multi-million-element array.

One thing not to port: the client's *within-series* merge
(`mergeHistogramSliceDelta`, histogram-aggregation.ts:300-315) takes
`expDps[0].scale` and offset without rescaling, and `sumBucketVectors` zero-pads
by length rather than by offset. SDKs legitimately change scale mid-stream as
the value range grows, so scale and offset can drift *within* one series. Moving
this server-side is the chance to run the full align path in both cases.

## Stats must come from the server

`seriesStatsFromPoints` (components/metrics/utils/aggregation.ts) computes
min, max, avg and total over the **chart points** — that is, after downsampling:

```ts
for (const p of points) { …; sum += p.value }
return { min, max, avg: sum / points.length, total: sum }
```

Under M4, min and max survive exactly by construction. avg and total do not,
and the reason is sharper than "sampling error": M4's output is **50% extremes
by construction**, so anything averaging or summing the retained points is
systematically extreme-biased rather than merely noisy. That applies to
`seriesStatsFromPoints` and to the client's aggregate views
(`aggregateSum`/`aggregateAverage`/`combinePool`).

The fix is cheap and belongs in the same GROUP BY: **return per-bucket `count`
and `sum` alongside the four M4 points**. Client-side averages and sums then
become exact rather than sampled. Rate view is already exact under M4, because
deltas of a cumulative counter telescope across retained points.

**They are already wrong today.** Those numbers are computed over ~2,000
LTTB-selected points out of, for a typical gauge stream, ~143,000 — so the
displayed average is the mean of an arbitrary subsample, and
`availableSeriesStatBadges` offers `total` for Sum + Delta + raw, which is a
straightforwardly incorrect number.

So the server should return per-series `min / max / avg / count / total`
computed over every datapoint in the window, alongside the reduced points. The
overlays then read exact values at any resolution, and an existing bug is
repaired rather than a new one introduced.

Histograms are unaffected: `metric-view-context.svelte.ts` returns an empty map
for `isHistogramKind`, so they never reach this path.

## Nothing is dropped silently

Three properties together, which is what makes the reduction defensible rather
than convenient:

1. **The chart is provably the same.** M4 for scalars, exact merges for
   histograms.
2. **The response says what it did** — true `datapointCount` beside the returned
   resolution, so the UI can state "showing 2,000 of 200,791".
3. **Zoom returns exact data, with no special case.** Narrow the window until it
   holds fewer datapoints than the budget and every one is returned. The
   detail panel already fetches datapoints by id, so exact values stay
   reachable regardless.

## Shape of the change

- `getMetric(streamID, start, end, resolution)` — the client knows its chart
  width; the server cannot.
- Reduction happens in SQL, in `queries/metrics/get_metric.sql`.
- `downsampleLTTB` stays for the case where the server returns full fidelity and
  the client still wants to thin.
- Wire response gains `datapointCount`, `resolution`, and per-series stats.

Expected effect, stated separately because the two paths differ:

- **Gauge/Sum**: 278 ms and tens of megabytes become a few milliseconds and a few
  hundred kilobytes. M4 is min/max/first/last — cheap aggregates over a scan.
- **Histograms**: the response shrinks by ~100x and the client stops doing the
  merge, but **the query does not get faster**. A merge must touch every input
  bucket of every datapoint, so it is Theta(datapoints x buckets) however it is
  written; the measured floor for the 200,791-datapoint stream is ~0.6 s. The
  win is transfer, JSON parsing and client CPU, not query time.

Worth knowing before optimising for it: in the reference corpus that stream has
**exactly one distinct scale, offset and zero_threshold** across all 200,791
datapoints. So the rescale path never fires on this data, and its no-op branch
is free (0.004 s). Scale drift is a correctness requirement, not a hot path —
which is an argument for correctness first and relational rewriting only if a
real workload proves it necessary.

## Every window change is a new query

Reduction moves the data the client no longer has, so panning and zooming stop
being local operations. Today the client holds every datapoint for the fetched
window and re-runs LTTB itself; afterwards each window change is a round trip.
That is affordable only because the query gets cheap, which is the same trade
the wire rewrite made for traces.

**Bucket boundaries must be absolute, not window-relative.** If buckets were
`start + i x width`, nudging the window by a second would shift every boundary,
re-elect every M4 point, and make the chart shimmer while panning — points
moving for no reason other than that the request changed.

The histogram path already gets this right and M4 should reuse it rather than
invent a second scheme:

```ts
histogramBucketStart: (timestampNs / bucketNs) * bucketNs   // epoch-aligned
```

and `BUCKET_LADDER` is chosen to preserve it — the sub-second rungs "divide a
second evenly, so they keep the stable-boundary property the rest of the ladder
has".

What that buys:

- panning slides data through fixed buckets, so points enter and leave rather
  than move
- only crossing a ladder rung changes resolution, which is a deliberate visible
  step instead of continuous churn
- the same (series, width, bucket) always produces the same answer, so responses
  are deterministic, comparable across requests, and cacheable if that ever
  matters

The rest is ordinary interaction work: debounce during a drag, fetch a little
wider than the visible window so small pans do not refetch, and keep rendering
current data while the new response lands.

## Exemplars are the thing that breaks

M4 elects points by *value*, and exemplars hang off individual datapoint ids
(`get_metric.sql`, exemplars_agg). So the datapoints carrying exemplars are
mostly not the ones M4 retains, and the exemplar badges and trace links in
`SeriesDatapointList.svelte` would quietly empty out — on exactly the dense
streams this reduction targets, and taking trace correlation with them.

Exemplars are sparse. Return **all** of them for the window, keyed by series,
joining exemplars to datapoints over the window rather than over the retained
ids. This has to land with M4, not after it.

Other frontend surfaces degrade acceptably, largely because #295 already
designed for datapoints going missing under retention:

- `?dp=` validates against payload ids and nulls out when absent, and the
  `series` param fallback still lands the user on the right line. Keeping real
  datapoint ids on M4-retained rows means surviving links keep working.
- Chart click selection resolves by nearest timestamp over the payload, so
  clickable points and payload points coincide by construction — better than
  today, where the click resolves against the full list the client then thins.
- The heatmap keys selection by bucket timestamp rather than datapoint id.
- `totalDatapointCount` and `histogramTimeseriesGroups.pointCount` count the
  payload and would under-report; both need wiring to the new `datapointCount`.

## Order to build it

Each step ships on its own:

1. **Per-series stats and true `datapointCount`.** Smallest change, fixes the
   existing `total` bug, no reduction semantics involved, and the "showing X of
   Y" UI needs it regardless.
2. **M4 for Gauge/Sum**, with per-bucket `count`/`sum`, `isfinite` filtering and
   dedup of the up-to-four elected rows. Client LTTB stays: it is a no-op
   passthrough when the input already fits, so it costs nothing and catches
   anything missed.
3. **Exemplar re-attachment**, in the same release as 2.
4. **Histogram and ExponentialHistogram merge**, temporality-branched, using the
   existing unused macros. Biggest win — the 200k-point stream is an
   ExponentialHistogram — and the most intricate, hence last.

Not worth doing: LTTB in SQL (recursive-CTE contortion for a weaker guarantee
than M4); moving the rate/sum/avg *chart views* server-side (with per-bucket
count and sum the client computes them exactly, and that logic is deeply
UI-coupled); sampling histograms; assuming scale and `zero_threshold` are
uniform within a stream.

## The gap that matters: cumulative histograms

Delta histograms merge. Cumulative ones do not, and that is the default.

The OTLP metrics exporter specification requires implementations to "set
temporality preference to Cumulative for all instrument kinds by default";
Delta is opt-in through `OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE`.
So a service instrumented with stock OpenTelemetry emits cumulative histograms,
and gets none of this reduction. The reference corpus is Delta only because
bargeboard configures it that way, which is why the implementation could be
verified at all -- and also why the gap was easy to miss.

The merge itself is understood: last-minus-first within the bucket, with a
clamp that falls back to the later slice when any bucket would go negative,
because that means the counter restarted. `diff_bucket_vectors` exists for it
and returns NULL as the reset signal. `subtractHistogramSlices` in the client is
the reference, now that it aligns scales rather than bailing on a mismatch.

### Getting cumulative data to test against

bargeboard emits Delta deliberately, and says why: "Cumulative histograms would
put every observation since lights-out in every column, which is why the heatmap
looked identically wide the whole race." That is a product decision about the
demo, not a limitation, so it should not be changed to suit a test.

It does not need to be. **Cumulative is the running sum of deltas**, so real
cumulative fixtures can be derived from the captures already taken -- same
observations, same distributions, cumulative semantics:

```sql
with flat as (
  select id, series_id, timestamp, explicit_bounds,
         generate_subscripts(bucket_counts, 1) as k, unnest(bucket_counts) as c
  from datapoints d join metric_streams s on s.id = d.stream_id
  where s.name = 'f1.driver.lap_time'
),
running as (
  select id, series_id, timestamp, explicit_bounds, k,
         sum(c) over (partition by series_id, k order by timestamp
                      rows between unbounded preceding and current row) as cum
  from flat
)
select id, series_id, timestamp, any_value(explicit_bounds) as explicit_bounds,
       list(cum order by k) as bucket_counts, sum(cum) as count
from running group by id, series_id, timestamp
```

On the season capture that yields 12,333 cumulative datapoints across 233
series, with real lap-time distributions rather than invented ones. The five
instruments that would qualify in the first place are `lap_time`, `sector_time`,
`pit_duration`, `top_speed` and `interval_distribution`.

It also gives a property worth asserting: merging a whole window of the derived
cumulative data must reproduce the sum of the original deltas over that range.
Same observations, two routes, one answer.

### The harness exists

`queries/cumulative_merge_test.go` drives randomized cumulative pairs through
the SQL macros and an independent Go implementation of the same rules, written
from the specification rather than transcribed from the SQL so that a shared
misreading is the only way both can be wrong together. Mutation-checked: swapping
the subtraction operands and dropping the reset check are both caught.

One thing it taught immediately. The first generator built `last` independently
of `first`, which made 238 of 300 cases counter resets -- the clamp was
exhaustively tested and the subtraction barely at all. Building `last` from
`first` by adding non-negative increments, the way a counter actually behaves,
moved that to 53 resets and 247 real subtractions.

What is still missing is *evidence for the implementation*. There is no cumulative histogram in the corpus to
check an implementation against, and a histogram merge that is subtly wrong
produces plausible quantiles rather than an error. The honest next step is to
synthesize cumulative data -- the same technique that exposed the
high-cardinality dictionary and the exemplar problem -- and differential-test
the SQL against the TypeScript before trusting it.

Until then histograms with cumulative temporality return every datapoint. That
is correct and slow, which is the right way round, but it should not be mistaken
for finished.

## Open questions

- Default resolution when the client does not send one. 2,000 matches today's
  constant, but M4 emits up to 4 points per bucket, so the natural parameter is
  bucket count rather than point count.
- Whether `seriesCount` in `searchMetricSummaries` should keep meaning "series
  with data in this window" (a distinct count over datapoints) or become a count
  over `metric_series`. They agree on an unbounded window and diverge on a
  narrow one; this is a semantics decision, not an optimisation.
