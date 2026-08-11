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

The machinery for this **already exists server-side and is unused** — macros
`downscale_exp_buckets`, `fold_below_cutoff`, `pad_left_to_offset` and
`sum_bucket_vectors` (ddl/macros/24-27) are referenced by no query. The client's
`mergeExpHistogramStreams` (histogram-merge.ts:140-206) is the semantics to
match.

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

Expected effect: 278 ms and tens of megabytes become a few milliseconds and a
few hundred kilobytes, and the cost scales with the chart rather than with the
retention window.

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

## Open questions

- Default resolution when the client does not send one. 2,000 matches today's
  constant, but M4 emits up to 4 points per bucket, so the natural parameter is
  bucket count rather than point count.
- Whether `seriesCount` in `searchMetricSummaries` should keep meaning "series
  with data in this window" (a distinct count over datapoints) or become a count
  over `metric_series`. They agree on an unbounded window and diverge on a
  narrow one; this is a semantics decision, not an optimisation.
