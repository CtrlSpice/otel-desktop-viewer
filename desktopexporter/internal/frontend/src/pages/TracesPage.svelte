<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import { router } from 'tinro5'
  import { telemetryAPI } from '@/services/telemetry-service'
  import {
    getTimeContext,
    selectionToQueryRangeMs,
  } from '@/contexts/time-context.svelte'
  import { formatTimestamp, formatDuration, traceSummaryDurationNs } from '@/utils/time'
  import {
    compareTraceSummaries,
    type TraceSummarySortColumn,
    type TraceSummarySortDirection,
    loadTraceListTableState,
    saveTraceListTableState,
  } from '@/utils/traces'
  import { setTraceListNavIds } from '@/stores/trace-list-nav.svelte'
  import type {
    TraceSummary,
    SearchResultEvent,
    TraceStats,
  } from '@/types/api-types'
  import type { SearchEditorAPI } from '@/components/SignalToolbar/search/search-editor-api'
  import SignalToolbar from '@/components/SignalToolbar/SignalToolbar.svelte'
  import SearchEditor from '@/components/SignalToolbar/search/SearchEditor.svelte'
  import DateTimeFilter from '@/components/SignalToolbar/datetime/DateTimeFilter.svelte'
  import { traceListStats } from '@/components/TraceList/trace-list-stats'
  import {
    ArrowDownIcon,
    ArrowLeftDoubleIcon,
    ArrowLeftIcon,
    ArrowRightDoubleIcon,
    ArrowRightIcon,
    TrashIcon,
  } from '@/icons'
  import { tableNav } from '@/utils/table-keyboard-nav'

  // --- types (table) ---
  type SortColumn = TraceSummarySortColumn
  type SortDirection = TraceSummarySortDirection

  // --- context ---
  let timeContext = getTimeContext()

  // --- state: API / list ---
  let traceSummaries = $state<TraceSummary[]>([])
  let loading = $state(true)
  let error = $state<string | null>(null)
  let mounted = $state(false)
  let searchError = $state<string | null>(null)

  // --- state: sort + pagination (persisted via localStorage) ---
  const savedTableState = loadTraceListTableState()
  let sortColumn = $state<SortColumn>(savedTableState.sortColumn)
  let sortDirection = $state<SortDirection>(savedTableState.sortDirection)
  let currentPage = $state(1)
  let rowsPerPage = $state(savedTableState.rowsPerPage)
  let rowsPerPageOptions = [10, 25, 50, 100]
  let rowsPerPagePopoverOpen = $state(false)

  // --- state: polling / refresh indicator ---
  let searchEditorApi = $state<SearchEditorAPI | null>(null)
  let baselineStats = $state<TraceStats | null>(null)
  let polledStats = $state<TraceStats | null>(null)
  const POLL_INTERVAL_MS = 3000

  // --- state: column resize ---
  import {
    fixed, flex,
    computeInitialWidths,
    redistributeWidths,
    computeBarPositions,
    startColumnResize,
  } from '@/utils/column-resize'

  const traceCols = [
    flex('traceId', 100, 2),
    fixed('rootIndicator', 48),
    flex('rootName', 100, 2),
    flex('service', 100, 2),
    flex('startTime', 120, 3),
    flex('duration', 80, 1),
    fixed('gap', 24),
    fixed('spans', 72),
    fixed('errors', 72),
    fixed('exceptions', 72),
  ]

  let activeResizeCol = $state<number | null>(null)
  let tableContainerEl = $state<HTMLDivElement | null>(null)
  let colWidths = $state(traceCols.map(d => d.min))

  let barPositions = $derived(computeBarPositions(traceCols, colWidths))

  $effect(() => {
    if (!tableContainerEl) return
    untrack(() => {
      colWidths = computeInitialWidths(traceCols, tableContainerEl!.clientWidth)
    })
    const ro = new ResizeObserver(entries => {
      const w = entries[0]?.contentRect.width
      if (w && activeResizeCol === null) {
        colWidths = redistributeWidths(traceCols, colWidths, w)
      }
    })
    ro.observe(tableContainerEl)
    return () => ro.disconnect()
  })

  function handleStartResize(colIndex: number, e: PointerEvent) {
    activeResizeCol = colIndex
    startColumnResize(
      traceCols,
      () => colWidths, colIndex, e,
      next => { colWidths = next },
      () => { activeResizeCol = null }
    )
  }

  /** Dim stats while refetching; stays on briefly after `loading` ends so opacity can animate. */
  let statsRowMuted = $state(false)

  // --- derived: table rows — traceSummaries → sortedTraces → paginatedTraces ---
  let sortedTraces = $derived.by(() => {
    const col = sortColumn
    const dir = sortDirection
    const rows = [...traceSummaries]
    rows.sort((a, b) => compareTraceSummaries(a, b, col, dir))
    return rows
  })

  let paginatedTraces = $derived.by(() => {
    const start = (currentPage - 1) * rowsPerPage
    const end = start + rowsPerPage
    return sortedTraces.slice(start, end)
  })

  $effect(() => {
    setTraceListNavIds(sortedTraces.map(t => t.traceID))
  })

  let totalPages = $derived(Math.ceil(sortedTraces.length / rowsPerPage))
  let startRow = $derived(
    sortedTraces.length === 0 ? 0 : (currentPage - 1) * rowsPerPage + 1
  )
  let endRow = $derived(
    Math.min(currentPage * rowsPerPage, sortedTraces.length)
  )

  let hasTraceRows = $derived(traceSummaries.length > 0)
  // --- derived: summary stats — full traceSummaries (not the current page) ---
  let listStats = $derived(traceListStats(traceSummaries))

  let refreshIndicatorText = $derived.by(() => {
    if (!baselineStats || !polledStats) return ''
    const parts: string[] = []
    const traceDelta = polledStats.traceCount - baselineStats.traceCount
    if (traceDelta > 0)
      parts.push(`+${traceDelta} trace${traceDelta !== 1 ? 's' : ''}`)
    const spanDelta = polledStats.spanCount - baselineStats.spanCount
    if (spanDelta > 0)
      parts.push(`+${spanDelta} span${spanDelta !== 1 ? 's' : ''}`)
    return parts.join(', ')
  })

  // --- effects ---
  $effect(() => {
    saveTraceListTableState({ sortColumn, sortDirection, rowsPerPage })
  })

  $effect(() => {
    const busy = loading && hasTraceRows
    if (busy) {
      statsRowMuted = true
      return
    }
    if (!statsRowMuted) {
      return
    }
    const id = setTimeout(() => {
      statsRowMuted = false
    }, 220)
    return () => clearTimeout(id)
  })

  $effect(() => {
    const popover = document.getElementById('rows-per-page-popover')
    if (popover) {
      const handleToggle = () => {
        rowsPerPagePopoverOpen = popover.matches(':popover-open')
      }
      popover.addEventListener('toggle', handleToggle)
      return () => popover.removeEventListener('toggle', handleToggle)
    }
  })

  /** Clamp page when row count shrinks (refresh, time range, etc.). */
  $effect(() => {
    const n = sortedTraces.length
    const pages = Math.max(1, Math.ceil(n / rowsPerPage))
    if (n > 0 && currentPage > pages) {
      currentPage = pages
    }
  })

  $effect(() => {
    void timeContext.selection
    if (mounted) {
      fetchTraces()
    }
  })

  $effect(() => {
    if (!mounted) return
    const id = setInterval(async () => {
      try {
        const s = await telemetryAPI.getStats()
        polledStats = s.traces
      } catch {
        /* polling failures are silent */
      }
    }, POLL_INTERVAL_MS)
    return () => clearInterval(id)
  })

  // --- handlers & loaders ---
  function traceDurationCellLabel(trace: TraceSummary): string {
    const ns = traceSummaryDurationNs(trace)
    return ns === undefined ? '' : formatDuration(ns)
  }

  function handleSort(column: SortColumn) {
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

  async function fetchTraces() {
    try {
      loading = true
      error = null

      const { start: startTime, end: endTime } = selectionToQueryRangeMs(
        timeContext.selection,
        Date.now()
      )

      traceSummaries = await telemetryAPI.searchTraces(startTime, endTime)
      const s = await telemetryAPI.getStats()
      baselineStats = s.traces
      polledStats = s.traces
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to fetch traces'
      console.error('Error fetching trace summaries:', err)
    } finally {
      loading = false
    }
  }

  function handleRefresh() {
    searchEditorApi?.clear()
    fetchTraces()
  }

  function handleSearchResults(event: SearchResultEvent) {
    if (event.signal === 'traces' && event.view === 'list') {
      loading = false
      error = null
      traceSummaries = event.results
    }
  }

  function navigateToTrace(traceID: string) {
    router.goto(`/trace/${traceID}`)
  }

  async function handleDeleteAllTraces() {
    try {
      await telemetryAPI.clearTraces()
      currentPage = 1
      setTraceListNavIds([])
      await fetchTraces()
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to delete traces'
      console.error('Error deleting traces:', err)
    }
  }

  // --- lifecycle ---
  onMount(async () => {
    await fetchTraces()
    mounted = true
  })
</script>

{#snippet toolbarTimeRange()}
  <DateTimeFilter />
{/snippet}

<!-- TracesPage: list view — script order: imports → types → pure cmp → context → state → derived → effects → handlers → onMount -->
<div
  class="flex min-h-0 min-w-0 w-full flex-1 flex-col gap-[var(--layout-gap)] pt-0"
>
  <!-- 1. Header + search -->
  <div class="page-toolbar-block">
    <SignalToolbar
      signal="traces"
      view="list"
      onRefresh={handleRefresh}
      {listStats}
      listStatsMuted={statsRowMuted}
      trailingFilters={[toolbarTimeRange]}
      {searchError}
      {refreshIndicatorText}
    >
      <SearchEditor
        signal="traces"
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
    <!-- 2a. Loading (no rows yet) -->
    {#if loading && !hasTraceRows}
      <div
        class="rounded-xl border border-base-300/70 bg-base-100/80 px-4 py-12 text-center text-base-content/60 shadow-surface-sm backdrop-blur-sm"
      >
        Loading traces…
      </div>
      <!-- 2b. Empty state -->
    {:else if !loading && !hasTraceRows}
      <div
        class="rounded-xl border border-base-300/70 bg-base-100/80 px-4 py-12 text-center shadow-surface-sm backdrop-blur-sm"
      >
        <p class="text-base-content/60">No traces in this time range</p>
        <p class="mt-2 text-sm text-base-content/50">
          Send telemetry to the exporter or adjust the time range
        </p>
      </div>
      <!-- 2c. Table + pagination -->
    {:else}
      <div
        class="flex min-h-0 flex-1 flex-col overflow-hidden rounded-xl border border-base-300/70 bg-base-100/80 shadow-surface-sm backdrop-blur-sm transition-opacity duration-200 {loading
          ? 'opacity-70'
          : 'opacity-100'}"
      >
        <div class="flex-1 min-h-0 overflow-x-auto overflow-y-auto" bind:this={tableContainerEl}>
          <div class="col-resize-context trace-list-col-resize">
            <table
              class="trace-list-table table table-fixed table-sm w-full border-collapse"
              use:tableNav={{
                rowIdAttr: 'trace-id',
                onSelect: id => navigateToTrace(id),
              }}
            >
              <colgroup>
                {#each colWidths as w}
                  <col style:width="{w}px" />
                {/each}
              </colgroup>
              <thead class="sticky top-0 z-10 table-header-surface">
                <tr class="table-header-row">
                  <th class="table-header-cell table-header-cell--left">
                    Trace ID
                  </th>
                  <th
                    class="table-header-cell table-header-cell--trace-root"
                    title="Has Root Span"
                  >
                    Root
                  </th>
                  <th
                    class="table-header-cell table-header-cell--sortable table-header-cell--left group"
                    onclick={() => handleSort('rootSpanName')}
                    role="button"
                    tabindex="0"
                    onkeydown={e =>
                      e.key === 'Enter' && handleSort('rootSpanName')}
                  >
                    <div class="table-header-sort">
                      <span class="table-header-sort__label"
                        >Root Span Name</span
                      >
                      <span class="table-header-sort__indicator">
                        <ArrowDownIcon
                          class="sort-indicator {sortColumn === 'rootSpanName'
                            ? 'sort-indicator--active'
                            : 'sort-indicator--inactive'} {sortColumn ===
                            'rootSpanName' && sortDirection === 'asc'
                            ? 'sort-indicator--asc'
                            : ''}"
                          aria-hidden="true"
                        />
                      </span>
                    </div>
                  </th>
                  <th
                    class="table-header-cell table-header-cell--sortable table-header-cell--left group"
                    onclick={() => handleSort('serviceName')}
                    role="button"
                    tabindex="0"
                    onkeydown={e =>
                      e.key === 'Enter' && handleSort('serviceName')}
                  >
                    <div class="table-header-sort">
                      <span class="table-header-sort__label">Service Name</span>
                      <span class="table-header-sort__indicator">
                        <ArrowDownIcon
                          class="sort-indicator {sortColumn === 'serviceName'
                            ? 'sort-indicator--active'
                            : 'sort-indicator--inactive'} {sortColumn ===
                            'serviceName' && sortDirection === 'asc'
                            ? 'sort-indicator--asc'
                            : ''}"
                          aria-hidden="true"
                        />
                      </span>
                    </div>
                  </th>
                  <th
                    class="table-header-cell table-header-cell--sortable table-header-cell--left group"
                    onclick={() => handleSort('startTime')}
                    role="button"
                    tabindex="0"
                    onkeydown={e =>
                      e.key === 'Enter' && handleSort('startTime')}
                  >
                    <div class="table-header-sort">
                      <span class="table-header-sort__label">Start Time</span>
                      <span class="table-header-sort__indicator">
                        <ArrowDownIcon
                          class="sort-indicator {sortColumn === 'startTime'
                            ? 'sort-indicator--active'
                            : 'sort-indicator--inactive'} {sortColumn ===
                            'startTime' && sortDirection === 'asc'
                            ? 'sort-indicator--asc'
                            : ''}"
                          aria-hidden="true"
                        />
                      </span>
                    </div>
                  </th>
                  <th
                    class="table-header-cell table-header-cell--sortable table-header-cell--right group"
                    onclick={() => handleSort('duration')}
                    role="button"
                    tabindex="0"
                    onkeydown={e => e.key === 'Enter' && handleSort('duration')}
                  >
                    <div class="table-header-sort table-header-sort--end">
                      <span class="table-header-sort__indicator">
                        <ArrowDownIcon
                          class="sort-indicator {sortColumn === 'duration'
                            ? 'sort-indicator--active'
                            : 'sort-indicator--inactive'} {sortColumn ===
                            'duration' && sortDirection === 'asc'
                            ? 'sort-indicator--asc'
                            : ''}"
                          aria-hidden="true"
                        />
                      </span>
                      <span>Duration</span>
                    </div>
                  </th>
                  <th></th>
                  <th
                    class="table-header-cell table-header-cell--sortable table-header-cell--right group"
                    onclick={() => handleSort('spanCount')}
                    role="button"
                    tabindex="0"
                    onkeydown={e =>
                      e.key === 'Enter' && handleSort('spanCount')}
                  >
                    <div class="table-header-sort table-header-sort--end">
                      <span class="table-header-sort__indicator">
                        <ArrowDownIcon
                          class="sort-indicator {sortColumn === 'spanCount'
                            ? 'sort-indicator--active'
                            : 'sort-indicator--inactive'} {sortColumn ===
                            'spanCount' && sortDirection === 'asc'
                            ? 'sort-indicator--asc'
                            : ''}"
                          aria-hidden="true"
                        />
                      </span>
                      <span>Spans</span>
                    </div>
                  </th>
                  <th
                    class="table-header-cell table-header-cell--sortable table-header-cell--right group"
                    onclick={() => handleSort('errorCount')}
                    role="button"
                    tabindex="0"
                    onkeydown={e =>
                      e.key === 'Enter' && handleSort('errorCount')}
                  >
                    <div class="table-header-sort table-header-sort--end">
                      <span class="table-header-sort__indicator">
                        <ArrowDownIcon
                          class="sort-indicator {sortColumn === 'errorCount'
                            ? 'sort-indicator--active'
                            : 'sort-indicator--inactive'} {sortColumn ===
                            'errorCount' && sortDirection === 'asc'
                            ? 'sort-indicator--asc'
                            : ''}"
                          aria-hidden="true"
                        />
                      </span>
                      <span>Errors</span>
                    </div>
                  </th>
                  <th
                    class="table-header-cell table-header-cell--sortable table-header-cell--right group"
                    onclick={() => handleSort('exceptionCount')}
                    role="button"
                    tabindex="0"
                    onkeydown={e =>
                      e.key === 'Enter' && handleSort('exceptionCount')}
                  >
                    <div class="table-header-sort table-header-sort--end">
                      <span class="table-header-sort__indicator">
                        <ArrowDownIcon
                          class="sort-indicator {sortColumn === 'exceptionCount'
                            ? 'sort-indicator--active'
                            : 'sort-indicator--inactive'} {sortColumn ===
                            'exceptionCount' && sortDirection === 'asc'
                            ? 'sort-indicator--asc'
                            : ''}"
                          aria-hidden="true"
                        />
                      </span>
                      <span>Exceptions</span>
                    </div>
                  </th>
                </tr>
              </thead>
              <tbody class="table-body-surface">
                {#each paginatedTraces as trace}
                  <tr
                    class="table-row cursor-pointer hover:bg-base-200 transition-colors"
                    data-trace-id={trace.traceID}
                    onclick={() => navigateToTrace(trace.traceID)}
                    role="button"
                    tabindex="0"
                  >
                    <td class="table-cell--trace-id" title={trace.traceID}>
                      <span class="trace-list-cell-text">{trace.traceID}</span>
                    </td>
                    <td class="table-cell--has-root">
                      {#if trace.rootSpan}
                        <span
                          class="inline-flex items-center justify-center w-6 h-6 rounded-full bg-success/20 text-success"
                        >
                          <svg class="w-4 h-4" viewBox="0 0 24 24">
                            <path d="m5 14l3.5 3.5L19 6.5" />
                          </svg>
                        </span>
                      {:else}
                        <span
                          class="inline-flex items-center justify-center w-6 h-6 rounded-full bg-error/20 text-error"
                        >
                          <svg class="w-4 h-4" viewBox="0 0 24 24">
                            <path d="M18 6L6 18m12 0L6 6" />
                          </svg>
                        </span>
                      {/if}
                    </td>
                    <td
                      class="table-cell trace-list-cell-truncate"
                      title={trace.rootSpan?.name?.trim()
                        ? trace.rootSpan.name
                        : undefined}
                    >
                      <span class="trace-list-cell-text">
                        {#if trace.rootSpan?.name}
                          {trace.rootSpan.name}
                        {:else}
                          <span class="text-base-content/50 italic">—</span>
                        {/if}
                      </span>
                    </td>
                    <td
                      class="table-cell trace-list-cell-truncate"
                      title={trace.rootSpan?.serviceName?.trim()
                        ? trace.rootSpan.serviceName
                        : undefined}
                    >
                      <span class="trace-list-cell-text">
                        {#if trace.rootSpan?.serviceName}
                          {trace.rootSpan.serviceName}
                        {:else}
                          <span class="text-base-content/50 italic">—</span>
                        {/if}
                      </span>
                    </td>
                    <td
                      class="table-cell text-base-content/80 trace-list-cell-truncate"
                      title={trace.rootSpan
                        ? formatTimestamp(
                            trace.rootSpan.startTime,
                            timeContext.timezone,
                            'milliseconds'
                          )
                        : undefined}
                    >
                      <span class="trace-list-cell-text">
                        {#if trace.rootSpan}
                          {formatTimestamp(
                            trace.rootSpan.startTime,
                            timeContext.timezone,
                            'milliseconds'
                          )}
                        {:else}
                          <span class="text-base-content/50 italic">—</span>
                        {/if}
                      </span>
                    </td>
                    <td
                      class="table-cell text-right tabular-nums text-base-content/80 trace-list-cell-truncate"
                      title={traceDurationCellLabel(trace) || undefined}
                    >
                      <span class="trace-list-cell-text"
                        >{traceDurationCellLabel(trace)}</span
                      >
                    </td>
                    <td></td>
                    <td class="table-cell--count">
                      {trace.spanCount}
                    </td>
                    <td
                      class="table-cell--count {trace.errorCount > 0
                        ? 'text-error'
                        : 'text-base-content/50'}"
                    >
                      {trace.errorCount}
                    </td>
                    <td
                      class="table-cell--count {trace.exceptionCount > 0
                        ? 'text-warning'
                        : 'text-base-content/50'}"
                    >
                      {trace.exceptionCount}
                    </td>
                  </tr>
                {/each}
              </tbody>
            </table>
            {#each barPositions as bar}
              <div
                class="col-resize-bar col-resize-bar--guide"
                class:col-resize-bar--active={activeResizeCol === bar.index}
                style:left="{bar.left}px"
                role="separator"
                aria-orientation="vertical"
                aria-label="Resize {traceCols[bar.index].id} column"
                onpointerdown={e => handleStartResize(bar.index, e)}
              >
                <div class="col-resize-bar__line"></div>
              </div>
            {/each}
          </div>
        </div>

        <!-- Pagination Controls -->
        {#if sortedTraces.length > 0}
          <div class="pagination-controls">
            <div class="pagination-rows-selector">
              <span class="pagination-label">Rows per page:</span>
              <button
                class="pagination-rows-button"
                popovertarget="rows-per-page-popover"
                style="anchor-name: --rows-per-page-anchor"
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
                  {startRow}–{endRow} of {sortedTraces.length} traces
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

            {#if hasTraceRows}
              <div class="pagination-controls__actions">
                <button
                  type="button"
                  class="btn btn-ghost btn-sm text-error"
                  onclick={handleDeleteAllTraces}
                  aria-label="Delete all traces"
                >
                  <TrashIcon class="h-3.5 w-3.5" aria-hidden="true" />
                  Delete all traces
                </button>
              </div>
            {/if}
          </div>
        {/if}
      </div>
    {/if}
  </div>

  <!-- Rows per page popover -->
  <div
    id="rows-per-page-popover"
    class="popover dropdown-content rows-per-page-popover"
    popover="auto"
  >
    {#each rowsPerPageOptions as option}
      <button
        class="pagination-popover-option {option === rowsPerPage
          ? 'pagination-popover-option--selected'
          : ''}"
        onclick={() => {
          handleRowsPerPageChange(option)
          document.getElementById('rows-per-page-popover')?.hidePopover()
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

  .rows-per-page-popover {
    /* Layout & Positioning — open upward (pagination sits at bottom; below overflows viewport). */
    @apply fixed z-50 px-0 py-1 mx-0 mb-2;
    position-anchor: --rows-per-page-anchor;
    bottom: anchor(--rows-per-page-anchor top);
    top: auto;
    left: anchor(--rows-per-page-anchor left);
    right: auto;
    position-try-fallbacks: flip-block;

    /* Visual Styling */
    @apply bg-base-100 rounded-md shadow-lg;
    @apply border border-base-300 text-base-content;
    @apply min-w-16;
  }
</style>
