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
  import { onMount, untrack } from 'svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import { metricTypeBadgeClass, metricTypeLabel } from '@/components/metrics/utils/metric-type'
  import {
    getTimeContext,
    selectionToQueryRangeMs,
  } from '@/contexts/time-context.svelte'
  import type {
    MetricData,
    MetricStats,
    SearchResultEvent,
  } from '@/types/api-types'
  import type { SearchEditorAPI } from '@/components/shared/Search/search-editor-api'
  import PageLayout from '@/components/shared/PageLayout.svelte'
  import DrawerSearchPanel from '@/components/shared/Drawer/DrawerSearchPanel.svelte'
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
  import { TrashIcon } from '@/icons'

  // --- context ---
  let timeContext = getTimeContext()

  // --- state: API / list ---
  let metrics = $state<MetricSummary[]>([])
  let loading = $state(true)
  let error = $state<string | null>(null)
  let mounted = $state(false)

  // --- state: sort ---
  let sortColumn = $state<MetricSortColumn>('lastSeen')
  let sortDirection = $state<MetricSortDirection>('desc')

  // --- state: selection ---
  let selectedKey = $state<string | null>(null)
  let selectedMetric = $state<MetricData | undefined>(undefined)
  let detailLoading = $state(false)

  // --- metric-view context ---
  // Owns per-metric reactive state shared by MetricChartView and
  // MetricDetailView (selection, expansion, histogram tab, legend
  // visibility, bucket-series fetches, plus all the cheap derivations
  // both panes need). We pass a getter so the context's identity stays
  // stable for this page's lifetime even as the user navigates between
  // metrics; the factory's effects re-seed per-metric state when the
  // metric.id changes.
  createMetricViewContext(() => selectedMetric)
  const metricCtx = getMetricViewContext()

  // --- state: polling / refresh indicator ---
  let searchEditorApi = $state<SearchEditorAPI | null>(null)
  let baselineStats = $state<MetricStats | null>(null)
  let polledStats = $state<MetricStats | null>(null)
  const POLL_INTERVAL_MS = 3000

  // --- derived ---
  let sortedMetrics = $derived.by(() => {
    const col = sortColumn
    const dir = sortDirection
    const rows = [...metrics]
    rows.sort((a, b) => compareMetrics(a, b, col, dir))
    return rows
  })

  let hasMetricRows = $derived(metrics.length > 0)

  let selectedSummary = $derived(
    selectedKey
      ? sortedMetrics.find(m => metricSummaryKey(m) === selectedKey)
      : undefined
  )

  let chartAggregationTabs = $derived(
    aggregationViewTabs(metricCtx.availableAggregationViews)
  )

  let showChartAggregationTabs = $derived(
    (selectedSummary?.metricType === 'Sum' ||
      selectedSummary?.metricType === 'Gauge') &&
      chartAggregationTabs.length > 1
  )

  let showChartHistogramTabs = $derived(
    selectedSummary?.metricType === 'Histogram' ||
      selectedSummary?.metricType === 'ExponentialHistogram'
  )

  let showChartTitleTabs = $derived(
    showChartAggregationTabs || showChartHistogramTabs
  )

  // Position of the currently-selected metric in the sorted list.
  // SignalFooter that lives in PageLayout's pageFooter slot
  // (page-level chrome spanning main + detail). Returns -1 when
  // nothing is selected (DetailNav renders all buttons disabled in
  // that case).
  let selectedIndex = $derived.by(() => {
    if (!selectedKey) return -1
    return sortedMetrics.findIndex(m => metricSummaryKey(m) === selectedKey)
  })

  function selectByIndex(i: number) {
    const target = sortedMetrics[i]
    if (target) selectedKey = metricSummaryKey(target)
  }
  function navFirst() {
    selectByIndex(0)
  }
  function navPrev() {
    if (selectedIndex > 0) selectByIndex(selectedIndex - 1)
  }
  function navNext() {
    if (selectedIndex >= 0 && selectedIndex < sortedMetrics.length - 1) {
      selectByIndex(selectedIndex + 1)
    }
  }
  function navLast() {
    selectByIndex(sortedMetrics.length - 1)
  }

  let refreshIndicatorText = $derived.by(() => {
    if (!baselineStats || !polledStats) return ''
    const parts: string[] = []
    const metricDelta = polledStats.metricCount - baselineStats.metricCount
    if (metricDelta > 0)
      parts.push(`+${metricDelta} metric${metricDelta !== 1 ? 's' : ''}`)
    const dpDelta = polledStats.dataPointCount - baselineStats.dataPointCount
    if (dpDelta > 0) parts.push(`+${dpDelta} dp${dpDelta !== 1 ? 's' : ''}`)
    return parts.join(', ')
  })

  // --- effects ---
  let lastValidIndex = $state(0)

  $effect(() => {
    const idx = selectedKey
      ? sortedMetrics.findIndex(m => metricSummaryKey(m) === selectedKey)
      : -1
    if (idx >= 0) {
      lastValidIndex = idx
    } else if (sortedMetrics.length > 0) {
      const fallback =
        sortedMetrics[Math.min(lastValidIndex, sortedMetrics.length - 1)]
      selectedKey = fallback ? metricSummaryKey(fallback) : null
    } else {
      selectedKey = null
    }
  })

  // Re-fetch metric detail ONLY when the selected metric's identity
  // changes -- not when the summary object reference churns. Polling
  // rebuilds `metrics` (and therefore `sortedMetrics`/`selectedSummary`)
  // every few seconds with fresh object references; if we depended on
  // the summary object directly the effect would re-fetch on every
  // poll, which would also re-fire the context's per-metric reset and
  // clobber per-metric view state (e.g. AggregationView, legend selections).
  // We read the *id* reactively (stable per metric) and grab the
  // current summary via untrack so its identity churn doesn't count
  // as a dep.
  $effect(() => {
    const id = selectedSummary
      ? metricSummaryKey(selectedSummary)
      : null
    if (!id) {
      selectedMetric = undefined
      return
    }
    const summary = untrack(() => selectedSummary)
    if (summary) fetchMetricDetail(summary)
  })

  $effect(() => {
    void timeContext.selection
    if (mounted) {
      fetchMetrics()
    }
  })

  $effect(() => {
    if (!mounted) return
    const id = setInterval(async () => {
      try {
        const s = await telemetryAPI.getStats()
        polledStats = s.metrics
      } catch {
        /* polling failures are silent */
      }
    }, POLL_INTERVAL_MS)
    return () => clearInterval(id)
  })

  // --- handlers ---
  function handleSortChange(value: string, direction: 'asc' | 'desc') {
    sortColumn = value as MetricSortColumn
    sortDirection = direction
  }

  function selectMetric(key: string) {
    selectedKey = key
  }

  async function fetchMetrics() {
    try {
      loading = true
      error = null
      const { start: startTime, end: endTime } = selectionToQueryRangeMs(
        timeContext.selection,
        Date.now()
      )
      metrics = await telemetryAPI.searchMetricSummaries(startTime, endTime)
      const s = await telemetryAPI.getStats()
      baselineStats = s.metrics
      polledStats = s.metrics
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load metrics'
    } finally {
      loading = false
    }
  }

  async function fetchMetricDetail(summary: MetricSummary) {
    try {
      detailLoading = true
      const { start: startTime, end: endTime } = selectionToQueryRangeMs(
        timeContext.selection,
        Date.now()
      )
      selectedMetric =
        (await telemetryAPI.getMetric(summary.id, startTime, endTime)) ??
        undefined
    } catch (err) {
      console.error('Failed to fetch metric detail:', err)
      selectedMetric = undefined
    } finally {
      detailLoading = false
    }
  }

  function handleSearchResults(event: SearchResultEvent) {
    if (event.signal === 'metrics') {
      loading = false
      error = null
      metrics = event.results
    }
  }

  function handleRefresh() {
    searchEditorApi?.clear()
    fetchMetrics()
  }

  async function handleDeleteAllMetrics() {
    try {
      await telemetryAPI.clearMetrics()
      selectedKey = null
      selectedMetric = undefined
      await fetchMetrics()
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to delete metrics'
    }
  }

  // --- lifecycle ---
  onMount(async () => {
    await fetchMetrics()
    mounted = true
  })
</script>

<div class="metrics-page">
  <PageLayout
    items={sortedMetrics}
    selectedId={selectedKey}
    drawerId="signal-drawer"
    drawerLabel="Metrics"
    onSelect={selectMetric}
    onRefresh={handleRefresh}
    refreshPulse={!!refreshIndicatorText}
    refreshAsideTip={refreshIndicatorText}
    {loading}
    itemKey={metricSummaryKey}
    resizableStorageKey="metric-detail-panels"
    minDetailPx={352}
  >
    {#snippet drawerChromeToolbar()}
      <DrawerSearchPanel
        segment="toolbar"
        signal="metrics"
        sortOptions={SORT_OPTIONS}
        sortValue={sortColumn}
        {sortDirection}
        onSortChange={handleSortChange}
      />
    {/snippet}

    {#snippet drawerSearch()}
      <DrawerSearchPanel
        segment="search"
        signal="metrics"
        sortOptions={SORT_OPTIONS}
        sortValue={sortColumn}
        {sortDirection}
        onSortChange={handleSortChange}
        onSearchResults={handleSearchResults}
        onSearchReady={api => (searchEditorApi = api)}
      />
    {/snippet}

    {#snippet itemSnippet(metric, selected)}
      <MetricCard {metric} {selected} onclick={selectMetric} />
    {/snippet}

    {#snippet drawerFooter()}
      <div class="flex items-center justify-between">
        <span class="text-xs tabular-nums text-rp-muted">
          {sortedMetrics.length} metric{sortedMetrics.length !== 1 ? 's' : ''}
        </span>
        <button
          type="button"
          class="btn btn-ghost btn-xs text-error"
          onclick={handleDeleteAllMetrics}
          aria-label="Delete all metrics"
        >
          <TrashIcon class="h-3 w-3" aria-hidden="true" />
          Delete all
        </button>
      </div>
    {/snippet}

    {#snippet main()}
      <!-- Page-level error / empty branches replace the chart pane
           entirely; the chart lives here on the happy path and the
           detail pane (Fields/Series) renders alongside. The
           SignalFooter is now page-level chrome (see pageFooter
           snippet below): always present, spans main + detail
           regardless of content state, and DetailNav self-disables
           when there is nothing to navigate. -->
        {#if selectedSummary}
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
        {#if error}
          <div class="metrics-page__placeholder alert alert-error">
            <span>Error: {error}</span>
          </div>
        {:else if loading && !hasMetricRows}
          <div class="metrics-page__placeholder metrics-empty">
            Loading metrics…
          </div>
        {:else if !loading && !hasMetricRows}
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
        index={selectedIndex}
        total={sortedMetrics.length}
        label="metric"
        onFirst={navFirst}
        onPrev={navPrev}
        onNext={navNext}
        onLast={navLast}
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
