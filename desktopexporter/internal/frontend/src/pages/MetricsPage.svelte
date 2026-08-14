<script module lang="ts">
  import type { MetricSummary } from '@/types/api-types'
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

  const SORT_OPTIONS = [
    { value: 'lastSeen', label: 'Last Seen' },
    { value: 'name', label: 'Name' },
    { value: 'metricType', label: 'Type' },
    { value: 'serviceName', label: 'Service Name' },
    { value: 'description', label: 'Description' },
    { value: 'dataPointCount', label: 'Datapoint Count' },
    { value: 'seriesCount', label: 'Timeseries Count' },
  ]

  export { metricTypeBadgeClass, metricTypeLabel } from '@/components/metrics/utils/metric-type'
</script>

<script lang="ts">
  import { METRIC_BUCKET_TARGET } from '@/contexts/metric-view-context.svelte'
  import { untrack } from 'svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import {
    getTimeContext,
    selectionToQueryRangeMs,
  } from '@/contexts/time-context.svelte'
  import { navigateToItem } from '@/route'
  import type { MetricData, MetricStats } from '@/types/api-types'
  import { createSignalListPage } from '@/contexts/signal-list-page.svelte'
  import PageLayout from '@/components/shared/PageLayout.svelte'
  import DrawerSearchPanel from '@/components/shared/Drawer/DrawerSearchPanel.svelte'
  import SignalDrawerFooter from '@/components/shared/Drawer/SignalDrawerFooter.svelte'
  import MetricCard from '@/components/metrics/MetricCard.svelte'
  import SignalBadges from '@/components/shared/SignalBadges.svelte'
  import MetricChartView from '@/components/metrics/Charts/MetricChartView.svelte'
  import MetricDetailView from '@/components/metrics/Detail/MetricDetailView.svelte'
  import SignalFooter from '@/components/shared/SignalFooter.svelte'
  import PaneHeader from '@/components/shared/PaneHeader.svelte'
  import type { AggregationView } from '@/components/metrics/utils/aggregation'
  import { aggregationViewTabs } from '@/components/metrics/utils/aggregation-view-tabs'
  import { histogramViewTabs } from '@/components/metrics/utils/histogram-view-tabs'
  import {
    createMetricViewContext,
    getMetricViewContext,
    type HistogramTab,
  } from '@/contexts/metric-view-context.svelte'

  let timeContext = getTimeContext()

  let baselineStats = $state<MetricStats | null>(null)
  let polledStats = $state<MetricStats | null>(null)
  let actionError = $state<string | null>(null)

  const page = createSignalListPage<MetricSummary>({
    signal: 'metrics',
    getItemId: metricSummaryKey,
    initialSort: { column: 'lastSeen', direction: 'desc' },
    compare: (a, b, col, dir) =>
      compareMetrics(a, b, col as MetricSortColumn, dir as MetricSortDirection),
    fetchList: async () => {
      const { start: startTime, end: endTime } = selectionToQueryRangeMs(
        timeContext.selection,
        Date.now()
      )
      const results = await telemetryAPI.searchMetricSummaries(startTime, endTime)
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

  createMetricViewContext(() => selectedMetric)
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

  // Re-fetch metric detail ONLY when the selected metric's identity changes --
  // not when the summary object reference churns. Polling rebuilds the list
  // every few seconds with fresh object references; depending on the summary
  // object directly would re-fetch on every poll and clobber per-metric view
  // state (AggregationView, legend selections, etc.).
  $effect(() => {
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

  function selectMetric(key: string) {
    page.selectItem(key)
  }

  async function fetchMetricDetail(summary: MetricSummary) {
    try {
      detailLoading = true
      const { start: startTime, end: endTime } = selectionToQueryRangeMs(
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
      selectedMetric =
        (await telemetryAPI.getMetric(
          summary.id,
          startTime,
          endTime,
          METRIC_BUCKET_TARGET
        )) ?? undefined
    } catch (err) {
      console.error('Failed to fetch metric detail:', err)
      selectedMetric = undefined
    } finally {
      detailLoading = false
    }
  }

  async function handleDeleteMetric(streamID: string) {
    actionError = null
    try {
      await telemetryAPI.deleteMetricStream(streamID)
      // Clear the detail pane before refetching: the $effect above keys off
      // page.selectedSummary, and the deleted stream is gone from the next
      // list fetch, so leaving it set would render a stale chart.
      if (page.selectedId === streamID) {
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
    selectedId={page.selectedId}
    drawerId="signal-drawer"
    drawerLabel="Metrics"
    onRefresh={page.handleRefresh}
    refreshPulse={page.refreshPulse}
    refreshAsideTip={page.refreshAsideTip}
    loading={page.loading}
    itemKey={metricSummaryKey}
    resizableStorageKey="metric-detail-panels"
    minDetailPx={352}
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
            activeId={showChartAggregationTabs
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
          <div class="metrics-page__placeholder alert alert-error">
            <span>Error: {displayError}</span>
          </div>
        {:else if page.loading && !hasMetricRows}
          <div class="metrics-page__placeholder metrics-empty">
            Loading metrics…
          </div>
        {:else if !page.loading && !hasMetricRows}
          <div class="metrics-page__placeholder metrics-empty">
            <p class="text-rp-subtle">No metrics in this time range</p>
            <p class="mt-2 text-sm text-rp-muted">
              Send telemetry to the exporter or adjust the time range
            </p>
          </div>
        {:else}
          <div class="metrics-page__chart">
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
