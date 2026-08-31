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
// Delta temporality -- that determines whether the store's deltas are read
// first.

import type { ChartPoint, ChartTimeseries } from '@/types/metric-chart-types'

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

/** Per-series reset markers (indices into the OUTPUT points array). */
export type ResetIndicesByKey = Map<string, number[]>

export function isCumulativeTemporality(temporality: string): boolean {
  return temporality === 'Cumulative'
}

/**
 * Default view for a metric on first open (no persisted choice).
 *
 *   - Cumulative *monotonic* Sum metrics → 'rate'. The raw chart is a
 *     featureless climbing staircase; rate (Δvalue / Δt) is what an
 *     operator is almost always looking for.
 *   - Everything else → 'raw'. Delta Sums, Gauges, and Sums of unknown
 *     temporality all look meaningful as-is, and the user can opt
 *     into aggregation from the menu.
 *
 * Monotonicity is part of the test, and must stay in step with
 * {@link availableAggregationViews}, which applies the same condition.
 * Rate differences consecutive readings and reads a fall as a counter
 * restart; a non-monotonic cumulative Sum is allowed to fall for real,
 * so every decrease would be reported as a reset and the rate would be
 * fiction.
 *
 * This tested temporality alone while the availability rule tested both,
 * so a non-monotonic cumulative Sum defaulted to a view its own menu did
 * not offer. Nothing rejected the mismatch: the tab bar rendered with no
 * tab active, and every row's sparkline drew nothing, because the rate of
 * a series' first bucket is null by definition and those series had only
 * the one bucket.
 *
 * `_seriesCount` is kept in the signature though unused — callers already
 * wire it, and it is cheap future-proofing if the rule ever needs it
 * (e.g. only default to Rate when seriesCount >= 2).
 */
export function defaultAggregationViewFor(
  metricType: string,
  temporality: string,
  isMonotonic: boolean | null,
  _seriesCount: number = 2
): AggregationView {
  if (
    metricType === 'Sum' &&
    isCumulativeTemporality(temporality) &&
    isMonotonic === true
  ) {
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

// --- 4. Cross-timeseries aggregation (Selected / Other / All) --------

/** Stable synthetic keys for the aggregate lines. */
export const AGG_KEY_SELECTED = '__agg:selected__'
export const AGG_KEY_ALL = '__agg:all__'
export const AGG_KEY_TOTAL = '__agg:total__'

export type AggregateLineKey =
  typeof AGG_KEY_SELECTED | typeof AGG_KEY_ALL | typeof AGG_KEY_TOTAL

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

/** Human scope name for aggregate summary lines, e.g. "selected series". */
export function aggregateSummaryScopeLabel(key: AggregateLineKey): string {
  switch (key) {
    case AGG_KEY_ALL:
      return 'all series'
    case AGG_KEY_SELECTED:
    case AGG_KEY_TOTAL:
      return 'selected series'
  }
}

/** Human-readable row label, e.g. "selected series μ". */
export function aggregateSummaryRowLabel(
  key: AggregateLineKey,
  view: AggregationView
): string {
  const scope = aggregateSummaryScopeLabel(key)
  const glyph = aggregateViewSymbol(view)
  return glyph ? `${scope} ${glyph}` : scope
}

export type AggregateSummaryRow = {
  key: AggregateLineKey
  /** Primary = selected/total aggregate; secondary = all-series line. */
  variant: 'primary' | 'secondary'
  label: string
  valueText: string
}

/** Build ordered aggregate summary rows for tooltip + selection legend. */
export function buildAggregateSummaryRows(
  keys: readonly AggregateLineKey[],
  view: AggregationView,
  valueAt: (key: AggregateLineKey) => number | undefined,
  formatValue: (value: number) => string
): AggregateSummaryRow[] {
  const rows: AggregateSummaryRow[] = []
  for (const key of keys) {
    const value = valueAt(key)
    if (value === undefined || !Number.isFinite(value)) continue
    rows.push({
      key,
      variant: key === AGG_KEY_ALL ? 'secondary' : 'primary',
      label: aggregateSummaryRowLabel(key, view),
      valueText: formatValue(value),
    })
  }
  return rows
}

/** Up to two cross-series lines, plus which keys they are, for the legend.
 *  The lines themselves are folded by the store; this names the shape the
 *  metric view context hands to the chart. */
export type AggregateResult = {
  lines: ChartTimeseries[]
  /** Which aggregate keys are present (for legend rendering). */
  presentKeys: AggregateLineKey[]
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

  const resampled: ChartPoint[] = []
  let lastWithData = -1
  for (let i = 0; i < centers.length; i++) {
    const center = centers[i]!
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
    if (last !== undefined) lastWithData = resampled.length
    // The source point moved to the center, not a fresh one: the store's
    // per-point fields (slope for the tangent, delta for resets) must survive
    // the move. The filler for an empty window is synthetic and carries none.
    resampled.push(
      last !== undefined
        ? { ...last, date: center }
        : { date: center, value: 0 }
    )
  }

  return {
    ...series,
    points: lastWithData >= 0 ? resampled.slice(0, lastWithData + 1) : [],
  }
}

// --- 5. Series stats -------------------------------------------------

/** Which stat badges to show on a single series row. */
export type SeriesStat = 'min' | 'max' | 'avg' | 'total'

export type SeriesStats = {
  min?: number
  max?: number
  avg?: number
  total?: number
}

/** Opinionated badge set for TimeseriesPanel rows (matches chart view). */
export function availableSeriesStatBadges(opts: {
  metricType: string
  temporality: string
  aggregationView: AggregationView
}): SeriesStat[] {
  const badges: SeriesStat[] = ['min', 'max']
  // Mean rate over the window is rarely actionable; chart uses slope at
  // selection instead. Other views keep avg.
  if (opts.aggregationView !== 'rate') {
    badges.push('avg')
  }
  // Window total: Sum + Delta + raw only — not Gauge, cumulative raw, or rate.
  if (
    opts.metricType === 'Sum' &&
    opts.temporality === 'Delta' &&
    opts.aggregationView === 'raw'
  ) {
    badges.push('total')
  }
  return badges
}

/** Compact label for rate-slope overlay chips. */
export function rateSlopeViewSymbol(): string {
  return 'slope'
}

/** Whether the chart should offer a rate-slope annotation at selection. */
export function availableRateSlopeOverlay(opts: {
  metricType: string
  temporality: string
  isMonotonic: boolean | null
  aggregationView: AggregationView
}): boolean {
  return (
    opts.metricType === 'Sum' &&
    isCumulativeTemporality(opts.temporality) &&
    opts.isMonotonic === true &&
    opts.aggregationView === 'rate'
  )
}

export type RateSlopeBucketSegment = {
  slope: number
  from: ChartPoint
  to: ChartPoint
}

/**
 * Slope segment ending at one exact selected chart point.
 */
export function rateSlopeBucketSegment(
  ratePoints: readonly ChartPoint[],
  selectedPoint: ChartPoint
): RateSlopeBucketSegment | undefined {
  if (ratePoints.length < 2) return undefined

  // Source identity is authoritative when present. Otherwise exact
  // nanoseconds identify the bucket; neither path falls back to milliseconds.
  const pointIdx =
    selectedPoint.sourceDatapointID !== undefined
      ? ratePoints.findIndex(
          point => point.sourceDatapointID === selectedPoint.sourceDatapointID
        )
      : selectedPoint.timestampNs !== undefined
        ? ratePoints.findIndex(
            point => point.timestampNs === selectedPoint.timestampNs
          )
        : ratePoints.indexOf(selectedPoint)
  if (pointIdx < 1) return undefined

  const from = ratePoints[pointIdx - 1]!
  const to = ratePoints[pointIdx]!
  const slope = to.slope
  if (slope === null || slope === undefined || !Number.isFinite(slope)) {
    return undefined
  }
  return { slope, from, to }
}
