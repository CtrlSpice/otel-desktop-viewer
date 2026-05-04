<script module lang="ts">
  import type { MetricSummary } from '@/types/api-types'
  import { metricSummaryKey } from '@/types/api-types'
  import { compareByStringField } from '@/utils/compare'

  // --- Sort ---

  export type MetricSortColumn = 'name' | 'type' | 'unit' | 'service'
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
      case 'type':
        cmp = compareByStringField(a, b, m => m.metricType)
        break
      case 'unit':
        cmp = compareByStringField(a, b, m => m.unit)
        break
      case 'service':
        cmp = compareByStringField(a, b, m => m.serviceName)
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
    { value: 'name', label: 'Name' },
    { value: 'type', label: 'Type' },
    { value: 'service', label: 'Service' },
  ]

  export { metricTypeBadgeClass, metricTypeLabel } from '@/utils/metric-type'
</script>

<script lang="ts">
  import { onMount } from 'svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
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
  import SignalListDrawer from '@/components/SignalListDrawer.svelte'
  import DrawerSearchPanel from '@/components/DrawerSearchPanel.svelte'
  import MetricCard from '@/components/MetricCard.svelte'
  import MetricDetailPanel from '@/components/MetricDetails/MetricDetailPanel.svelte'
  import { TrashIcon } from '@/icons'

  // --- context ---
  let timeContext = getTimeContext()

  // --- state: API / list ---
  let metrics = $state<MetricSummary[]>([])
  let loading = $state(true)
  let error = $state<string | null>(null)
  let mounted = $state(false)

  // --- state: sort ---
  let sortColumn = $state<MetricSortColumn>('name')
  let sortDirection = $state<MetricSortDirection>('asc')

  // --- state: selection ---
  let selectedKey = $state<string | null>(null)
  let selectedMetric = $state<MetricData | undefined>(undefined)
  let detailLoading = $state(false)

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
  $effect(() => {
    if (
      !selectedKey ||
      !sortedMetrics.some(m => metricSummaryKey(m) === selectedKey)
    ) {
      const first = sortedMetrics[0]
      selectedKey = first ? metricSummaryKey(first) : null
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
        (await telemetryAPI.getMetric(
          summary.name,
          summary.unit,
          summary.metricType,
          summary.aggregationTemporality ?? '',
          summary.isMonotonic === null ? '' : String(summary.isMonotonic),
          summary.scopeName,
          summary.scopeVersion,
          summary.serviceName,
          startTime,
          endTime
        )) ?? undefined
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
  <SignalListDrawer
    items={sortedMetrics}
    selectedId={selectedKey}
    drawerId="signal-drawer"
    label="Metrics"
    count={sortedMetrics.length}
    storageKey="metric-drawer"
    onSelect={selectMetric}
    onRefresh={handleRefresh}
    refreshPulse={!!refreshIndicatorText}
    refreshAsideTip={refreshIndicatorText}
    itemKey={metricSummaryKey}
  >
    {#snippet drawerChrome()}
      <DrawerSearchPanel
        segment="chrome"
        signal="metrics"
        sortOptions={SORT_OPTIONS}
        sortValue={sortColumn}
        {sortDirection}
        onSortChange={handleSortChange}
      />
    {/snippet}

    {#snippet drawerChromeToolbar()}
      <DrawerSearchPanel
        segment="toolbar"
        signal="metrics"
        sortOptions={SORT_OPTIONS}
        sortValue={sortColumn}
        {sortDirection}
        onSortChange={handleSortChange}
        onRefresh={handleRefresh}
        refreshPulse={!!refreshIndicatorText}
        refreshAsideTip={refreshIndicatorText}
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
        onRefresh={handleRefresh}
        refreshPulse={!!refreshIndicatorText}
        onSearchResults={handleSearchResults}
        onSearchReady={api => (searchEditorApi = api)}
      />
    {/snippet}

    {#snippet itemSnippet(metric, selected)}
      <MetricCard {metric} {selected} onclick={selectMetric} />
    {/snippet}

    {#snippet footer()}
      <div class="flex items-center justify-between">
        <span class="text-xs tabular-nums text-base-content/50">
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

    {#snippet children()}
      <div class="metrics-content">
        <div class="metrics-content__body">
          {#if error}
            <div class="alert alert-error">
              <span>Error: {error}</span>
            </div>
          {:else if loading && !hasMetricRows}
            <div class="metrics-empty">Loading metrics…</div>
          {:else if !loading && !hasMetricRows}
            <div class="metrics-empty">
              <p class="text-base-content/60">No metrics in this time range</p>
              <p class="mt-2 text-sm text-base-content/50">
                Send telemetry to the exporter or adjust the time range
              </p>
            </div>
          {:else}
            <div class="metrics-detail">
              <MetricDetailPanel metric={selectedMetric} />
            </div>
          {/if}
        </div>
      </div>
    {/snippet}
  </SignalListDrawer>
</div>

<style lang="postcss">
  @reference "../app.css";

  .metrics-page {
    @apply flex min-h-0 min-w-0 w-full flex-1;
  }

  .metrics-content {
    @apply relative flex min-h-0 min-w-0 flex-1 flex-col;
  }

  .metrics-content__body {
    @apply flex min-h-0 min-w-0 flex-1 flex-col p-[var(--layout-gap)];
  }

  .metrics-detail {
    @apply flex-1 min-h-0 min-w-0 overflow-hidden rounded-xl border border-base-300/70 bg-base-100/80 shadow-surface-sm backdrop-blur-sm;
  }

  .metrics-empty {
    @apply rounded-xl border border-base-300/70 bg-base-100/80 px-4 py-12 text-center text-base-content/60 shadow-surface-sm backdrop-blur-sm;
  }
</style>
