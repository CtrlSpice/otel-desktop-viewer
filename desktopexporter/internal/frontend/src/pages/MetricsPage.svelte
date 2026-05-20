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

  export { metricTypeBadgeClass, metricTypeLabel } from '@/utils/metric-type'
</script>

<script lang="ts">
  import { onMount } from 'svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import { metricTypeBadgeClass, metricTypeLabel } from '@/utils/metric-type'
  import {
    getTimeContext,
    selectionToQueryRangeMs,
  } from '@/contexts/time-context.svelte'
  import type {
    MetricData,
    MetricStats,
    SearchResultEvent,
  } from '@/types/api-types'
  import type { SearchEditorAPI } from '@/components/SignalToolbar/search/search-editor-api'
  import PageLayout from '@/components/PageLayout.svelte'
  import ResizablePanels from '@/components/ResizablePanels.svelte'
  import DrawerSearchPanel from '@/components/DrawerSearchPanel.svelte'
  import MetricCard from '@/components/MetricCard.svelte'
  import SignalBadges from '@/components/SignalBadges.svelte'
  import MetricChartView from '@/components/MetricDetails/MetricChartView.svelte'
  import MetricDetailView from '@/components/MetricDetails/MetricDetailView.svelte'
  import TimeseriesPanel from '@/components/MetricDetails/TimeseriesPanel.svelte'
  import SignalFooter from '@/components/SignalFooter.svelte'
  import PaneHeader from '@/components/PaneHeader.svelte'
  import {
    createMetricViewContext,
    getMetricViewContext,
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

  let chartTimeRange = $derived.by(():
    | { startMs: number; endMs: number }
    | undefined => {
    const summary = selectedSummary
    if (!summary) return undefined
    if (summary.metricType === 'Gauge' || summary.metricType === 'Sum') {
      let min = Infinity
      let max = -Infinity
      for (const ts of metricCtx.gaugeSumChartTimeseries) {
        for (const p of ts.points) {
          const t = p.date.getTime()
          if (t < min) min = t
          if (t > max) max = t
        }
      }
      if (!Number.isFinite(min)) return undefined
      return { startMs: min, endMs: max }
    }
    if (summary.metricType === 'Histogram') {
      const qr = selectionToQueryRangeMs(
        timeContext.selection,
        Date.now()
      )
      return { startMs: qr.start, endMs: qr.end }
    }
    return undefined
  })

  // Position of the currently-selected metric in the sorted list.
  // Powers the DetailNav prev/next/first/last controls in the
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

  $effect(() => {
    const summary = selectedSummary
    if (!summary) {
      selectedMetric = undefined
      return
    }
    fetchMetricDetail(summary)
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
      metrics = event.results as unknown as MetricSummary[]
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
    minDetailPx={240}
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
           detail pane (Fields/Datapoints) renders alongside. The
           SignalFooter is now page-level chrome (see pageFooter
           snippet below): always present, spans main + detail
           regardless of content state, and DetailNav self-disables
           when there is nothing to navigate. -->
      {#if selectedSummary}
          <PaneHeader
            mode="title"
            title={selectedSummary.name}
            subtitle={selectedSummary.serviceName?.trim() || undefined}
            timeRange={chartTimeRange}
            rounded={false}
            ariaLabel="Metric chart"
          >
            {#snippet badge()}
              <SignalBadges
                signal="metric"
                metricType={selectedSummary.metricType}
                aggregationTemporality={selectedSummary.aggregationTemporality}
                isMonotonic={selectedSummary.isMonotonic}
              />
            {/snippet}
          </PaneHeader>
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
          <!-- Vertical split: chart on top, timeseries panel on bottom.
               stackBreakpoint=Infinity forces the stacked (vertical-
               resize) variant of ResizablePanels regardless of width;
               we always want this one stacked even at desktop widths.
               Bottom slot is a placeholder for now -- TimeseriesPanel
               lands in the next step. -->
          <div class="metrics-page__split">
            <ResizablePanels
              defaultLeftWidth={0.6}
              minLeftWidth={0.25}
              minRightWidth={0.15}
              minLeftPx={200}
              minRightPx={128}
              maxRightPx={320}
              storageKey="metrics:vsplit"
              stackBreakpoint={Number.POSITIVE_INFINITY}
              stackedResizeHandle="panel-header"
            >
              {#snippet leftPanel()}
                <div class="metrics-page__chart">
                  <MetricChartView />
                </div>
              {/snippet}
              {#snippet rightPanel()}
                <TimeseriesPanel />
              {/snippet}
            </ResizablePanels>
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
    @apply flex min-h-0 min-w-0 w-full flex-1;
  }

  .metrics-page__chart {
    @apply flex min-h-0 min-w-0 flex-1 flex-col overflow-hidden;
  }

  /* Vertical split host: the ResizablePanels needs a min-sized flex
     parent so it can claim available height inside the metrics-page
     column. Same shrink/min-size discipline as the placeholders. */
  .metrics-page__split {
    @apply flex-1 min-h-0 min-w-0;
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
