<script lang="ts">
  import { onMount } from 'svelte'
  import { router } from 'tinro5'
  import { telemetryAPI } from '@/services/telemetry-service'
  import {
    getTimeContext,
    selectionToQueryRangeMs,
  } from '@/contexts/time-context.svelte'
  import { formatTimestamp } from '@/utils/time'
  import { formatDuration, traceSummaryDurationNs } from '@/utils/duration'
  import {
    compareTraceSummaries,
    type TraceSummarySortColumn,
    type TraceSummarySortDirection,
  } from '@/utils/trace-summary-sort'
  import { setTraceListNavIds } from '@/stores/trace-list-nav.svelte'
  import {
    loadTraceListTableState,
    saveTraceListTableState,
  } from '@/utils/trace-list-table-state'
  import type { TraceSummary, SearchResultEvent } from '@/types/api-types'
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

  // --- state: selection ---
  let selectedTraceIDs = $state(new Set<string>())

  // --- state: column resize ---
  import type { FixedColumn, ResizableColumn, ElasticColumn } from '@/types/column-sizing'

  const cols = {
    checkbox:      { kind: 'fixed', width: 40 } satisfies FixedColumn,
    traceId:       { kind: 'resizable', min: 100, default: 300 } satisfies ResizableColumn,
    rootIndicator: { kind: 'fixed', width: 48 } satisfies FixedColumn,
    rootName:      { kind: 'resizable', min: 100, default: 200 } satisfies ResizableColumn,
    service:       { kind: 'resizable', min: 100, default: 200 } satisfies ResizableColumn,
    startTime:     { kind: 'elastic', min: 120 } satisfies ElasticColumn,
    duration:      { kind: 'fixed', width: 128 } satisfies FixedColumn,
    spans:         { kind: 'fixed', width: 88 } satisfies FixedColumn,
    errors:        { kind: 'fixed', width: 88 } satisfies FixedColumn,
    exceptions:    { kind: 'fixed', width: 104 } satisfies FixedColumn,
  }

  const COL_CHECKBOX = cols.checkbox.width
  const COL_ROOT_INDICATOR = cols.rootIndicator.width
  const MIN_COL_W = cols.traceId.min
  const MIN_ELASTIC_COL = cols.startTime.min
  const COL_TRAILING_FIXED = cols.duration.width + cols.spans.width + cols.errors.width + cols.exceptions.width
  const FIXED_TOTAL = COL_CHECKBOX + COL_ROOT_INDICATOR + COL_TRAILING_FIXED

  let traceIdColW = $state(cols.traceId.default)
  let rootNameColW = $state(cols.rootName.default)
  let serviceColW = $state(cols.service.default)
  let tableEl = $state<HTMLTableElement | null>(null)

  type ResizeCol = 'traceId' | 'rootName' | 'service'
  let activeResizeCol = $state<ResizeCol | null>(null)

  let tableMinWidth = $derived(
    COL_CHECKBOX + traceIdColW + COL_ROOT_INDICATOR + rootNameColW + serviceColW + MIN_ELASTIC_COL + COL_TRAILING_FIXED
  )

  let barLeftPx = $derived({
    traceId: COL_CHECKBOX + traceIdColW,
    rootName: COL_CHECKBOX + traceIdColW + COL_ROOT_INDICATOR + rootNameColW,
    service: COL_CHECKBOX + traceIdColW + COL_ROOT_INDICATOR + rootNameColW + serviceColW,
  })

  function startResizeCol(col: ResizeCol, e: PointerEvent) {
    e.preventDefault()
    const startX = e.clientX
    const startW = col === 'traceId' ? traceIdColW
                 : col === 'rootName' ? rootNameColW
                 : serviceColW
    const target = e.currentTarget as HTMLElement
    target.setPointerCapture(e.pointerId)
    activeResizeCol = col

    function onMove(ev: PointerEvent) {
      const containerW = tableEl?.closest('.overflow-x-auto')?.clientWidth ?? Infinity
      const others = col === 'traceId' ? rootNameColW + serviceColW
                   : col === 'rootName' ? traceIdColW + serviceColW
                   : traceIdColW + rootNameColW
      const maxW = containerW - FIXED_TOTAL - MIN_ELASTIC_COL - others
      const next = Math.min(maxW, Math.max(MIN_COL_W, startW + (ev.clientX - startX)))

      if (col === 'traceId') traceIdColW = next
      else if (col === 'rootName') rootNameColW = next
      else serviceColW = next
    }

    function end() {
      activeResizeCol = null
      target.removeEventListener('pointermove', onMove)
      target.removeEventListener('pointerup', end)
      target.removeEventListener('pointercancel', end)
    }

    target.addEventListener('pointermove', onMove)
    target.addEventListener('pointerup', end)
    target.addEventListener('pointercancel', end)
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
  let someSelected = $derived(selectedTraceIDs.size > 0)
  // --- derived: summary stats — full traceSummaries (not the current page) ---
  let listStats = $derived(traceListStats(traceSummaries))

  let traceDeleteLabel = $derived(
    someSelected
      ? `Delete ${selectedTraceIDs.size} trace${selectedTraceIDs.size !== 1 ? 's' : ''}`
      : 'Delete all traces'
  )

  let traceDeleteAriaLabel = $derived(
    someSelected
      ? `Delete ${selectedTraceIDs.size} selected trace${selectedTraceIDs.size !== 1 ? 's' : ''}`
      : 'Delete all traces in this time range'
  )

  // --- effects ---
  $effect(() => {
    saveTraceListTableState({ sortColumn, sortDirection, rowsPerPage })
  })

  $effect(() => {
    void sortedTraces
    selectedTraceIDs = new Set()
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
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to fetch traces'
      console.error('Error fetching trace summaries:', err)
    } finally {
      loading = false
    }
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

  async function handleDelete() {
    try {
      if (someSelected) {
        await telemetryAPI.deleteTraces([...selectedTraceIDs])
      } else {
        await telemetryAPI.clearTraces()
      }
      selectedTraceIDs = new Set()
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
      onRefresh={fetchTraces}
      {listStats}
      listStatsMuted={statsRowMuted}
      trailingFilters={[toolbarTimeRange]}
      {searchError}

    >
      <SearchEditor
        signal="traces"
        view="list"
        inToolbar
        onSearchResults={handleSearchResults}
        onSearchError={(err) => (searchError = err)}
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
        <div class="flex-1 min-h-0 overflow-x-auto overflow-y-auto">
          <div class="col-resize-context trace-list-col-resize">
            <table
              bind:this={tableEl}
              class="trace-list-table table table-fixed table-sm w-full border-collapse"
              style:min-width="{tableMinWidth}px"
            >
              <colgroup>
                <col style:width="{COL_CHECKBOX}px" />
                <col style:width="{traceIdColW}px" />
                <col style:width="{COL_ROOT_INDICATOR}px" />
                <col style:width="{rootNameColW}px" />
                <col style:width="{serviceColW}px" />
                <col /><!-- Start Time: elastic, absorbs remaining width -->
                <col style="width: 8rem" />
                <col style="width: 5.5rem" />
                <col style="width: 5.5rem" />
                <col style="width: 6.5rem" />
              </colgroup>
              <thead class="sticky top-0 z-10 table-header-surface">
              <tr class="table-header-row">
                <th class="table-header-cell table-header-cell--checkbox">
                  <input
                    type="checkbox"
                    class="checkbox checkbox-xs checkbox-primary"
                    checked={someSelected}
                    indeterminate={someSelected}
                    onchange={() => {
                      if (someSelected) {
                        selectedTraceIDs = new Set()
                      } else {
                        selectedTraceIDs = new Set(
                          paginatedTraces.map(t => t.traceID)
                        )
                      }
                    }}
                    aria-label="Select all on this page"
                  />
                </th>
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
                    <span class="table-header-sort__label">Root Span Name</span>
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
                  onkeydown={e => e.key === 'Enter' && handleSort('startTime')}
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
                <th
                  class="table-header-cell table-header-cell--sortable table-header-cell--right group"
                  onclick={() => handleSort('spanCount')}
                  role="button"
                  tabindex="0"
                  onkeydown={e => e.key === 'Enter' && handleSort('spanCount')}
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
                  onkeydown={e => e.key === 'Enter' && handleSort('errorCount')}
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
                  class="table-row cursor-pointer hover:bg-base-200 transition-colors {selectedTraceIDs.has(
                    trace.traceID
                  )
                    ? 'bg-primary/5'
                    : ''}"
                  onclick={() => navigateToTrace(trace.traceID)}
                  role="button"
                  tabindex="0"
                  onkeydown={e =>
                    e.key === 'Enter' && navigateToTrace(trace.traceID)}
                >
                  <td
                    class="table-cell--checkbox"
                    onclick={e => e.stopPropagation()}
                    onkeydown={e => e.stopPropagation()}
                  >
                    <input
                      type="checkbox"
                      class="checkbox checkbox-xs checkbox-primary"
                      checked={selectedTraceIDs.has(trace.traceID)}
                      onchange={() => {
                        const next = new Set(selectedTraceIDs)
                        if (next.has(trace.traceID)) {
                          next.delete(trace.traceID)
                        } else {
                          next.add(trace.traceID)
                        }
                        selectedTraceIDs = next
                      }}
                      aria-label="Select trace {trace.traceID}"
                    />
                  </td>
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
            <div
              class="col-resize-bar col-resize-bar--guide"
              class:col-resize-bar--active={activeResizeCol === 'traceId'}
              style:left="{barLeftPx.traceId}px"
              role="separator"
              aria-orientation="vertical"
              aria-label="Resize Trace ID column"
              onpointerdown={e => startResizeCol('traceId', e)}
            >
              <div class="col-resize-bar__line"></div>
            </div>
            <div
              class="col-resize-bar col-resize-bar--guide"
              class:col-resize-bar--active={activeResizeCol === 'rootName'}
              style:left="{barLeftPx.rootName}px"
              role="separator"
              aria-orientation="vertical"
              aria-label="Resize Root Span Name column"
              onpointerdown={e => startResizeCol('rootName', e)}
            >
              <div class="col-resize-bar__line"></div>
            </div>
            <div
              class="col-resize-bar col-resize-bar--guide"
              class:col-resize-bar--active={activeResizeCol === 'service'}
              style:left="{barLeftPx.service}px"
              role="separator"
              aria-orientation="vertical"
              aria-label="Resize Service Name column"
              onpointerdown={e => startResizeCol('service', e)}
            >
              <div class="col-resize-bar__line"></div>
            </div>
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
                <span
                  class="pagination-delete-label hidden sm:inline max-w-[12rem] truncate text-sm text-base-content/60"
                  title={traceDeleteLabel}
                >
                  {traceDeleteLabel}
                </span>
                <button
                  type="button"
                  class="btn btn-soft btn-error btn-sm btn-circle"
                  onclick={handleDelete}
                  aria-label={traceDeleteAriaLabel}
                  title={traceDeleteAriaLabel}
                >
                  <TrashIcon class="h-4 w-4" aria-hidden="true" />
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
