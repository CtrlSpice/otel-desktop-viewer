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

  // --- Table state (localStorage persistence) ---

  const METRIC_TABLE_STORAGE_KEY = 'otel-desktop-viewer:metric-list-table-state-v1'

  interface MetricListTableState {
    sortColumn: MetricSortColumn
    sortDirection: MetricSortDirection
    rowsPerPage: number
  }

  const METRIC_TABLE_DEFAULTS: MetricListTableState = {
    sortColumn: 'name',
    sortDirection: 'asc',
    rowsPerPage: 25,
  }

  const VALID_METRIC_SORT_COLUMNS: ReadonlySet<string> = new Set<MetricSortColumn>([
    'name',
    'type',
    'unit',
    'service',
    'datapoints',
  ])

  const VALID_ROWS_PER_PAGE: ReadonlySet<number> = new Set([10, 25, 50, 100])

  function loadMetricListTableState(): MetricListTableState {
    if (typeof localStorage === 'undefined') return { ...METRIC_TABLE_DEFAULTS }
    try {
      const raw = localStorage.getItem(METRIC_TABLE_STORAGE_KEY)
      if (!raw) return { ...METRIC_TABLE_DEFAULTS }
      const o = JSON.parse(raw) as Partial<MetricListTableState>
      return {
        sortColumn: VALID_METRIC_SORT_COLUMNS.has(o.sortColumn ?? '')
          ? (o.sortColumn as MetricSortColumn)
          : METRIC_TABLE_DEFAULTS.sortColumn,
        sortDirection:
          o.sortDirection === 'asc' || o.sortDirection === 'desc'
            ? o.sortDirection
            : METRIC_TABLE_DEFAULTS.sortDirection,
        rowsPerPage: VALID_ROWS_PER_PAGE.has(o.rowsPerPage ?? -1)
          ? o.rowsPerPage!
          : METRIC_TABLE_DEFAULTS.rowsPerPage,
      }
    } catch {
      return { ...METRIC_TABLE_DEFAULTS }
    }
  }

  function saveMetricListTableState(state: MetricListTableState): void {
    if (typeof localStorage === 'undefined') return
    localStorage.setItem(METRIC_TABLE_STORAGE_KEY, JSON.stringify(state))
  }

  export { metricTypeBadgeClass, metricTypeLabel } from '@/utils/metric-type'
</script>

<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import { metricTypeBadgeClass, metricTypeLabel } from '@/utils/metric-type'
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
  import ResizablePanels from '@/components/ResizablePanels.svelte'
  import MetricDetailPanel from '@/components/MetricDetails/MetricDetailPanel.svelte'
  import {
    ArrowDownIcon,
    ArrowLeftDoubleIcon,
    ArrowLeftIcon,
    ArrowRightDoubleIcon,
    ArrowRightIcon,
    TrashIcon,
  } from '@/icons'
  import {
    flex,
    computeInitialWidths,
    redistributeWidths,
    computeBarPositions,
    startColumnResize,
  } from '@/utils/column-resize'
  import { tableNav } from '@/utils/table-keyboard-nav'

  // --- context ---
  let timeContext = getTimeContext()

  // --- state: API / list ---
  let metrics = $state<MetricData[]>([])
  let loading = $state(true)
  let error = $state<string | null>(null)
  let mounted = $state(false)
  let searchError = $state<string | null>(null)

  // --- state: sort + pagination (persisted via localStorage) ---
  const savedTableState = loadMetricListTableState()
  let sortColumn = $state<MetricSortColumn>(savedTableState.sortColumn)
  let sortDirection = $state<MetricSortDirection>(savedTableState.sortDirection)
  let currentPage = $state(1)
  let rowsPerPage = $state(savedTableState.rowsPerPage)
  let rowsPerPageOptions = [10, 25, 50, 100]
  let rowsPerPagePopoverOpen = $state(false)

  // --- state: selection ---
  let selectedMetricId = $state<string | null>(null)

  // --- state: polling / refresh indicator ---
  let searchEditorApi = $state<SearchEditorAPI | null>(null)
  let baselineStats = $state<MetricStats | null>(null)
  let polledStats = $state<MetricStats | null>(null)
  const POLL_INTERVAL_MS = 3000

  // --- state: column resize ---
  const metricCols = [
    flex('name', 120, 4),
    flex('type', 60, 1),
    flex('unit', 50, 1),
    flex('service', 80, 2),
    flex('datapoints', 40, 1),
  ]

  let activeResizeCol = $state<number | null>(null)
  let metricContainerEl = $state<HTMLDivElement | null>(null)
  let colWidths = $state(metricCols.map(d => d.min))

  let barPositions = $derived(computeBarPositions(metricCols, colWidths))

  $effect(() => {
    if (!metricContainerEl) return
    untrack(() => {
      colWidths = computeInitialWidths(metricCols, metricContainerEl!.clientWidth)
    })
    const ro = new ResizeObserver(entries => {
      const w = entries[0]?.contentRect.width
      if (w && activeResizeCol === null) {
        colWidths = redistributeWidths(metricCols, colWidths, w)
      }
    })
    ro.observe(metricContainerEl)
    return () => ro.disconnect()
  })

  function handleStartResize(colIndex: number, e: PointerEvent) {
    activeResizeCol = colIndex
    startColumnResize(
      metricCols,
      () => colWidths, colIndex, e,
      next => { colWidths = next },
      () => { activeResizeCol = null }
    )
  }

  // --- derived: table rows ---
  let sortedMetrics = $derived.by(() => {
    const col = sortColumn
    const dir = sortDirection
    const rows = [...metrics]
    rows.sort((a, b) => compareMetrics(a, b, col, dir))
    return rows
  })

  let paginatedMetrics = $derived.by(() => {
    const start = (currentPage - 1) * rowsPerPage
    const end = start + rowsPerPage
    return sortedMetrics.slice(start, end)
  })

  let totalPages = $derived(Math.ceil(sortedMetrics.length / rowsPerPage))
  let startRow = $derived(
    sortedMetrics.length === 0 ? 0 : (currentPage - 1) * rowsPerPage + 1
  )
  let endRow = $derived(
    Math.min(currentPage * rowsPerPage, sortedMetrics.length)
  )

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
    saveMetricListTableState({ sortColumn, sortDirection, rowsPerPage })
  })

  $effect(() => {
    const first = paginatedMetrics[0]
    if (!selectedMetricId || !sortedMetrics.some(m => m.id === selectedMetricId)) {
      selectedMetricId = first?.id ?? null
    }
  })

  $effect(() => {
    const popover = document.getElementById('metric-rows-per-page-popover')
    if (popover) {
      const handleToggle = () => {
        rowsPerPagePopoverOpen = popover.matches(':popover-open')
      }
      popover.addEventListener('toggle', handleToggle)
      return () => popover.removeEventListener('toggle', handleToggle)
    }
  })

  $effect(() => {
    const n = sortedMetrics.length
    const pages = Math.max(1, Math.ceil(n / rowsPerPage))
    if (n > 0 && currentPage > pages) {
      currentPage = pages
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
  function handleSort(column: MetricSortColumn) {
    if (sortColumn === column) {
      sortDirection = sortDirection === 'asc' ? 'desc' : 'asc'
    } else {
      sortColumn = column
      sortDirection = 'asc'
    }
    currentPage = 1
  }

  function handleRowsPerPageChange(newRowsPerPage: number) {
    rowsPerPage = newRowsPerPage
    currentPage = 1
  }

  function goToPage(page: number) {
    if (page >= 1 && page <= totalPages) {
      currentPage = page
    }
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
    const idx = paginatedMetrics.findIndex(m => m.id === metricId)
    const nextIdx = idx < paginatedMetrics.length - 1 ? idx + 1 : idx - 1
    const nextId = nextIdx >= 0 ? paginatedMetrics[nextIdx]?.id ?? null : null
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
      currentPage = 1
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

{#snippet sortIndicator(column: MetricSortColumn)}
  <span class="table-header-sort__indicator">
    <ArrowDownIcon
      class="sort-indicator {sortColumn === column
        ? 'sort-indicator--active'
        : 'sort-indicator--inactive'} {sortColumn === column && sortDirection === 'asc'
        ? 'sort-indicator--asc'
        : ''}"
      aria-hidden="true"
    />
  </span>
{/snippet}

<div
  class="flex min-h-0 min-w-0 w-full flex-1 flex-col gap-[var(--layout-gap)] pt-0"
>
  <div class="page-toolbar-block">
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

  {#if error}
    <div class="alert alert-error">
      <span>Error: {error}</span>
    </div>
  {/if}

  <div class="flex min-h-0 flex-1 flex-col gap-[var(--layout-gap)]">
    {#if loading && !hasMetricRows}
      <div
        class="rounded-xl border border-base-300/70 bg-base-100/80 px-4 py-12 text-center text-base-content/60 shadow-surface-sm backdrop-blur-sm"
      >
        Loading metrics…
      </div>
    {:else if !loading && !hasMetricRows}
      <div
        class="rounded-xl border border-base-300/70 bg-base-100/80 px-4 py-12 text-center shadow-surface-sm backdrop-blur-sm"
      >
        <p class="text-base-content/60">No metrics in this time range</p>
        <p class="mt-2 text-sm text-base-content/50">
          Send telemetry to the exporter or adjust the time range
        </p>
      </div>
    {:else}
      <div class="min-h-0 flex-1">
        <ResizablePanels
          defaultLeftWidth={0.6}
          minLeftWidth={0.3}
          minRightWidth={0.2}
          storageKey="metric-detail-panels"
        >
          {#snippet leftPanel()}
            <div
              class="flex h-full flex-col overflow-hidden rounded-xl border border-base-300/70 bg-base-100/80 shadow-surface-sm backdrop-blur-sm transition-opacity duration-200 {loading
                ? 'opacity-70'
                : 'opacity-100'}"
            >
              <div class="flex-1 min-h-0 overflow-x-auto overflow-y-auto" bind:this={metricContainerEl}>
                <div class="col-resize-context">
                <table
                  class="metric-list-table table table-fixed table-sm w-full border-collapse"
                  use:tableNav={{
                    rowIdAttr: 'metric-id',
                    onSelect: id => { selectedMetricId = id },
                  }}
                >
                  <colgroup>
                    {#each colWidths as w, i (i)}
                      <col style:width="{w}px" />
                    {/each}
                  </colgroup>
                  <thead class="sticky top-0 z-10 table-header-surface">
                    <tr class="table-header-row">
                      <th
                        class="table-header-cell table-header-cell--sortable table-header-cell--left group"
                        onclick={() => handleSort('name')}
                        role="button"
                        tabindex="0"
                        onkeydown={e => e.key === 'Enter' && handleSort('name')}
                      >
                        <div class="table-header-sort">
                          <span class="table-header-sort__label">Name</span>
                          {@render sortIndicator('name')}
                        </div>
                      </th>
                      <th
                        class="table-header-cell table-header-cell--sortable table-header-cell--left group"
                        onclick={() => handleSort('type')}
                        role="button"
                        tabindex="0"
                        onkeydown={e => e.key === 'Enter' && handleSort('type')}
                      >
                        <div class="table-header-sort">
                          <span class="table-header-sort__label">Type</span>
                          {@render sortIndicator('type')}
                        </div>
                      </th>
                      <th
                        class="table-header-cell table-header-cell--sortable table-header-cell--left group"
                        onclick={() => handleSort('unit')}
                        role="button"
                        tabindex="0"
                        onkeydown={e => e.key === 'Enter' && handleSort('unit')}
                      >
                        <div class="table-header-sort">
                          <span class="table-header-sort__label">Unit</span>
                          {@render sortIndicator('unit')}
                        </div>
                      </th>
                      <th
                        class="table-header-cell table-header-cell--sortable table-header-cell--left group"
                        onclick={() => handleSort('service')}
                        role="button"
                        tabindex="0"
                        onkeydown={e => e.key === 'Enter' && handleSort('service')}
                      >
                        <div class="table-header-sort">
                          <span class="table-header-sort__label">Service</span>
                          {@render sortIndicator('service')}
                        </div>
                      </th>
                      <th
                        class="table-header-cell table-header-cell--sortable table-header-cell--right group"
                        onclick={() => handleSort('datapoints')}
                        role="button"
                        tabindex="0"
                        onkeydown={e => e.key === 'Enter' && handleSort('datapoints')}
                      >
                        <div class="table-header-sort table-header-sort--end">
                          <span class="table-header-sort__label">DPs</span>
                          {@render sortIndicator('datapoints')}
                        </div>
                      </th>
                    </tr>
                  </thead>
                  <tbody class="table-body-surface">
                    {#each paginatedMetrics as metric (metric.id)}
                      {@const selected = selectedMetricId === metric.id}
                      {@const mType = getMetricType(metric)}
                      {@const service = getServiceName(metric.resource) ?? ''}
                      <tr
                        class="metric-row cursor-pointer transition-colors {selected ? 'table-row--selected' : 'hover:bg-base-200'}"
                        data-metric-id={metric.id}
                        onclick={() => selectMetric(metric.id)}
                        role="button"
                        tabindex="0"
                      >
                        <td
                          class="metric-cell truncate"
                          title={metric.name}
                        >
                          {metric.name}
                        </td>
                        <td class="metric-cell">
                          <span class={metricTypeBadgeClass(mType)}>
                            {metricTypeLabel(mType)}
                          </span>
                        </td>
                        <td class="metric-cell truncate text-base-content/70" title={metric.unit}>
                          {metric.unit || '—'}
                        </td>
                        <td class="metric-cell truncate" title={service}>
                          {service || '—'}
                        </td>
                        <td class="metric-cell text-right tabular-nums">
                          {metric.datapoints.length}
                        </td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
                {#each barPositions as bar (bar.index)}
                  <div
                    class="col-resize-bar col-resize-bar--guide"
                    class:col-resize-bar--active={activeResizeCol === bar.index}
                    style:left="{bar.left}px"
                    role="separator"
                    aria-orientation="vertical"
                    aria-label="Resize {metricCols[bar.index].id} column"
                    onpointerdown={e => handleStartResize(bar.index, e)}
                  >
                    <div class="col-resize-bar__line"></div>
                  </div>
                {/each}
                </div>
              </div>

              <!-- Pagination -->
              {#if sortedMetrics.length > 0}
                <div class="pagination-controls">
                  <div class="pagination-rows-selector">
                    <span class="pagination-label">Rows per page:</span>
                    <button
                      class="pagination-rows-button"
                      popovertarget="metric-rows-per-page-popover"
                      style="anchor-name: --metric-rows-per-page-anchor"
                    >
                      <span>{rowsPerPage}</span>
                      <svg
                        class="w-3 h-3 popover-indicator {rowsPerPagePopoverOpen
                          ? 'popover-indicator--open'
                          : ''}"
                        viewBox="0 0 24 24"
                      >
                        <path d="M18 9s-4.419 6-6 6s-6-6-6-6" />
                      </svg>
                    </button>
                  </div>

                  <div class="pagination-controls__center">
                    <div
                      class="flex min-w-0 flex-nowrap items-center justify-center gap-1.5"
                    >
                      <button
                        type="button"
                        class="btn btn-ghost btn-sm btn-circle"
                        disabled={currentPage === 1}
                        onclick={() => goToPage(1)}
                        aria-label="First page"
                      >
                        <ArrowLeftDoubleIcon class="h-4 w-4" aria-hidden="true" />
                      </button>
                      <button
                        type="button"
                        class="btn btn-ghost btn-sm btn-circle"
                        disabled={currentPage === 1}
                        onclick={() => goToPage(currentPage - 1)}
                        aria-label="Previous page"
                      >
                        <ArrowLeftIcon class="h-4 w-4" aria-hidden="true" />
                      </button>
                      <div
                        class="flex min-h-8 min-w-[10rem] items-center justify-center rounded-lg bg-base-200/50 px-3 text-sm tabular-nums text-base-content/70"
                      >
                        {startRow}–{endRow} of {sortedMetrics.length} metrics
                      </div>
                      <button
                        type="button"
                        class="btn btn-ghost btn-sm btn-circle"
                        disabled={currentPage === totalPages}
                        onclick={() => goToPage(currentPage + 1)}
                        aria-label="Next page"
                      >
                        <ArrowRightIcon class="h-4 w-4" aria-hidden="true" />
                      </button>
                      <button
                        type="button"
                        class="btn btn-ghost btn-sm btn-circle"
                        disabled={currentPage === totalPages}
                        onclick={() => goToPage(totalPages)}
                        aria-label="Last page"
                      >
                        <ArrowRightDoubleIcon class="h-4 w-4" aria-hidden="true" />
                      </button>
                    </div>
                  </div>

                  {#if hasMetricRows}
                    <div class="pagination-controls__actions">
                      <button
                        type="button"
                        class="btn btn-ghost btn-sm text-error"
                        onclick={handleDeleteAllMetrics}
                        aria-label="Delete all metrics"
                      >
                        <TrashIcon class="h-3.5 w-3.5" aria-hidden="true" />
                        Delete all metrics
                      </button>
                    </div>
                  {/if}
                </div>
              {/if}
            </div>
          {/snippet}
          {#snippet rightPanel()}
            <div
              class="h-full overflow-hidden rounded-xl border border-base-300/70 bg-base-100/80 shadow-surface-sm backdrop-blur-sm"
            >
              <MetricDetailPanel metric={selectedMetric} onDelete={handleDeleteMetric} />
            </div>
          {/snippet}
        </ResizablePanels>
      </div>
    {/if}
  </div>

  <!-- Rows per page popover -->
  <div
    id="metric-rows-per-page-popover"
    class="popover dropdown-content metric-rows-per-page-popover"
    popover="auto"
  >
    {#each rowsPerPageOptions as option (option)}
      <button
        class="pagination-popover-option {option === rowsPerPage
          ? 'pagination-popover-option--selected'
          : ''}"
        onclick={() => {
          handleRowsPerPageChange(option)
          document.getElementById('metric-rows-per-page-popover')?.hidePopover()
        }}
      >
        {#if option === rowsPerPage}
          <svg class="w-4 h-4 text-primary" viewBox="0 0 24 24">
            <path d="m5 14l3.5 3.5L19 6.5" />
          </svg>
        {:else}
          <span class="w-4 h-4"></span>
        {/if}
        <span>{option}</span>
      </button>
    {/each}
  </div>
</div>

<style lang="postcss">
  @reference "../app.css";

  .metric-row {
    height: var(--table-row-h);
  }

  .metric-cell {
    @apply px-4 py-2 align-middle text-sm;
  }

  .metric-rows-per-page-popover {
    @apply fixed z-50 px-0 py-1 mx-0 mb-2;
    position-anchor: --metric-rows-per-page-anchor;
    bottom: anchor(--metric-rows-per-page-anchor top);
    top: auto;
    left: anchor(--metric-rows-per-page-anchor left);
    right: auto;
    position-try-fallbacks: flip-block;
    @apply bg-base-100 rounded-md shadow-lg;
    @apply border border-base-300 text-base-content;
    @apply min-w-16;
  }
</style>
