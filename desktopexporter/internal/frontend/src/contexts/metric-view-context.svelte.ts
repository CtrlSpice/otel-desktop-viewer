/*
 * Shared chart and detail state for one metric view. Mutable choices live in
 * one state cell; computed values derive from that state and the current metric.
 * The metric getter keeps the context identity stable across navigation.
 */
import { createContext, untrack } from 'svelte'
import { SvelteSet } from 'svelte/reactivity'
import type {
  MetricViewData,
  MetricSeriesViewData,
  AggregateBucket,
  MetricType,
  DataPoint,
  HistogramDataPoint,
  ExponentialHistogramDataPoint,
  Attributes,
  ScalarViewBucket,
  ScalarAggregate,
  SparklinePoint,
} from '@/types/api-types'
import {
  rateBucketStartForSourceDatapoint,
  timeseriesToChartTimeseries,
} from '@/components/metrics/utils/chart-projection'
import {
  distinguishingResourceAttributes,
  seriesLabelsByKey,
} from '@/utils/series-labels'
import {
  seriesBucketsToSlices,
  buildVisibleSeriesQuantileChartTimeseries,
  DEFAULT_ACTIVE_HISTOGRAM_QUANTILE_KEY,
  DEFAULT_HISTOGRAM_QUANTILES,
  histogramDatapointToChartDatapoint,
  histogramSliceToChartDatapoint,
  parseQuantileSeriesKey,
  quantileKeyFromValue,
  type HistogramAggregationError,
  type HistogramChartDataPoint,
  type HistogramSlicePoint,
} from '@/components/metrics/utils/histogram-aggregation'
import {
  heatmapColumnSelectionAt,
  heatmapColumnEndNs as columnEndNs,
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
  availableSeriesStatBadges,
  isAggregateLineKey,
  rateSlopeBucketSegment,
  availableRateSlopeOverlay,
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
  remapColorAssignments,
  releaseColor,
  seedColorAssignments,
  syncColorAssignments,
  type TimeseriesColorByKey,
} from '@/components/metrics/utils/metric-timeseries-colors'
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

const [getMetricViewContext, setMetricViewContext] =
  createContext<MetricViewContext>()

const CHART_POINTS_PER_SERIES = 2000

/** Store buckets, each contributing up to four chart points. */
export const METRIC_BUCKET_TARGET = CHART_POINTS_PER_SERIES / 4

/** Maximum store buckets for Sum, Average and Rate views. */
export const SCALAR_VIEW_BUCKETS = 120

/** Row sparkline width in pixels. */
export const SPARKLINE_WIDTH_PX = 128

/** Sparkline buckets, each contributing minimum and maximum points. */
export const SPARKLINE_BUCKETS = SPARKLINE_WIDTH_PX / 2

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

type MetricViewState = {
  selectedDatapointID: string | null
  selectedSeriesKey: string | null
  selectionSource: 'chart' | 'detail' | null
  expandedDatapoints: SvelteSet<string>
  expandedTimeseries: SvelteSet<string>
  activeHistogramTab: HistogramTab
  histogramScope: HistogramScope
  selectedHistogramBucketStart: bigint | null
  selectedQuantileKey: string | null
  visibleSeries: SvelteSet<string>
  timeseriesColorByKey: TimeseriesColorByKey
  aggregationView: AggregationView
  showAllSeriesAggregate: boolean
  showSelectionStatOverlays: boolean
  activeQuantileOverlays: SvelteSet<string>
}

type TransformedSeries = {
  series: ChartTimeseries[]
  resets: ResetIndicesByKey
}

type HistogramAggregationResult = {
  perAttribute: HistogramSlicePoint[]
  heatmap: HistogramSlicePoint[]
  summary: HistogramSlicePoint | null
  error: BucketSeriesError | null
  aggregatedError: BucketSeriesError | null
}

export interface MetricViewContext {
  readonly metric: MetricViewData | undefined
  readonly metricType: MetricType
  readonly temporality: string
  readonly temporalityCode: number | null
  readonly isMonotonic: boolean | null
  readonly isHistogramKind: boolean
  readonly isUnsafeTemporality: boolean
  readonly totalDatapointCount: number

  readonly selectedDatapointID: string | null
  /** Where the current datapoint selection came from. Chart clicks
   *  drive the plot overlay only; detail-pane scroll/expand waits for
   *  unified routing. */
  readonly selectionSource: 'chart' | 'detail' | null
  readonly expandedDatapoints: SvelteSet<string>
  /** Per-timeseries expansion (keyed by seriesRef). Used by the
   * TimeseriesPanel to reveal an inline datapoints table under a
   * row. Independent of expandedDatapoints (which keys per-datapoint
   * exemplar expansion within SeriesDatapointList). */
  readonly expandedTimeseries: SvelteSet<string>
  readonly activeHistogramTab: HistogramTab
  readonly histogramScope: HistogramScope
  readonly selectedDatapoint: DataPoint | undefined

  /** Series keys in store order, without chart-point projection. */
  readonly gaugeSumSeriesKeys: readonly string[]
  readonly gaugeSumLegendTimeseries: LegendTimeseries[]
  /** Exact timestamp of the displayed selected point. Raw views use the source
   * datapoint timestamp; Rate uses the synthetic bucket containing that source. */
  readonly highlightedTimestamp: bigint | null
  /** Attributes key of the timeseries owning `selectedDatapoint`, or
   *  `null` when nothing is selected. Used by the chart to draw the
   *  selection dot in the right series's color. */
  readonly selectedSeriesKey: string | null

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

  readonly histogramLegendTimeseries: LegendTimeseries[]
  readonly histogramTimeseriesCount: number
  /** Series checked in the legend. Empty means none for filterable metrics. */
  readonly visibleSeries: SvelteSet<string>
  /** Resets and seeds view state before the metric renders. */
  seedForMetric(metric: MetricViewData | undefined): void
  /** Received series datapoints, or undefined until fetched. */
  seriesDatapoints(seriesKey: string): DataPoint[] | undefined
  readonly heatmapBucketSeries: HistogramSlicePoint[] | null
  readonly bucketSeriesError: BucketSeriesError | null
  readonly aggregatedDatapoint: HistogramChartDataPoint | undefined
  readonly aggregatedError: BucketSeriesError | null
  readonly histogramChartDatapoint: HistogramChartDataPoint | undefined
  readonly histogramChartError: BucketSeriesError | null
  readonly activeHistogramDp:
    HistogramDataPoint | ExponentialHistogramDataPoint | undefined
  /** Selected column bounds. End is one nanosecond before the next column. */
  readonly heatmapColumnStartNs: bigint | null
  readonly heatmapColumnEndNs: bigint | null
  /** Quantile line key (e.g. `"0.95"`) when selection came from a quantile chart point click. */
  readonly selectedQuantileKey: string | null
  readonly heatmapColumnSelection: HeatmapColumnSelection | null
  readonly quantilePointSelection: QuantilePointSelection | null
  readonly quantileChartTimeseries: ChartTimeseries[]
  readonly quantileColorByKey: TimeseriesColorByKey
  readonly activeQuantileOverlays: SvelteSet<string>

  readonly filteredTimeseries: MetricViewData['timeseries']
  /** Checked timeseries → colour from the stem-rotated pool. Unchecked rows
   *  have no entry; their checkbox uses neutral. */
  readonly timeseriesColorByKey: TimeseriesColorByKey
  /** Stem-rotated 10-colour pool (`pool[0]` = metric-type stem). */
  readonly timeseriesChartColors: string[]
  readonly legendFilterActive: boolean
  /** Sparkline points per Series tab row, keyed by `seriesRef`, reduced by
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
  /** Toggle chart visibility and persist immediately. */
  toggleTimeseriesVisible(key: string, checked: boolean): void
  /** Uncheck every timeseries and release all colour assignments. */
  clearAllTimeseriesVisible(): void
  /** Toggle selection + (optionally) expansion + jump to the
   * histogram tab in bucket scope. Used by the detail view. */
  onDatapointClick(dp: DataPoint): void
  /** Heatmap column click: toggle the exact selected time bucket. */
  onHeatmapSelect(timestampNs: bigint): void
  /** Time-series chart point click: select one exact source datapoint. */
  onChartPointClick(seriesKey: string, datapointID: string): void
  /** Quantiles tab: toggle sticky bucket selection at the clicked x. */
  onQuantileChartPointClick(
    seriesKey: string,
    timestampNs: bigint,
    quantileKey?: string | null
  ): void
  /** Clear only selection owned by a chart interaction. Detail-pane datapoint
   * selection remains intact when a histogram cursor clears its transient cell. */
  clearChartSelection(): void
  setActiveQuantileOverlay(quantileKey: string): void
}

/**
 * Turn the store's aggregate buckets into the slice shape the heatmap draws.
 *
 * Every number is already merged. The store owns
 * scale alignment, zero-threshold folding and the vector sums, so this only
 * maps field names and widens the counts to numbers.
 */
export function aggregateToSlices(
  buckets: AggregateBucket[] | null
): HistogramSlicePoint[] {
  if (!buckets) return []
  return buckets.map(b => {
    const totals = {
      count: b.count,
      sum: b.sum ?? undefined,
      // Derived from the buckets server-side; a merge cannot carry the
      // originals through.
      min: b.min,
      max: b.max,
    }
    // Explicit and exponential bucket fields are mutually exclusive.
    if (b.explicitBounds !== undefined || b.bucketCounts !== undefined) {
      return {
        kind: 'histogram' as const,
        timestamp: BigInt(b.timestamp),
        seriesRef: '',
        bounds: b.explicitBounds ?? [],
        counts: b.bucketCounts ?? [],
        totals,
        quantiles: b.quantiles ?? null,
      }
    }
    return {
      kind: 'expHistogram' as const,
      timestamp: BigInt(b.timestamp),
      seriesRef: '',
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
  getMetric: () => MetricViewData | undefined,
  /** Store-computed aggregate for the current legend selection. */
  getAggregate: () => AggregateBucket[] | null = () => null,
  /** The same merge over a single bucket spanning the window. */
  getAggregateSummary: () => AggregateBucket | null = () => null,
  /** Store-computed scalar folds for checked and full series pools. */
  getScalarAggregate: () => ScalarAggregate | null = () => null,
  /** Received datapoints for one series, once fetched. */
  getSeriesDatapoints: (seriesKey: string) => DataPoint[] | undefined = () =>
    undefined,
  /** Per-series merges over the selected heatmap column, once fetched. */
  getColumnDistribution: () => MetricViewData | undefined = () => undefined
): MetricViewContext {
  // The only mutable per-metric state cell.
  const view = $state<MetricViewState>({
    selectedDatapointID: null,
    // Explicitly chosen series, independent of any datapoint selection.
    selectedSeriesKey: null,
    selectionSource: null,
    expandedDatapoints: new SvelteSet<string>(),
    expandedTimeseries: new SvelteSet<string>(),
    activeHistogramTab: 'heatmap',
    histogramScope: 'window',
    selectedHistogramBucketStart: null,
    selectedQuantileKey: null,
    visibleSeries: new SvelteSet<string>(),
    timeseriesColorByKey: new Map<string, string>(),
    aggregationView: 'raw',
    showAllSeriesAggregate: false,
    showSelectionStatOverlays: true,
    activeQuantileOverlays: new SvelteSet([
      DEFAULT_ACTIVE_HISTOGRAM_QUANTILE_KEY,
    ]),
  })

  // The route stores stable sub-view choices; heatmap column selection is transient.
  // Compare route changes with this snapshot because defaults need not be in the URL.

  let urlMetricViewSnapshot: MetricViewQuery | null = null

  // An absent `agg` restores the seeded aggregation.
  let seededAggregationView: AggregationView = 'raw'

  type PendingUrlDatapoint = { id: string; seriesKey: string }

  // A raw-only datapoint cannot be validated until its URL-named series has
  // been fetched. Keep that identity private until it resolves so chart
  // selection continues to see either one exact datapoint or no datapoint.
  let pendingUrlDatapoint = $state<PendingUrlDatapoint | null>(null)
  // Once a loaded series proves a pending id absent, suppress that same URL
  // request until navigation leaves it. A later cache refresh must not make a
  // datapoint appear selected without a new navigation event.
  let rejectedUrlDatapoint: PendingUrlDatapoint | null = null

  function metricParseContext(): MetricViewParseContext {
    const index = selectableDatapointIndex
    return {
      isHistogramKind,
      allowedAggs: availableAggregationViewsList,
      datapointIDs: index.datapointIDs,
      seriesKeys: index.seriesKeys,
    }
  }

  type ParsedMetricUrl = {
    query: MetricViewQuery
    pending: PendingUrlDatapoint | null
  }

  function samePendingDatapoint(
    a: PendingUrlDatapoint | null,
    b: PendingUrlDatapoint | null
  ): boolean {
    return (
      a !== null && b !== null && a.id === b.id && a.seriesKey === b.seriesKey
    )
  }

  /** Parse the route while allowing a datapoint whose named series is loading. */
  function parseMetricUrl(): ParsedMetricUrl {
    const routeQuery = readRoute().query
    const parsed = parseMetricViewQuery(routeQuery, metricParseContext())
    const requestedID = routeQuery.dp || null
    const rawRequest =
      requestedID && parsed.series
        ? { id: requestedID, seriesKey: parsed.series }
        : null

    if (
      rejectedUrlDatapoint &&
      !samePendingDatapoint(rejectedUrlDatapoint, rawRequest)
    ) {
      rejectedUrlDatapoint = null
    }

    if (!requestedID) return { query: parsed, pending: null }

    if (samePendingDatapoint(rejectedUrlDatapoint, rawRequest)) {
      return { query: { ...parsed, dp: null }, pending: null }
    }

    const index = selectableDatapointIndex
    const owner = index.seriesKeyByDatapointID.get(requestedID)
    if (owner !== undefined) {
      const ownedByUrlSeries = parsed.series === null || parsed.series === owner
      return {
        query: { ...parsed, dp: ownedByUrlSeries ? requestedID : null },
        pending: null,
      }
    }

    if (!rawRequest || index.loadedRawSeries.has(rawRequest.seriesKey)) {
      return { query: { ...parsed, dp: null }, pending: null }
    }

    return {
      query: { ...parsed, dp: rawRequest.id },
      pending: rawRequest,
    }
  }

  function viewStateToMetricViewQuery(): MetricViewQuery {
    const datapointID =
      view.selectedDatapointID ?? pendingUrlDatapoint?.id ?? null
    if (isHistogramKind) {
      return {
        kind: 'histogram',
        htab: view.activeHistogramTab,
        hscope: view.histogramScope,
        dp: datapointID,
        series: selectedSeriesKey,
      }
    }
    return {
      kind: 'timeseries',
      // Write raw explicitly so it differs from an absent aggregation choice.
      agg: view.aggregationView,
      dp: datapointID,
      series: selectedSeriesKey,
    }
  }

  /** Applies the validated route sub-view; absent `agg` restores the seeded value. */
  function applyMetricUrlToView(parsed = parseMetricUrl()): void {
    const q = parsed.query
    urlMetricViewSnapshot = q
    pendingUrlDatapoint = parsed.pending

    if (q.kind === 'timeseries') {
      view.aggregationView = q.agg ?? seededAggregationView
    } else {
      view.activeHistogramTab = q.htab
      view.histogramScope = q.hscope
    }

    if (q.dp && selectableDatapointIndex.datapointByID.has(q.dp)) {
      view.selectedDatapointID = q.dp
      view.selectionSource = 'detail'
    } else {
      view.selectedDatapointID = null
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
    // it. At the scalar cap, bounded reveal replaces the last current series.
    if (q.series) revealSeries(q.series)

    // This is the fetch trigger for a raw-only URL datapoint. TimeseriesPanel
    // opens the nested datapoint section after the exact id resolves.
    if (parsed.pending) view.expandedTimeseries.add(parsed.pending.seriesKey)
  }

  /**
   * Ensures a series is in the visible set for its metric kind.
   *
   * @param key - series id
   *
   * @remarks
   * Adds when there is capacity. At the scalar cap, replaces the last current
   * series so a shared link remains drawable without changing saved choices.
   */
  function revealSeries(key: string): void {
    if (view.visibleSeries.has(key)) return

    const visible = new SvelteSet(view.visibleSeries)
    const assigned = new Map(view.timeseriesColorByKey)
    if (!isHistogramKind && visible.size >= MAX_VISIBLE_TIMESERIES) {
      const evicted = [...visible].at(-1)
      if (evicted) {
        visible.delete(evicted)
        releaseColor(assigned, evicted)
      }
    }

    if (acquireColor(timeseriesChartColors, assigned, key) === null) return
    visible.add(key)
    view.visibleSeries = visible
    replaceColorAssignments(assigned)
  }

  function writeMetricUrl(mode: HistoryMode): void {
    const q = viewStateToMetricViewQuery()
    urlMetricViewSnapshot = q
    setMetricViewQuery(q, mode)
  }

  const metricType = $derived.by(
    (): MetricType => getMetric()?.metricType ?? 'Empty'
  )

  function* allDatapoints(
    m: MetricViewData | undefined
  ): IterableIterator<DataPoint> {
    if (!m) return
    for (const ts of m.timeseries) {
      for (const dp of ts.datapoints) yield dp
    }
  }

  function datapointTemporality(dp: DataPoint): string | undefined {
    switch (dp.metricType) {
      case 'Sum':
      case 'Histogram':
      case 'ExponentialHistogram':
        return dp.aggregationTemporality
      case 'Gauge':
        return undefined
    }
  }

  /** Selection index over chart rows and fetched received rows. Received rows win. */
  const selectableDatapointIndex = $derived.by(() => {
    const datapointByID = new Map<string, DataPoint>()
    const seriesKeyByDatapointID = new Map<string, string>()
    const datapointIDs = new Set<string>()
    const seriesKeys = new Set<string>()
    const loadedRawSeries = new Set<string>()
    const m = getMetric()

    if (m) {
      for (const ts of m.timeseries) {
        const seriesKey = ts.seriesRef
        seriesKeys.add(seriesKey)

        for (const datapoint of ts.datapoints) {
          datapointByID.set(datapoint.id, datapoint)
          seriesKeyByDatapointID.set(datapoint.id, seriesKey)
          datapointIDs.add(datapoint.id)
        }

        const raw = getSeriesDatapoints(seriesKey)
        if (raw === undefined) continue
        loadedRawSeries.add(seriesKey)
        for (const datapoint of raw) {
          datapointByID.set(datapoint.id, datapoint)
          seriesKeyByDatapointID.set(datapoint.id, seriesKey)
          datapointIDs.add(datapoint.id)
        }
      }
    }

    return {
      datapointByID,
      seriesKeyByDatapointID,
      datapointIDs,
      seriesKeys,
      loadedRawSeries,
    }
  })

  const temporality = $derived.by(() => {
    const m = getMetric()
    if (m?.aggregationTemporality) return m.aggregationTemporality
    for (const dp of allDatapoints(m)) {
      const t = datapointTemporality(dp)
      if (t) return t
    }
    return ''
  })

  const temporalityCode = $derived.by((): number | null => {
    return getMetric()?.aggregationTemporalityCode ?? null
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

  const isUnsafeTemporality = $derived.by(() => {
    if (
      metricType !== 'Histogram' &&
      metricType !== 'ExponentialHistogram' &&
      metricType !== 'Sum'
    ) {
      return false
    }
    return temporalityCode !== 1 && temporalityCode !== 2
  })

  // Use the store's window count because the response may narrow or reduce datapoints.
  const totalDatapointCount = $derived(getMetric()?.datapointCount ?? 0)

  /** Series keys used to seed visibility without projecting chart points. */
  const gaugeSumKeys = $derived.by((): string[] => {
    const m = getMetric()
    if (!m || (metricType !== 'Gauge' && metricType !== 'Sum')) return []
    return m.timeseries.map(ts => ts.seriesRef)
  })

  /** Scalar series metadata and on-demand chart-point projection. */
  const gaugeSumGroups = $derived.by(() => {
    const m = getMetric()
    const scalar = metricType === 'Gauge' || metricType === 'Sum'
    const source = m && scalar ? m.timeseries : []

    const labelByKey = seriesLabelsByKey(source)
    const labelFor = (ts: MetricSeriesViewData) =>
      labelByKey.get(ts.seriesRef) ?? ts.seriesRef

    return {
      timeseries: source,
      keys: source.map(ts => ts.seriesRef),
      labels: source.map(ts => ({
        key: ts.seriesRef,
        label: labelFor(ts),
      })),
      /** Project visible series in store order without further thinning. */
      projectVisible: (visible: { has: (key: string) => boolean }) =>
        timeseriesToChartTimeseries(
          source.filter(ts => visible.has(ts.seriesRef)),
          labelFor
        ).chartTimeseries,
    }
  })

  const gaugeSumLegendTimeseries = $derived.by((): LegendTimeseries[] => {
    const m = getMetric()
    if (!m) return []
    const distinguishing = distinguishingResourceAttributes(m.timeseries)
    return m.timeseries.map(ts => ({
      key: ts.seriesRef,
      attributes: [
        ...ts.attributes,
        ...(distinguishing.get(ts.seriesRef) ?? []),
      ],
    }))
  })

  /**
   * The store's per-bucket answer for one view, as chart points.
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
      // Rate points carry the store's slope for the tangent overlay; the
      // other views have no tangent and their points stay two fields.
      const slope = view === 'rate' ? (b.slope ?? null) : undefined
      if (value === null) {
        if (b.sampleCount > 0) continue
        if (view === 'avg') continue
        out.push({
          date: new Date(Number(b.bucketStart / 1_000_000n)),
          value: 0,
          timestampNs: b.bucketStart,
          slope,
        })
        continue
      }
      out.push({
        date: new Date(Number(b.bucketStart / 1_000_000n)),
        value,
        timestampNs: b.bucketStart,
        slope,
      })
    }
    return out
  }

  /** Projects store-reduced sparkline samples at their received timestamps. */
  function sparklinePoints(points: SparklinePoint[] | null): ChartPoint[] {
    if (!points) return []
    return points.map(p => ({
      date: new Date(Number(p.timestamp / 1_000_000n)),
      value: p.value,
      timestampNs: p.timestamp,
    }))
  }

  /** Per-series lines for a store-computed view, plus the buckets whose counter
   *  restarted -- which the rate chart marks. */
  function seriesFromViews(
    series: readonly { key: string; label: string }[],
    view: 'sum' | 'avg' | 'rate'
  ): TransformedSeries {
    const m = getMetric()
    const resets: ResetIndicesByKey = new Map()
    if (!m) return { series: [], resets }
    const byKey = new Map(m.timeseries.map(ts => [ts.seriesRef, ts.views]))
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

  /** Visible raw series, or store-computed per-series rates in Rate view. */
  const rawTransformed = $derived.by((): TransformedSeries => {
    if (gaugeSumGroups.keys.length === 0)
      return {
        series: [],
        resets: new Map<string, number[]>(),
      }
    if (view.aggregationView === 'rate') {
      return seriesFromViews(
        gaugeSumGroups.labels.filter(s => view.visibleSeries.has(s.key)),
        'rate'
      )
    }
    // Sum and Average are cross-series views; per-series lines stay raw.
    return {
      series: gaugeSumGroups.projectVisible(view.visibleSeries),
      resets: new Map<string, number[]>(),
    }
  })

  /** Store-reduced row sparklines for every scalar series. Rate view uses rate buckets. */
  const sparklinePointsByKey = $derived.by(
    (): ReadonlyMap<string, readonly ChartPoint[]> => {
      if (isHistogramKind) return new Map()
      // Unsafe temporality cannot distinguish running totals from interval counts.
      if (isUnsafeTemporality) return new Map()
      const out = new Map<string, readonly ChartPoint[]>()
      const rate = view.aggregationView === 'rate'
      for (const ts of getMetric()?.timeseries ?? []) {
        out.set(
          ts.seriesRef,
          rate ? viewPoints(ts.views, 'rate') : sparklinePoints(ts.sparkline)
        )
      }
      return out
    }
  )

  const seriesStatsByKey = $derived.by((): ReadonlyMap<string, SeriesStats> => {
    const out = new Map<string, SeriesStats>()
    if (isHistogramKind || isUnsafeTemporality) return out

    // Raw, Sum and Average use whole-window source stats; Rate uses rate-line stats.
    if (view.aggregationView === 'rate') {
      for (const ts of getMetric()?.timeseries ?? []) {
        const rs = ts.rateStats
        if (!rs) continue
        out.set(ts.seriesRef, { min: rs.min, max: rs.max, avg: rs.avg })
      }
      return out
    }

    for (const ts of getMetric()?.timeseries ?? []) {
      const st = ts.stats
      if (!st) continue
      out.set(ts.seriesRef, {
        min: st.min,
        max: st.max,
        avg: st.avg,
        total: st.sum,
      })
    }
    return out
  })

  const availableSeriesStatBadgesList = $derived.by((): SeriesStat[] => {
    if (isHistogramKind || isUnsafeTemporality) return []
    return availableSeriesStatBadges({
      metricType,
      temporality,
      aggregationView: view.aggregationView,
    })
  })

  /** Store-folded Selected and All lines. Equal pools render once as Total. */
  const aggregatedTransformed = $derived.by((): AggregateResult => {
    const pools = getScalarAggregate()
    if (!pools) return { lines: [], presentKeys: [] }
    const v = view.aggregationView
    if (v === 'raw') return { lines: [], presentKeys: [] }
    const allPoints = viewPoints(pools.all, v)
    if (allPoints.length === 0) return { lines: [], presentKeys: [] }

    const checkedCount = view.visibleSeries.size
    const total = gaugeSumGroups.keys.length
    // Empty selection shows All; full selection shows the same pool as Total.
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

  /** Aggregate legend keys after duplicate-line and visibility rules. */
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
    const keys = gaugeSumGroups.keys
    if (keys.length < 2) return false
    let selectedCount = 0
    for (const key of keys) {
      if (view.visibleSeries.has(key)) selectedCount++
    }
    return selectedCount !== keys.length
  })

  const showChartStatOverlaysToggleVisible = $derived.by((): boolean => {
    return metricType === 'Gauge' || metricType === 'Sum'
  })

  const rateSlopeOverlayAvailable = $derived.by((): boolean => {
    if (isHistogramKind || isUnsafeTemporality) return false
    return availableRateSlopeOverlay({
      metricType,
      temporality,
      isMonotonic,
      aggregationView: view.aggregationView,
    })
  })

  /** Effective store window. Null endpoints mean there is no chart domain. */
  const chartDataTimeRange = $derived.by(
    (): { startMs: number; endMs: number } | undefined => {
      const effective = getMetric()?.window.effective
      if (
        !effective ||
        effective.startNs === null ||
        effective.endNs === null ||
        effective.endNs < effective.startNs
      ) {
        return undefined
      }
      const startMs = Number(effective.startNs / 1_000_000n)
      const endMs = Number(effective.endNs / 1_000_000n)
      return { startMs, endMs: Math.max(startMs + 1, endMs) }
    }
  )

  /** Display series with raw lines first and aggregates aligned to their bucket grid. */
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

  /** Reset markers are shown only in raw mode. */
  const sumResetIndicesByKey = $derived.by((): ResetIndicesByKey => {
    if (view.aggregationView !== 'raw') return new Map()
    return rawTransformed.resets
  })

  /** Which AggregationView options the dropdown should offer. Driven by metric
   *  type + shape + series count. See availableAggregationViews() in
   *  aggregation.ts for the rules. */
  const availableAggregationViewsList = $derived.by((): AggregationView[] => {
    if (isUnsafeTemporality) return ['raw']
    return availableAggregationViews(
      metricType,
      temporality,
      isMonotonic,
      gaugeSumGroups.keys.length
    )
  })

  const selectedDatapoint = $derived.by((): DataPoint | undefined => {
    const id = view.selectedDatapointID
    return id === null
      ? undefined
      : selectableDatapointIndex.datapointByID.get(id)
  })

  const highlightedTimestamp = $derived.by((): bigint | null => {
    const dp = selectedDatapoint
    if (!dp || (dp.metricType !== 'Gauge' && dp.metricType !== 'Sum')) {
      return null
    }
    if (view.aggregationView !== 'rate') return dp.timestamp

    const datapointID = view.selectedDatapointID
    if (datapointID === null) return null
    const owner =
      selectableDatapointIndex.seriesKeyByDatapointID.get(datapointID)
    const series = getMetric()?.timeseries.find(
      candidate => candidate.seriesRef === owner
    )
    return series
      ? (rateBucketStartForSourceDatapoint(series, datapointID) ?? null)
      : null
  })

  /** Series owning the selected datapoint, or the explicitly selected series. */
  const selectedSeriesKey = $derived.by((): string | null => {
    const m = getMetric()
    if (!m) return null

    // An exact datapoint selection takes precedence over a series selection.
    const id = view.selectedDatapointID
    if (id !== null) {
      const owner = selectableDatapointIndex.seriesKeyByDatapointID.get(id)
      if (owner !== undefined) return owner
    }

    // Otherwise fall back to an explicitly chosen series. This is what a link
    // restores once its datapoint has aged out: the point is gone, the line is
    // still there, and the user still lands on it.
    if (view.selectedSeriesKey !== null) {
      const known = m.timeseries.some(
        ts => ts.seriesRef === view.selectedSeriesKey
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
    const point = series.points.find(
      candidate => candidate.timestampNs === highlightedTimestamp
    )
    return point
      ? rateSlopeBucketSegment(series.points, point)?.slope
      : undefined
  })

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
        best = dp
      }
    }
    return best
  })

  const histogramTimeseriesGroups = $derived.by(
    (): HistogramTimeseriesGroup[] => {
      const m = getMetric()
      if (!m || !isHistogramKind) return []
      // Use the store's window count because datapoints may be narrowed out.
      return m.timeseries.map(ts => ({
        key: ts.seriesRef,
        attributes: ts.attributes,
        pointCount: ts.datapointCount,
      }))
    }
  )

  const histogramAggregation = $derived.by((): HistogramAggregationResult => {
    const m = getMetric()
    const empty = {
      perAttribute: [],
      heatmap: [],
      summary: null,
      error: null,
      aggregatedError: null,
    }
    if (!m || !isHistogramKind) return empty
    if (isUnsafeTemporality) {
      const err = histogramAggregationErrorToBucketSeriesError({
        kind: 'unspecified',
        message: `Aggregation temporality is ${temporality || 'unknown'}`,
      })
      return { ...empty, error: err, aggregatedError: err }
    }

    // Report incompatible bounds only when no merged buckets survived.
    const mismatch = m.boundsMismatch
    if (mismatch && m.timeseries.every(ts => ts.datapoints.length === 0)) {
      const err = histogramAggregationErrorToBucketSeriesError({
        kind: 'boundsMismatch',
        message:
          `Histogram bounds disagree across datapoints in this window ` +
          `(${mismatch.seriesBuckets} series buckets, ` +
          `${mismatch.aggregateBuckets} aggregate buckets could not be merged)`,
      })
      return { ...empty, error: err, aggregatedError: err }
    }

    // The store has already bucketed, merged and resolved temporality.
    const perAttribute = seriesBucketsToSlices(m.timeseries)

    // Null means the aggregate is loading or no series is selected.
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
    (): HistogramChartDataPoint | undefined => {
      const m = getMetric()
      const summary = histogramAggregation.summary
      if (!m || !summary || temporalityCode === null) return undefined
      return histogramSliceToChartDatapoint(
        summary,
        `${m.metricRef}:aggregated`,
        temporality,
        temporalityCode
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
        return dp
      }
      return undefined
    }
  )

  const histogramChartDatapoint = $derived.by(
    (): HistogramChartDataPoint | undefined => {
      if (view.histogramScope === 'window') return aggregatedDatapoint
      return histogramBucketDatapoint
        ? histogramDatapointToChartDatapoint(histogramBucketDatapoint)
        : undefined
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

  const heatmapColumnStartNs = $derived(view.selectedHistogramBucketStart)

  /**
   * Selected column end. Uses the next start because local-time columns vary
   * across DST; the final column uses the preceding gap or the window end.
   */
  const heatmapColumnEndNs = $derived.by((): bigint | null => {
    const startNs = view.selectedHistogramBucketStart
    if (startNs === null) return null
    const series = heatmapBucketSeries
    if (!series || series.length === 0) return null
    const windowEndNs = getMetric()?.window.effective.endNs
    if (windowEndNs === null || windowEndNs === undefined) return null
    return columnEndNs(
      series.map(s => s.timestamp),
      startNs,
      windowEndNs
    )
  })

  const heatmapColumnSelection = $derived.by(
    (): HeatmapColumnSelection | null => {
      if (view.selectedHistogramBucketStart === null) return null
      const series = heatmapBucketSeries
      if (!series || series.length === 0) return null
      return heatmapColumnSelectionAt(series, view.selectedHistogramBucketStart)
    }
  )

  const quantilePointSelection = $derived.by(
    (): QuantilePointSelection | null => {
      if (view.selectedHistogramBucketStart === null) return null
      const merged = heatmapBucketSeries
      if (!merged || merged.length === 0) return null
      // Wait for the exact selected-column response.
      const column = getColumnDistribution()
      if (!column) return null
      const columnSlices = seriesBucketsToSlices(column.timeseries)
      if (columnSlices.length === 0) return null
      return quantilePointSelectionAt(
        columnSlices,
        merged,
        view.selectedHistogramBucketStart,
        // All-visible contains every key; none-visible is empty.
        view.visibleSeries
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
      view.visibleSeries,
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

  // An empty set means no visible series only for metric shapes with a legend.
  const hasSeriesFilter = $derived(
    metricType === 'Gauge' || metricType === 'Sum' || isHistogramKind
  )

  const visibleDpCanonicalKeys = $derived.by((): Set<string> =>
    hasSeriesFilter ? view.visibleSeries : new Set()
  )

  const filteredTimeseries = $derived.by(() => {
    const m = getMetric()
    if (!m) return []
    const filter = visibleDpCanonicalKeys
    if (filter === null) return m.timeseries
    return m.timeseries.filter(ts => filter.has(ts.seriesRef))
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
      const n = Math.max(legendOrderKeys.length, view.visibleSeries.size, 1)
      return categoricalPalette(n, stem, theme)
    }
    return categoricalPalette(MAX_VISIBLE_TIMESERIES, stem, theme)
  })

  const legendFilterActive = $derived(visibleDpCanonicalKeys !== null)

  let assignedColorPool: readonly string[] = []

  function replaceColorAssignments(
    next: TimeseriesColorByKey,
    pool: readonly string[] = timeseriesChartColors
  ) {
    view.timeseriesColorByKey = next
    assignedColorPool = [...pool]
  }

  /** Ensure every visible series has a colour assignment. */
  function ensureColorAssignments(
    visible: ReadonlySet<string>,
    legendKeys: readonly string[]
  ) {
    if (visible.size === 0) {
      replaceColorAssignments(new Map())
      return
    }
    // A non-empty map may still contain assignments for a different metric.
    let allAssigned = true
    for (const key of visible) {
      if (!view.timeseriesColorByKey.has(key)) {
        allAssigned = false
        break
      }
    }
    if (allAssigned) return
    // Use the same shape-specific pool as every other assignment path.
    replaceColorAssignments(
      seedColorAssignments(timeseriesChartColors, visible, legendKeys)
    )
  }

  /** Reset and seed every per-metric choice synchronously before rendering. */
  function seedForMetric(m: MetricViewData | undefined) {
    const metricRef = m?.metricRef

    pendingUrlDatapoint = null
    rejectedUrlDatapoint = null
    view.selectedDatapointID = null
    view.selectionSource = null
    view.selectedHistogramBucketStart = null
    view.selectedQuantileKey = null
    view.expandedDatapoints.clear()
    view.expandedTimeseries.clear()
    view.activeHistogramTab = 'heatmap'
    view.histogramScope = 'window'

    // Use an allowed persisted view, otherwise the metric's default.
    const persistedAggregationView = metricRef
      ? loadPersistedAggregationView(metricRef, availableAggregationViewsList)
      : null
    const defaultAggregation = defaultAggregationViewFor(
      metricType,
      temporality,
      isMonotonic,
      gaugeSumKeys.length
    )
    // `raw` is always available when the computed default is not.
    seededAggregationView =
      persistedAggregationView ??
      (availableAggregationViewsList.includes(defaultAggregation)
        ? defaultAggregation
        : 'raw')
    view.aggregationView = seededAggregationView
    view.showSelectionStatOverlays = true
    view.showAllSeriesAggregate = metricRef
      ? loadPersistedShowAllSeriesAggregate(metricRef)
      : false
    view.activeQuantileOverlays = new SvelteSet([
      DEFAULT_ACTIVE_HISTOGRAM_QUANTILE_KEY,
    ])

    // Scalars cap visible series; histogram selection remains uncapped.
    const histIsHistogram =
      m?.metricType === 'Histogram' || m?.metricType === 'ExponentialHistogram'
    if (histIsHistogram) {
      const histKeys = m ? m.timeseries.map(ts => ts.seriesRef) : []
      const histVisible = new SvelteSet(
        metricRef && histKeys.length > 0
          ? resolveTimeseriesVisible(
              histKeys,
              metricRef,
              DEFAULT_VISIBLE_TIMESERIES,
              null
            )
          : []
      )
      view.visibleSeries = histVisible
      const histPool = categoricalPalette(
        Math.max(histKeys.length, 1),
        metricTypeStem(metricType),
        themeSignal.value
      )
      replaceColorAssignments(
        seedColorAssignments(histPool, histVisible, histKeys)
      )
    } else {
      const gsKeys = gaugeSumKeys
      const gsVisible = new SvelteSet(
        metricRef
          ? resolveTimeseriesVisible(gsKeys, metricRef)
          : gsKeys.slice(0, MAX_VISIBLE_TIMESERIES)
      )
      view.visibleSeries = gsVisible
      const pool = categoricalPalette(
        MAX_VISIBLE_TIMESERIES,
        metricTypeStem(metricType),
        themeSignal.value
      )
      replaceColorAssignments(seedColorAssignments(pool, gsVisible, gsKeys))
    }

    // A valid route sub-view overrides seeded defaults.
    applyMetricUrlToView()
  }

  // Apply external route changes without echoing this context's writes.
  $effect(() => {
    const unsubscribe = subscribeToRoute(() => {
      // Registration invokes synchronously; keep callback reads untracked.
      untrack(() => {
        const parsed = parseMetricUrl()
        const fromUrl = parsed.query
        if (
          urlMetricViewSnapshot &&
          metricViewQueriesEqual(fromUrl, urlMetricViewSnapshot)
        ) {
          return
        }
        applyMetricUrlToView(parsed)
      })
    })
    return unsubscribe
  })

  // Remap colours when the theme or shape-specific palette changes.
  $effect(() => {
    const nextPool = timeseriesChartColors
    if (
      assignedColorPool.length === 0 ||
      assignedColorPool.join('\0') === nextPool.join('\0')
    ) {
      return
    }
    replaceColorAssignments(
      remapColorAssignments(
        view.timeseriesColorByKey,
        assignedColorPool,
        nextPool
      ),
      nextPool
    )
  })

  $effect(() => {
    const m = getMetric()
    const metricRef = m?.metricRef
    if (!metricRef) return

    if (metricType === 'Gauge' || metricType === 'Sum') {
      const keys = gaugeSumGroups.keys
      void keys.join('\0')
      const next = reconcileTimeseriesVisible(
        view.visibleSeries,
        keys,
        metricRef
      )

      if (!visibleKeyListsEqual(view.visibleSeries, next)) {
        const visible = new SvelteSet(next)
        view.visibleSeries = visible
        const assigned = new Map(view.timeseriesColorByKey)
        syncColorAssignments(timeseriesChartColors, assigned, visible, keys)
        replaceColorAssignments(assigned)
      } else {
        ensureColorAssignments(view.visibleSeries, keys)
      }
      return
    }

    if (!isHistogramKind) return
    const keys = histogramTimeseriesGroups.map(g => g.key)
    if (keys.length === 0) return
    void keys.join('\0')
    const next = reconcileTimeseriesVisible(
      view.visibleSeries,
      keys,
      metricRef,
      null
    )
    if (!visibleKeyListsEqual(view.visibleSeries, next)) {
      const visible = new SvelteSet(next)
      view.visibleSeries = visible
      const assigned = new Map(view.timeseriesColorByKey)
      syncColorAssignments(timeseriesChartColors, assigned, visible, keys)
      replaceColorAssignments(assigned)
    } else {
      ensureColorAssignments(view.visibleSeries, keys)
    }
  })

  // Resolve a route datapoint once its named received series has loaded.
  $effect(() => {
    const pending = pendingUrlDatapoint
    if (!pending) return

    const raw = getSeriesDatapoints(pending.seriesKey)
    if (raw === undefined) return

    pendingUrlDatapoint = null
    const owner = selectableDatapointIndex.seriesKeyByDatapointID.get(
      pending.id
    )
    const ownerVisible =
      !hasSeriesFilter || view.visibleSeries.has(pending.seriesKey)
    if (owner !== pending.seriesKey || !ownerVisible) {
      rejectedUrlDatapoint = pending
      if (
        urlMetricViewSnapshot?.dp === pending.id &&
        urlMetricViewSnapshot.series === pending.seriesKey
      ) {
        urlMetricViewSnapshot = { ...urlMetricViewSnapshot, dp: null }
      }
      if (
        view.selectedDatapointID === null &&
        view.selectionSource === 'detail'
      ) {
        view.selectionSource = null
      }
      return
    }

    view.selectedDatapointID = pending.id
    // A transient heatmap/quantile selection can coexist with this exact detail
    // datapoint. Preserve its chart ownership until clearChartSelection restores
    // the detail source.
    if (view.selectionSource !== 'chart') view.selectionSource = 'detail'
  })

  // Clear an exact selection when its datapoint disappears or its series is hidden.
  $effect(() => {
    const id = view.selectedDatapointID
    if (id === null) return

    const m = getMetric()
    if (!m) return

    const owner = selectableDatapointIndex.seriesKeyByDatapointID.get(id)
    if (owner === undefined) {
      view.selectedDatapointID = null
      view.selectionSource = null
      return
    }

    // Shapes with no legend filter never seed a selection, and an unseeded
    // empty set would read as "nothing is checked" and clear the selection.
    if (!hasSeriesFilter) return
    if (!view.visibleSeries.has(owner)) {
      view.selectedDatapointID = null
      view.selectionSource = null
    }
  })

  // Replace an aggregation choice that the current metric no longer supports.
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
    // Persist the replacement so the invalid choice is not restored.
    const metricRef = getMetric()?.metricRef
    if (metricRef) savePersistedAggregationView(metricRef, next)
  })

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
    const metricRef = getMetric()?.metricRef
    if (metricRef) savePersistedAggregationView(metricRef, next)
    writeMetricUrl('replace')
  }

  function setShowAllSeriesAggregate(next: boolean) {
    view.showAllSeriesAggregate = next
    const metricRef = getMetric()?.metricRef
    if (metricRef) savePersistedShowAllSeriesAggregate(metricRef, next)
  }

  function setActiveQuantileOverlay(quantileKey: string) {
    view.activeQuantileOverlays = new SvelteSet([quantileKey])
  }

  function setShowSelectionStatOverlays(next: boolean) {
    view.showSelectionStatOverlays = next
  }

  function toggleTimeseriesVisible(key: string, checked: boolean) {
    const metricRef = getMetric()?.metricRef
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

    const next = new SvelteSet(view.visibleSeries)
    if (checked) next.add(key)
    else next.delete(key)
    view.visibleSeries = next
    if (metricRef) savePersistedTimeseriesVisible(metricRef, next)
  }

  function clearAllTimeseriesVisible() {
    replaceColorAssignments(new Map())
    const metricRef = getMetric()?.metricRef
    view.visibleSeries = new SvelteSet()
    if (metricRef) savePersistedTimeseriesVisible(metricRef, view.visibleSeries)
  }

  function onDatapointClick(dp: DataPoint) {
    pendingUrlDatapoint = null
    rejectedUrlDatapoint = null
    view.selectionSource = 'detail'
    view.selectedHistogramBucketStart = null
    view.selectedQuantileKey = null
    view.selectedDatapointID = view.selectedDatapointID === dp.id ? null : dp.id
    if (view.selectedDatapointID === null) {
      view.selectionSource = null
    }
    if (isHistogramKind && view.selectedDatapointID !== null) {
      view.activeHistogramTab = 'histogram'
      view.histogramScope = 'bucket'
    } else if (isHistogramKind && view.selectedDatapointID === null) {
      view.histogramScope = 'window'
    }
    if (dp.flags > 0 || dp.exemplars.length > 0) {
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

  function onHeatmapSelect(timestampNs: bigint) {
    if (view.selectedHistogramBucketStart === timestampNs) {
      view.selectedHistogramBucketStart = null
      view.selectedQuantileKey = null
      if (view.selectionSource === 'chart') restoreDetailSelectionSource()
      return
    }
    view.selectionSource = 'chart'
    view.selectedHistogramBucketStart = timestampNs
    view.selectedQuantileKey = null
  }

  function onChartPointClick(seriesKey: string, datapointID: string) {
    if (isAggregateLineKey(seriesKey)) return
    const m = getMetric()
    if (!m) return
    const ts = m.timeseries.find(t => t.seriesRef === seriesKey)
    if (!ts?.datapoints.some(datapoint => datapoint.id === datapointID)) return

    pendingUrlDatapoint = null
    rejectedUrlDatapoint = null
    view.selectionSource = 'chart'
    view.selectedDatapointID = datapointID
    writeMetricUrl('push')
  }

  function onQuantileChartPointClick(
    _seriesKey: string,
    timestampNs: bigint,
    quantileKey: string | null = null
  ) {
    if (quantileKey === null) {
      // Plot/tooltip click: same bucket toggles off (heatmap parity).
      if (view.selectedHistogramBucketStart === timestampNs) {
        view.selectedHistogramBucketStart = null
        view.selectedQuantileKey = null
        if (view.selectionSource === 'chart') restoreDetailSelectionSource()
        return
      }
    } else if (
      view.selectedHistogramBucketStart === timestampNs &&
      view.selectedQuantileKey === quantileKey
    ) {
      view.selectedHistogramBucketStart = null
      view.selectedQuantileKey = null
      if (view.selectionSource === 'chart') restoreDetailSelectionSource()
      return
    }
    view.selectionSource = 'chart'
    view.selectedHistogramBucketStart = timestampNs
    view.selectedQuantileKey = quantileKey
  }

  function clearChartSelection() {
    const hadHistogramSelection =
      view.selectedHistogramBucketStart !== null ||
      view.selectedQuantileKey !== null
    if (hadHistogramSelection) {
      view.selectedHistogramBucketStart = null
      view.selectedQuantileKey = null
      if (view.selectionSource === 'chart') {
        // A heatmap/quantile pick can coexist with a raw histogram datapoint
        // selected in the detail pane. Clearing the transient chart cell must
        // reveal that selection again rather than erase it.
        restoreDetailSelectionSource()
      }
      return
    }

    if (view.selectionSource !== 'chart') return
    const hadDatapointSelection = view.selectedDatapointID !== null
    view.selectedDatapointID = null
    view.selectionSource = null
    if (hadDatapointSelection) writeMetricUrl('push')
  }

  function restoreDetailSelectionSource() {
    view.selectionSource = view.selectedDatapointID === null ? null : 'detail'
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
    get temporalityCode() {
      return temporalityCode
    },
    get isMonotonic() {
      return isMonotonic
    },
    get isHistogramKind() {
      return isHistogramKind
    },
    get isUnsafeTemporality() {
      return isUnsafeTemporality
    },
    get totalDatapointCount() {
      return totalDatapointCount
    },

    get selectedDatapointID() {
      return view.selectedDatapointID
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
    get visibleSeries() {
      return view.visibleSeries
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
    get heatmapColumnStartNs() {
      return heatmapColumnStartNs
    },
    get heatmapColumnEndNs() {
      return heatmapColumnEndNs
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
    toggleTimeseriesVisible,
    clearAllTimeseriesVisible,
    onDatapointClick,
    onHeatmapSelect,
    onChartPointClick,
    onQuantileChartPointClick,
    clearChartSelection,
    setActiveQuantileOverlay,
  }

  setMetricViewContext(ctx)
  return ctx
}

function histogramAggregationErrorToBucketSeriesError(
  err: HistogramAggregationError
): BucketSeriesError {
  return err
}

export { getMetricViewContext }
