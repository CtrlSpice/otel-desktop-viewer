<script module lang="ts">
  import type { MetricSummary, ScalarAggregate } from '@/types/api-types'
  import { metricSummaryKey } from '@/types/api-types'
  import {
    compareByStringField,
    compareByTimestampField,
  } from '@/utils/compare'

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
  } from '@/components/metrics/utils/histogram-aggregation'
  import type { AggregateBucket } from '@/types/api-types'
  import { untrack } from 'svelte'
  import { SvelteMap } from 'svelte/reactivity'
  import {
    telemetryAPI,
    type QueryTimeBound,
  } from '@/services/telemetry-service'
  import {
    getTimeContext,
    selectionToQueryRangeNs,
  } from '@/contexts/time-context.svelte'
  import { resolveTimezoneName, timezoneOffsetMinutes } from '@/utils/time'
  import { navigateToItem } from '@/route'
  import type {
    DataPoint,
    MetricViewData,
    MetricStats,
  } from '@/types/api-types'
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
  import { INGESTION_ISSUES_ID } from '@/components/shared/IngestionIssues.svelte'
  import type { ImportFailure } from '@/types/import-types'
  import PaneHeader, { paneTabID } from '@/components/shared/PaneHeader.svelte'
  import ExportButton from '@/components/shared/ExportButton.svelte'
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

  let {
    importFailure = null,
    onViewIssue,
  }: {
    importFailure?: ImportFailure | null
    onViewIssue?: (event: MouseEvent) => void
  } = $props()

  const METRIC_CHART_PANEL_ID = 'metric-chart-tabpanel'

  const SORT_OPTIONS: SortOption<MetricSortColumn>[] = [
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

  const page = createSignalListPage<MetricSummary, MetricSortColumn>({
    signal: 'metrics',
    getItemID: metricSummaryKey,
    initialSort: { column: 'lastSeen', direction: 'desc' },
    compare: compareMetrics,
    fetchList: async () => {
      const { startTime, endTime } = selectionToQueryRangeNs(
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

  let selectedMetric = $state<MetricViewData | undefined>(undefined)
  let detailLoading = $state(false)

  // Cross-series histogram merge for the current legend selection.
  let selectedAggregate = $state<AggregateBucket[] | null>(null)
  // The store computes the whole-window summary independently of chart columns.
  let selectedAggregateSummary = $state<AggregateBucket | null>(null)
  // Scalar aggregates for the checked and full series pools.
  let selectedScalarAggregate = $state<ScalarAggregate | null>(null)

  // Unreduced expanded-series datapoints, cleared when their request scope changes.
  let seriesDatapoints = new SvelteMap<string, DataPoint[]>()
  let seriesDatapointsKey = ''
  let seriesDatapointsScopeKey = ''
  let expandedSeriesSnapshot = new Set<string>()
  let seriesDatapointsMetricID = $derived.by(() => {
    const summaryID = page.selectedSummary?.metricRef
    return summaryID && selectedMetric?.metricRef === summaryID
      ? summaryID
      : null
  })

  // Per-series distributions for the selected heatmap column.
  let columnDistribution = $state<MetricViewData | undefined>(undefined)
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

  // Polling may replace a summary object without changing its metric identity.
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

  // Aggregates depend on legend selection; metric detail depends on metric identity.
  let aggregateTimer: ReturnType<typeof setTimeout> | undefined
  let aggregateToken = 0

  $effect(() => {
    const summary = page.selectedSummary
    // Sorting keeps equivalent selection sets stable while preserving reactivity.
    const visibleKeys = [...metricCtx.visibleSeries].sort()

    // The metric response seeds the initial legend selection.
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

    // Coalesce rapid legend changes.
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
      const quantiles = DEFAULT_HISTOGRAM_QUANTILES
      const isHistogramMetric =
        summary.metricType === 'Histogram' ||
        summary.metricType === 'ExponentialHistogram'
      // Histograms narrow the merge; scalars keep All intact and name Selected.
      const narrowTo = isHistogramMetric ? visibleKeys : null
      const scalarSelected = isHistogramMetric ? [] : visibleKeys
      const [buckets, whole] = await Promise.all([
        telemetryAPI.getMetricAggregateView(
          summary.metricRef,
          startTime,
          endTime,
          HEATMAP_BUCKET_TARGET,
          narrowTo,
          quantiles,
          tzOffsetNs(),
          // Keep pooled and per-series lines on the same bucket grid.
          SCALAR_VIEW_BUCKETS,
          scalarSelected,
          tzName()
        ),
        // Only histograms have a whole-window distribution summary.
        isHistogramMetric
          ? telemetryAPI.getMetricAggregateView(
              summary.metricRef,
              startTime,
              endTime,
              1,
              narrowTo,
              quantiles,
              tzOffsetNs(),
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
    metric: MetricViewData
  ): { startTime: bigint; endTime: bigint } | null {
    const { startNs, endNs } = metric.window.effective
    if (startNs === null || endNs === null || endNs < startNs) return null
    return { startTime: startNs, endTime: endNs }
  }

  // Expanded lists require received datapoints, so fetch one unreduced series on demand.
  $effect(() => {
    const metricRef = seriesDatapointsMetricID
    const expanded = [...metricCtx.expandedTimeseries]
    const selection = timeContext.selection
    const timezone = timeContext.tz
    // Presets keep their duration here; requests resolve their moving bounds.
    const scopeKey = metricRef
      ? JSON.stringify([metricRef, selection, timezone])
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
    if (!metricRef || expanded.length === 0) return
    // Collapsing one row while another remains open is not a refetch trigger.
    if (!scopeChanged && !expansionTriggered) return

    const now = Date.now()
    const { startTime, endTime } = selectionToQueryRangeNs(selection, now)
    const timezoneOffsetNs = tzOffsetNs(now)
    const timezoneName = tzName()
    // Capture every input that can change the store's answer. A structured key
    // avoids collisions with attribute-derived series and metric identifiers.
    const key = JSON.stringify([
      metricRef,
      startTime?.toString() ?? null,
      endTime?.toString() ?? null,
      timezone,
      timezoneOffsetNs,
      timezoneName ?? null,
    ])
    if (key !== seriesDatapointsKey) {
      seriesDatapointsKey = key
      seriesDatapoints.clear()
    }

    for (const seriesKey of expanded) {
      // Cache publication must not trigger another live-window request.
      if (untrack(() => seriesDatapoints.has(seriesKey))) continue
      void fetchSeriesDatapoints({
        metricRef,
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
    metricRef: string
    seriesKey: string
    startTime: QueryTimeBound
    endTime: QueryTimeBound
    timezoneOffsetNs: number
    timezoneName: string | undefined
    cacheKey: string
  }

  async function fetchSeriesDatapoints(request: SeriesDatapointsRequest) {
    const {
      metricRef,
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
      const result = await telemetryAPI.getMetricView(
        metricRef,
        startTime,
        endTime,
        0,
        [seriesKey],
        [],
        timezoneOffsetNs,
        undefined,
        undefined,
        undefined,
        timezoneName
      )
      // Do not publish a response into a different metric or window scope.
      if (cacheKey !== seriesDatapointsKey) return
      const series = result?.timeseries.find(t => t.seriesRef === seriesKey)
      // Cache terminal empty results so pending URL selections can resolve.
      seriesDatapoints.set(seriesKey, series?.datapoints ?? [])
    } catch (err) {
      console.error('Failed to fetch series datapoints:', err)
    } finally {
      seriesInFlight.delete(requestKey)
    }
  }

  /** The offset to align store-side buckets to, in nanoseconds. */
  function tzOffsetNs(now = Date.now()): number {
    return timezoneOffsetMinutes(timeContext.tz, now) * 60 * 1_000_000_000
  }

  /** Zone used to resolve bucket offsets across DST transitions. Undefined for UTC. */
  function tzName(): string | undefined {
    const name = resolveTimezoneName(timeContext.tz)
    return name === 'UTC' ? undefined : name
  }

  // Fetch one merged distribution per series for the selected heatmap column.
  $effect(() => {
    // Primitive bounds avoid refetching when only the heatmap array identity changes.
    const startNs = metricCtx.heatmapColumnStartNs
    const endNs = metricCtx.heatmapColumnEndNs
    const summary = page.selectedSummary
    if (startNs === null || endNs === null || !summary) {
      columnDistribution = undefined
      return
    }
    const token = ++columnToken
    // Clear stale column data while the new request is in flight.
    columnDistribution = undefined
    void (async () => {
      try {
        const result = await telemetryAPI.getMetricView(
          summary.metricRef,
          // Preserve the inclusive nanosecond column boundary.
          startNs,
          endNs,
          1,
          undefined,
          DEFAULT_HISTOGRAM_QUANTILES,
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

  // Identifies the current detail request so stale responses cannot replace it.
  let detailToken = 0

  // Fetch datapoints for newly visible series and coalesce rapid legend changes.
  let datapointTimer: ReturnType<typeof setTimeout> | undefined

  $effect(() => {
    const summary = page.selectedSummary
    const metric = selectedMetric
    const checked = metricCtx.visibleSeries
    const visible = [...checked].sort()
    if (!summary || !metric || visible.length === 0) return

    const missing = metric.timeseries.some(
      ts => checked.has(ts.seriesRef) && ts.datapoints.length === 0
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
    /** Series that include datapoints. Null lets the store apply the initial limit. */
    datapointSeries: string[] | null = null,
    /** Whether to seed per-metric state from the result. */
    reseed = true
  ) {
    const token = ++detailToken
    try {
      detailLoading = true
      const { startTime, endTime } = selectionToQueryRangeNs(
        timeContext.selection,
        Date.now()
      )
      // Scalar lines and histogram heatmaps use different display resolutions.
      const isHistogramMetric =
        summary.metricType === 'Histogram' ||
        summary.metricType === 'ExponentialHistogram'
      const bucketTarget = isHistogramMetric
        ? HEATMAP_BUCKET_TARGET
        : METRIC_BUCKET_TARGET
      const result =
        (await telemetryAPI.getMetricView(
          summary.metricRef,
          startTime,
          endTime,
          bucketTarget,
          // Keep every series row; narrow only datapoints below.
          undefined,
          // Quantiles apply only to histogram bucket vectors.
          isHistogramMetric ? DEFAULT_HISTOGRAM_QUANTILES : undefined,
          // Bucket boundaries follow the reader's calendar rather than the
          // epoch. 0 is UTC, which is what the store assumes without this.
          tzOffsetNs(),
          SCALAR_VIEW_BUCKETS,
          SPARKLINE_BUCKETS,
          undefined,
          tzName(),
          // Include datapoints only for drawn series; keep all series metadata.
          datapointSeries ??
            persistedVisibleKeys(summary.metricRef) ??
            undefined,
          DEFAULT_VISIBLE_TIMESERIES
        )) ?? undefined
      // A slower earlier request must not overwrite a newer answer.
      if (token !== detailToken) return
      selectedMetric = result
      // Seed state before the result renders.
      if (reseed) metricCtx.seedForMetric(selectedMetric)
    } catch (err) {
      console.error('Failed to fetch metric detail:', err)
      if (token !== detailToken) return
      selectedMetric = undefined
      metricCtx.seedForMetric(undefined)
    } finally {
      // Only the current request owns the loading state.
      if (token === detailToken) detailLoading = false
    }
  }

  async function handleDeleteMetric(metricRef: string) {
    actionError = null
    try {
      await telemetryAPI.deleteMetric(metricRef)
      // Clear a deleted selection before the list refetches.
      if (page.selectedID === metricRef) {
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
            {#snippet leading()}
              {#key selectedSummary.metricRef}
                <ExportButton signal="metric" id={selectedSummary.metricRef} />
              {/key}
            {/snippet}
          </PaneHeader>
        {:else}
          <PaneHeader
            mode="title"
            title={selectedSummary.name}
            subtitle={selectedSummary.serviceName?.trim() || undefined}
            ariaLabel="Metric chart"
          >
            {#snippet badge()}{@render metricChartHeaderBadge()}{/snippet}
            {#snippet leading()}
              {#key selectedSummary.metricRef}
                <ExportButton signal="metric" id={selectedSummary.metricRef} />
              {/key}
            {/snippet}
          </PaneHeader>
        {/if}
      {/if}
      {#if displayError || importFailure}
        <div
          {...chartTabPanelAttrs}
          class="metrics-page__placeholder alert alert-error"
        >
          {#if displayError}
            <span>Error: {displayError}</span>
          {:else if importFailure}
            <span>Import issue in {importFailure.fileName}.</span>
            <a class="link" href="/#{INGESTION_ISSUES_ID}" onclick={onViewIssue}
              >View issue</a
            >
          {/if}
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
          ? () => handleDeleteMetric(page.selectedSummary!.metricRef)
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

  .metrics-page__placeholder {
    @apply m-[var(--layout-gap)];
  }

  .metrics-empty {
    @apply px-4 py-12 text-center;
    color: var(--color-subtle);
  }
</style>
