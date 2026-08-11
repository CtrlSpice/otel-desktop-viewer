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

### Histogram and ExponentialHistogram: merge, do not sample

These are not scalar series. The UI renders a heatmap, quantiles over time, or a
single distribution, and it currently does not downsample them at all — which is
why the largest stream in the corpus is the exponential histogram.

The natural reduction is **adding bucket counts within a time bucket**, and that
is exact: a merged histogram yields the same quantiles and the same heatmap
column as the individual ones. There is no fidelity trade here, only arithmetic.

The complication is ExponentialHistogram scale. Two histograms only merge
directly if they share a scale; otherwise the finer must be downscaled to the
coarser, which is lossless in the same way (adjacent buckets combine). Zero
counts and negative buckets have to travel with the merge rather than being
dropped.

## Stats must come from the server

`seriesStatsFromPoints` (components/metrics/utils/aggregation.ts) computes
min, max, avg and total over the **chart points** — that is, after downsampling:

```ts
for (const p of points) { …; sum += p.value }
return { min, max, avg: sum / points.length, total: sum }
```

Under M4, min and max survive exactly by construction. avg and total do not,
since M4 deliberately over-samples extremes.

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

## Open questions

- Default resolution when the client does not send one. 2,000 matches today's
  constant, but M4 emits up to 4 points per bucket, so the natural parameter is
  bucket count rather than point count.
- Whether `seriesCount` in `searchMetricSummaries` should keep meaning "series
  with data in this window" (a distinct count over datapoints) or become a count
  over `metric_series`. They agree on an unbounded window and diverge on a
  narrow one; this is a semantics decision, not an optimisation.
