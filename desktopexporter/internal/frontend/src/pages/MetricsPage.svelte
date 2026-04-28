<script module lang="ts">
  import type { MetricData, MetricType } from '@/types/api-types'
  import {
    compareByStringField,
    compareByTimestampField,
  } from '@/utils/compare'
  import { getServiceName } from '@/utils/resource'

  // --- Sort ---

  export type MetricSortColumn = 'name' | 'type' | 'unit' | 'service' | 'datapoints'
  export type MetricSortDirection = 'asc' | 'desc'

  function getMetricType(m: MetricData): string {
    return m.datapoints[0]?.metricType ?? 'Empty'
  }

  function compareMetrics(
    a: MetricData,
    b: MetricData,
    col: MetricSortColumn,
    dir: MetricSortDirection
  ): number {
    let cmp: number
    switch (col) {
      case 'name':
        cmp = compareByStringField(a, b, m => m.name)
        break
      case 'type':
        cmp = compareByStringField(a, b, m => getMetricType(m))
        break
      case 'unit':
        cmp = compareByStringField(a, b, m => m.unit)
        break
      case 'service':
        cmp = compareByStringField(a, b, m => getServiceName(m.resource))
        break
      case 'datapoints':
        cmp = a.datapoints.length - b.datapoints.length
        break
      default:
        cmp = 0
    }

    return cmp !== 0
      ? dir === 'asc'
        ? cmp
        : -cmp
      : a.id.localeCompare(b.id)
  }

  const SORT_OPTIONS = [
    { value: 'name', label: 'Name' },
    { value: 'type', label: 'Type' },
    { value: 'service', label: 'Service' },
    { value: 'datapoints', label: 'Datapoints' },
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
  import type { SearchResultEvent, MetricStats } from '@/types/api-types'
  import type { SearchEditorAPI } from '@/components/SignalToolbar/search/search-editor-api'
  import SignalToolbar from '@/components/SignalToolbar/SignalToolbar.svelte'
  import SearchEditor from '@/components/SignalToolbar/search/SearchEditor.svelte'
  import DateTimeFilter from '@/components/SignalToolbar/datetime/DateTimeFilter.svelte'
  import SignalListDrawer from '@/components/SignalListDrawer.svelte'
  import MetricCard from '@/components/MetricCard.svelte'
  import MetricDetailPanel from '@/components/MetricDetails/MetricDetailPanel.svelte'
  import { ChartHistogramIcon, TrashIcon } from '@/icons'

  // --- context ---
  let timeContext = getTimeContext()

  // --- state: API / list ---
  let metrics = $state<MetricData[]>([])
  let loading = $state(true)
  let error = $state<string | null>(null)
  let mounted = $state(false)
  let searchError = $state<string | null>(null)

  // --- state: sort ---
  let sortColumn = $state<MetricSortColumn>('name')
  let sortDirection = $state<MetricSortDirection>('asc')

  // --- state: selection ---
  let selectedMetricId = $state<string | null>(null)

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

  let selectedMetric = $derived(
    selectedMetricId ? sortedMetrics.find(m => m.id === selectedMetricId) : undefined
  )

  let refreshIndicatorText = $derived.by(() => {
    if (!baselineStats || !polledStats) return ''
    const parts: string[] = []
    const metricDelta = polledStats.metricCount - baselineStats.metricCount
    if (metricDelta > 0)
      parts.push(`+${metricDelta} metric${metricDelta !== 1 ? 's' : ''}`)
    const dpDelta = polledStats.dataPointCount - baselineStats.dataPointCount
    if (dpDelta > 0)
      parts.push(`+${dpDelta} dp${dpDelta !== 1 ? 's' : ''}`)
    return parts.join(', ')
  })

  // --- effects ---
  $effect(() => {
    if (!selectedMetricId || !sortedMetrics.some(m => m.id === selectedMetricId)) {
      selectedMetricId = sortedMetrics[0]?.id ?? null
    }
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

  function selectMetric(metricId: string) {
    selectedMetricId = metricId
  }

  async function fetchMetrics() {
    try {
      loading = true
      error = null
      const { start: startTime, end: endTime } = selectionToQueryRangeMs(
        timeContext.selection,
        Date.now()
      )
      metrics = await telemetryAPI.getMetrics(startTime, endTime, undefined)
      const s = await telemetryAPI.getStats()
      baselineStats = s.metrics
      polledStats = s.metrics
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load metrics'
    } finally {
      loading = false
    }
  }

  function handleRefresh() {
    searchEditorApi?.clear()
    fetchMetrics()
  }

  function handleSearchResults(event: SearchResultEvent) {
    if (event.signal === 'metrics' && event.view === 'list') {
      loading = false
      error = null
      metrics = event.results
    }
  }

  async function handleDeleteMetric(metricId: string) {
    const idx = sortedMetrics.findIndex(m => m.id === metricId)
    const nextIdx = idx < sortedMetrics.length - 1 ? idx + 1 : idx - 1
    const nextId = nextIdx >= 0 ? sortedMetrics[nextIdx]?.id ?? null : null
    try {
      await telemetryAPI.deleteMetrics([metricId])
      selectedMetricId = nextId
      await fetchMetrics()
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to delete metric'
    }
  }

  async function handleDeleteAllMetrics() {
    try {
      await telemetryAPI.clearMetrics()
      selectedMetricId = null
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

{#snippet toolbarTimeRange()}
  <DateTimeFilter />
{/snippet}

<div class="metrics-page">
  <SignalListDrawer
    items={sortedMetrics}
    selectedId={selectedMetricId}
    drawerId="signal-drawer"
    label="Metrics"
    count={sortedMetrics.length}
    sortOptions={SORT_OPTIONS}
    sortValue={sortColumn}
    {sortDirection}
    storageKey="metric-drawer"
    onSelect={selectMetric}
    onSortChange={handleSortChange}
  >
    {#snippet icon()}
      <ChartHistogramIcon />
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
        <div class="metrics-content__toolbar">
          <SignalToolbar
            signal="metrics"
            view="list"
            onRefresh={handleRefresh}
            trailingFilters={[toolbarTimeRange]}
            {searchError}
            {refreshIndicatorText}
          >
            <SearchEditor
              signal="metrics"
              view="list"
              inToolbar
              onSearchResults={handleSearchResults}
              onSearchError={err => (searchError = err)}
              onReady={api => (searchEditorApi = api)}
            />
          </SignalToolbar>
        </div>

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
              <MetricDetailPanel
                metric={selectedMetric}
                onDelete={handleDeleteMetric}
              />
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
    @apply flex min-h-0 min-w-0 flex-1 flex-col;
  }

  .metrics-content__toolbar {
    @apply shrink-0 border-b border-base-300/40 bg-base-100/60 px-[var(--layout-gap)] py-2 backdrop-blur-sm;
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
