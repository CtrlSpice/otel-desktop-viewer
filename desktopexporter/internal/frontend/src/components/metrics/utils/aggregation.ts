// Datapoint math for the metrics chart. Pure functions, no Svelte
// imports, so the metric view context (a plain .ts module) can use them
// without dragging in component-side type resolution.
//
// Three layers in this file:
//   1. Types + shape predicates (what view to show, what's cumulative).
//   2. Building blocks: cumulative→delta conversion (with reset
//      detection) and chart-bucket math.
//   3. Per-view aggregation functions (raw / sum / avg / rate) that
//      take ChartTimeseries[] in and return ChartTimeseries[] out, and
//      per-overlay reductions (min / max) for the optional reference
//      lines.
//
// All aggregation/overlay functions are pure (no I/O, no globals).
// Callers must say up front whether their source is Cumulative or
// Delta temporality -- that determines whether cumulativeToDeltas runs
// first.

import type {
  ChartPoint,
  ChartTimeseries,
} from '@/types/metric-chart-types'

// --- 1. Types + shape predicates --------------------------------------

/**
 * How to render a multi-series numeric metric on the chart. Applies
 * to both Sum and Gauge metrics (Gauges only offer 'raw' and 'avg';
 * see {@link availableAggregationViews}). 'sum' here is the Σ
 * aggregation across series, not the metric type — those collide
 * naming-wise but the value 'sum' is genuinely about summation.
 */
export type AggregationView = 'raw' | 'sum' | 'avg' | 'rate'

/** Compact glyph beside aggregate labels (tooltip, etc.). */
export function aggregateViewSymbol(view: AggregationView): string | null {
  switch (view) {
    case 'sum':
      return 'Σ'
    case 'avg':
      return 'μ'
    case 'rate':
      // Δvalue/Δt; slash makes "per time" explicit vs bare Δ (resets).
      return 'Δ/t'
    default:
      return null
  }
}

/** Accessible name for {@link aggregateViewSymbol}. */
export function aggregateViewSymbolTitle(view: AggregationView): string {
  switch (view) {
    case 'sum':
      return 'Sum'
    case 'avg':
      return 'Average'
    case 'rate':
      return 'Rate'
    default:
      return ''
  }
}

/** Which pool an aggregate line covers. */
export type AggregateScope = 'checked' | 'all'

/** Compact label for aggregate lines: "μ · checked", "Σ · all", etc. */
export function aggregateScopeLabel(
  view: AggregationView,
  scope: AggregateScope
): string | null {
  const glyph = aggregateViewSymbol(view)
  if (!glyph) return null
  return `${glyph} · ${scope}`
}

/** Accessible description for aggregate scope labels. */
export function aggregateScopeLabelTitle(
  view: AggregationView,
  scope: AggregateScope
): string {
  const kind = aggregateViewSymbolTitle(view).toLowerCase()
  return scope === 'checked'
    ? `Show ${kind} across checked series`
    : `Show ${kind} across all series`
}

/** Optional horizontal reference lines overlaid on the chart. */
export type SumOverlay = 'min' | 'max' | 'avg' | 'total'

/** Per-series reset markers (indices into the OUTPUT points array). */
export type ResetIndicesByKey = Map<string, number[]>

export function isCumulativeTemporality(temporality: string): boolean {
  return temporality === 'Cumulative'
}

/**
 * Default view for a metric on first open (no persisted choice).
 *
 *   - Cumulative Sum metrics → 'rate'. The raw chart is a featureless
 *     climbing staircase; rate (Δvalue / Δt) is what an operator is
 *     almost always looking for. Applies regardless of monotonicity:
 *     non-monotonic cumulative counters are rare and still benefit
 *     from delta-per-second over a bare running total.
 *   - Everything else → 'raw'. Delta Sums, Gauges, and Sums of unknown
 *     temporality all look meaningful as-is, and the user can opt
 *     into aggregation from the menu.
 *
 * `_isMonotonic` and `_seriesCount` are kept in the signature even
 * though unused — callers already wire them, and they're cheap
 * future-proofing if we want to refine the rule later (e.g. only
 * default to Rate when seriesCount >= 2).
 */
export function defaultAggregationViewFor(
  metricType: string,
  temporality: string,
  _isMonotonic: boolean | null,
  _seriesCount: number = 2
): AggregationView {
  if (metricType === 'Sum' && isCumulativeTemporality(temporality)) {
    return 'rate'
  }
  return 'raw'
}

/**
 * Which AggregationView options the dropdown should offer for the
 * current metric. Rules (intersected):
 *
 *   - 'raw' is always available.
 *   - 'sum' / 'avg' require seriesCount >= 2: aggregating one series
 *     produces the same shape as raw (just bucketed), which is not
 *     useful enough to clutter the menu.
 *   - 'rate' requires the source to be cumulative-monotonic. This
 *     constraint is shape-driven, not count-driven: rate of a single
 *     cumulative-monotonic counter is the natural per-second view
 *     (climbing staircase -> spiky deltas) and still meaningful with
 *     one series.
 *   - Gauge: only 'raw' and 'avg'. 'sum' is omitted because summing
 *     scalars across series usually mixes apples and oranges
 *     (e.g. "sum of CPU%" has no clean interpretation); when sum-
 *     across-series *is* meaningful, the source should have been a
 *     Sum/Counter metric. 'rate' is omitted because Gauge isn't
 *     cumulative.
 */
export function availableAggregationViews(
  metricType: string,
  temporality: string,
  isMonotonic: boolean | null,
  seriesCount: number
): AggregationView[] {
  if (metricType !== 'Sum' && metricType !== 'Gauge') return ['raw']
  const out: AggregationView[] = ['raw']
  if (seriesCount >= 2) {
    if (metricType === 'Sum') out.push('sum')
    out.push('avg')
  }
  if (
    metricType === 'Sum' &&
    isCumulativeTemporality(temporality) &&
    isMonotonic === true
  ) {
    out.push('rate')
  }
  return out
}

// --- 2. Building blocks -----------------------------------------------

/**
 * Convert a cumulative-temporality series (each point = running total)
 * into per-interval deltas (each output point = increment since the
 * previous point). The first input point cannot produce an interval
 * -- there's no prior reference -- so the output has length N-1.
 *
 * Reset handling: when point[n].value < point[n-1].value on a
 * monotonic counter, the process restarted (container redeployed,
 * collector reset, etc.). The conventional fix (matching Prometheus's
 * rate()) is to treat the new value as the partial-interval increment
 * and flag the index so the UI can mark it. Without this, a counter
 * reset would render as a huge negative delta and tank the chart.
 *
 * `resets` contains indices into the OUTPUT array, not the input.
 */
export function cumulativeToDeltas(
  points: ChartPoint[]
): { points: ChartPoint[]; resets: number[] } {
  const out: ChartPoint[] = []
  const resets: number[] = []
  for (let i = 1; i < points.length; i++) {
    const prev = points[i - 1]!
    const cur = points[i]!
    let delta = cur.value - prev.value
    if (delta < 0) {
      // Counter reset between prev and cur. Assume `cur.value` is the
      // increment that accumulated since the reset (the prior counter
      // is gone -- best we can do without a fresher reference).
      delta = cur.value
      resets.push(out.length)
    }
    out.push({ date: cur.date, value: delta })
  }
  return { points: out, resets }
}

/**
 * Slice `points` into `bucketCount` equal-time buckets spanning the
 * series' first and last timestamps. Used by sum/avg/rate so the
 * chart shows one output point per visible bucket rather than one
 * per native datapoint -- this is what makes "Sum" mean "total in
 * this minute" rather than "total in this 100-ms interval".
 *
 * If `bucketCount` is undefined or >= input length, each input point
 * becomes its own bucket (identity bucketing). Empty buckets in the
 * middle of the range still appear in the output so the chart's x-
 * axis stays evenly spaced; the caller decides how to render empty
 * buckets (0 for sum/rate; skip for avg).
 *
 * Returns one array per bucket. Buckets are tagged with the bucket's
 * midpoint timestamp via the returned `bucketCenters`.
 */
export function bucketize(
  points: ChartPoint[],
  bucketCount: number | undefined
): { buckets: ChartPoint[][]; bucketCenters: Date[]; bucketSeconds: number } {
  if (points.length === 0) {
    return { buckets: [], bucketCenters: [], bucketSeconds: 0 }
  }
  if (!bucketCount || bucketCount >= points.length) {
    // Identity bucketing: each input point is its own bucket. Bucket
    // "seconds" is the average interval between consecutive points.
    const firstMs = points[0]!.date.getTime()
    const lastMs = points[points.length - 1]!.date.getTime()
    const span = lastMs - firstMs
    const avgIntervalMs =
      points.length > 1 ? span / (points.length - 1) : 0
    return {
      buckets: points.map(p => [p]),
      bucketCenters: points.map(p => p.date),
      bucketSeconds: avgIntervalMs / 1000,
    }
  }
  const startMs = points[0]!.date.getTime()
  const endMs = points[points.length - 1]!.date.getTime()
  const span = endMs - startMs
  const bucketMs = span / bucketCount
  const buckets: ChartPoint[][] = Array.from({ length: bucketCount }, () => [])
  for (const p of points) {
    let idx = Math.floor((p.date.getTime() - startMs) / bucketMs)
    if (idx === bucketCount) idx = bucketCount - 1 // last point edge
    buckets[idx]!.push(p)
  }
  const bucketCenters = Array.from(
    { length: bucketCount },
    (_, i) => new Date(startMs + (i + 0.5) * bucketMs)
  )
  return { buckets, bucketCenters, bucketSeconds: bucketMs / 1000 }
}

// Internal: project one timeseries' points to delta-space if
// `cumulative` is true, otherwise pass through. Centralizes the
// reset bookkeeping so every per-view aggregation handles cumulative
// sources identically.
function toWorkingPoints(
  points: ChartPoint[],
  cumulative: boolean
): { points: ChartPoint[]; resets: number[] } {
  if (!cumulative) return { points, resets: [] }
  return cumulativeToDeltas(points)
}

// --- 3. Per-view aggregation functions --------------------------------

/**
 * Options shared by every aggregation. `cumulative` says whether the
 * input is cumulative-temporality (triggers cumulativeToDeltas before
 * the view's math). `bucketCount` is the desired chart x-resolution
 * for bucketed views; raw passes it to its caller's downsampler
 * separately.
 */
export type AggregateOpts = {
  cumulative: boolean
  /** Target number of x-axis buckets for sum/avg/rate. Undefined ⇒
   *  one bucket per native point. */
  bucketCount?: number
}

/**
 * Raw view: emit the timeseries as-is. For cumulative input that
 * means the climbing line stays climbing; the user wanted to see the
 * source data. No reset detection (resets are visible as drops on the
 * raw chart already; flagging them only matters for derived views
 * where the math hides them).
 */
export function aggregateRaw(
  series: ChartTimeseries[],
  _opts: AggregateOpts
): { series: ChartTimeseries[]; resets: ResetIndicesByKey } {
  // Defensive copy: callers shouldn't mutate the input's points array.
  const out = series.map(s => ({ ...s, points: s.points.slice() }))
  return { series: out, resets: new Map() }
}

/**
 * Sum view: total value per bucket. For cumulative input we first
 * convert to per-interval deltas, then sum the deltas in each bucket
 * -- so "Sum" over a 1-minute bucket of a cumulative counter shows
 * "total events in that minute". For delta input we just sum the
 * deltas directly. Empty buckets render as 0 so the chart keeps an
 * even baseline.
 */
export function aggregateSum(
  series: ChartTimeseries[],
  opts: AggregateOpts
): { series: ChartTimeseries[]; resets: ResetIndicesByKey } {
  const resets: ResetIndicesByKey = new Map()
  const out: ChartTimeseries[] = []
  for (const s of series) {
    // Sum reduces raw values directly; only Rate needs deltas.
    // Cumulative input is already a running total — delta-converting
    // first would turn Sum into "events in this window," which is a
    // different (and not-what-Sum-means) metric.
    const work = { points: s.points, resets: [] as number[] }
    const { buckets, bucketCenters } = bucketize(work.points, opts.bucketCount)
    const points: ChartPoint[] = buckets.map((bucket, i) => {
      let total = 0
      for (const p of bucket) total += p.value
      return { date: bucketCenters[i]!, value: total }
    })
    out.push({ ...s, points })
    if (work.resets.length > 0) {
      // Map reset indices from delta-space to bucket-space: a reset
      // at delta index `r` shows up in the bucket that contains that
      // delta's timestamp. We approximate by translating each reset's
      // bucket from `buckets` directly (cheap; one O(buckets) walk).
      const bucketResets: number[] = []
      for (const r of work.resets) {
        const ts = work.points[r]!.date.getTime()
        for (let i = 0; i < bucketCenters.length; i++) {
          if (buckets[i]!.some(p => p.date.getTime() === ts)) {
            bucketResets.push(i)
            break
          }
        }
      }
      if (bucketResets.length > 0) resets.set(s.key, bucketResets)
    }
  }
  return { series: out, resets }
}

/**
 * Average view: arithmetic mean of values per bucket. Same cumulative-
 * conversion path as Sum. Empty buckets are skipped (NaN) rather than
 * forced to 0 -- "average of nothing is 0" would be misleading; the
 * caller's chart renderer should treat undefined as a gap.
 *
 * Concretely we use `value: 0` for empty buckets to keep the
 * ChartPoint type tight, but this is a known limitation -- if it
 * matters we can switch ChartPoint to allow nullable value later.
 */
export function aggregateAverage(
  series: ChartTimeseries[],
  opts: AggregateOpts
): { series: ChartTimeseries[]; resets: ResetIndicesByKey } {
  const resets: ResetIndicesByKey = new Map()
  const out: ChartTimeseries[] = []
  for (const s of series) {
    // Avg averages raw values directly; only Rate needs deltas.
    // For a cumulative counter, "average reading in the window" is
    // the mean of the running totals — the delta-then-average path
    // would compute mean per-scrape *delta*, which is essentially
    // rate × bucketSeconds and not what "Average" implies.
    const work = { points: s.points, resets: [] as number[] }
    const { buckets, bucketCenters } = bucketize(work.points, opts.bucketCount)
    // Empty buckets are skipped: the mean of zero samples is undefined,
    // and forcing it to 0 plants a misleading dive at trailing/middle gaps.
    const points: ChartPoint[] = []
    for (let i = 0; i < buckets.length; i++) {
      const bucket = buckets[i]!
      if (bucket.length === 0) continue
      let total = 0
      for (const p of bucket) total += p.value
      points.push({ date: bucketCenters[i]!, value: total / bucket.length })
    }
    out.push({ ...s, points })
    if (work.resets.length > 0) {
      const bucketResets: number[] = []
      for (const r of work.resets) {
        const ts = work.points[r]!.date.getTime()
        for (let i = 0; i < bucketCenters.length; i++) {
          if (buckets[i]!.some(p => p.date.getTime() === ts)) {
            bucketResets.push(i)
            break
          }
        }
      }
      if (bucketResets.length > 0) resets.set(s.key, bucketResets)
    }
  }
  return { series: out, resets }
}

/**
 * Rate view: per-second value. For cumulative monotonic counters
 * this is the headline view -- "requests per second", "errors per
 * second", etc. We compute (Σ deltas in bucket) ÷ (bucket seconds).
 * For delta input the formula collapses to (Σ values) ÷ (bucket
 * seconds), which is sane: deltas-per-second.
 *
 * Bucket seconds comes from bucketize(); for identity bucketing it's
 * the mean inter-point interval, which gives a per-point rate that
 * matches what users expect from Prometheus's irate().
 */
export function aggregateRate(
  series: ChartTimeseries[],
  opts: AggregateOpts
): { series: ChartTimeseries[]; resets: ResetIndicesByKey } {
  const resets: ResetIndicesByKey = new Map()
  const out: ChartTimeseries[] = []
  for (const s of series) {
    const work = toWorkingPoints(s.points, opts.cumulative)
    const {
      buckets,
      bucketCenters,
      bucketSeconds,
    } = bucketize(work.points, opts.bucketCount)
    const points: ChartPoint[] = buckets.map((bucket, i) => {
      if (bucketSeconds === 0) {
        return { date: bucketCenters[i]!, value: 0 }
      }
      let total = 0
      for (const p of bucket) total += p.value
      return { date: bucketCenters[i]!, value: total / bucketSeconds }
    })
    out.push({ ...s, points })
    if (work.resets.length > 0) {
      const bucketResets: number[] = []
      for (const r of work.resets) {
        const ts = work.points[r]!.date.getTime()
        for (let i = 0; i < bucketCenters.length; i++) {
          if (buckets[i]!.some(p => p.date.getTime() === ts)) {
            bucketResets.push(i)
            break
          }
        }
      }
      if (bucketResets.length > 0) resets.set(s.key, bucketResets)
    }
  }
  return { series: out, resets }
}

// --- 4. Cross-timeseries aggregation (Selected / Other / All) --------

/** Stable synthetic keys for the aggregate lines. */
export const AGG_KEY_SELECTED = '__agg:selected__'
export const AGG_KEY_ALL = '__agg:all__'
export const AGG_KEY_TOTAL = '__agg:total__'

export type AggregateLineKey =
  | typeof AGG_KEY_SELECTED
  | typeof AGG_KEY_ALL
  | typeof AGG_KEY_TOTAL

/** Checkbox label for the optional all-series aggregate toggle. */
export function aggregateAllToggleLabel(view: AggregationView): string {
  if (view === 'raw') return 'Show aggregate across all series'
  return aggregateScopeLabelTitle(view, 'all')
}

/** Label for a synthetic aggregate line key in tooltips / legend. */
export function aggregateLineLabel(
  key: AggregateLineKey,
  view: AggregationView
): string {
  switch (key) {
    case AGG_KEY_SELECTED:
    case AGG_KEY_TOTAL:
      return aggregateScopeLabel(view, 'checked') ?? 'checked'
    case AGG_KEY_ALL:
      return aggregateScopeLabel(view, 'all') ?? 'all'
  }
}

export type AggregateResult = {
  lines: ChartTimeseries[]
  /** Which aggregate keys are present (for legend rendering). */
  presentKeys: AggregateLineKey[]
}

/**
 * Combine multiple timeseries into up to two aggregate lines
 * (Selected, All).
 *
 * `selected` = the timeseries the user has checked (≤10).
 * `all`      = every timeseries on the metric.
 * `view`     = 'sum' | 'avg' | 'rate' (never 'raw'; caller gates).
 *
 * Collapse rules:
 *   - selected empty → single "All" line (nothing to compare against).
 *   - selected covers all → single "Total" line (Selected ≡ All; two
 *     identical lines would just stack).
 *   - otherwise → Selected + All (two lines).
 *
 * Each contributing timeseries is delta-converted once (when
 * cumulative), then points are flattened per pool, bucketed, and
 * reduced per the view. Reset bookkeeping is intentionally dropped
 * on the output — resets are a per-series concern that the aggregate
 * smears over.
 */
export function aggregateSelectedAndAll(
  selected: ChartTimeseries[],
  all: ChartTimeseries[],
  view: 'sum' | 'avg' | 'rate',
  opts: AggregateOpts
): AggregateResult {
  const isCollapsedAll = selected.length === 0
  const isCollapsedTotal = selected.length === all.length

  if (isCollapsedAll) {
    const line = combinePool(all, AGG_KEY_ALL, 'All', view, opts)
    return { lines: line ? [line] : [], presentKeys: [AGG_KEY_ALL] }
  }

  if (isCollapsedTotal) {
    const line = combinePool(all, AGG_KEY_TOTAL, 'Total', view, opts)
    return { lines: line ? [line] : [], presentKeys: [AGG_KEY_TOTAL] }
  }

  const lines: ChartTimeseries[] = []
  const presentKeys: AggregateLineKey[] = []

  const selLine = combinePool(selected, AGG_KEY_SELECTED, 'Selected', view, opts)
  if (selLine) { lines.push(selLine); presentKeys.push(AGG_KEY_SELECTED) }

  const allLine = combinePool(all, AGG_KEY_ALL, 'All', view, opts)
  if (allLine) { lines.push(allLine); presentKeys.push(AGG_KEY_ALL) }

  return { lines, presentKeys }
}

/**
 * Flatten a pool of timeseries into one combined aggregate line.
 * Steps:
 *   1. Delta-convert each timeseries independently (preserves per-
 *      series reset handling; resets are then discarded).
 *   2. Merge all points into a single array, sorted by date.
 *   3. Bucketize the merged stream with an ADAPTIVE bucket count.
 *   4. Reduce per bucket: sum, mean, or rate.
 *
 * Bucket count is driven by the POOL SIZE, not by `opts.bucketCount`.
 * We target ~poolSize points per bucket (one per series on average),
 * which is exactly the density needed for the bucket-and-sum to
 * actually combine sibling samples instead of leaving them isolated.
 *
 * The intuition: when N series sample at the same cadence (typical
 * for collector-scraped metrics), each bucket should be roughly
 * "one sampling moment wide" so it gathers the N siblings into a
 * single sum. Too many buckets (e.g. 120 across short data) →
 * adjacent series samples land in different buckets and the line
 * looks spiky / zero-interlaced. Too few → real temporal structure
 * gets blurred.
 *
 * Formula: target ≈ allPoints.length / poolSize, capped at the
 * caller's `opts.bucketCount` so we never exceed chart resolution.
 */
function combinePool(
  pool: ChartTimeseries[],
  key: string,
  label: string,
  view: 'sum' | 'avg' | 'rate',
  opts: AggregateOpts
): ChartTimeseries | null {
  if (pool.length === 0) return null

  // 1. Project to "working points." Only Rate needs deltas — Sum and
  //    Avg reduce raw values directly. For a cumulative counter,
  //    "Sum across series at time t" means adding the running totals;
  //    "Avg" means averaging them. Delta-converting first would turn
  //    Sum into "count of events in window" and Avg into "mean delta,"
  //    which are useful but are not what the labels say.
  const needsDeltas = view === 'rate' && opts.cumulative
  const allPoints: ChartPoint[] = []
  for (const s of pool) {
    const { points } = needsDeltas
      ? cumulativeToDeltas(s.points)
      : { points: s.points }
    for (const p of points) allPoints.push(p)
  }
  if (allPoints.length === 0) return null

  // 2. Sort merged points by time
  allPoints.sort((a, b) => a.date.getTime() - b.date.getTime())

  // 3. Bucketize with a pool-size-aware count (see header comment).
  const cap = opts.bucketCount ?? 120
  const target = Math.ceil(allPoints.length / Math.max(pool.length, 1))
  const adaptiveBucketCount = Math.min(cap, Math.max(1, target))
  const { buckets, bucketCenters, bucketSeconds } = bucketize(
    allPoints,
    adaptiveBucketCount
  )

  // 4. Reduce per bucket. Sum/Rate treat an empty bucket as 0 ("no
  // events in this window"); Average leaves it out, because "the mean
  // of zero samples" is undefined and forcing 0 makes the line dive
  // at trailing/middle gaps.
  const points: ChartPoint[] = []
  for (let i = 0; i < buckets.length; i++) {
    const bucket = buckets[i]!
    const center = bucketCenters[i]!

    if (bucket.length === 0) {
      if (view === 'avg') continue
      points.push({ date: center, value: 0 })
      continue
    }

    let total = 0
    for (const p of bucket) total += p.value

    switch (view) {
      case 'sum':
        points.push({ date: center, value: total })
        break
      case 'avg':
        points.push({ date: center, value: total / bucket.length })
        break
      case 'rate':
        points.push({
          date: center,
          value: bucketSeconds === 0 ? 0 : total / bucketSeconds,
        })
        break
    }
  }

  return { key, label, points }
}

/**
 * Resample one series onto fixed bucket-center dates so it shares the
 * same x-grid as cross-timeseries aggregate lines. Each window is the
 * half-open interval between adjacent center midpoints; we take the
 * **last** raw point in the window (typical scrape cadence).
 *
 * Used when raw + aggregate overlay is on — without this, layerchart
 * tooltips hunt between mismatched timestamps (native scrape vs bucket
 * centers) and flicker between per-series and totals rows.
 */
export function resampleSeriesToBucketCenters(
  series: ChartTimeseries,
  centers: readonly Date[]
): ChartTimeseries {
  if (centers.length === 0) return series
  const points = [...series.points].sort(
    (a, b) => a.date.getTime() - b.date.getTime()
  )
  const step =
    centers.length > 1
      ? (centers[centers.length - 1]!.getTime() - centers[0]!.getTime()) /
        (centers.length - 1)
      : 60_000

  const resampled: ChartPoint[] = centers.map((center, i) => {
    const lo =
      i === 0
        ? center.getTime() - step / 2
        : (centers[i - 1]!.getTime() + center.getTime()) / 2
    const hi =
      i === centers.length - 1
        ? center.getTime() + step / 2
        : (center.getTime() + centers[i + 1]!.getTime()) / 2
    let last: ChartPoint | undefined
    for (const p of points) {
      const t = p.date.getTime()
      if (t >= lo && t < hi) last = p
    }
    return { date: center, value: last?.value ?? 0 }
  })

  return { ...series, points: resampled }
}

// --- 5. Overlay reductions -------------------------------------------

/**
 * Lowest y-value across ALL visible series' points. Returns undefined
 * when the union is empty so the caller can decide not to draw the
 * line. Designed to be reusable for histogram charts later: they call
 * the same function on their own ChartTimeseries-shaped input.
 */
export function overlayMin(series: ChartTimeseries[]): number | undefined {
  let lo: number | undefined
  for (const s of series) {
    for (const p of s.points) {
      if (lo === undefined || p.value < lo) lo = p.value
    }
  }
  return lo
}

/**
 * Highest y-value across ALL visible series' points. Symmetric with
 * overlayMin; same reusability notes.
 */
export function overlayMax(series: ChartTimeseries[]): number | undefined {
  let hi: number | undefined
  for (const s of series) {
    for (const p of s.points) {
      if (hi === undefined || p.value > hi) hi = p.value
    }
  }
  return hi
}

/**
 * Arithmetic mean of all values across all visible series. Returns
 * undefined when the union is empty so the caller can decide not to
 * draw the line. Note this is the mean of POST-VIEW values: on a Sum
 * chart you're getting the mean of per-bucket sums; on a Rate chart
 * the mean of per-bucket rates. That's intentional -- the overlay
 * annotates whatever the chart is currently showing.
 */
export function overlayAverage(series: ChartTimeseries[]): number | undefined {
  let total = 0
  let count = 0
  for (const s of series) {
    for (const p of s.points) {
      total += p.value
      count++
    }
  }
  return count === 0 ? undefined : total / count
}

/**
 * Sum of all values across all visible series, post-view. On a Sum
 * chart this gives the grand total of all per-bucket sums (i.e. the
 * window total). On a Rate chart it sums rates, which is rarely
 * meaningful -- the UI should probably hide the Total overlay when
 * the view is Rate, but the math here stays neutral.
 */
export function overlayTotal(series: ChartTimeseries[]): number | undefined {
  let total = 0
  let count = 0
  for (const s of series) {
    for (const p of s.points) {
      total += p.value
      count++
    }
  }
  return count === 0 ? undefined : total
}
