<script module lang="ts">
  import type { MetricSummary, ScalarAggregate } from '@/types/api-types'
  import { metricSummaryKey } from '@/types/api-types'
  import {
    compareByStringField,
    compareByTimestampField,
  } from '@/utils/compare'

  // --- Sort ---

  export type MetricSortColumn =
    | 'name'
    | 'metricType'
    | 'serviceName'
    | 'description'
    | 'dataPointCount'
    | 'seriesCount'
    | 'lastSeen'
  export type MetricSortDirection = 'asc' | 'desc'

  function compareMetrics(
    a: MetricSummary,
    b: MetricSummary,
    col: MetricSortColumn,
    dir: MetricSortDirection
  ): number {
    let cmp: number
    switch (col) {
      case 'name':
        cmp = compareByStringField(a, b, m => m.name)
        break
      case 'metricType':
        cmp = compareByStringField(a, b, m => m.metricType)
        break
      case 'serviceName':
        cmp = compareByStringField(a, b, m => m.serviceName)
        break
      case 'description':
        cmp = compareByStringField(a, b, m => m.description)
        break
      case 'dataPointCount':
        cmp = a.dataPointCount - b.dataPointCount
        break
      case 'seriesCount':
        cmp = a.seriesCount - b.seriesCount
        break
      case 'lastSeen':
        cmp = compareByTimestampField(a, b, m => m.lastSeen)
        break
      default:
        cmp = 0
    }

    return cmp !== 0
      ? dir === 'asc'
        ? cmp
        : -cmp
      : metricSummaryKey(a).localeCompare(metricSummaryKey(b))
  }

  export {
    metricTypeBadgeClass,
    metricTypeLabel,
  } from '@/components/metrics/utils/metric-type'
</script>

<script lang="ts">
  import {
    METRIC_BUCKET_TARGET,
    SCALAR_VIEW_BUCKETS,
    SPARKLINE_BUCKETS,
  } from '@/contexts/metric-view-context.svelte'
  import {
    DEFAULT_VISIBLE_TIMESERIES,
    persistedVisibleKeys,
  } from '@/components/metrics/utils/metric-timeseries-visible'
  import {
    DEFAULT_HISTOGRAM_QUANTILES,
    HEATMAP_BUCKET_TARGET,
    localOffsetNs,
  } from '@/components/metrics/utils/histogram-aggregation'
  import type { JsonAggregateBucket } from '@/types/wire-types'
  import { untrack } from 'svelte'
  import { SvelteMap } from 'svelte/reactivity'
  import {
    telemetryAPI,
    type QueryTimeBound,
  } from '@/services/telemetry-service'
  import {
    getTimeContext,
    selectionToQueryRangeMs,
  } from '@/contexts/time-context.svelte'
  import { navigateToItem } from '@/route'
  import type { DataPoint, MetricData, MetricStats } from '@/types/api-types'
  import {
    createSignalListPage,
    type SortOption,
  } from '@/contexts/signal-list-page.svelte'
  import PageLayout from '@/components/shared/PageLayout.svelte'
  import DrawerSearchPanel from '@/components/shared/Drawer/DrawerSearchPanel.svelte'
  import SignalDrawerFooter from '@/components/shared/Drawer/SignalDrawerFooter.svelte'
  import MetricCard from '@/components/metrics/MetricCard.svelte'
  import SignalBadges from '@/components/shared/SignalBadges.svelte'
  import MetricChartView from '@/components/metrics/Charts/MetricChartView.svelte'
  import MetricDetailView from '@/components/metrics/Detail/MetricDetailView.svelte'
  import SignalFooter from '@/components/shared/SignalFooter.svelte'
  import PaneHeader, { paneTabID } from '@/components/shared/PaneHeader.svelte'
  import type { AggregationView } from '@/components/metrics/utils/aggregation'
  import {
    PANEL_DEFAULT_REM,
    PANEL_MIN_REM,
    remToPx,
  } from '@/state/panel-width'
  import { aggregationViewTabs } from '@/components/metrics/utils/aggregation-view-tabs'
  import { histogramViewTabs } from '@/components/metrics/utils/histogram-view-tabs'
  import {
    createMetricViewContext,
    getMetricViewContext,
    type HistogramTab,
  } from '@/contexts/metric-view-context.svelte'

  const METRIC_CHART_PANEL_ID = 'metric-chart-tabpanel'

  const SORT_OPTIONS: SortOption[] = [
    { value: 'lastSeen', label: 'Last Seen', defaultDirection: 'desc' },
    { value: 'name', label: 'Name' },
    { value: 'metricType', label: 'Type' },
    { value: 'serviceName', label: 'Service Name' },
    { value: 'description', label: 'Description' },
    {
      value: 'dataPointCount',
      label: 'Datapoint Count',
      defaultDirection: 'desc',
    },
    {
      value: 'seriesCount',
      label: 'Timeseries Count',
      defaultDirection: 'desc',
    },
  ]

  let timeContext = getTimeContext()

  let baselineStats = $state<MetricStats | null>(null)
  let polledStats = $state<MetricStats | null>(null)
  let actionError = $state<string | null>(null)

  const page = createSignalListPage<MetricSummary>({
    signal: 'metrics',
    getItemID: metricSummaryKey,
    initialSort: { column: 'lastSeen', direction: 'desc' },
    compare: (a, b, col, dir) =>
      compareMetrics(a, b, col as MetricSortColumn, dir as MetricSortDirection),
    fetchList: async () => {
      const { startTime, endTime } = selectionToQueryRangeMs(
        timeContext.selection,
        Date.now()
      )
      const results = await telemetryAPI.searchMetricSummaries(
        startTime,
        endTime
      )
      const s = await telemetryAPI.getStats()
      baselineStats = s.metrics
      polledStats = s.metrics
      return results
    },
    pollStats: async () => {
      const s = await telemetryAPI.getStats()
      polledStats = s.metrics
    },
    refreshFromStats: () => {
      if (!baselineStats || !polledStats) {
        return { pulse: false, tip: '' }
      }
      const parts: string[] = []
      const metricDelta = polledStats.metricCount - baselineStats.metricCount
      if (metricDelta > 0) {
        parts.push(`+${metricDelta} metric${metricDelta !== 1 ? 's' : ''}`)
      }
      const dpDelta = polledStats.dataPointCount - baselineStats.dataPointCount
      if (dpDelta > 0) {
        parts.push(`+${dpDelta} dp${dpDelta !== 1 ? 's' : ''}`)
      }
      return { pulse: parts.length > 0, tip: parts.join(', ') }
    },
  })

  let selectedMetric = $state<MetricData | undefined>(undefined)
  let detailLoading = $state(false)

  // The store's cross-series merge for the current legend selection. Null
  // until the first fetch resolves, and whenever the metric changes.
  let selectedAggregate = $state<JsonAggregateBucket[] | null>(null)
  // The same merge over one bucket spanning the window. A window summary is
  // not the last column of a chart, and it is not the columns added together
  // either -- it is the merge asked a different question, so the store answers
  // it rather than the client approximating from what it already has.
  let selectedAggregateSummary = $state<JsonAggregateBucket | null>(null)
  // The store's cross-series fold for a scalar metric: the checked pool and the
  // full pool, on the same bucket grid the per-series views use. Refetched when
  // the legend changes, which is the whole reason it is separate from the
  // metric's own response.
  let selectedScalarAggregate = $state<ScalarAggregate | null>(null)

  // Unreduced datapoints per series, fetched when a series is expanded.
  //
  // Keyed by series id, cleared whenever the metric or the window changes so a
  // stale series can never be shown under a new question. A SvelteMap because
  // the list reads it during render.
  let seriesDatapoints = new SvelteMap<string, DataPoint[]>()
  let seriesDatapointsKey = ''
  let seriesDatapointsScopeKey = ''
  let expandedSeriesSnapshot = new Set<string>()
  let seriesDatapointsMetricID = $derived.by(() => {
    const summaryID = page.selectedSummary?.id
    return summaryID && selectedMetric?.id === summaryID ? summaryID : null
  })

  // Each series merged over the selected heatmap column, fetched on click.
  //
  // The column and the per-series lines are drawn at different resolutions, and
  // neither can be pinned to the other: the heatmap wants a column per few
  // pixels, the quantile lines want enough points not to visibly step. So the
  // only way to answer "what did each series look like in this column" is to
  // ask for that column.
  let columnDistribution = $state<MetricData | undefined>(undefined)
  let columnToken = 0

  createMetricViewContext(
    () => selectedMetric,
    () => selectedAggregate,
    () => selectedAggregateSummary,
    () => selectedScalarAggregate,
    key => seriesDatapoints.get(key),
    () => columnDistribution
  )
  const metricCtx = getMetricViewContext()

  let hasMetricRows = $derived(page.items.length > 0)
  let displayError = $derived(page.error ?? actionError)

  let chartAggregationTabs = $derived(
    aggregationViewTabs(metricCtx.availableAggregationViews)
  )

  let showChartAggregationTabs = $derived(
    (page.selectedSummary?.metricType === 'Sum' ||
      page.selectedSummary?.metricType === 'Gauge') &&
      chartAggregationTabs.length > 1
  )

  let showChartHistogramTabs = $derived(
    page.selectedSummary?.metricType === 'Histogram' ||
      page.selectedSummary?.metricType === 'ExponentialHistogram'
  )

  let showChartTitleTabs = $derived(
    showChartAggregationTabs || showChartHistogramTabs
  )
  let activeChartTabID = $derived(
    showChartAggregationTabs
      ? metricCtx.aggregationView
      : metricCtx.activeHistogramTab
  )
  let chartTabPanelAttrs = $derived.by(() =>
    showChartTitleTabs
      ? {
          id: METRIC_CHART_PANEL_ID,
          role: 'tabpanel' as const,
          'aria-labelledby': paneTabID(METRIC_CHART_PANEL_ID, activeChartTabID),
          tabindex: 0,
        }
      : {}
  )

  // Re-fetch when the selected metric identity or time selection changes, but
  // not when polling merely replaces the summary object with an equivalent one.
  $effect(() => {
    void timeContext.selection
    const id = page.selectedSummary
      ? metricSummaryKey(page.selectedSummary)
      : null
    if (!id) {
      selectedMetric = undefined
      return
    }
    const summary = untrack(() => page.selectedSummary)
    if (summary) fetchMetricDetail(summary)
  })

  // The aggregate is fetched separately from the metric, because the two have
  // different lifetimes. Per-series quantiles are additive -- fetched once with
  // the metric, and any subset's lines are already in hand -- while the merge
  // is specific to the selection and has to be recomputed when the legend
  // changes.
  //
  // Folding this into the detail effect would break that effect's contract: it
  // depends on the metric identity alone and untracks the summary, because
  // polling churns object references and re-fetching would clobber legend
  // state. It would also fetch twice per metric, since the legend selection is
  // seeded from the response it would then depend on.
  let aggregateTimer: ReturnType<typeof setTimeout> | undefined
  let aggregateToken = 0

  $effect(() => {
    const summary = page.selectedSummary
    // Read reactively so a toggle re-runs this. Sorted so a set rebuilt with
    // the same members does not look like a change.
    //
    // One selection; which parameter carries it is the shape's decision, made
    // in fetchAggregate. A histogram narrows with seriesIDs -- the merge sees
    // only the checked series. A scalar narrows with nothing and names its
    // checked set separately, because its All pool must keep folding every
    // series: narrowing there would quietly turn "all" into "all of the
    // checked ones". Sending the selection through the wrong parameter once
    // folded ten series into a line labelled All, back when two boxes made
    // that possible.
    const visibleKeys = [...metricCtx.visibleSeries].sort()

    // Wait for the metric itself. The legend selection is seeded from that
    // response, so fetching before it arrives asks for the empty set -- which
    // now correctly means "no series" and returns nothing.
    // Widened past histograms: a scalar metric's cross-series lines are folded
    // by the same endpoint now, and they depend on the selection for the same
    // reason the histogram merge does.
    if (!summary || !selectedMetric) {
      selectedAggregate = null
      selectedAggregateSummary = null
      selectedScalarAggregate = null
      return
    }
    const bounds = effectiveMetricBounds(selectedMetric)
    if (!bounds) {
      selectedAggregate = null
      selectedAggregateSummary = null
      selectedScalarAggregate = null
      return
    }

    // Coalesce rapid toggles into one request. Ticking through five series
    // should ask the store once, not five times.
    clearTimeout(aggregateTimer)
    const token = ++aggregateToken
    aggregateTimer = setTimeout(() => {
      void fetchAggregate(summary, visibleKeys, bounds, token)
    }, 120)

    return () => clearTimeout(aggregateTimer)
  })

  async function fetchAggregate(
    summary: MetricSummary,
    visibleKeys: string[],
    bounds: { startTime: bigint; endTime: bigint },
    token: number
  ) {
    try {
      const { startTime, endTime } = bounds
      // The store's quantiles, computed once per datapoint from its bucket
      // vector. Recomputing them per render costs seconds on the main thread:
      // 2,700 bucket walks for one render of this metric.
      const quantiles = DEFAULT_HISTOGRAM_QUANTILES as unknown as number[]
      // Same answer as the detail fetch gives, so the aggregate is bucketed
      // over the window the series beneath it were bucketed over.
      // Both shapes of the same question, issued together so they cannot
      // disagree about the window or the selection.
      // The narrowing parameter belongs to the histogram merge alone. A scalar
      // sends none: its pools are named by selectedSeriesIDs, which narrows
      // nothing.
      const isHistogramMetric =
        summary.metricType === 'Histogram' ||
        summary.metricType === 'ExponentialHistogram'
      // The selection travels through exactly one parameter per shape. The
      // other side gets its empty form deliberately, not another copy: a
      // histogram's selection sent as selectedSeriesIDs, or a scalar's as
      // seriesIDs, narrows an aggregate that must fold every series.
      const narrowTo = isHistogramMetric ? visibleKeys : null
      const scalarSelected = isHistogramMetric ? [] : visibleKeys
      const [buckets, whole] = await Promise.all([
        telemetryAPI.getMetricAggregate(
          summary.id,
          startTime,
          endTime,
          HEATMAP_BUCKET_TARGET,
          narrowTo,
          quantiles,
          tzOffsetNs(),
          // The same grid getMetric asked for, or the pooled lines would be
          // bucketed against different boundaries than the per-series lines
          // drawn beneath them.
          SCALAR_VIEW_BUCKETS,
          scalarSelected,
          tzName()
        ),
        // The whole-window merge, and only a histogram has one.
        //
        // Its single field feeds the summary distribution; a Gauge or Sum has
        // no bucket vectors to merge, so the answer is null by construction --
        // the store proves it rather than discovering it, since
        // aggregateShapeFor drops the merge chain outright for those types.
        // Asking anyway spent a round trip and a full query plan per legend
        // toggle to be told null, measured at 23ms against a 27ms toggle.
        //
        // The scalar pools do not come from here. They ride on the bucketed
        // call above, which is the one asked for the view grid.
        isHistogramMetric
          ? telemetryAPI.getMetricAggregate(
              summary.id,
              startTime,
              endTime,
              1,
              narrowTo,
              quantiles,
              tzOffsetNs(),
              // This call collapses to one bucket, but that bucket's
              // boundaries still follow the calendar the other call's do.
              0,
              undefined,
              tzName()
            )
          : null,
      ])
      // A slower earlier request must not overwrite a newer answer.
      if (token === aggregateToken) {
        selectedAggregate = buckets?.aggregate ?? null
        selectedAggregateSummary = whole?.aggregate?.[0] ?? null
        // The scalar pools come off the bucketed call, which is the one asked
        // for the view grid. The whole-window call collapses to a single bucket
        // and has no line to draw.
        selectedScalarAggregate = buckets?.scalarAggregate ?? null
      }
    } catch (err) {
      console.error('Failed to fetch metric aggregate:', err)
      if (token === aggregateToken) {
        selectedAggregate = null
        selectedAggregateSummary = null
        selectedScalarAggregate = null
      }
    }
  }

  function selectMetric(key: string) {
    page.selectItem(key)
  }

  function effectiveMetricBounds(
    metric: MetricData
  ): { startTime: bigint; endTime: bigint } | null {
    const { startNs, endNs } = metric.window.effective
    if (startNs === null || endNs === null || endNs < startNs) return null
    return { startTime: startNs, endTime: endNs }
  }

  // Expanding a series fetches that series as it was sent: no reduction, no
  // quantiles, one series.
  //
  // The chart may show a reduction -- that is what a chart is for. The list may
  // not: it is the view that answers "what did my service actually send", and
  // for a reduced histogram `metric.timeseries[].datapoints` are the store's
  // merged buckets rather than datapoints. On the reference capture the window
  // held 17,076 and the response carried 3,094.
  //
  // Fetched on expansion rather than with the metric because it is the only
  // place that wants every row, and narrow enough to be cheap: the store
  // filters by series before reducing, so one series of that same metric is
  // 1,001 datapoints in 132 ms. Asking for all of them up front would put the
  // whole unreduced stream on the wire to render twenty rows.
  $effect(() => {
    const streamID = seriesDatapointsMetricID
    const expanded = [...metricCtx.expandedTimeseries]
    const selection = timeContext.selection
    const timezone = timeContext.tz
    // This stable identity invalidates an old metric/window request even while
    // every series is collapsed. Presets use their stored duration here; their
    // moving absolute bounds are resolved only when a request is triggered.
    const scopeKey = streamID
      ? JSON.stringify([streamID, selection, timezone])
      : ''
    const scopeChanged = scopeKey !== seriesDatapointsScopeKey
    if (scopeChanged) {
      seriesDatapointsScopeKey = scopeKey
      seriesDatapointsKey = ''
      seriesDatapoints.clear()
    }
    const expansionTriggered = expanded.some(
      seriesKey => !expandedSeriesSnapshot.has(seriesKey)
    )
    expandedSeriesSnapshot = new Set(expanded)
    if (!streamID || expanded.length === 0) return
    // Collapsing one row while another remains open is not a refetch trigger.
    if (!scopeChanged && !expansionTriggered) return

    const now = Date.now()
    const { startTime, endTime } = selectionToQueryRangeMs(selection, now)
    const timezoneOffsetNs = tzOffsetNs(now)
    const timezoneName = tzName()
    // Capture every input that can change the store's answer. A structured key
    // avoids collisions with attribute-derived series and metric identifiers.
    const key = JSON.stringify([
      streamID,
      startTime,
      endTime,
      timezone,
      timezoneOffsetNs,
      timezoneName ?? null,
    ])
    if (key !== seriesDatapointsKey) {
      seriesDatapointsKey = key
      seriesDatapoints.clear()
    }

    for (const seriesKey of expanded) {
      // Cache publication must not be a new live-window request trigger.
      if (untrack(() => seriesDatapoints.has(seriesKey))) continue
      void fetchSeriesDatapoints({
        streamID,
        seriesKey,
        startTime,
        endTime,
        timezoneOffsetNs,
        timezoneName,
        cacheKey: key,
      })
    }
  })

  const seriesInFlight = new Set<string>()

  type SeriesDatapointsRequest = {
    streamID: string
    seriesKey: string
    startTime: QueryTimeBound
    endTime: QueryTimeBound
    timezoneOffsetNs: number
    timezoneName: string | undefined
    cacheKey: string
  }

  async function fetchSeriesDatapoints(request: SeriesDatapointsRequest) {
    const {
      streamID,
      seriesKey,
      startTime,
      endTime,
      timezoneOffsetNs,
      timezoneName,
      cacheKey,
    } = request
    const requestKey = JSON.stringify([cacheKey, seriesKey])
    if (seriesInFlight.has(requestKey)) return
    seriesInFlight.add(requestKey)
    try {
      const result = await telemetryAPI.getMetric(
        streamID,
        startTime,
        endTime,
        // No reduction. This is the request the whole feature is about.
        0,
        [seriesKey],
        // None: the list shows what arrived, and quantiles are a derived
        // statistic the client computes anyway.
        [],
        timezoneOffsetNs,
        undefined,
        undefined,
        undefined,
        timezoneName
      )
      // The window may have moved while this was in flight; a late answer to a
      // superseded question must not land in the new cache.
      if (cacheKey !== seriesDatapointsKey) return
      const series = result?.timeseries.find(t => t.attributesKey === seriesKey)
      // An omitted series and a null response are both terminal answers. Cache
      // an empty array so URL-backed pending selections can reject instead of
      // waiting forever for a value that will never arrive.
      seriesDatapoints.set(seriesKey, series?.datapoints ?? [])
    } catch (err) {
      console.error('Failed to fetch series datapoints:', err)
    } finally {
      seriesInFlight.delete(requestKey)
    }
  }

  /** The offset to align store-side buckets to, in nanoseconds. */
  function tzOffsetNs(now = Date.now()): number {
    if (timeContext.tz === 'UTC') return 0
    return Number(localOffsetNs(BigInt(now) * 1_000_000n))
  }

  /** The zone that offset was sampled from, so the store can resolve it per
   *  datapoint instead of applying one sample to the whole window -- a window
   *  crossing a DST transition changes offset partway through. Undefined in
   *  UTC, which has no transitions to resolve. */
  function tzName(): string | undefined {
    if (timeContext.tz === 'UTC') return undefined
    return Intl.DateTimeFormat().resolvedOptions().timeZone
  }

  // Fetch the clicked heatmap column's per-series distribution.
  //
  // targetBuckets is 1 and the window is the column, so the store merges each
  // series across exactly that range -- the same merge it does for the
  // aggregate, but one entry per series rather than one across them.
  $effect(() => {
    // Read as two primitives, so a recomputation landing on the same column is
    // not a change. The heatmap array gets a new identity on every aggregate
    // response -- ticking one series in the legend is enough -- and depending on
    // an object would refetch and blank the panel for a column that never moved.
    const startNs = metricCtx.heatmapColumnStartNs
    const endNs = metricCtx.heatmapColumnEndNs
    const summary = page.selectedSummary
    if (startNs === null || endNs === null || !summary) {
      columnDistribution = undefined
      return
    }
    const token = ++columnToken
    // Cleared first, so a stale column can never be read against a new
    // selection while the new one is in flight.
    columnDistribution = undefined
    void (async () => {
      try {
        const result = await telemetryAPI.getMetric(
          summary.id,
          // Exact nanoseconds, not milliseconds: the end sits one nanosecond
          // short of the next column, and rounding it would drop the column's
          // final millisecond of readings.
          startNs,
          endNs,
          1,
          undefined,
          DEFAULT_HISTOGRAM_QUANTILES as unknown as number[],
          tzOffsetNs(),
          0,
          0,
          undefined,
          tzName()
        )
        if (token !== columnToken) return
        columnDistribution = result ?? undefined
      } catch (err) {
        if (token !== columnToken) return
        console.error('Failed to fetch heatmap column distribution:', err)
        columnDistribution = undefined
      }
    })()
  })

  // Which detail fetch is current. The aggregate fetch has had one of these
  // since it was split out; this one did not, so a response for a metric the
  // user had already navigated away from would still be assigned -- showing the
  // wrong metric's data, and building the chart a second time to do it. Walking
  // the list with the pager reproduced it every time; clicking one metric and
  // waiting did not, which is why it looked like a rendering problem.
  let detailToken = 0

  // Datapoints arrive only for the series being drawn, so checking a series
  // that was not drawn before leaves it with an empty line until its datapoints
  // are fetched. This notices that and asks for them.
  //
  // Debounced with the same 120ms the aggregate uses, and for the same reason:
  // ticking through five series should ask once. Reseeding is off, because the
  // reader's selection is what triggered this and re-deriving it from the
  // response would throw the change away.
  let datapointTimer: ReturnType<typeof setTimeout> | undefined

  $effect(() => {
    const summary = page.selectedSummary
    const metric = selectedMetric
    // One visibility set for every shape now. This used to watch the scalar
    // set alone, which is empty for a histogram by construction, so the effect
    // bailed on its first line and a narrowed-out histogram series stayed
    // blank forever -- worse for a histogram than for a scalar, which at least
    // keeps its sparkline, stats and view buckets.
    const checked = metricCtx.visibleSeries
    const visible = [...checked].sort()
    if (!summary || !metric || visible.length === 0) return

    const missing = metric.timeseries.some(
      ts => checked.has(ts.attributesKey) && ts.datapoints.length === 0
    )
    if (!missing) return

    clearTimeout(datapointTimer)
    datapointTimer = setTimeout(() => {
      void fetchMetricDetail(summary, visible, false)
    }, 120)

    return () => clearTimeout(datapointTimer)
  })

  async function fetchMetricDetail(
    summary: MetricSummary,
    /** Which series need their datapoints. Null on the first fetch of a metric,
     *  where the selection is chosen from the response and the store applies
     *  the same "first N" rule instead. */
    datapointSeries: string[] | null = null,
    /** Whether to reset the per-metric view state from the result. False when
     *  refetching for datapoints the reader just asked for by checking a box:
     *  reseeding there would discard the very change that triggered it. */
    reseed = true
  ) {
    const token = ++detailToken
    try {
      detailLoading = true
      const { startTime, endTime } = selectionToQueryRangeMs(
        timeContext.selection,
        Date.now()
      )
      // Ask the store to reduce to roughly one bucket per chart point it will
      // draw. It keeps up to four per bucket -- first, last, min, max -- so the
      // line is the same as it would be with every datapoint, and it ignores
      // the request for histograms, which need merging rather than sampling.
      //
      // This is what stops a dense stream shipping tens of megabytes to draw a
      // few thousand pixels: measured at 46.4 MB and 640 ms down to 0.36 MB and
      // 60 ms on a 242,324-datapoint stream.
      // A histogram gets the heatmap's target, which is a resolution decision
      // rather than the line chart's point budget. Histogram datapoints carry a
      // bucket vector each, so the line-chart target fetched an order of
      // magnitude more payload than any heatmap can show.
      const isHistogramMetric =
        summary.metricType === 'Histogram' ||
        summary.metricType === 'ExponentialHistogram'
      const bucketTarget = isHistogramMetric
        ? HEATMAP_BUCKET_TARGET
        : METRIC_BUCKET_TARGET
      const result =
        (await telemetryAPI.getMetric(
          summary.id,
          startTime,
          endTime,
          bucketTarget,
          // Never narrowed here: dropping a series from the response would
          // take its row, sparkline and view buckets with it. Only its
          // datapoints are narrowed, further down.
          undefined,
          // Computed by the store, read by the client. Both halves of the
          // response need them: the per-series lines and the merged columns.
          //
          // Histograms only. A quantile is a question about a bucket vector, so
          // a Gauge or Sum has no answer to give, and asking for one is not
          // free: on a 22-series Gauge it costs 2,937 ms against 298 ms, for
          // byte-identical output.
          isHistogramMetric
            ? (DEFAULT_HISTOGRAM_QUANTILES as unknown as number[])
            : undefined,
          // Bucket boundaries follow the reader's calendar rather than the
          // epoch. 0 is UTC, which is what the store assumes without this.
          tzOffsetNs(),
          // Resolution for the Sum / Average / Rate views, which bucket for a
          // different chart than the election thins for.
          SCALAR_VIEW_BUCKETS,
          // Resolution for the per-row sparklines. The store sends one for
          // every series, checked or not, because the sparkline is how the
          // reader decides which series is worth checking.
          SPARKLINE_BUCKETS,
          // The separate aggregate request carries the live legend selection.
          undefined,
          tzName(),
          // Datapoints only for the series that will be drawn -- the rest of
          // each series still arrives. A previous visit's selection can be
          // named outright; a first visit cannot, because the selection is
          // chosen from this very response, so the store takes the same "first
          // N" the seeding would.
          datapointSeries ?? persistedVisibleKeys(summary.id) ?? undefined,
          DEFAULT_VISIBLE_TIMESERIES
        )) ?? undefined
      // A slower earlier request must not overwrite a newer answer.
      if (token !== detailToken) return
      selectedMetric = result
      // Same statement, before anything renders: the chart must not build once
      // for the previous metric's visible set and colours, and again for this
      // one's.
      if (reseed) metricCtx.seedForMetric(selectedMetric)
    } catch (err) {
      console.error('Failed to fetch metric detail:', err)
      if (token !== detailToken) return
      selectedMetric = undefined
      metricCtx.seedForMetric(undefined)
    } finally {
      // Only the current request owns the spinner; a superseded one clearing it
      // would say "loaded" while the answer is still on its way.
      if (token === detailToken) detailLoading = false
    }
  }

  async function handleDeleteMetric(streamID: string) {
    actionError = null
    try {
      await telemetryAPI.deleteMetricStream(streamID)
      // Clear the detail pane before refetching: the $effect above keys off
      // page.selectedSummary, and the deleted stream is gone from the next
      // list fetch, so leaving it set would render a stale chart.
      if (page.selectedID === streamID) {
        navigateToItem('metrics', null, 'replace')
        selectedMetric = undefined
      }
      await page.runListFetch()
    } catch (err) {
      actionError =
        err instanceof Error ? err.message : 'Failed to delete metric'
    }
  }

  async function handleDeleteAllMetrics() {
    actionError = null
    try {
      await telemetryAPI.clearMetrics()
      navigateToItem('metrics', null, 'replace')
      selectedMetric = undefined
      await page.runListFetch()
    } catch (err) {
      actionError =
        err instanceof Error ? err.message : 'Failed to delete metrics'
    }
  }
</script>

<div class="metrics-page">
  <PageLayout
    items={page.sortedItems}
    selectedID={page.selectedID}
    drawerID="signal-drawer"
    drawerLabel="Metrics"
    onRefresh={page.handleRefresh}
    refreshPulse={page.refreshPulse}
    refreshAsideTip={page.refreshAsideTip}
    loading={page.loading}
    itemKey={metricSummaryKey}
    resizableStorageKey="metric-detail-panels"
    defaultDetailRem={PANEL_DEFAULT_REM}
    minMainPx={remToPx(PANEL_DEFAULT_REM)}
    minDetailPx={remToPx(PANEL_MIN_REM)}
  >
    {#snippet drawerChromeToolbar()}
      <DrawerSearchPanel
        segment="toolbar"
        signal="metrics"
        sortOptions={SORT_OPTIONS}
        sortValue={page.sortColumn}
        sortDirection={page.sortDirection}
        onSortChange={page.handleSortChange}
      />
    {/snippet}

    {#snippet drawerSearch()}
      <DrawerSearchPanel
        segment="search"
        signal="metrics"
        sortOptions={SORT_OPTIONS}
        sortValue={page.sortColumn}
        sortDirection={page.sortDirection}
        onSortChange={page.handleSortChange}
        onSearchResults={page.handleSearchResults}
        onSearchReady={api => (page.searchEditorApi = api)}
      />
    {/snippet}

    {#snippet itemSnippet(metric, selected)}
      <MetricCard {metric} {selected} onclick={selectMetric} />
    {/snippet}

    {#snippet drawerFooter()}
      <SignalDrawerFooter
        count={page.sortedItems.length}
        label="metric"
        onDeleteAll={handleDeleteAllMetrics}
      />
    {/snippet}

    {#snippet main()}
      <!-- Page-level error / empty branches replace the chart pane
           entirely; the chart lives here on the happy path and the
           detail pane (Fields/Series) renders alongside. The
           SignalFooter is now page-level chrome (see pageFooter
           snippet below): always present, spans main + detail
           regardless of content state, and DetailNav self-disables
           when there is nothing to navigate. -->
      {#if page.selectedSummary}
        {@const selectedSummary = page.selectedSummary}
        {#snippet metricChartHeaderBadge()}
          <SignalBadges
            signal="metric"
            metricType={selectedSummary.metricType}
            aggregationTemporality={selectedSummary.aggregationTemporality}
            isMonotonic={selectedSummary.isMonotonic}
          />
        {/snippet}

        {@const histogramChartTabs = histogramViewTabs()}

        {#if showChartTitleTabs}
          <PaneHeader
            mode="title-tabs"
            title={selectedSummary.name}
            subtitle={selectedSummary.serviceName?.trim() || undefined}
            tabs={showChartAggregationTabs
              ? chartAggregationTabs
              : histogramChartTabs}
            activeID={showChartAggregationTabs
              ? metricCtx.aggregationView
              : metricCtx.activeHistogramTab}
            onSelect={id => {
              if (showChartAggregationTabs) {
                metricCtx.setAggregationView(id as AggregationView)
              } else {
                metricCtx.setActiveHistogramTab(id as HistogramTab)
              }
            }}
            ariaLabel="Metric chart"
            tabPanelID={METRIC_CHART_PANEL_ID}
          >
            {#snippet badge()}{@render metricChartHeaderBadge()}{/snippet}
          </PaneHeader>
        {:else}
          <PaneHeader
            mode="title"
            title={selectedSummary.name}
            subtitle={selectedSummary.serviceName?.trim() || undefined}
            ariaLabel="Metric chart"
          >
            {#snippet badge()}{@render metricChartHeaderBadge()}{/snippet}
          </PaneHeader>
        {/if}
      {/if}
      {#if displayError}
        <div
          {...chartTabPanelAttrs}
          class="metrics-page__placeholder alert alert-error"
        >
          <span>Error: {displayError}</span>
        </div>
      {:else if page.loading && !hasMetricRows}
        <div
          {...chartTabPanelAttrs}
          class="metrics-page__placeholder metrics-empty"
        >
          Loading metrics…
        </div>
      {:else if !page.loading && !hasMetricRows}
        <div
          {...chartTabPanelAttrs}
          class="metrics-page__placeholder metrics-empty"
        >
          <p class="text-rp-subtle">No metrics in this time range</p>
          <p class="mt-2 text-sm text-rp-muted">
            Send telemetry to the exporter or adjust the time range
          </p>
        </div>
      {:else}
        <div {...chartTabPanelAttrs} class="metrics-page__chart">
          <MetricChartView />
        </div>
      {/if}
    {/snippet}

    {#snippet detail()}
      <MetricDetailView />
    {/snippet}

    {#snippet pageFooter()}
      <SignalFooter
        index={page.selectedIndex}
        total={page.sortedItems.length}
        label="metric"
        onFirst={page.selectFirst}
        onPrev={() => page.selectByOffset(-1)}
        onNext={() => page.selectByOffset(1)}
        onLast={page.selectLast}
        onDelete={page.selectedSummary
          ? () => handleDeleteMetric(page.selectedSummary!.id)
          : undefined}
      />
    {/snippet}
  </PageLayout>
</div>

<style lang="postcss">
  @reference "../app.css";

  .metrics-page {
    @apply flex min-h-0 min-w-0 w-full flex-1 flex-col;
  }

  .metrics-page__chart {
    @apply flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden;
  }

  /*
   * Page-level placeholder branches (error / loading / empty list).
   * Take the full main pane so the surrounding chrome already
   * provides the card framing -- no double-card.
   */
  .metrics-page__placeholder {
    @apply m-[var(--layout-gap)];
  }

  .metrics-empty {
    @apply px-4 py-12 text-center;
    color: var(--color-subtle);
  }
</style>
