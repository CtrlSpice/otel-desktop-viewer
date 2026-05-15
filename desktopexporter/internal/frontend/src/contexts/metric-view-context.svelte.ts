/*
 * MetricViewContext: per-metric reactive state that BOTH the chart
 * view (main) and the detail view (Fields/Datapoints) need to read
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
 *   - The bucket-series fetch (Histogram / ExponentialHistogram) is
 *     owned here too, because both panes care about its result and
 *     the chart's tab state is tied to its loading state.
 *   - `$effect` is used in exactly two places: (1) reset per-metric
 *     view state when the metric identity changes; (2) drive the
 *     bucket-series fetch. Everywhere else is pure derivation.
 *
 * The factory takes a getter for the current `metric` rather than a
 * value, so the context object's identity stays stable for the page
 * lifetime even as the user navigates between metrics. (The
 * underlying `selectedMetric` cell lives on MetricsPage.)
 */
import { setContext, getContext } from 'svelte'
import { SvelteSet } from 'svelte/reactivity'
import type {
  MetricData,
  MetricType,
  DataPoint,
  HistogramDataPoint,
  ExponentialHistogramDataPoint,
  BucketSeriesPoint,
  HistogramBucketPoint,
  ExpHistogramBucketPoint,
  Attributes,
} from '@/types/api-types'
import {
  telemetryAPI,
  JsonRpcError,
  ErrCodeUnspecifiedTemporality,
  ErrCodeHistogramBoundsMismatch,
} from '@/services/telemetry-service'
import { timeseriesToChartTimeseries } from '@/components/MetricCharts/MetricTimeSeriesChart.svelte'
import type { Timeseries as LegendTimeseries } from '@/components/MetricCharts/TimeseriesLegend.svelte'
import { MAX_VISIBLE_TIMESERIES } from '@/utils/timeseries-palette'
import {
  getTimeContext,
  selectionToQueryRangeMs,
} from '@/contexts/time-context.svelte'

const KEY = 'metric-view'

// --- Types --------------------------------------------------------

export type HistogramTab = 'heatmap' | 'aggregated' | 'snapshot'

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
  readonly expandedDatapoints: SvelteSet<string>
  /** Per-timeseries expansion (keyed by attributesKey). Used by the
   * TimeseriesPanel to reveal an inline datapoints table under a
   * row. Independent of expandedDatapoints (which keys per-datapoint
   * exemplar expansion in the legacy detail tab). */
  readonly expandedTimeseries: SvelteSet<string>
  readonly activeHistogramTab: HistogramTab
  readonly selectedDatapoint: DataPoint | undefined

  // -- Gauge/Sum chart wiring --
  readonly gaugeSumChartTimeseries: ReturnType<
    typeof timeseriesToChartTimeseries
  >['chartTimeseries']
  readonly gaugeSumLegendTimeseries: LegendTimeseries[]
  readonly gaugeSumVisible: SvelteSet<string>
  readonly highlightedTimestamp: bigint | null

  // -- Histogram chart wiring --
  readonly histogramLegendTimeseries: LegendTimeseries[]
  readonly histogramTimeseriesCount: number
  readonly histogramVisible: SvelteSet<string>
  readonly visibleBucketSeries: BucketSeriesPoint[] | null
  readonly bucketSeriesLoading: boolean
  readonly bucketSeriesError: BucketSeriesError | null
  readonly aggregatedDatapoint:
    | HistogramDataPoint
    | ExponentialHistogramDataPoint
    | undefined
  readonly aggregatedLoading: boolean
  readonly aggregatedError: BucketSeriesError | null
  readonly activeHistogramDp:
    | HistogramDataPoint
    | ExponentialHistogramDataPoint
    | undefined
  readonly heatmapSelectedTimestamp: number | null

  // -- Detail view wiring --
  readonly filteredTimeseries: MetricData['timeseries']
  readonly timeseriesColorIndex: Map<string, number>
  readonly legendFilterActive: boolean

  // -- Methods --
  /** Toggle per-timeseries expansion (TimeseriesPanel chevron). */
  toggleTimeseriesExpanded(key: string): void
  setActiveHistogramTab(tab: HistogramTab): void
  /** Replace the visible-set for the Gauge/Sum legend. The legend
   * keeps a `bind:visibleKeys` model; we expose a setter so the
   * sole writer is still us. */
  setGaugeSumVisible(next: SvelteSet<string>): void
  setHistogramVisible(next: SvelteSet<string>): void
  /** Toggle selection + (optionally) expansion + force the snapshot
   * tab on histograms. Single entry point used by both the chart
   * (heatmap clicks) and the detail view (datapoint row clicks). */
  onDatapointClick(dp: DataPoint): void
  /** Heatmap clicks land on a bucket-start ms; resolve to a real
   * datapoint inside the bucket window. */
  onHeatmapSelect(timestampMs: number): void
}

// --- Factory ------------------------------------------------------

export function createMetricViewContext(
  getMetric: () => MetricData | undefined
): MetricViewContext {
  const timeContext = getTimeContext()

  // The ONE per-metric mutable cell. Reset by the effect below when
  // the metric identity changes; otherwise written only by methods
  // on this context.
  const view = $state({
    selectedDatapointId: null as string | null,
    expandedDatapoints: new SvelteSet<string>(),
    expandedTimeseries: new SvelteSet<string>(),
    activeHistogramTab: 'heatmap' as HistogramTab,
    gaugeSumVisible: new SvelteSet<string>(),
    histogramVisible: new SvelteSet<string>(),
  })

  const bucketState = $state({
    bucketSeries: null as BucketSeriesPoint[] | null,
    bucketSeriesLoading: false,
    bucketSeriesError: null as BucketSeriesError | null,
    aggregatedPoint: null as BucketSeriesPoint | null,
    aggregatedLoading: false,
    aggregatedError: null as BucketSeriesError | null,
  })

  // -- Pure derivations of `metric` --
  const metricType = $derived<MetricType>(
    getMetric()?.timeseries[0]?.datapoints[0]?.metricType ?? 'Empty'
  )

  function* allDatapoints(
    m: MetricData | undefined
  ): IterableIterator<DataPoint> {
    if (!m) return
    for (const ts of m.timeseries) {
      for (const dp of ts.datapoints) yield dp
    }
  }

  const temporality = $derived.by(() => {
    for (const dp of allDatapoints(getMetric())) {
      const t = (dp as { aggregationTemporality?: string }).aggregationTemporality
      if (t) return t
    }
    return ''
  })

  const isMonotonic = $derived.by((): boolean | null => {
    if (metricType !== 'Sum') return null
    for (const dp of allDatapoints(getMetric())) {
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
      const t = (dp as { aggregationTemporality?: string }).aggregationTemporality
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
    return timeseriesToChartTimeseries(m.timeseries)
  })

  const gaugeSumLegendTimeseries = $derived.by((): LegendTimeseries[] => {
    const m = getMetric()
    if (!m) return []
    return m.timeseries.map(ts => ({
      key: ts.attributesKey,
      attributes: ts.attributes,
      badge: `${ts.datapoints.length} dp${ts.datapoints.length === 1 ? '' : 's'}`,
    }))
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

  const activeHistogramDp = $derived.by(() => {
    const dp = selectedDatapoint
    if (
      dp &&
      (dp.metricType === 'Histogram' || dp.metricType === 'ExponentialHistogram')
    ) {
      return dp as HistogramDataPoint | ExponentialHistogramDataPoint
    }
    return latestHistogramDp
  })

  const heatmapSelectedTimestamp = $derived.by((): number | null => {
    const dp = selectedDatapoint ?? latestHistogramDp
    if (!dp) return null
    return Number(dp.timestamp / 1_000_000n)
  })

  const histogramTimeseriesGroups = $derived.by(
    (): HistogramTimeseriesGroup[] => {
      const series = bucketState.bucketSeries
      if (!series || series.length === 0) return []
      const byKey = new Map<string, HistogramTimeseriesGroup>()
      const order: string[] = []
      for (const pt of series) {
        const key = pt.attributesKey
        const existing = byKey.get(key)
        if (existing) {
          existing.pointCount += 1
        } else {
          byKey.set(key, { key, attributes: pt.attributes, pointCount: 1 })
          order.push(key)
        }
      }
      return order.map(k => byKey.get(k)!)
    }
  )

  // Badge counts raw datapoints in the current window (same source as
  // the inline expanded table in TimeseriesPanel and as the Gauge/Sum
  // branch above), NOT bucket-series points -- the latter is bounded
  // by the heatmap step grid (~100 buckets) and would diverge from
  // what the user sees when they expand the row. We still walk the
  // bucket series for the timeseries *order* (newest-active first,
  // matching the heatmap row order); per-key counts come from the
  // metric record. Timeseries with no raw datapoints in the window
  // are still listed (count 0) so the row count matches the heatmap.
  const histogramLegendTimeseries = $derived.by((): LegendTimeseries[] => {
    const m = getMetric()
    if (!m) return []
    const datapointsByKey = new Map<string, number>()
    for (const ts of m.timeseries) {
      datapointsByKey.set(ts.attributesKey, ts.datapoints.length)
    }
    return histogramTimeseriesGroups.map(g => {
      const count = datapointsByKey.get(g.key) ?? 0
      return {
        key: g.key,
        attributes: g.attributes,
        badge: `${count} dp${count === 1 ? '' : 's'}`,
      }
    })
  })

  const visibleBucketSeries = $derived.by(() => {
    const series = bucketState.bucketSeries
    if (!series) return null
    if (view.histogramVisible.size === histogramTimeseriesGroups.length) {
      return series
    }
    return series.filter(pt => view.histogramVisible.has(pt.attributesKey))
  })

  const aggregatedDatapoint = $derived.by(():
    | HistogramDataPoint
    | ExponentialHistogramDataPoint
    | undefined => {
    const point = bucketState.aggregatedPoint
    const m = getMetric()
    if (!point || !m) return undefined
    const id = `${m.id}:aggregated`
    if (point.kind === 'histogram') {
      const p = point as HistogramBucketPoint
      return {
        id,
        metricType: 'Histogram',
        timestamp: p.timestamp,
        startTime: p.timestamp,
        flags: 0,
        exemplars: [],
        count: p.totals.count,
        sum: p.totals.sum,
        min: p.totals.min ?? 0,
        max: p.totals.max ?? 0,
        explicitBounds: p.bounds,
        bucketCounts: p.counts,
        aggregationTemporality: temporality || 'Delta',
      }
    }
    const p = point as ExpHistogramBucketPoint
    return {
      id,
      metricType: 'ExponentialHistogram',
      timestamp: p.timestamp,
      startTime: p.timestamp,
      flags: 0,
      exemplars: [],
      count: p.totals.count,
      sum: p.totals.sum,
      min: p.totals.min ?? 0,
      max: p.totals.max ?? 0,
      scale: p.scale,
      zeroCount: p.zeroCount,
      zeroThreshold: p.zeroThreshold,
      positiveBucketOffset: p.positiveOffset,
      positiveBucketCounts: p.positiveCounts,
      negativeBucketOffset: p.negativeOffset,
      negativeBucketCounts: p.negativeCounts,
      aggregationTemporality: temporality || 'Delta',
    }
  })

  // -- Detail-view wiring (legend filter coupling) --
  const visibleDpCanonicalKeys = $derived.by((): Set<string> | null => {
    if (metricType === 'Gauge' || metricType === 'Sum') {
      if (view.gaugeSumVisible.size === gaugeSumGroups.keys.length) return null
      return view.gaugeSumVisible
    }
    if (isHistogramKind) {
      if (view.histogramVisible.size === histogramTimeseriesGroups.length) {
        return null
      }
      return view.histogramVisible
    }
    return null
  })

  const filteredTimeseries = $derived.by(() => {
    const m = getMetric()
    if (!m) return []
    const filter = visibleDpCanonicalKeys
    if (filter === null) return m.timeseries
    return m.timeseries.filter(ts => filter.has(ts.attributesKey))
  })

  const timeseriesColorIndex = $derived.by((): Map<string, number> => {
    const idx = new Map<string, number>()
    if (metricType === 'Gauge' || metricType === 'Sum') {
      gaugeSumLegendTimeseries.forEach((ts, i) => idx.set(ts.key, i))
    } else if (isHistogramKind) {
      histogramTimeseriesGroups.forEach((g, i) => idx.set(g.key, i))
    }
    return idx
  })

  const legendFilterActive = $derived(visibleDpCanonicalKeys !== null)

  // -- Effects (the only mutating side-channels) --

  // (1) Reset per-metric view state when the metric identity changes.
  // Reading metric.id (not the object) ties the effect to the right
  // dependency; internal updates to the metric (e.g. polling) won't
  // fire this. The visible-key sets are also re-seeded here from the
  // first-N-keys-of-the-new-metric default.
  $effect(() => {
    const m = getMetric()
    void m?.id

    view.selectedDatapointId = null
    view.expandedDatapoints.clear()
    view.expandedTimeseries.clear()
    view.activeHistogramTab = 'heatmap'

    const gsKeys = gaugeSumGroups.keys
    view.gaugeSumVisible = new SvelteSet(
      gsKeys.slice(0, MAX_VISIBLE_TIMESERIES)
    )
    // Histogram visible is re-seeded by a separate effect because its
    // candidate keys come from bucketSeries (asynchronous), not from
    // the metric directly.
    view.histogramVisible = new SvelteSet()
  })

  // (2) Re-seed histogram visible-set whenever the bucket-series fetch
  // returns. We can't seed it in the metric-identity effect because
  // bucketSeries arrives later.
  $effect(() => {
    const keys = histogramTimeseriesGroups.map(g => g.key)
    if (keys.length === 0) return
    if (view.histogramVisible.size > 0) return
    view.histogramVisible = new SvelteSet(keys.slice(0, MAX_VISIBLE_TIMESERIES))
  })

  // (3) Bucket-series fetch. Per-attribute (full breakdown for the
  // heatmap legend) AND merged single-bucket (for the Aggregated tab)
  // are issued in parallel because they share the same window but
  // differ only in maxPoints. cancelled flag stops late responses
  // from a prior metric clobbering the current state.
  $effect(() => {
    const m = getMetric()
    const id = m?.id
    const t = metricType
    const start = queryRange.start
    const end = queryRange.end

    if (!id || (t !== 'Histogram' && t !== 'ExponentialHistogram')) {
      bucketState.bucketSeries = null
      bucketState.bucketSeriesError = null
      bucketState.bucketSeriesLoading = false
      bucketState.aggregatedPoint = null
      bucketState.aggregatedError = null
      bucketState.aggregatedLoading = false
      return
    }

    let cancelled = false
    bucketState.bucketSeries = null
    bucketState.bucketSeriesError = null
    bucketState.bucketSeriesLoading = true
    bucketState.aggregatedPoint = null
    bucketState.aggregatedError = null
    bucketState.aggregatedLoading = true

    telemetryAPI
      .getMetricBucketSeries(id, 'per-attribute', start, end, 100)
      .then(result => {
        if (cancelled) return
        bucketState.bucketSeries = result
        bucketState.bucketSeriesLoading = false
      })
      .catch(err => {
        if (cancelled) return
        bucketState.bucketSeriesLoading = false
        bucketState.bucketSeriesError = categorizeBucketSeriesError(err)
      })

    telemetryAPI
      .getMetricBucketSeries(id, 'merged', start, end, 1)
      .then(result => {
        if (cancelled) return
        bucketState.aggregatedPoint = result.length > 0 ? result[0] : null
        bucketState.aggregatedLoading = false
      })
      .catch(err => {
        if (cancelled) return
        bucketState.aggregatedLoading = false
        bucketState.aggregatedError = categorizeBucketSeriesError(err)
      })

    return () => {
      cancelled = true
    }
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
  }

  function setGaugeSumVisible(next: SvelteSet<string>) {
    view.gaugeSumVisible = next
  }

  function setHistogramVisible(next: SvelteSet<string>) {
    view.histogramVisible = next
  }

  function onDatapointClick(dp: DataPoint) {
    view.selectedDatapointId =
      view.selectedDatapointId === dp.id ? null : dp.id
    if (isHistogramKind && view.selectedDatapointId !== null) {
      view.activeHistogramTab = 'snapshot'
    }
    if (dp.exemplars.length > 0) {
      if (view.expandedDatapoints.has(dp.id)) {
        view.expandedDatapoints.delete(dp.id)
      } else {
        view.expandedDatapoints.add(dp.id)
      }
    }
  }

  function onHeatmapSelect(timestampMs: number) {
    const m = getMetric()
    if (!m) return
    // Bucket width matches the backend: max(1ms, (end-start)/100).
    const bucketWidthMs = Math.max(
      1,
      Math.floor((queryRange.end - queryRange.start) / 100)
    )
    const bucketStart = BigInt(timestampMs) * 1_000_000n
    const bucketEnd = BigInt(timestampMs + bucketWidthMs) * 1_000_000n
    // Walk per-timeseries (not allDatapoints) so we can also expand
    // the owning timeseries in the bottom panel for the user. The
    // panel watches expandedTimeseries + selectedDatapointId to
    // scroll + highlight the matching row (step-4 sync).
    for (const ts of m.timeseries) {
      for (const dp of ts.datapoints) {
        if (dp.timestamp >= bucketStart && dp.timestamp < bucketEnd) {
          view.selectedDatapointId = dp.id
          view.activeHistogramTab = 'snapshot'
          view.expandedTimeseries.add(ts.attributesKey)
          return
        }
      }
    }
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
    get expandedDatapoints() {
      return view.expandedDatapoints
    },
    get expandedTimeseries() {
      return view.expandedTimeseries
    },
    get activeHistogramTab() {
      return view.activeHistogramTab
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

    get histogramLegendTimeseries() {
      return histogramLegendTimeseries
    },
    get histogramTimeseriesCount() {
      return histogramTimeseriesGroups.length
    },
    get histogramVisible() {
      return view.histogramVisible
    },
    get visibleBucketSeries() {
      return visibleBucketSeries
    },
    get bucketSeriesLoading() {
      return bucketState.bucketSeriesLoading
    },
    get bucketSeriesError() {
      return bucketState.bucketSeriesError
    },
    get aggregatedDatapoint() {
      return aggregatedDatapoint
    },
    get aggregatedLoading() {
      return bucketState.aggregatedLoading
    },
    get aggregatedError() {
      return bucketState.aggregatedError
    },
    get activeHistogramDp() {
      return activeHistogramDp
    },
    get heatmapSelectedTimestamp() {
      return heatmapSelectedTimestamp
    },

    get filteredTimeseries() {
      return filteredTimeseries
    },
    get timeseriesColorIndex() {
      return timeseriesColorIndex
    },
    get legendFilterActive() {
      return legendFilterActive
    },

    toggleTimeseriesExpanded,
    setActiveHistogramTab,
    setGaugeSumVisible,
    setHistogramVisible,
    onDatapointClick,
    onHeatmapSelect,
  }

  setContext(KEY, ctx)
  return ctx
}

export function getMetricViewContext(): MetricViewContext {
  return getContext<MetricViewContext>(KEY)
}

function categorizeBucketSeriesError(err: unknown): BucketSeriesError {
  if (err instanceof JsonRpcError) {
    if (err.code === ErrCodeUnspecifiedTemporality) {
      return { kind: 'unspecified', message: err.message }
    }
    if (err.code === ErrCodeHistogramBoundsMismatch) {
      return { kind: 'boundsMismatch', message: err.message }
    }
  }
  return {
    kind: 'other',
    message: err instanceof Error ? err.message : String(err),
  }
}
