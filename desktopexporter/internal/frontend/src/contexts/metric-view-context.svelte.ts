/*
 * MetricViewContext: per-metric reactive state that BOTH the chart
 * view (main) and the detail view (Fields/Series) need to read
 * and, in some cases, write. Modeled on time-context.svelte.ts:
 * the page calls `createMetricViewContext(...)` once at mount,
 * children call `getMetricViewContext()` to read derivations and
 * invoke methods.
 *
 * Design:
 *   - One `$state` cell holds the only mutable per-metric values
 *     (selection, expansion, active histogram tab, legend visibility).
 *   - Everything else is `$derived` from (metric, that cell, time
 *     window). No second source of truth.
 *   - Histogram heatmap / summary are derived client-side from the same
 *     getMetric payload as Gauge/Sum (no bucket-series RPC).
 *   - `$effect` is used for: (1) reset per-metric view state when the
 *     metric identity changes; (2) seed / reconcile legend visibility.
 *     Everywhere else is pure derivation.
 *
 * The factory takes a getter for the current `metric` rather than a
 * value, so the context object's identity stays stable for the page
 * lifetime even as the user navigates between metrics. (The
 * underlying `selectedMetric` cell lives on MetricsPage.)
 */
import type { JsonAggregateBucket } from '@/types/wire-types'
import { setContext, getContext, untrack } from 'svelte'
import { SvelteSet } from 'svelte/reactivity'
import type {
  MetricData,
  MetricTimeseries,
  MetricType,
  DataPoint,
  HistogramDataPoint,
  ExponentialHistogramDataPoint,
  Attributes,
  ScalarViewBucket,
  ScalarAggregate,
  SparklinePoint,
} from '@/types/api-types'
import { timeseriesToChartTimeseries } from '@/components/metrics/utils/chart-projection'
import {
  distinguishingResourceAttributes,
  seriesLabelsByKey,
} from '@/utils/series-labels'
import {
  seriesBucketsToSlices,
  buildVisibleSeriesQuantileChartTimeseries,
  DEFAULT_ACTIVE_HISTOGRAM_QUANTILE_KEY,
  DEFAULT_HISTOGRAM_QUANTILES,
  histogramSliceToDatapoint,
  isHistogramAggregationError,
  parseQuantileSeriesKey,
  quantileKeyFromValue,
  type HistogramAggregationError,
  type HistogramSlicePoint,
} from '@/components/metrics/utils/histogram-aggregation'
import {
  heatmapColumnSelectionAt,
  type HeatmapColumnSelection,
} from '@/components/metrics/utils/heatmap-column-selection'
import {
  quantilePointSelectionAt,
  type QuantilePointSelection,
} from '@/components/metrics/utils/quantile-point-selection'
import {
  AGG_KEY_ALL,
  AGG_KEY_SELECTED,
  AGG_KEY_TOTAL,
  availableAggregationViews,
  defaultAggregationViewFor,
  isCumulativeTemporality,
  availableSeriesStatBadges,
  availableRateSlopeOverlay,
  rateSlopeAtPoint,
  seriesStatsFromPoints,
  resampleSeriesToBucketCenters,
  type AggregateLineKey,
  type AggregateResult,
  type AggregationView,
  type ResetIndicesByKey,
  type SeriesStat,
  type SeriesStats,
} from '@/components/metrics/utils/aggregation'
import type {
  ChartPoint,
  ChartTimeseries,
  LegendTimeseries,
} from '@/types/metric-chart-types'
import {
  AGG_COLOR_ALL,
  AGG_COLOR_SELECTED,
  categoricalPalette,
} from '@/utils/chart-palette'
import { metricTypeStem } from '@/components/metrics/utils/metric-type'
import { themeSignal } from '@/state/theme.svelte'
import {
  DEFAULT_VISIBLE_TIMESERIES,
  MAX_VISIBLE_TIMESERIES,
  loadPersistedAggregationView,
  loadPersistedShowAllSeriesAggregate,
  reconcileTimeseriesVisible,
  resolveTimeseriesVisible,
  savePersistedAggregationView,
  savePersistedShowAllSeriesAggregate,
  savePersistedTimeseriesVisible,
  visibleKeyListsEqual,
} from '@/components/metrics/utils/metric-timeseries-visible'
import {
  acquireColor,
  releaseColor,
  seedColorAssignments,
  syncColorAssignments,
  type TimeseriesColorByKey,
} from '@/components/metrics/utils/metric-timeseries-colors'
import {
  getTimeContext,
  isDefaultUnboundedWindow,
  selectionToQueryRangeMs,
} from '@/contexts/time-context.svelte'
import {
  metricViewQueriesEqual,
  parseMetricViewQuery,
  readRoute,
  setMetricViewQuery,
  subscribeToRoute,
  type HistoryMode,
  type MetricViewParseContext,
  type MetricViewQuery,
} from '@/route'

const KEY = 'metric-view'

// How many points per series the line chart is willing to draw.
const CHART_POINTS_PER_SERIES = 2000

/**
 * How many time buckets to ask the store to reduce a window to.
 *
 * Buckets, not points -- which is the distinction this constant used to lose.
 * It was defined as CHART_POINTS_PER_SERIES, but the store's M4 election keeps
 * up to four points per bucket (first, last, min, max), so asking for 2,000
 * buckets shipped up to 8,000 points per series and the client then ran LTTB to
 * compress them back down to 2,000.
 *
 * That second pass was not merely wasted work. M4 elects those four points
 * precisely so the drawn line is identical to the line through every datapoint;
 * LTTB picks the visually significant ones and drops the rest, including the
 * extremes M4 kept on purpose. The compression undid the guarantee the
 * reduction existed to provide.
 *
 * One reduction, in the store, sized so its output is what the chart draws.
 */
export const METRIC_BUCKET_TARGET = CHART_POINTS_PER_SERIES / 4

/**
 * How many buckets the Sum / Average / Rate views aggregate onto.
 *
 * A chart-resolution number, and deliberately not METRIC_BUCKET_TARGET: the
 * election thins a line while keeping its shape, these bucket it for a
 * different chart. The store's ladder rounds this down to a nameable width, so
 * it is a ceiling rather than an exact count.
 */
export const SCALAR_VIEW_BUCKETS = 120

/**
 * How wide a row sparkline is drawn, in px.
 *
 * Exported so the reduction and the box it is drawn into cannot drift apart:
 * TimeseriesPanel sizes the SVG from this, and SPARKLINE_BUCKETS is derived
 * from it below.
 */
export const SPARKLINE_WIDTH_PX = 128

/**
 * How many buckets to reduce a series to for its row sparkline.
 *
 * Half the pixel width, because the store keeps each bucket's min and its max
 * -- two points per bucket, so the row receives about one point per pixel.
 *
 * A third resolution, and deliberately neither of the other two. The row used
 * to be handed the charting points: up to METRIC_BUCKET_TARGET * 4 of them, a
 * budget sized for a chart hundreds of pixels wide, drawn into this box at
 * roughly fifteen points per pixel, for every series in the panel.
 */
export const SPARKLINE_BUCKETS = SPARKLINE_WIDTH_PX / 2

// --- Types --------------------------------------------------------

export type HistogramTab = 'heatmap' | 'quantiles' | 'histogram'

export type HistogramScope = 'window' | 'bucket'

export type BucketSeriesError =
  | { kind: 'unspecified'; message: string }
  | { kind: 'boundsMismatch'; message: string }
  | { kind: 'other'; message: string }

type HistogramTimeseriesGroup = {
  key: string
  attributes: Attributes
  pointCount: number
}

export interface MetricViewContext {
  // -- Metric identity / shape --
  readonly metric: MetricData | undefined
  readonly metricType: MetricType
  readonly temporality: string
  readonly isMonotonic: boolean | null
  readonly isHistogramKind: boolean
  readonly isUnspecifiedTemporality: boolean
  readonly totalDatapointCount: number

  // -- Selection / view state --
  readonly selectedDatapointId: string | null
  /** Where the current datapoint selection came from. Chart clicks
   *  drive the plot overlay only; detail-pane scroll/expand waits for
   *  unified routing. */
  readonly selectionSource: 'chart' | 'detail' | null
  readonly expandedDatapoints: SvelteSet<string>
  /** Per-timeseries expansion (keyed by attributesKey). Used by the
   * TimeseriesPanel to reveal an inline datapoints table under a
   * row. Independent of expandedDatapoints (which keys per-datapoint
   * exemplar expansion within SeriesDatapointList). */
  readonly expandedTimeseries: SvelteSet<string>
  readonly activeHistogramTab: HistogramTab
  readonly histogramScope: HistogramScope
  readonly selectedDatapoint: DataPoint | undefined

  // -- Gauge/Sum chart wiring --
  /** Series keys the metric carries, in store order. Deliberately keys rather
   *  than projected points: the only question asked of it is whether the metric
   *  has any scalar series at all, and projecting every one of them -- checked
   *  or not -- would be a lot of work for a boolean. */
  readonly gaugeSumSeriesKeys: readonly string[]
  readonly gaugeSumLegendTimeseries: LegendTimeseries[]
  readonly gaugeSumVisible: SvelteSet<string>
  readonly highlightedTimestamp: bigint | null
  /** Attributes key of the timeseries owning `selectedDatapoint`, or
   *  `null` when nothing is selected. Used by the chart to draw the
   *  selection dot in the right series's color. */
  readonly selectedSeriesKey: string | null

  // -- Sum-view wiring (Sum metrics only; Gauge ignores these) --
  /** Current view selection: 'raw' | 'sum' | 'avg' | 'rate'. */
  readonly aggregationView: AggregationView
  /** Which view options the dropdown should offer (varies by metric
   *  type, temporality, monotonicity, and series count). Always
   *  contains at least 'raw'. */
  readonly availableAggregationViews: AggregationView[]
  /** Post-view series the chart actually plots. Raw mode: per-series
   *  visibility-filtered lines. Aggregated modes: raw lines plus up to
   *  two cross-timeseries lines (Selected, All). */
  readonly transformedGaugeSumChartTimeseries: ChartTimeseries[]
  /** Which aggregate line keys are present (for legend rendering).
   *  Empty when aggregationView === 'raw'. */
  readonly aggregatePresentKeys: AggregateLineKey[]
  /** Whether the optional all-series aggregate line is shown. */
  readonly showAllSeriesAggregate: boolean
  /** Show the all-series aggregate toggle in the chart control bar. */
  readonly showAllSeriesAggregateToggleVisible: boolean
  /** Whether min / max / avg selection overlays render on the chart. */
  readonly showSelectionStatOverlays: boolean
  /** Show the stat overlay toggle in the chart control bar. */
  readonly showChartStatOverlaysToggleVisible: boolean
  /** Sum + cumulative + monotonic + rate: offer rate slope at selection. */
  readonly rateSlopeOverlayAvailable: boolean
  /** Rate slope (Δrate/Δt) at the selected bucket, or undefined. */
  readonly selectedRateSlope: number | undefined
  /** Start/end of data plotted in the chart (gauge/sum points or histogram window). */
  readonly chartDataTimeRange: { startMs: number; endMs: number } | undefined
  /** Reset markers per series, indexed into the transformed (visible)
   * series' output points. Only populated in raw mode. */
  readonly sumResetIndicesByKey: ResetIndicesByKey

  // -- Histogram chart wiring --
  readonly histogramLegendTimeseries: LegendTimeseries[]
  readonly histogramTimeseriesCount: number
  readonly histogramVisible: SvelteSet<string>
  /** True when the chart narrowed itself to the data because no window was
   *  asked for, so the axis can say so rather than quietly cropping. */
  readonly histogramAxisFitToData: boolean
  /**
   * The datapoints a series actually carries, as sent, or undefined until they
   * have been fetched.
   *
   * Separate from `metric.timeseries[].datapoints`, which for a reduced
   * histogram are the store's merged buckets. The chart is free to draw a
   * reduction; the list is not free to show one, because the list is the answer
   * to "what did my service send". Fetched per series on expansion rather than
   * with the metric, since it is the one place that needs every row.
   */
  /**
   * Reset and seed this metric's view state, synchronously.
   *
   * Called by the page in the same statement that assigns the metric, so the
   * visible set, colours and sub-view are in place before anything renders.
   * As an effect this ran after the first render, so the chart built once for
   * the outgoing metric's state and again for the incoming one.
   */
  seedForMetric(metric: MetricData | undefined): void
  seriesDatapoints(seriesKey: string): DataPoint[] | undefined
  readonly heatmapBucketSeries: HistogramSlicePoint[] | null
  readonly bucketSeriesError: BucketSeriesError | null
  readonly aggregatedDatapoint:
    HistogramDataPoint | ExponentialHistogramDataPoint | undefined
  readonly aggregatedError: BucketSeriesError | null
  readonly histogramChartDatapoint:
    HistogramDataPoint | ExponentialHistogramDataPoint | undefined
  readonly histogramChartError: BucketSeriesError | null
  readonly activeHistogramDp:
    HistogramDataPoint | ExponentialHistogramDataPoint | undefined
  readonly heatmapSelectedTimestamp: number | null
  /** Quantile line key (e.g. `"0.95"`) when selection came from a quantile chart point click. */
  readonly selectedQuantileKey: string | null
  readonly heatmapColumnSelection: HeatmapColumnSelection | null
  readonly quantilePointSelection: QuantilePointSelection | null
  readonly quantileChartTimeseries: ChartTimeseries[]
  readonly quantileColorByKey: TimeseriesColorByKey
  readonly activeQuantileOverlays: SvelteSet<string>

  // -- Detail view wiring --
  readonly filteredTimeseries: MetricData['timeseries']
  /** Checked timeseries → colour from the stem-rotated pool. Unchecked rows
   *  have no entry; their checkbox uses neutral. */
  readonly timeseriesColorByKey: TimeseriesColorByKey
  /** Stem-rotated 10-colour pool (`pool[0]` = metric-type stem). */
  readonly timeseriesChartColors: string[]
  readonly legendFilterActive: boolean
  /** Sparkline points per Series tab row, keyed by `attributesKey`, reduced by
   *  the store to the row's own width. Reflects the current AggregationView
   *  (the rate buckets in rate view, the sparkline reduction otherwise).
   *  Covers every candidate series, not just visible ones -- an unchecked row
   *  keeps its shape, because that is what tells the reader to check it. Empty
   *  for histogram / unspecified temporality. */
  readonly sparklineByKey: ReadonlyMap<string, readonly ChartPoint[]>
  /** Min / max / avg / (sometimes) total per row. From the store's whole-window
   *  stats, except in rate view, where they describe {@link sparklineByKey}'s
   *  transform instead. */
  readonly seriesStatsByKey: ReadonlyMap<string, SeriesStats>
  /** Which stat badges TimeseriesPanel should render for this metric/view. */
  readonly availableSeriesStatBadges: readonly SeriesStat[]

  // -- Methods --
  /** Toggle per-timeseries expansion (TimeseriesPanel chevron). */
  toggleTimeseriesExpanded(key: string): void
  setActiveHistogramTab(tab: HistogramTab): void
  setHistogramScope(scope: HistogramScope): void
  setAggregationView(next: AggregationView): void
  setShowAllSeriesAggregate(next: boolean): void
  setShowSelectionStatOverlays(next: boolean): void
  /** Replace the visible-set for the Gauge/Sum legend. The legend
   * keeps a `bind:visibleKeys` model; we expose a setter so the
   * sole writer is still us. */
  setGaugeSumVisible(next: SvelteSet<string>): void
  setHistogramVisible(next: SvelteSet<string>): void
  /** Toggle chart visibility and persist immediately. */
  toggleTimeseriesVisible(key: string, checked: boolean): void
  /** Uncheck every timeseries and release all colour assignments. */
  clearAllTimeseriesVisible(): void
  /** Toggle selection + (optionally) expansion + jump to the
   * histogram tab in bucket scope. Used by the detail view. */
  onDatapointClick(dp: DataPoint): void
  /** Heatmap column click: toggle the selected time bucket (stay on heatmap). */
  onHeatmapSelect(timestampMs: number): void
  /** Time-series chart point click: resolve series + x to a datapoint
   * and sync selection with the Series tab. Aggregate lines are ignored. */
  onChartPointClick(seriesKey: string, clickedAt: Date): void
  /** Quantiles tab: toggle sticky bucket selection at the clicked x. */
  onQuantileChartPointClick(
    seriesKey: string,
    clickedAt: Date,
    quantileKey?: string | null
  ): void
  setActiveQuantileOverlay(quantileKey: string): void
}

// --- Factory ------------------------------------------------------

/**
 * Turn the store's aggregate buckets into the slice shape the heatmap draws.
 *
 * A rename, not a computation: every number is already merged. The store owns
 * scale alignment, zero-threshold folding and the vector sums, so this only
 * maps field names and widens the counts to numbers.
 */
function aggregateToSlices(
  buckets: JsonAggregateBucket[] | null
): HistogramSlicePoint[] {
  if (!buckets) return []
  return buckets.map(b => {
    const totals = {
      count: b.count,
      sum: b.sum,
      // Derived from the buckets server-side; a merge cannot carry the
      // originals through.
      min: b.min,
      max: b.max,
    }
    // Explicit bounds or exponential, never both. Reading only the exponential
    // fields left every explicit-bounds histogram with empty arrays and an
    // empty chart, while its totals and quantiles were right -- which is why it
    // looked like a rendering problem rather than a missing branch.
    if (b.explicitBounds && b.explicitBounds.length > 0) {
      return {
        kind: 'histogram' as const,
        timestamp: BigInt(b.timestamp),
        attributesKey: '',
        bounds: b.explicitBounds,
        counts: b.bucketCounts ?? [],
        totals,
        quantiles: b.quantiles ?? null,
      }
    }
    return {
      kind: 'expHistogram' as const,
      timestamp: BigInt(b.timestamp),
      attributesKey: '',
      scale: b.scale ?? 0,
      zeroThreshold: b.zeroThreshold ?? 0,
      zeroCount: b.zeroCount ?? 0,
      positiveOffset: b.positiveBucketOffset ?? 0,
      positiveCounts: b.positiveBucketCounts ?? [],
      negativeOffset: b.negativeBucketOffset ?? 0,
      negativeCounts: b.negativeBucketCounts ?? [],
      totals,
      quantiles: b.quantiles ?? null,
    }
  })
}

export function createMetricViewContext(
  getMetric: () => MetricData | undefined,
  /**
   * The store's cross-series aggregate for the current legend selection.
   *
   * A getter rather than something fetched here: this context is a derivation
   * layer over a metric and owes its predictability to not doing IO. The page
   * owns the fetch, this owns what the numbers mean.
   */
  getAggregate: () => JsonAggregateBucket[] | null = () => null,
  /** The same merge over a single bucket spanning the window. */
  getAggregateSummary: () => JsonAggregateBucket | null = () => null,
  /**
   * The store's cross-series fold for a scalar metric: the checked pool and the
   * full pool, on the same bucket grid the per-series views use.
   *
   * A getter for the same reason the histogram aggregate is one -- it depends
   * on the legend selection, so the page refetches it while this layer only
   * decides what the numbers mean.
   */
  getScalarAggregate: () => ScalarAggregate | null = () => null,
  /** Unreduced datapoints for one series, once the page has fetched them. */
  getSeriesDatapoints: (seriesKey: string) => DataPoint[] | undefined = () =>
    undefined
): MetricViewContext {
  const timeContext = getTimeContext()

  // The ONE per-metric mutable cell. Reset by the effect below when
  // the metric identity changes; otherwise written only by methods
  // on this context.
  const view = $state({
    selectedDatapointId: null as string | null,
    // Explicitly chosen series, independent of any datapoint selection.
    selectedSeriesKey: null as string | null,
    selectionSource: null as 'chart' | 'detail' | null,
    expandedDatapoints: new SvelteSet<string>(),
    expandedTimeseries: new SvelteSet<string>(),
    activeHistogramTab: 'heatmap' as HistogramTab,
    histogramScope: 'window' as HistogramScope,
    selectedHistogramBucketStart: null as bigint | null,
    selectedQuantileKey: null as string | null,
    gaugeSumVisible: new SvelteSet<string>(),
    histogramVisible: new SvelteSet<string>(),
    timeseriesColorByKey: new Map<string, string>() as TimeseriesColorByKey,
    // Aggregation-view state. `aggregationView` defaults to 'raw' (Gauge metrics never
    // touch this); the per-metric reset effect re-derives the smart
    // default from (temporality, isMonotonic) when the user navigates
    // between metrics. When histograms grow their own overlays we'll add
    // histogram-specific state next to this, not generalize prematurely.
    aggregationView: 'raw' as AggregationView,
    showAllSeriesAggregate: false,
    showSelectionStatOverlays: true,
    activeQuantileOverlays: new SvelteSet([
      DEFAULT_ACTIVE_HISTOGRAM_QUANTILE_KEY,
    ]),
  })

  /** Histogram visibility is seeded once per stream id. */
  let histogramVisibleSeededForStreamId: string | null = null

  // --- URL <-> metric sub-view sync ---------------------------------
  //
  // The selected metric lives in the path (`/metrics/<id>`, owned by
  // MetricsPage); these four item-scoped query params carry the sub-view so a
  // shared snapshot link and the browser back/forward buttons restore it:
  //   agg=<aggregationView>   htab=<activeHistogramTab>
  //   hscope=<histogramScope> dp=<selectedDatapointId>
  // Tab/datapoint picks push a history entry (navigational); aggregation/scope
  // adjustments replace (silent). The heatmap/quantile bucket selection stays
  // transient (not a stable datapoint id) and is out of the URL this iteration.
  //
  // Reconciliation mirrors time-context's snapshot pattern: the view and the
  // URL legitimately disagree when the aggregation is a smart default or a
  // localStorage restore (those are never written to the URL), so the router
  // subscription compares the URL against the last sub-view we wrote/applied
  // -- not against the live view. Only a mismatch there is an external change
  // (back/forward, shared link, another module stripping our params).

  // The metric sub-view currently frozen in the URL. Null until the
  // per-metric effect first applies the URL.
  let urlMetricViewSnapshot: MetricViewQuery | null = null

  function metricParseContext(): MetricViewParseContext {
    const datapointIds = new Set<string>()
    for (const dp of allDatapoints(getMetric())) datapointIds.add(dp.id)
    const seriesKeys = new Set<string>()
    for (const ts of getMetric()?.timeseries ?? [])
      seriesKeys.add(ts.attributesKey)
    return {
      isHistogramKind,
      allowedAggs: availableAggregationViewsList,
      datapointIds,
      seriesKeys,
    }
  }

  function viewStateToMetricViewQuery(): MetricViewQuery {
    if (isHistogramKind) {
      return {
        kind: 'histogram',
        htab: view.activeHistogramTab,
        hscope: view.histogramScope,
        dp: view.selectedDatapointId,
        series: selectedSeriesKey,
      }
    }
    return {
      kind: 'timeseries',
      agg: view.aggregationView === 'raw' ? null : view.aggregationView,
      dp: view.selectedDatapointId,
      series: selectedSeriesKey,
    }
  }

  /**
   * Applies the URL's metric sub-view to the view state.
   *
   * `resetMissingAgg` picks the semantics for an absent `agg` param
   * (`q.agg` is pre-validated: null when missing or invalid):
   * - `false` (metric load): keep the smart default / persisted choice the
   *   per-metric effect just set. A bare URL carries no opinion on load.
   * - `true` (external navigation): reset to 'raw'. The serializer omits
   *   `agg` for 'raw', so on back/forward an absent param IS the state the
   *   user navigated to (#235).
   */
  function applyMetricUrlToView(resetMissingAgg: boolean): void {
    const q = parseMetricViewQuery(readRoute().query, metricParseContext())
    urlMetricViewSnapshot = q

    if (q.kind === 'timeseries') {
      if (q.agg) {
        view.aggregationView = q.agg as AggregationView
      } else if (resetMissingAgg) {
        view.aggregationView = 'raw'
      }
    } else {
      view.activeHistogramTab = q.htab
      view.histogramScope = q.hscope
    }

    if (q.dp) {
      view.selectedDatapointId = q.dp
      view.selectionSource = 'detail'
    } else {
      view.selectedDatapointId = null
      view.selectionSource = null
    }

    // The series survives independently of the datapoint. A link older than a
    // retention window still names a live series long after the specific point
    // it was built from has been pruned, so restoring it is what keeps a
    // shared link meaningful rather than silently empty.
    view.selectedSeriesKey = q.series

    // A named series has to be *drawn*, or the link is worse than useless: the
    // default visible set is the first MAX_VISIBLE_TIMESERIES, so a link to
    // series 40 of 63 would otherwise land on a chart that does not contain
    // it. Adding rather than replacing keeps the rest of the user's view.
    if (q.series) revealSeries(q.series)
  }

  /**
   * Ensures a series is in the visible set for its metric kind.
   *
   * @param key - series id
   *
   * @remarks
   * Only ever adds. A shared link should bring its series into view without
   * throwing away whatever else the recipient had showing.
   */
  function revealSeries(key: string): void {
    const visible = isHistogramKind
      ? view.histogramVisible
      : view.gaugeSumVisible
    if (!visible.has(key)) visible.add(key)
  }

  function writeMetricUrl(mode: HistoryMode): void {
    const q = viewStateToMetricViewQuery()
    urlMetricViewSnapshot = q
    setMetricViewQuery(q, mode)
  }

  // -- Pure derivations of `metric` --
  const metricType = $derived.by((): MetricType => {
    const m = getMetric()
    if (m?.metricType) return m.metricType
    return m?.timeseries[0]?.datapoints[0]?.metricType ?? 'Empty'
  })

  function* allDatapoints(
    m: MetricData | undefined
  ): IterableIterator<DataPoint> {
    if (!m) return
    for (const ts of m.timeseries) {
      for (const dp of ts.datapoints) yield dp
    }
  }

  const temporality = $derived.by(() => {
    const m = getMetric()
    if (m?.aggregationTemporality) return m.aggregationTemporality
    for (const dp of allDatapoints(m)) {
      const t = (dp as { aggregationTemporality?: string })
        .aggregationTemporality
      if (t) return t
    }
    return ''
  })

  const isMonotonic = $derived.by((): boolean | null => {
    if (metricType !== 'Sum') return null
    const m = getMetric()
    if (m?.isMonotonic != null) return m.isMonotonic
    for (const dp of allDatapoints(m)) {
      if (dp.metricType === 'Sum') return dp.isMonotonic
    }
    return null
  })

  const isHistogramKind = $derived(
    metricType === 'Histogram' || metricType === 'ExponentialHistogram'
  )

  const isUnspecifiedTemporality = $derived.by(() => {
    if (
      metricType !== 'Histogram' &&
      metricType !== 'ExponentialHistogram' &&
      metricType !== 'Sum'
    ) {
      return false
    }
    for (const dp of allDatapoints(getMetric())) {
      const t = (dp as { aggregationTemporality?: string })
        .aggregationTemporality
      if (t === 'Unspecified') return true
    }
    return false
  })

  const totalDatapointCount = $derived(
    getMetric()?.timeseries.reduce(
      (acc, ts) => acc + ts.datapoints.length,
      0
    ) ?? 0
  )

  const queryRange = $derived(
    selectionToQueryRangeMs(timeContext.selection, Date.now())
  )

  // -- Gauge/Sum chart + legend --

  /**
   * The series keys, without projecting any points.
   *
   * Cheap on purpose, and separate from gaugeSumGroups for that reason: the
   * per-metric reset effect needs the keys in order to seed the visible set and
   * the colours, and it must not have to build the chart to get them.
   */
  const gaugeSumKeys = $derived.by((): string[] => {
    const m = getMetric()
    if (!m || (metricType !== 'Gauge' && metricType !== 'Sum')) return []
    return m.timeseries.map(ts => ts.attributesKey)
  })

  /**
   * Chart points per series, built once the metric's view state has been seeded.
   *
   * The gate is what stops every selection drawing the chart twice. Assigning
   * selectedMetric invalidates this derivation, so the chart rendered
   * immediately -- with the *previous* metric's visible set and colours -- and
   * then the per-metric reset effect ran, wrote gaugeSumVisible, the aggregation
   * view and the colour assignments, and the whole pipeline ran again. Both
   * passes projected every datapoint into a Date and rebuilt every path.
   *
   * Measured on a 22-series Gauge: 46 path writes for 23 lines, 63,115 Date
   * constructions for ~23,600 datapoints, and two blocking tasks of 1,246 ms
   * each -- identical, because it was the same work twice.
   *
   * Waiting for the seed costs one cheap empty render and saves an expensive
   * wasted one. The output of that first pass was never seen: it was replaced in
   * the same beat by the second.
   */
  const gaugeSumGroups = $derived.by(() => {
    const m = getMetric()
    const scalar = metricType === 'Gauge' || metricType === 'Sum'
    const source = m && scalar ? m.timeseries : []

    // Projected per series and on demand, not all at once.
    //
    // A hidden series needs no chart points: rawTransformed filters to the
    // visible set before the chart sees anything, and a row's sparkline and stat
    // badges come from the store. Projecting one costs a Date per datapoint, so
    // the work is worth deferring until a line is actually drawn.
    //
    // The cache lives inside this $derived, so it is discarded when the metric
    // changes and no invalidation has to be arranged. Checking a series later
    // projects it then, once.
    const cache = new Map<string, ChartTimeseries>()
    const byKey = new Map(source.map(ts => [ts.attributesKey, ts]))
    // Resolved once for the whole metric: which resource attributes tell the
    // series apart is a question about the set, not about any one of them.
    const labelByKey = seriesLabelsByKey(source)
    const labelFor = (ts: MetricTimeseries) =>
      labelByKey.get(ts.attributesKey) ?? ts.attributesKey

    function project(key: string): ChartTimeseries | undefined {
      const hit = cache.get(key)
      if (hit) return hit
      const ts = byKey.get(key)
      if (!ts) return undefined
      // No client-side thinning: the store's M4 election is the reduction, and
      // it is sized so its output is what the chart draws.
      const built = timeseriesToChartTimeseries([ts], labelFor).chartTimeseries[0]
      if (built) cache.set(key, built)
      return built
    }

    /** Source order throughout: the legend assigns colour positionally, so a
     *  projected list must not reorder what the store sent. */
    function projectMatching(
      include: (key: string) => boolean
    ): ChartTimeseries[] {
      const out: ChartTimeseries[] = []
      for (const ts of source) {
        if (!include(ts.attributesKey)) continue
        const built = project(ts.attributesKey)
        if (built) out.push(built)
      }
      return out
    }

    return {
      /** The unprojected series, for questions answerable from datapoint
       *  counts and timestamps without building chart points. */
      timeseries: source,
      keys: source.map(ts => ts.attributesKey),
      /** Key and label only -- what the store's own per-bucket views need,
       *  since those are keyed by series and carry their own points. */
      labels: source.map(ts => ({
        key: ts.attributesKey,
        label: labelFor(ts),
      })),
      project,
      projectAll: () => projectMatching(() => true),
      projectVisible: (visible: { has: (key: string) => boolean }) =>
        projectMatching(key => visible.has(key)),
    }
  })

  const gaugeSumLegendTimeseries = $derived.by((): LegendTimeseries[] => {
    const m = getMetric()
    if (!m) return []
    const distinguishing = distinguishingResourceAttributes(m.timeseries)
    return m.timeseries.map(ts => ({
      key: ts.attributesKey,
      attributes: [
        ...ts.attributes,
        ...(distinguishing.get(ts.attributesKey) ?? []),
      ],
    }))
  })

  // -- Sum view transformations --
  //
  // Two modes:
  //   - Raw (aggregationView === 'raw'): per-series lines, visibility-filtered.
  //     Same as before; the chart gets N lines for the checked series.
  //   - Aggregated (sum/avg/rate): up to 2 cross-timeseries aggregate
  //     lines (Selected, All), folded by the store. Collapse rules:
  //     selected empty → one "All" line; selected covers all → one
  //     "Total" line; otherwise 2.

  /**
   * The store's per-bucket answer for one view, as chart points.
   *
   * A projection of the `views` the response carries -- no bucketing, no
   * differencing, no reset detection. All three used to happen here, over chart
   * points that had already been through the M4 election, on a grid derived
   * from each series' own first and last point.
   *
   * An empty bucket reads differently per view, which is why the store sends
   * null rather than zero. Sum and Rate mean "nothing happened in this window",
   * so they draw a zero. Average has no answer -- the mean of nothing is not
   * nought -- so it leaves a gap. A bucket that *has* samples but no value is
   * the first bucket of a cumulative series: no predecessor, so no interval,
   * and nothing to draw either.
   */
  function viewPoints(
    views: ScalarViewBucket[] | null,
    view: 'sum' | 'avg' | 'rate'
  ): ChartPoint[] {
    if (!views) return []
    const out: ChartPoint[] = []
    for (const b of views) {
      const value = view === 'sum' ? b.sum : view === 'avg' ? b.avg : b.rate
      if (value === null) {
        if (b.sampleCount > 0) continue
        if (view === 'avg') continue
        out.push({
          date: new Date(Number(b.bucketStart / 1_000_000n)),
          value: 0,
        })
        continue
      }
      out.push({ date: new Date(Number(b.bucketStart / 1_000_000n)), value })
    }
    return out
  }

  /**
   * The store's sparkline reduction as chart points.
   *
   * A projection and nothing more -- the reduction is min and max per bucket,
   * done in SQL. Each point keeps the timestamp its sample actually occurred
   * at rather than its bucket's start, so a spike leans the way it happened.
   */
  function sparklinePoints(points: SparklinePoint[] | null): ChartPoint[] {
    if (!points) return []
    return points.map(p => ({
      date: new Date(Number(p.timestamp / 1_000_000n)),
      value: p.value,
    }))
  }

  /** Per-series lines for a store-computed view, plus the buckets whose counter
   *  restarted -- which the rate chart marks. */
  function seriesFromViews(
    series: readonly { key: string; label: string }[],
    view: 'sum' | 'avg' | 'rate'
  ): { series: ChartTimeseries[]; resets: ResetIndicesByKey } {
    const m = getMetric()
    const resets: ResetIndicesByKey = new Map()
    if (!m) return { series: [], resets }
    const byKey = new Map(m.timeseries.map(ts => [ts.attributesKey, ts.views]))
    const out: ChartTimeseries[] = []
    for (const s of series) {
      const views = byKey.get(s.key) ?? null
      out.push({ ...s, points: viewPoints(views, view) })
      if (!views) continue
      const flagged: number[] = []
      views.forEach((b, i) => {
        if (b.hasReset) flagged.push(i)
      })
      if (flagged.length > 0) resets.set(s.key, flagged)
    }
    return { series: out, resets }
  }

  /** Per-series lines for the visibility-filtered set.
   *
   *  - In Raw / Sum / Avg views: pass through unchanged. The raw
   *    cumulative climb (or untouched gauge sample) is what the user
   *    wants alongside the cross-series aggregate.
   *  - In Rate view: convert each visible series into its own
   *    per-series rate (Prometheus's `rate()` for one series).
   *    Otherwise the cumulative raw lines dwarf the aggregate-rate
   *    lines and the rate looks flat. Per-series rate keeps every
   *    line in the same units (events/sec) so the y-axis fits
   *    naturally and you can see *which* series is contributing to
   *    the aggregate rate. Bucket count is shared with the
   *    aggregate so step boundaries line up. */
  const rawTransformed = $derived.by(() => {
    if (gaugeSumGroups.keys.length === 0)
      return {
        series: [] as ChartTimeseries[],
        resets: new Map() as ResetIndicesByKey,
      }
    // Rate draws the store's per-series buckets, which are keyed by series and
    // carry their own points, so this branch projects nothing: only the key and
    // label travel.
    if (view.aggregationView === 'rate') {
      return seriesFromViews(
        gaugeSumGroups.labels.filter(s => view.gaugeSumVisible.has(s.key)),
        'rate'
      )
    }
    // Raw / Sum / Avg all draw the series as it was sampled: Sum and Avg are
    // cross-series aggregations, and neither means anything applied to one
    // series.
    return {
      series: gaugeSumGroups.projectVisible(view.gaugeSumVisible),
      resets: new Map() as ResetIndicesByKey,
    }
  })

  /** Sparkline points for each Series tab row (`attributesKey`).
   *
   *  Runs over EVERY candidate series, not just the visible ones, so an
   *  unchecked row still gets a shape: the sparkline is the affordance that
   *  tells the reader which row is worth checking, so withholding it from the
   *  rows they have not chosen yet defeats it.
   *
   *  Both branches are server-reduced and bounded by the row's own width, which
   *  is a different budget from the chart's: CHART_POINTS_PER_SERIES into a
   *  128px box is roughly fifteen points per pixel.
   *
   *  Source follows the current aggregationView:
   *    - 'rate'                  → the store's rate buckets, the same numbers
   *                                the main chart's per-series rate draws.
   *    - 'raw' / 'sum' / 'avg'   → the store's sparkline reduction: min and max
   *                                per bucket, so a spike survives at this size
   *                                where an average would flatten it. Sum/Avg
   *                                are cross-series aggregations that don't
   *                                apply per row, so a row's own line stays in
   *                                raw units.
   *
   *  Histogram metrics return an empty map — TimeseriesPanel renders
   *  a placeholder slot for histogram rows until per-series sparkbar
   *  data is wired up. */
  const sparklinePointsByKey = $derived.by(
    (): ReadonlyMap<string, readonly ChartPoint[]> => {
      if (isHistogramKind) return new Map()
      // Unspecified temporality means we can't tell whether the values
      // are running totals or per-interval counts -- the same numbers
      // mean two very different lines depending on which it is. The
      // main chart blanks itself + shows UnspecifiedTemporalityCallout
      // for the same reason; row projections should follow that lead
      // rather than guessing.
      if (isUnspecifiedTemporality) return new Map()
      const out = new Map<string, readonly ChartPoint[]>()
      const rate = view.aggregationView === 'rate'
      for (const ts of getMetric()?.timeseries ?? []) {
        out.set(
          ts.attributesKey,
          rate ? viewPoints(ts.views, 'rate') : sparklinePoints(ts.sparkline)
        )
      }
      return out
    }
  )

  const seriesStatsByKey = $derived.by((): ReadonlyMap<string, SeriesStats> => {
    const out = new Map<string, SeriesStats>()
    if (isHistogramKind || isUnspecifiedTemporality) return out

    // Raw, Sum and Avg all leave a row's own line in raw units -- Sum and Avg
    // are cross-series aggregations, and neither means anything applied to one
    // series -- so all three read the store's stats. Those are computed over
    // every datapoint in the window rather than the reduced ones the client
    // holds, which is the only way to get them right: an avg taken over
    // reduced points is the mean of a sample, and a total is short by the
    // reduction factor. `total` is offered as a badge for Sum + Delta + raw.
    //
    // Rate is the exception, and for a reason rather than by omission: a rate
    // row is a transform of the series, not the series in different units, so
    // its stats have to describe the transform. They come off the store's rate
    // buckets -- the same numbers the row's sparkline draws, so the badge and
    // the shape above it can never disagree.
    if (view.aggregationView === 'rate') {
      for (const [key, points] of sparklinePointsByKey) {
        out.set(key, seriesStatsFromPoints(points))
      }
      return out
    }

    for (const ts of getMetric()?.timeseries ?? []) {
      const st = ts.stats
      if (!st) continue
      out.set(ts.attributesKey, {
        min: st.min,
        max: st.max,
        avg: st.avg,
        total: st.sum,
      })
    }
    return out
  })

  const availableSeriesStatBadgesList = $derived.by((): SeriesStat[] => {
    if (isHistogramKind || isUnspecifiedTemporality) return []
    return availableSeriesStatBadges({
      metricType,
      temporality,
      aggregationView: view.aggregationView,
    })
  })

  /** Aggregated mode: Selected + All cross-timeseries lines, as the store
   *  folded them.
   *
   *  The collapse rules stay here because they are labelling, not arithmetic:
   *  nothing checked draws All by itself; everything checked makes Selected and
   *  All the same line, which is drawn once as Total; otherwise both.
   *
   *  The pools themselves come off the wire, folded over every datapoint on the
   *  same absolute grid the per-series views use -- so the aggregate and the
   *  lines beneath it share an x-axis by construction rather than by both
   *  guessing the same bucket count. */
  const aggregatedTransformed = $derived.by((): AggregateResult => {
    const pools = getScalarAggregate()
    if (!pools) return { lines: [], presentKeys: [] }
    const v = view.aggregationView as 'sum' | 'avg' | 'rate'
    const allPoints = viewPoints(pools.all, v)
    if (allPoints.length === 0) return { lines: [], presentKeys: [] }

    const checkedCount = view.gaugeSumVisible.size
    const total = gaugeSumGroups.keys.length
    // Selected covering everything means the two lines are identical; drawing
    // both would just stack them.
    if (checkedCount === 0 || checkedCount === total) {
      const key = checkedCount === 0 ? AGG_KEY_ALL : AGG_KEY_TOTAL
      const label = checkedCount === 0 ? 'All' : 'Total'
      return {
        lines: [{ key, label, points: allPoints }],
        presentKeys: [key],
      }
    }

    const lines: ChartTimeseries[] = []
    const presentKeys: AggregateLineKey[] = []
    const selectedPoints = viewPoints(pools.selected, v)
    if (selectedPoints.length > 0) {
      lines.push({
        key: AGG_KEY_SELECTED,
        label: 'Selected',
        points: selectedPoints,
      })
      presentKeys.push(AGG_KEY_SELECTED)
    }
    lines.push({ key: AGG_KEY_ALL, label: 'All', points: allPoints })
    presentKeys.push(AGG_KEY_ALL)
    return { lines, presentKeys }
  })

  /** Which aggregate line keys are present (for legend + color slots).
   *  Suppressed when N=1 because the "All series" aggregate would just
   *  be a duplicate of the single raw line. With 0–1 checked, omit
   *  Selected — only All (or Total when every series is checked).
   *  All is omitted unless the user has toggled it on. */
  const aggregatePresentKeys = $derived.by((): AggregateLineKey[] => {
    if (view.aggregationView === 'raw') return []
    if (gaugeSumGroups.keys.length < 2) return []
    let keys = aggregatedTransformed.presentKeys
    if (rawTransformed.series.length < 2) {
      keys = keys.filter(k => k !== AGG_KEY_SELECTED)
    }
    if (!view.showAllSeriesAggregate) {
      keys = keys.filter(k => k !== AGG_KEY_ALL)
    }
    return keys
  })

  /** Show the optional all-series aggregate toggle in the chart control bar. */
  const showAllSeriesAggregateToggleVisible = $derived.by((): boolean => {
    if (view.aggregationView === 'raw') return false
    // Keys alone answer this -- it is a count of checkboxes, not of points.
    const keys = gaugeSumGroups.keys
    if (keys.length < 2) return false
    let selectedCount = 0
    for (const key of keys) {
      if (view.gaugeSumVisible.has(key)) selectedCount++
    }
    // All series checked → aggregate collapses to Total; nothing extra
    // to toggle.
    return selectedCount !== keys.length
  })

  const showChartStatOverlaysToggleVisible = $derived.by((): boolean => {
    return metricType === 'Gauge' || metricType === 'Sum'
  })

  const rateSlopeOverlayAvailable = $derived.by((): boolean => {
    if (isHistogramKind || isUnspecifiedTemporality) return false
    return availableRateSlopeOverlay({
      metricType,
      temporality,
      isMonotonic,
      aggregationView: view.aggregationView,
    })
  })

  /**
   * The window the chart draws, and whether it was narrowed to the data.
   *
   * "All" is the absence of a choice, so a chart may fit its own data: two
   * hours of a race across a fifty-six year axis is a hairline, and the
   * emptiness around it carries nothing. Any other selection is a request, and
   * the gap between what was asked for and what arrived is part of the answer
   * -- a metric that stopped reporting should look like one.
   *
   * Gauges and Sums have always fitted, by taking the extent of their own
   * points; histograms were the one signal drawn against the raw query range,
   * which is the asymmetry this closes.
   *
   * Derived from the metric, not from the clock, so polling cannot slide the
   * axis under someone mid-read.
   */
  const histogramAxisWindow = $derived.by(() => {
    const qr = selectionToQueryRangeMs(timeContext.selection, Date.now())
    const asked = {
      startNs: BigInt(qr.start) * 1_000_000n,
      endNs: BigInt(qr.end) * 1_000_000n,
      fitToData: false,
    }
    // The store reports the window it reduced over, so the axis draws the same
    // window the buckets were cut from. Scanning the returned datapoints
    // instead measured bucket *starts* -- the reduction's own output -- and so
    // could never reveal that the reduction had collapsed.
    const w = getMetric()?.window
    if (!w?.fittedToData || w.startNs === null || w.endNs === null) return asked
    if (w.endNs <= w.startNs) return asked
    // End is exclusive downstream, so a point on the last timestamp still
    // lands inside the final bucket.
    return { startNs: w.startNs, endNs: w.endNs + 1n, fitToData: true }
  })

  const chartDataTimeRange = $derived.by(
    (): { startMs: number; endMs: number } | undefined => {
      if (isHistogramKind) {
        // The same window the buckets were built over -- the header states
        // what was drawn, not what was requested.
        const { startNs, endNs } = histogramAxisWindow
        return {
          startMs: Number(startNs / 1_000_000n),
          endMs: Number(endNs / 1_000_000n),
        }
      }
      if (metricType === 'Gauge' || metricType === 'Sum') {
        // Read off the datapoints' epoch milliseconds rather than projected
        // points. This is the extent of the data, so it needs no chart points --
        // and asking for them here would force every series in the response to
        // be projected, including ones no line is drawn for.
        let min = Infinity
        let max = -Infinity
        for (const ts of gaugeSumGroups.timeseries) {
          for (const dp of ts.datapoints) {
            const t = dp.timestampMs
            if (t < min) min = t
            if (t > max) max = t
          }
        }
        if (!Number.isFinite(min)) return undefined
        return { startMs: min, endMs: max }
      }
      return undefined
    }
  )

  /** Final post-view series the chart actually plots.
   *
   *  - Raw mode: per-series visibility-filtered lines.
   *  - Aggregated + fewer than 2 timeseries on the metric: raw only
   *    (aggregate menu options are hidden anyway).
   *  - Aggregated + 0–1 checked: checked raw lines + All (or Total
   *    when all are checked). No Selected aggregate — it duplicates
   *    the lone raw line. Check a second series to add Selected.
   *  - Aggregated + 2+ checked: raw lines + Selected; All only when
   *    toggled on via showAllSeriesAggregate.
   *
   *  Raw lines are placed first so aggregates draw on top in the
   *  chart's natural render order. When aggregates are present, raw
   *  series are resampled onto the aggregate bucket-center grid so
   *  bisect-x tooltips and highlight dots share one x axis instead of
   *  flickering between scrape timestamps and bucket midpoints. */
  const transformedGaugeSumChartTimeseries = $derived.by(
    (): ChartTimeseries[] => {
      if (view.aggregationView === 'raw') return rawTransformed.series
      if (gaugeSumGroups.keys.length < 2) return rawTransformed.series
      const selectedCount = rawTransformed.series.length
      let aggLines = aggregatedTransformed.lines
      if (selectedCount < 2) {
        aggLines = aggLines.filter(l => l.key !== AGG_KEY_SELECTED)
      }
      if (!view.showAllSeriesAggregate) {
        aggLines = aggLines.filter(l => l.key !== AGG_KEY_ALL)
      }
      if (aggLines.length === 0) return rawTransformed.series

      const centers = aggLines[0]!.points.map(p => p.date)
      const alignedRaw = rawTransformed.series.map(s =>
        resampleSeriesToBucketCenters(s, centers)
      )
      return [...alignedRaw, ...aggLines]
    }
  )

  /** Reset markers — only relevant in raw mode. */
  const sumResetIndicesByKey = $derived.by((): ResetIndicesByKey => {
    if (view.aggregationView !== 'raw') return new Map()
    return rawTransformed.resets
  })

  /** Which AggregationView options the dropdown should offer. Driven by metric
   *  type + shape + series count. See availableAggregationViews() in
   *  aggregation.ts for the rules. */
  const availableAggregationViewsList = $derived.by((): AggregationView[] => {
    return availableAggregationViews(
      metricType,
      temporality,
      isMonotonic,
      gaugeSumGroups.keys.length
    )
  })

  // -- Selection-derived values --
  const selectedDatapoint = $derived.by((): DataPoint | undefined => {
    const m = getMetric()
    if (!m || !view.selectedDatapointId) return undefined
    for (const dp of allDatapoints(m)) {
      if (dp.id === view.selectedDatapointId) return dp
    }
    return undefined
  })

  const highlightedTimestamp = $derived.by((): bigint | null => {
    const dp = selectedDatapoint
    if (dp && (dp.metricType === 'Gauge' || dp.metricType === 'Sum')) {
      return dp.timestamp
    }
    return null
  })

  /** Attributes key of the timeseries that owns `selectedDatapoint`, or
   *  `null` when nothing is selected. The chart uses this to draw a
   *  colored dot on the selection rule at the selected series's value,
   *  and to flag which row in the mini-legend is "the one you picked"
   *  vs. aggregates shown alongside for comparison. */
  const selectedSeriesKey = $derived.by((): string | null => {
    const m = getMetric()
    if (!m) return null

    // A selected datapoint is the more specific answer and wins: it names both
    // the series and the point within it.
    const id = view.selectedDatapointId
    if (id !== null) {
      for (const ts of m.timeseries) {
        if (ts.datapoints.some(dp => dp.id === id)) return ts.attributesKey
      }
    }

    // Otherwise fall back to an explicitly chosen series. This is what a link
    // restores once its datapoint has aged out: the point is gone, the line is
    // still there, and the user still lands on it.
    if (view.selectedSeriesKey !== null) {
      const known = m.timeseries.some(
        ts => ts.attributesKey === view.selectedSeriesKey
      )
      if (known) return view.selectedSeriesKey
    }
    return null
  })

  const selectedRateSlope = $derived.by((): number | undefined => {
    if (!rateSlopeOverlayAvailable) return undefined
    const key = selectedSeriesKey
    if (!key || highlightedTimestamp === null) return undefined
    const series = transformedGaugeSumChartTimeseries.find(s => s.key === key)
    if (!series) return undefined
    const at = new Date(Number(highlightedTimestamp / 1_000_000n))
    return rateSlopeAtPoint(series.points, at)
  })

  // -- Histogram chart wiring --
  const latestHistogramDp = $derived.by(() => {
    const m = getMetric()
    if (!m || !isHistogramKind) return undefined
    let best: HistogramDataPoint | ExponentialHistogramDataPoint | undefined
    for (const dp of allDatapoints(m)) {
      if (
        dp.metricType !== 'Histogram' &&
        dp.metricType !== 'ExponentialHistogram'
      )
        continue
      if (!best || dp.timestamp > best.timestamp) {
        best = dp as HistogramDataPoint | ExponentialHistogramDataPoint
      }
    }
    return best
  })

  const histogramTimeseriesGroups = $derived.by(
    (): HistogramTimeseriesGroup[] => {
      const m = getMetric()
      if (!m || !isHistogramKind) return []
      const startNs = BigInt(queryRange.start) * 1_000_000n
      const endNs = BigInt(queryRange.end) * 1_000_000n
      return m.timeseries.map(ts => {
        let pointCount = 0
        for (const dp of ts.datapoints) {
          if (
            dp.metricType !== 'Histogram' &&
            dp.metricType !== 'ExponentialHistogram'
          ) {
            continue
          }
          if (dp.timestamp >= startNs && dp.timestamp < endNs) pointCount++
        }
        return {
          key: ts.attributesKey,
          attributes: ts.attributes,
          pointCount,
        }
      })
    }
  )

  // Always the set. All-visible is a set holding every key; none-visible is
  // empty. Returning null for "all" gave one state two encodings and put the
  // burden of telling empty from absent on every consumer.
  const histogramVisibleKeys = $derived.by(
    (): Set<string> => view.histogramVisible
  )

  const histogramAggregation = $derived.by(() => {
    const m = getMetric()
    const empty = {
      perAttribute: [] as HistogramSlicePoint[],
      heatmap: [] as HistogramSlicePoint[],
      summary: null as HistogramSlicePoint | null,
      error: null as BucketSeriesError | null,
      aggregatedError: null as BucketSeriesError | null,
    }
    if (!m || !isHistogramKind) return empty
    if (isUnspecifiedTemporality) {
      const err = histogramAggregationErrorToBucketSeriesError({
        kind: 'unspecified',
        message: 'Aggregation temporality is Unspecified',
      })
      return { ...empty, error: err, aggregatedError: err }
    }

    // One slice per store bucket. The store has already bucketed, merged and
    // resolved temporality; re-doing any of that here was the 3s of blocked
    // main thread on every histogram selection, and got Cumulative wrong.
    const perAttribute = seriesBucketsToSlices(m.timeseries)

    // The cross-series merge comes from the store, which aligned scales,
    // folded the zero threshold and summed the vectors. Merging it again here
    // would be the same arithmetic in a second language -- the duplication
    // that produced #351, where the two implementations disagreed about where
    // the zero region ended.
    //
    // Null while a fetch is in flight, or when nothing is selected. Both are
    // "no columns to draw" rather than an error.
    const heatmapResult = aggregateToSlices(getAggregate())
    const summary = getAggregateSummary()
    const summaryResult = summary ? aggregateToSlices([summary])[0]! : null

    return {
      perAttribute,
      heatmap: heatmapResult,
      summary: summaryResult,
      error: null,
      aggregatedError: null,
    }
  })

  // Badge counts the rows this series contributed to the window, from the same
  // source as the inline expanded table in TimeseriesPanel and the Gauge/Sum
  // branch above.
  //
  // For a reduced histogram those rows are the store's merged buckets, not raw
  // datapoints -- the comment here claimed "raw" and went on claiming it after
  // the merge moved into SQL, because both are `ts.datapoints` and neither the
  // type nor the shape changed underneath it. MetricData.datapointCount is what
  // holds the window's real total; on the reference capture the two read 3,094
  // and 17,076.
  const histogramLegendTimeseries = $derived.by((): LegendTimeseries[] => {
    const m = getMetric()
    if (!m) return []
    const distinguishing = distinguishingResourceAttributes(m.timeseries)
    return histogramTimeseriesGroups.map(g => ({
      key: g.key,
      attributes: [...g.attributes, ...(distinguishing.get(g.key) ?? [])],
    }))
  })

  const heatmapBucketSeries = $derived.by((): HistogramSlicePoint[] | null => {
    if (!isHistogramKind) return null
    if (histogramAggregation.error) return []
    return histogramAggregation.heatmap
  })

  const aggregatedDatapoint = $derived.by(
    (): HistogramDataPoint | ExponentialHistogramDataPoint | undefined => {
      const m = getMetric()
      const summary = histogramAggregation.summary
      if (!m || !summary) return undefined
      return histogramSliceToDatapoint(
        summary,
        `${m.id}:aggregated`,
        temporality || 'Delta'
      )
    }
  )

  const histogramBucketDatapoint = $derived.by(
    (): HistogramDataPoint | ExponentialHistogramDataPoint | undefined => {
      const m = getMetric()
      if (!m || !isHistogramKind) return undefined

      const dp = selectedDatapoint
      if (
        dp &&
        (dp.metricType === 'Histogram' ||
          dp.metricType === 'ExponentialHistogram')
      ) {
        return dp as HistogramDataPoint | ExponentialHistogramDataPoint
      }
      return undefined
    }
  )

  const histogramChartDatapoint = $derived.by(
    (): HistogramDataPoint | ExponentialHistogramDataPoint | undefined => {
      if (view.histogramScope === 'window') return aggregatedDatapoint
      return histogramBucketDatapoint
    }
  )

  const histogramChartError = $derived.by((): BucketSeriesError | null => {
    if (view.histogramScope !== 'window') return null
    return histogramAggregation.aggregatedError
  })

  const activeHistogramDp = $derived.by(() => {
    const pinned = histogramBucketDatapoint
    if (pinned) return pinned
    return latestHistogramDp
  })

  const heatmapSelectedTimestamp = $derived.by((): number | null => {
    if (view.selectedHistogramBucketStart === null) return null
    return Number(view.selectedHistogramBucketStart / 1_000_000n)
  })

  const heatmapColumnSelection = $derived.by(
    (): HeatmapColumnSelection | null => {
      if (view.selectedHistogramBucketStart === null) return null
      const series = heatmapBucketSeries
      if (!series || series.length === 0) return null
      return heatmapColumnSelectionAt(
        series,
        view.selectedHistogramBucketStart,
        temporality || 'Delta'
      )
    }
  )

  const quantilePointSelection = $derived.by(
    (): QuantilePointSelection | null => {
      if (view.selectedHistogramBucketStart === null) return null
      const perAttribute = histogramAggregation.perAttribute
      const merged = heatmapBucketSeries
      if (!Array.isArray(perAttribute) || perAttribute.length === 0) return null
      if (!merged || merged.length === 0) return null
      return quantilePointSelectionAt(
        perAttribute,
        merged,
        view.selectedHistogramBucketStart,
        histogramVisibleKeys,
        temporality || 'Delta'
      )
    }
  )

  const quantileChartTimeseries = $derived.by((): ChartTimeseries[] => {
    if (!isHistogramKind || histogramAggregation.error) return []
    const perAttribute = histogramAggregation.perAttribute
    if (!Array.isArray(perAttribute) || perAttribute.length === 0) return []

    const activeQuantiles = DEFAULT_HISTOGRAM_QUANTILES.filter(q =>
      view.activeQuantileOverlays.has(quantileKeyFromValue(q))
    )
    if (activeQuantiles.length === 0) return []

    return buildVisibleSeriesQuantileChartTimeseries(
      perAttribute,
      activeQuantiles,
      histogramVisibleKeys,
      seriesLabelsByKey(getMetric()?.timeseries ?? [])
    )
  })

  const quantileColorByKey = $derived.by((): TimeseriesColorByKey => {
    const map = new Map<string, string>()
    for (const ts of quantileChartTimeseries) {
      const parsed = parseQuantileSeriesKey(ts.key)
      if (!parsed) continue
      const color = view.timeseriesColorByKey.get(parsed.seriesKey)
      if (color) map.set(ts.key, color)
    }
    return map
  })

  // -- Detail-view wiring (legend filter coupling) --
  const visibleDpCanonicalKeys = $derived.by((): Set<string> => {
    if (metricType === 'Gauge' || metricType === 'Sum')
      return view.gaugeSumVisible
    if (isHistogramKind) return view.histogramVisible
    return new Set()
  })

  const filteredTimeseries = $derived.by(() => {
    const m = getMetric()
    if (!m) return []
    const filter = visibleDpCanonicalKeys
    if (filter === null) return m.timeseries
    return m.timeseries.filter(ts => filter.has(ts.attributesKey))
  })

  const legendOrderKeys = $derived.by((): string[] => {
    if (metricType === 'Gauge' || metricType === 'Sum') {
      return gaugeSumLegendTimeseries.map(ts => ts.key)
    }
    if (isHistogramKind) {
      return histogramTimeseriesGroups.map(g => g.key)
    }
    return []
  })

  const timeseriesChartColors = $derived.by(() => {
    const stem = metricTypeStem(metricType)
    const theme = themeSignal.value
    if (isHistogramKind) {
      const n = Math.max(legendOrderKeys.length, view.histogramVisible.size, 1)
      return categoricalPalette(n, stem, theme)
    }
    return categoricalPalette(MAX_VISIBLE_TIMESERIES, stem, theme)
  })

  const legendFilterActive = $derived(visibleDpCanonicalKeys !== null)

  function currentVisibleKeys(): SvelteSet<string> {
    return isHistogramKind ? view.histogramVisible : view.gaugeSumVisible
  }

  function replaceColorAssignments(next: TimeseriesColorByKey) {
    view.timeseriesColorByKey = next
  }

  /** Seed assignments when visible keys exist but the map is empty (e.g.
   *  telemetry arrived after the metric-reset effect ran with no keys). */
  function ensureColorAssignments(
    visible: ReadonlySet<string>,
    legendKeys: readonly string[]
  ) {
    if (visible.size === 0) {
      replaceColorAssignments(new Map())
      return
    }
    // Important: we cannot short-circuit on `view.timeseriesColorByKey.size > 0`.
    // On a metric switch, effect (2b) can fire before effect (1) has re-seeded
    // the colour map, so the map is non-empty but full of the *previous*
    // metric's keys. A size-only check would skip the reseed and leave the new
    // metric's series rendering neutral (visible-but-uncoloured) until the
    // next toggle. Only short-circuit when every currently-visible key already
    // has a colour assignment.
    let allAssigned = true
    for (const key of visible) {
      if (!view.timeseriesColorByKey.has(key)) {
        allAssigned = false
        break
      }
    }
    if (allAssigned) return
    const pool = categoricalPalette(
      MAX_VISIBLE_TIMESERIES,
      metricTypeStem(metricType),
      themeSignal.value
    )
    replaceColorAssignments(seedColorAssignments(pool, visible, legendKeys))
  }

  /**
   * Reset and seed every per-metric view choice, synchronously, for a metric
   * that has just arrived.
   *
   * Called by the page in the same statement that assigns the metric, before
   * anything renders. It used to be an $effect, which meant it ran *after* the
   * first render: the chart built once with the outgoing metric's visible set
   * and colours, this wrote the incoming metric's, and the chart built again.
   * Both passes projected every datapoint and rebuilt every path. Measured on a
   * 22-series Gauge: 46 path writes for 23 lines, 63,115 Date constructions for
   * ~23,600 datapoints, and two identical 1,246 ms blocking tasks.
   *
   * Histogram visibility is seeded here too, from the metric's own series keys.
   * It had its own effect waiting for those keys to "arrive", which is a
   * distinction that only existed because this ran late -- the keys are in the
   * metric the moment it does.
   */
  function seedForMetric(m: MetricData | undefined) {
    const streamId = m?.id

    view.selectedDatapointId = null
    view.selectionSource = null
    view.selectedHistogramBucketStart = null
    view.selectedQuantileKey = null
    view.expandedDatapoints.clear()
    view.expandedTimeseries.clear()
    view.activeHistogramTab = 'heatmap'
    view.histogramScope = 'window'

    // Sum view + overlays reset per metric. Persisted choice wins when
    // it's still allowed for this metric's current shape; otherwise we
    // fall back to the smart default (cumulative Sum → Rate, else Raw).
    // Gauge metrics never read aggregationView, so the value here is don't-care
    // for them; the menu component checks metricType before rendering.
    const persistedAggregationView = streamId
      ? loadPersistedAggregationView(streamId, availableAggregationViewsList)
      : null
    const defaultAggregation = defaultAggregationViewFor(
      metricType,
      temporality,
      isMonotonic,
      gaugeSumKeys.length
    )
    // Clamped to what this metric actually offers. The persisted value is
    // already validated against the same list; the default was not, so the two
    // rules only had to disagree once to seed a view the user could neither see
    // selected nor switch away from. 'raw' is in the list unconditionally.
    view.aggregationView =
      persistedAggregationView ??
      (availableAggregationViewsList.includes(defaultAggregation)
        ? defaultAggregation
        : 'raw')
    view.showSelectionStatOverlays = true
    view.showAllSeriesAggregate = streamId
      ? loadPersistedShowAllSeriesAggregate(streamId)
      : false
    view.activeQuantileOverlays = new SvelteSet([
      DEFAULT_ACTIVE_HISTOGRAM_QUANTILE_KEY,
    ])

    const gsKeys = gaugeSumKeys
    const gsVisible = new SvelteSet(
      streamId
        ? resolveTimeseriesVisible(gsKeys, streamId)
        : gsKeys.slice(0, MAX_VISIBLE_TIMESERIES)
    )
    view.gaugeSumVisible = gsVisible
    const pool = categoricalPalette(
      MAX_VISIBLE_TIMESERIES,
      metricTypeStem(metricType),
      themeSignal.value
    )
    const gsColors = seedColorAssignments(pool, gsVisible, gsKeys)
    // Histogram visibility, from the metric's own keys. No waiting: the series
    // are in the metric that was just handed to us.
    const histKeys = m ? m.timeseries.map(ts => ts.attributesKey) : []
    const histVisible = new SvelteSet(
      streamId && histKeys.length > 0
        ? resolveTimeseriesVisible(
            histKeys,
            streamId,
            DEFAULT_VISIBLE_TIMESERIES,
            null
          )
        : []
    )
    view.histogramVisible = histVisible
    histogramVisibleSeededForStreamId =
      histKeys.length > 0 ? (streamId ?? null) : null
    if (histKeys.length > 0) {
      const histPool = categoricalPalette(
        Math.max(histKeys.length, 1),
        metricTypeStem(metricType),
        themeSignal.value
      )
      replaceColorAssignments(
        seedColorAssignments(histPool, histVisible, histKeys)
      )
    } else {
      replaceColorAssignments(gsColors)
    }

    // URL > the per-metric defaults set above, so a shared deep link (or
    // back/forward into this metric) restores the sub-view, validated against
    // the metric that just loaded.
    //
    // No untrack needed any more: this is a plain call from the fetch, not a
    // reactive scope, so reading the router query here cannot make a
    // time-window write re-trigger the whole per-metric reset.
    applyMetricUrlToView(false)
  }

  // -- Effects (the only mutating side-channels) --

  // (1b) Re-apply the sub-view when the URL's metric params disagree with the
  // last sub-view we wrote or applied (the snapshot) — i.e. browser
  // back/forward, a shared link landing on the current metric, or another
  // module stripping our params. Comparing against the snapshot (not the live
  // view) keeps unrelated query writes (e.g. the time window) from clobbering
  // a smart-default aggregation that was never mirrored to the URL, while our
  // own writes are skipped as a no-op echo.
  $effect(() => {
    const unsubscribe = subscribeToRoute(() => {
      const fromUrl = parseMetricViewQuery(
        readRoute().query,
        metricParseContext()
      )
      if (
        urlMetricViewSnapshot &&
        metricViewQueriesEqual(fromUrl, urlMetricViewSnapshot)
      ) {
        return
      }
      applyMetricUrlToView(true)
    })
    return unsubscribe
  })

  // (2b) Same metric stream, new telemetry: prune attribute keys that have
  // gone away and colour any that have appeared. Polling can add or drop series
  // within a stream; seeding cannot see those, because it runs once per metric.
  $effect(() => {
    const m = getMetric()
    const streamId = m?.id
    if (!streamId) return

    if (metricType === 'Gauge' || metricType === 'Sum') {
      const keys = gaugeSumGroups.keys
      void keys.join('\0')
      // Effect (1) may have run before gaugeSumGroups.keys settled (metric
      // selection + data flow are not synchronous), leaving an empty
      // gaugeSumVisible against non-empty keys. Re-resolve from persisted
      // / defaults so the user doesn't have to reload to see a colour on
      // a single-default-series chart.
      // Reconcile only. The initial seed happens in seedForMetric, before this
      // ever runs, so there is no "visible set is empty against non-empty keys"
      // case left to repair -- that condition existed because seeding was an
      // effect that could land after the keys did. Same for the URL: it is
      // applied against the metric's own series, which are known at seed time.
      const next = reconcileTimeseriesVisible(
        view.gaugeSumVisible,
        keys,
        streamId
      )

      if (!visibleKeyListsEqual(view.gaugeSumVisible, next)) {
        const visible = new SvelteSet(next)
        view.gaugeSumVisible = visible
        const assigned = new Map(view.timeseriesColorByKey)
        syncColorAssignments(timeseriesChartColors, assigned, visible, keys)
        replaceColorAssignments(assigned)
      } else {
        ensureColorAssignments(view.gaugeSumVisible, keys)
      }
      return
    }

    if (!isHistogramKind) return
    const keys = histogramTimeseriesGroups.map(g => g.key)
    if (keys.length === 0) return
    void keys.join('\0')
    const next = reconcileTimeseriesVisible(
      view.histogramVisible,
      keys,
      streamId,
      null
    )
    if (!visibleKeyListsEqual(view.histogramVisible, next)) {
      const visible = new SvelteSet(next)
      view.histogramVisible = visible
      const assigned = new Map(view.timeseriesColorByKey)
      syncColorAssignments(timeseriesChartColors, assigned, visible, keys)
      replaceColorAssignments(assigned)
    } else {
      ensureColorAssignments(view.histogramVisible, keys)
    }
  })

  // (2c) Auto-clear selection when its owning timeseries goes hidden.
  //
  // The datapoints panel scopes what's shown by the visible set, so a
  // selectedDatapointId pointing at a hidden timeseries becomes an
  // orphan: invisible in the list, but still wired up to chart markers,
  // detail pane, etc. Snap it to null whenever its timeseries is no
  // longer in the active visibility filter (gaugeSumVisible for
  // Gauge/Sum, histogramVisible for histograms). Works for all four
  // metric kinds because the only thing that varies is which set we
  // consult.
  $effect(() => {
    const id = view.selectedDatapointId
    if (id === null) return

    const m = getMetric()
    if (!m) return

    let ownerKey: string | null = null
    for (const ts of m.timeseries) {
      if (ts.datapoints.some(dp => dp.id === id)) {
        ownerKey = ts.attributesKey
        break
      }
    }
    if (ownerKey === null) {
      // Stale id (data refresh dropped the datapoint). Clear so the
      // detail pane doesn't render against ghost data.
      view.selectedDatapointId = null
      view.selectionSource = null
      return
    }

    const visible =
      metricType === 'Gauge' || metricType === 'Sum'
        ? view.gaugeSumVisible
        : isHistogramKind
          ? view.histogramVisible
          : null
    if (visible === null) return
    if (!visible.has(ownerKey)) {
      view.selectedDatapointId = null
      view.selectionSource = null
    }
  })

  // (3) Coerce the current AggregationView back to the smart default when it
  // leaves the available set. Triggers on series-count changes (e.g.
  // polling drops the metric from N=5 to N=1, hiding Sum/Avg) and on
  // shape changes (rare, but defensible). Reads availableAggregationViewsList
  // reactively; only writes when there's a real mismatch so we don't
  // fight the user's choice.
  $effect(() => {
    const allowed = availableAggregationViewsList
    if (allowed.includes(view.aggregationView)) return
    const next = defaultAggregationViewFor(
      metricType,
      temporality,
      isMonotonic,
      gaugeSumGroups.keys.length
    )
    view.aggregationView = next
    // Persist the coerced value too: otherwise localStorage keeps the
    // stale (now-invalid) choice and we re-coerce on every load until
    // the user touches the menu.
    const streamId = getMetric()?.id
    if (streamId) savePersistedAggregationView(streamId, next)
  })

  // -- Methods --
  function toggleTimeseriesExpanded(key: string) {
    if (view.expandedTimeseries.has(key)) {
      view.expandedTimeseries.delete(key)
    } else {
      view.expandedTimeseries.add(key)
    }
  }

  function setActiveHistogramTab(tab: HistogramTab) {
    view.activeHistogramTab = tab
    // Back returns to the prior tab.
    writeMetricUrl('push')
  }

  function setHistogramScope(scope: HistogramScope) {
    view.histogramScope = scope
    writeMetricUrl('replace')
  }

  function setAggregationView(next: AggregationView) {
    view.aggregationView = next
    const streamId = getMetric()?.id
    if (streamId) savePersistedAggregationView(streamId, next)
    // localStorage still remembers it per metric.
    writeMetricUrl('replace')
  }

  function setShowAllSeriesAggregate(next: boolean) {
    view.showAllSeriesAggregate = next
    const streamId = getMetric()?.id
    if (streamId) savePersistedShowAllSeriesAggregate(streamId, next)
  }

  function setActiveQuantileOverlay(quantileKey: string) {
    view.activeQuantileOverlays = new SvelteSet([quantileKey])
  }

  function setShowSelectionStatOverlays(next: boolean) {
    view.showSelectionStatOverlays = next
  }

  function setGaugeSumVisible(next: SvelteSet<string>) {
    view.gaugeSumVisible = next
  }

  function setHistogramVisible(next: SvelteSet<string>) {
    view.histogramVisible = next
  }

  function toggleTimeseriesVisible(key: string, checked: boolean) {
    const streamId = getMetric()?.id
    let pool = timeseriesChartColors
    const assigned = new Map(view.timeseriesColorByKey)
    if (checked) {
      if (acquireColor(pool, assigned, key) === null) {
        if (!isHistogramKind) return
        pool = categoricalPalette(
          Math.max(pool.length, assigned.size + 1, legendOrderKeys.length),
          metricTypeStem(metricType),
          themeSignal.value
        )
        if (acquireColor(pool, assigned, key) === null) return
      }
    } else {
      releaseColor(assigned, key)
    }
    replaceColorAssignments(assigned)

    if (isHistogramKind) {
      const next = new SvelteSet(view.histogramVisible)
      if (checked) next.add(key)
      else next.delete(key)
      view.histogramVisible = next
      if (streamId) savePersistedTimeseriesVisible(streamId, next)
      return
    }
    const next = new SvelteSet(view.gaugeSumVisible)
    if (checked) next.add(key)
    else next.delete(key)
    view.gaugeSumVisible = next
    if (streamId) savePersistedTimeseriesVisible(streamId, next)
  }

  function clearAllTimeseriesVisible() {
    replaceColorAssignments(new Map())
    const streamId = getMetric()?.id
    if (isHistogramKind) {
      view.histogramVisible = new SvelteSet()
      if (streamId)
        savePersistedTimeseriesVisible(streamId, view.histogramVisible)
      return
    }
    view.gaugeSumVisible = new SvelteSet()
    if (streamId) savePersistedTimeseriesVisible(streamId, view.gaugeSumVisible)
  }

  function onDatapointClick(dp: DataPoint) {
    view.selectionSource = 'detail'
    view.selectedHistogramBucketStart = null
    view.selectedQuantileKey = null
    view.selectedDatapointId = view.selectedDatapointId === dp.id ? null : dp.id
    if (view.selectedDatapointId === null) {
      view.selectionSource = null
    }
    if (isHistogramKind && view.selectedDatapointId !== null) {
      view.activeHistogramTab = 'histogram'
      view.histogramScope = 'bucket'
    } else if (isHistogramKind && view.selectedDatapointId === null) {
      view.histogramScope = 'window'
    }
    if (dp.exemplars.length > 0) {
      if (view.expandedDatapoints.has(dp.id)) {
        view.expandedDatapoints.delete(dp.id)
      } else {
        view.expandedDatapoints.add(dp.id)
      }
    }
    // Back returns to the prior selection. For histograms the pick also
    // moves the tab/scope, so carry those in the same history entry.
    writeMetricUrl('push')
  }

  function onHeatmapSelect(timestampMs: number) {
    let tsNs = BigInt(timestampMs) * 1_000_000n
    const series = heatmapBucketSeries
    if (series) {
      const match = series.find(
        s => Number(s.timestamp / 1_000_000n) === timestampMs
      )
      if (match) tsNs = match.timestamp
    }
    if (view.selectedHistogramBucketStart === tsNs) {
      view.selectedHistogramBucketStart = null
      view.selectedQuantileKey = null
      if (view.selectionSource === 'chart') {
        view.selectionSource = null
      }
      return
    }
    view.selectionSource = 'chart'
    view.selectedHistogramBucketStart = tsNs
    view.selectedQuantileKey = null
  }

  function onChartPointClick(seriesKey: string, clickedAt: Date) {
    if (
      seriesKey === AGG_KEY_SELECTED ||
      seriesKey === AGG_KEY_ALL ||
      seriesKey === AGG_KEY_TOTAL
    ) {
      return
    }
    const m = getMetric()
    if (!m) return
    const ts = m.timeseries.find(t => t.attributesKey === seriesKey)
    if (!ts || ts.datapoints.length === 0) return

    const targetMs = clickedAt.getTime()
    const targetNs = BigInt(targetMs) * 1_000_000n

    for (const dp of ts.datapoints) {
      if (dp.timestamp === targetNs) {
        view.selectionSource = 'chart'
        view.selectedDatapointId = dp.id
        writeMetricUrl('push')
        return
      }
    }

    let best: DataPoint | undefined
    let bestDist = Infinity
    for (const dp of ts.datapoints) {
      const ms = Number(dp.timestamp / 1_000_000n)
      const dist = Math.abs(ms - targetMs)
      if (dist < bestDist) {
        bestDist = dist
        best = dp
      }
    }
    if (best) {
      view.selectionSource = 'chart'
      view.selectedDatapointId = best.id
      writeMetricUrl('push')
    }
  }

  function onQuantileChartPointClick(
    _seriesKey: string,
    clickedAt: Date,
    quantileKey: string | null = null
  ) {
    let tsNs = BigInt(clickedAt.getTime()) * 1_000_000n
    const series = heatmapBucketSeries
    if (series) {
      const match = series.find(
        s => Number(s.timestamp / 1_000_000n) === clickedAt.getTime()
      )
      if (match) tsNs = match.timestamp
    }
    if (quantileKey === null) {
      // Plot/tooltip click: same bucket toggles off (heatmap parity).
      if (view.selectedHistogramBucketStart === tsNs) {
        view.selectedHistogramBucketStart = null
        view.selectedQuantileKey = null
        if (view.selectionSource === 'chart') {
          view.selectionSource = null
        }
        return
      }
    } else if (
      view.selectedHistogramBucketStart === tsNs &&
      view.selectedQuantileKey === quantileKey
    ) {
      view.selectedHistogramBucketStart = null
      view.selectedQuantileKey = null
      if (view.selectionSource === 'chart') {
        view.selectionSource = null
      }
      return
    }
    view.selectionSource = 'chart'
    view.selectedHistogramBucketStart = tsNs
    view.selectedQuantileKey = quantileKey
  }

  const ctx: MetricViewContext = {
    get metric() {
      return getMetric()
    },
    get metricType() {
      return metricType
    },
    get temporality() {
      return temporality
    },
    get isMonotonic() {
      return isMonotonic
    },
    get isHistogramKind() {
      return isHistogramKind
    },
    get isUnspecifiedTemporality() {
      return isUnspecifiedTemporality
    },
    get totalDatapointCount() {
      return totalDatapointCount
    },

    get selectedDatapointId() {
      return view.selectedDatapointId
    },
    get selectionSource() {
      return view.selectionSource
    },
    get expandedDatapoints() {
      return view.expandedDatapoints
    },
    get expandedTimeseries() {
      return view.expandedTimeseries
    },
    get activeHistogramTab() {
      return view.activeHistogramTab
    },
    get histogramScope() {
      return view.histogramScope
    },
    get selectedDatapoint() {
      return selectedDatapoint
    },

    get gaugeSumSeriesKeys() {
      return gaugeSumGroups.keys
    },
    get gaugeSumLegendTimeseries() {
      return gaugeSumLegendTimeseries
    },
    get gaugeSumVisible() {
      return view.gaugeSumVisible
    },
    get highlightedTimestamp() {
      return highlightedTimestamp
    },
    get selectedSeriesKey() {
      return selectedSeriesKey
    },

    get aggregationView() {
      return view.aggregationView
    },
    get availableAggregationViews() {
      return availableAggregationViewsList
    },
    get transformedGaugeSumChartTimeseries() {
      return transformedGaugeSumChartTimeseries
    },
    get aggregatePresentKeys() {
      return aggregatePresentKeys
    },
    get showAllSeriesAggregate() {
      return view.showAllSeriesAggregate
    },
    get showAllSeriesAggregateToggleVisible() {
      return showAllSeriesAggregateToggleVisible
    },
    get showSelectionStatOverlays() {
      return view.showSelectionStatOverlays
    },
    get showChartStatOverlaysToggleVisible() {
      return showChartStatOverlaysToggleVisible
    },
    get rateSlopeOverlayAvailable() {
      return rateSlopeOverlayAvailable
    },
    get selectedRateSlope() {
      return selectedRateSlope
    },
    get chartDataTimeRange() {
      return chartDataTimeRange
    },
    get sumResetIndicesByKey() {
      return sumResetIndicesByKey
    },

    get histogramLegendTimeseries() {
      return histogramLegendTimeseries
    },
    get histogramTimeseriesCount() {
      return histogramTimeseriesGroups.length
    },
    get histogramVisible() {
      return view.histogramVisible
    },
    get histogramAxisFitToData() {
      return histogramAxisWindow.fitToData
    },
    seedForMetric,
    seriesDatapoints(seriesKey: string) {
      return getSeriesDatapoints(seriesKey)
    },
    get heatmapBucketSeries() {
      return heatmapBucketSeries
    },
    get bucketSeriesError() {
      return histogramAggregation.error
    },
    get aggregatedDatapoint() {
      return aggregatedDatapoint
    },
    get aggregatedError() {
      return histogramAggregation.aggregatedError
    },
    get histogramChartDatapoint() {
      return histogramChartDatapoint
    },
    get histogramChartError() {
      return histogramChartError
    },
    get activeHistogramDp() {
      return activeHistogramDp
    },
    get heatmapSelectedTimestamp() {
      return heatmapSelectedTimestamp
    },
    get selectedQuantileKey() {
      return view.selectedQuantileKey
    },
    get heatmapColumnSelection() {
      return heatmapColumnSelection
    },
    get quantilePointSelection() {
      return quantilePointSelection
    },
    get quantileChartTimeseries() {
      return quantileChartTimeseries
    },
    get quantileColorByKey() {
      return quantileColorByKey
    },
    get activeQuantileOverlays() {
      return view.activeQuantileOverlays
    },

    get filteredTimeseries() {
      return filteredTimeseries
    },
    get timeseriesColorByKey() {
      if (view.aggregationView === 'raw') return view.timeseriesColorByKey
      const merged = new Map(view.timeseriesColorByKey)
      merged.set(AGG_KEY_SELECTED, AGG_COLOR_SELECTED)
      merged.set(AGG_KEY_ALL, AGG_COLOR_ALL)
      merged.set(AGG_KEY_TOTAL, AGG_COLOR_SELECTED)
      return merged
    },
    get timeseriesChartColors() {
      return timeseriesChartColors
    },
    get legendFilterActive() {
      return legendFilterActive
    },
    get sparklineByKey() {
      return sparklinePointsByKey
    },
    get seriesStatsByKey() {
      return seriesStatsByKey
    },
    get availableSeriesStatBadges() {
      return availableSeriesStatBadgesList
    },

    toggleTimeseriesExpanded,
    setActiveHistogramTab,
    setHistogramScope,
    setAggregationView,
    setShowAllSeriesAggregate,
    setShowSelectionStatOverlays,
    setGaugeSumVisible,
    setHistogramVisible,
    toggleTimeseriesVisible,
    clearAllTimeseriesVisible,
    onDatapointClick,
    onHeatmapSelect,
    onChartPointClick,
    onQuantileChartPointClick,
    setActiveQuantileOverlay,
  }

  setContext(KEY, ctx)
  return ctx
}

export function getMetricViewContext(): MetricViewContext {
  return getContext<MetricViewContext>(KEY)
}

function histogramAggregationErrorToBucketSeriesError(
  err: HistogramAggregationError
): BucketSeriesError {
  return err
}
