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
  MetricType,
  DataPoint,
  HistogramDataPoint,
  ExponentialHistogramDataPoint,
  Attributes,
} from '@/types/api-types'
import { timeseriesToChartTimeseries } from '@/components/metrics/utils/chart-projection'
import { distinguishingResourceAttributes } from '@/utils/series-labels'
import {
  buildHistogramTimeMergedSeries,
  heatmapColumnsForWidth,
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
  aggregateRate,
  aggregateRaw,
  aggregateSelectedAndAll,
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

// Per-series LTTB budget for the line chart. Below this count the raw
// samples are rendered; above it we downsample keeping first+last and
// picking the most visually significant point per bucket.
const CHART_POINTS_PER_SERIES = 2000

/**
 * How many time buckets to ask the store to reduce a window to.
 *
 * Matched to the chart's point budget so the fetch and the draw cannot drift
 * apart: asking for fewer buckets than the chart can render throws away detail
 * that would have been visible, and asking for many more ships datapoints the
 * chart will immediately discard.
 *
 * The store keeps up to four points per bucket, so the response can hold more
 * points than this -- and the client's LTTB pass still runs, as a no-op when
 * the input already fits.
 */
export const METRIC_BUCKET_TARGET = CHART_POINTS_PER_SERIES

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
  readonly gaugeSumChartTimeseries: ReturnType<
    typeof timeseriesToChartTimeseries
  >['chartTimeseries']
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
  /** Post-view chart points per Series tab row, keyed by `attributesKey`.
   *  Shared source for sparklines and row stat badges. Reflects the
   *  current AggregationView (rate vs raw bucketing). Covers every
   *  candidate series, not just visible ones. Empty for histogram /
   *  unspecified temporality. */
  readonly sparklineByKey: ReadonlyMap<string, readonly ChartPoint[]>
  /** Min / max / avg / (sometimes) total per row, from {@link sparklineByKey}. */
  readonly seriesStatsByKey: ReadonlyMap<string, SeriesStats>
  /** Which stat badges TimeseriesPanel should render for this metric/view. */
  readonly availableSeriesStatBadges: readonly SeriesStat[]

  // -- Methods --
  /** Toggle per-timeseries expansion (TimeseriesPanel chevron). */
  toggleTimeseriesExpanded(key: string): void
  /** Reported by the heatmap once its plot area is laid out, and on resize.
   *  The column count follows the chart rather than a constant. */
  reportHeatmapWidth(px: number): void
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
  return buckets.map(b => ({
    kind: 'expHistogram' as const,
    timestamp: BigInt(b.timestamp),
    attributesKey: '',
    scale: b.scale,
    zeroThreshold: b.zeroThreshold,
    zeroCount: b.zeroCount,
    positiveOffset: b.positiveBucketOffset,
    positiveCounts: b.positiveBucketCounts,
    negativeOffset: b.negativeBucketOffset,
    negativeCounts: b.negativeBucketCounts,
    totals: {
      count: b.count,
      sum: b.sum,
      // Derived from the buckets server-side; a merge cannot carry the
      // originals through.
      min: b.min,
      max: b.max,
    },
  }))
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
  getAggregateSummary: () => JsonAggregateBucket | null = () => null
): MetricViewContext {
  const timeContext = getTimeContext()

  // The ONE per-metric mutable cell. Reset by the effect below when
  // the metric identity changes; otherwise written only by methods
  // on this context.
  // The heatmap's measured plot width, in pixels. 0 until it lays out, which
  // heatmapColumnsForWidth reads as "not measured yet".
  let heatmapWidthPx = $state(0)

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
  const gaugeSumGroups = $derived.by(() => {
    const m = getMetric()
    if (!m || (metricType !== 'Gauge' && metricType !== 'Sum')) {
      return { chartTimeseries: [], keys: [] as string[] }
    }
    return timeseriesToChartTimeseries(m.timeseries, {
      downsampleTo: CHART_POINTS_PER_SERIES,
    })
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
  //     lines (Selected, All) via aggregateSelectedAndAll(). Collapse
  //     rules: selected empty → one "All" line; selected covers all →
  //     one "Total" line; otherwise 2.

  const SUM_AUTO_BUCKET_COUNT_CAP = 120

  /** Shared bucket count for rate-view raw lines AND cross-series
   *  aggregates so their staircases align.
   *
   *  Mirrors `combinePool`'s formula: target ≈ allPoints / poolSize
   *  (= average points per series), capped at the chart-resolution
   *  cap. When per-series `aggregateRate` and pooled aggregate both
   *  receive this as `bucketCount`, they end up with the same N
   *  buckets over (effectively) the same time span — the visible
   *  series all share the metric's scrape cadence in practice.
   *
   *  Only meaningful when aggregationView is aggregated; raw view ignores
   *  bucketCount entirely. */
  const sharedBucketCount = $derived.by((): number => {
    const all = gaugeSumGroups.chartTimeseries
    if (all.length === 0) return SUM_AUTO_BUCKET_COUNT_CAP
    let total = 0
    for (const s of all) total += s.points.length
    if (total === 0) return SUM_AUTO_BUCKET_COUNT_CAP
    const target = Math.ceil(total / Math.max(all.length, 1))
    return Math.min(SUM_AUTO_BUCKET_COUNT_CAP, Math.max(1, target))
  })

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
    const all = gaugeSumGroups.chartTimeseries
    if (all.length === 0)
      return {
        series: [] as ChartTimeseries[],
        resets: new Map() as ResetIndicesByKey,
      }
    const visible = all.filter(s => view.gaugeSumVisible.has(s.key))
    const cumulative =
      metricType === 'Sum' && isCumulativeTemporality(temporality)
    if (view.aggregationView === 'rate') {
      return aggregateRate(visible, {
        cumulative,
        bucketCount: sharedBucketCount,
      })
    }
    return aggregateRaw(visible, {
      cumulative,
      bucketCount: SUM_AUTO_BUCKET_COUNT_CAP,
    })
  })

  /** Post-view chart points for each Series tab row (`attributesKey`).
   *
   *  Runs over EVERY candidate series (not just visible) so unchecked
   *  rows still get a shape and stats to scan.
   *
   *  Transform follows the current aggregationView:
   *    - 'rate'                  → per-series rate (matches the main
   *                                chart's per-series rate transform).
   *    - 'raw' / 'sum' / 'avg'   → bucketed raw values. Sum/Avg are
   *                                cross-series aggregations that
   *                                don't apply per row, so a row's
   *                                own line stays in raw units.
   *
   *  Histogram metrics return an empty map — TimeseriesPanel renders
   *  a placeholder slot for histogram rows until per-series sparkbar
   *  data is wired up. Sparklines and stat badges both read this map. */
  const seriesRowPointsByKey = $derived.by(
    (): ReadonlyMap<string, readonly ChartPoint[]> => {
      if (isHistogramKind) return new Map()
      // Unspecified temporality means we can't tell whether the values
      // are running totals or per-interval counts -- the same numbers
      // mean two very different lines depending on which it is. The
      // main chart blanks itself + shows UnspecifiedTemporalityCallout
      // for the same reason; row projections should follow that lead
      // rather than guessing.
      if (isUnspecifiedTemporality) return new Map()
      const all = gaugeSumGroups.chartTimeseries
      if (all.length === 0) return new Map()
      const cumulative =
        metricType === 'Sum' && isCumulativeTemporality(temporality)
      const transformed =
        view.aggregationView === 'rate'
          ? aggregateRate(all, { cumulative, bucketCount: sharedBucketCount })
          : aggregateRaw(all, {
              cumulative,
              bucketCount: SUM_AUTO_BUCKET_COUNT_CAP,
            })
      const out = new Map<string, readonly ChartPoint[]>()
      for (const s of transformed.series) out.set(s.key, s.points)
      return out
    }
  )

  const seriesStatsByKey = $derived.by((): ReadonlyMap<string, SeriesStats> => {
    const out = new Map<string, SeriesStats>()

    // In the raw view the store's numbers win, because they are the only
    // correct ones: seriesStatsFromPoints runs over chart points, which have
    // already been thinned to CHART_POINTS_PER_SERIES, so its avg is the mean
    // of a sample and its total is short by the thinning factor -- and `total`
    // is offered as a badge for Sum + Delta + raw.
    //
    // Derived views (sum / avg / rate) keep computing client-side: those points
    // are transformed rather than sampled, so their stats describe the
    // transform, and no server-side equivalent would match.
    const raw = view.aggregationView === 'raw'
    const fromServer = new Map<string, SeriesStats>()
    if (raw) {
      for (const ts of getMetric()?.timeseries ?? []) {
        const st = ts.stats
        if (!st) continue
        fromServer.set(ts.attributesKey, {
          min: st.min,
          max: st.max,
          avg: st.avg,
          total: st.sum,
        })
      }
    }

    for (const [key, points] of seriesRowPointsByKey) {
      out.set(key, fromServer.get(key) ?? seriesStatsFromPoints(points))
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

  /** Aggregated mode: Selected + All cross-timeseries lines. */
  const aggregatedTransformed = $derived.by((): AggregateResult => {
    const all = gaugeSumGroups.chartTimeseries
    if (all.length === 0) return { lines: [], presentKeys: [] }
    const selected = all.filter(s => view.gaugeSumVisible.has(s.key))
    const v = view.aggregationView as 'sum' | 'avg' | 'rate'
    const opts = {
      cumulative: metricType === 'Sum' && isCumulativeTemporality(temporality),
      bucketCount: sharedBucketCount,
    }
    return aggregateSelectedAndAll(selected, all, v, opts)
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
    if (gaugeSumGroups.keys.length < 2) return false
    const all = gaugeSumGroups.chartTimeseries
    if (all.length === 0) return false
    const selectedCount = all.filter(s =>
      view.gaugeSumVisible.has(s.key)
    ).length
    // All series checked → aggregate collapses to Total; nothing extra
    // to toggle.
    return selectedCount !== all.length
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

  const chartDataTimeRange = $derived.by(
    (): { startMs: number; endMs: number } | undefined => {
      if (isHistogramKind) {
        const qr = selectionToQueryRangeMs(timeContext.selection, Date.now())
        return { startMs: qr.start, endMs: qr.end }
      }
      if (metricType === 'Gauge' || metricType === 'Sum') {
        let min = Infinity
        let max = -Infinity
        for (const ts of gaugeSumGroups.chartTimeseries) {
          for (const p of ts.points) {
            const t = p.date.getTime()
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

    const startNs = BigInt(queryRange.start) * 1_000_000n
    const endNs = BigInt(queryRange.end) * 1_000_000n
    const perAttribute = buildHistogramTimeMergedSeries(
      m.timeseries,
      startNs,
      endNs,
      // Was a hardcoded 100, which matched nothing the chart had measured --
      // the plot binds its own clientWidth and nothing read it.
      heatmapColumnsForWidth(heatmapWidthPx),
      temporality,
      // Align columns to whichever clock the axis is labelled in, so a
      // day-scale bucket breaks where the reader expects the day to break.
      timeContext.tz
    )
    if ('kind' in perAttribute) {
      const err = histogramAggregationErrorToBucketSeriesError(perAttribute)
      return { ...empty, error: err, aggregatedError: err }
    }

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

  // Badge counts raw datapoints in the current window (same source as
  // the inline expanded table in TimeseriesPanel and as the Gauge/Sum
  // branch above), NOT heatmap time buckets.
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
      histogramVisibleKeys
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

  // -- Effects (the only mutating side-channels) --

  // (1) Reset per-metric view state when the metric identity changes.
  // Reading metric.id (not the object) ties the effect to the right
  // dependency; internal updates to the metric (e.g. polling) won't
  // fire this. Visible keys are restored from localStorage (per metric
  // stream id) when possible.
  $effect(() => {
    const m = getMetric()
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
    view.aggregationView =
      persistedAggregationView ??
      defaultAggregationViewFor(
        metricType,
        temporality,
        isMonotonic,
        gaugeSumGroups.keys.length
      )
    view.showSelectionStatOverlays = true
    view.showAllSeriesAggregate = streamId
      ? loadPersistedShowAllSeriesAggregate(streamId)
      : false
    view.activeQuantileOverlays = new SvelteSet([
      DEFAULT_ACTIVE_HISTOGRAM_QUANTILE_KEY,
    ])

    const gsKeys = gaugeSumGroups.keys
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
    replaceColorAssignments(seedColorAssignments(pool, gsVisible, gsKeys))
    // Histogram visible is re-seeded by a separate effect when series
    // keys are known. Do not clear colour assignments here -- that
    // would wipe the gauge/sum seed we just wrote above.
    histogramVisibleSeededForStreamId = null
    view.histogramVisible = new SvelteSet()

    // URL > the per-metric defaults set above. Runs on metric change so a
    // shared deep link (or back/forward into this metric) restores the
    // sub-view, validated against the freshly loaded metric. `untrack` keeps
    // the router query out of this effect's deps — otherwise a time-window
    // write (start/end) would re-trigger the whole per-metric reset.
    untrack(() => {
      applyMetricUrlToView(false)
    })
  })

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

  // (2) Seed histogram visibility once per stream when series keys arrive.
  // Do not use size === 0 as "unseeded" — an empty set is valid after the
  // user unchecks every series.
  $effect(() => {
    const m = getMetric()
    const streamId = m?.id
    const keys = histogramTimeseriesGroups.map(g => g.key)
    if (!streamId || keys.length === 0) return
    if (histogramVisibleSeededForStreamId === streamId) return
    const histVisible = new SvelteSet(
      resolveTimeseriesVisible(keys, streamId, DEFAULT_VISIBLE_TIMESERIES, null)
    )
    view.histogramVisible = histVisible
    const pool = categoricalPalette(
      Math.max(keys.length, 1),
      metricTypeStem(metricType),
      themeSignal.value
    )
    replaceColorAssignments(seedColorAssignments(pool, histVisible, keys))
    histogramVisibleSeededForStreamId = streamId
  })

  // (2b) Same metric stream, new telemetry: prune stale attribute keys and
  // re-resolve if the visible set is empty.
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
      const needsInitialSeed =
        view.gaugeSumVisible.size === 0 && keys.length > 0
      const next = needsInitialSeed
        ? resolveTimeseriesVisible(keys, streamId)
        : reconcileTimeseriesVisible(view.gaugeSumVisible, keys, streamId)

      // A series named in the URL has to end up drawn, and this is the first
      // point where that can be decided: effect (1) applies the URL before
      // gaugeSumGroups.keys have settled, so the id could not be validated
      // there and anything it revealed would be rebuilt away by the reconcile
      // above. Doing it here, against known keys, is what makes a deep link to
      // series 40 of 63 actually show series 40 rather than the default first
      // ten.
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

  // Only meaningful changes are recorded: heatmapColumnsForWidth quantises, so
  // writing every pixel of a drag would invalidate the derivation without
  // changing its answer.
  function reportHeatmapWidth(px: number) {
    if (heatmapColumnsForWidth(px) === heatmapColumnsForWidth(heatmapWidthPx)) {
      return
    }
    heatmapWidthPx = px
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

    get gaugeSumChartTimeseries() {
      return gaugeSumGroups.chartTimeseries
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
      return seriesRowPointsByKey
    },
    get seriesStatsByKey() {
      return seriesStatsByKey
    },
    get availableSeriesStatBadges() {
      return availableSeriesStatBadgesList
    },

    toggleTimeseriesExpanded,
    reportHeatmapWidth,
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
