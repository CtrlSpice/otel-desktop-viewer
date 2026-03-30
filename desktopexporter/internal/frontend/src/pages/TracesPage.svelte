<script lang="ts">
  import { onMount } from "svelte"
  import { router } from "tinro5"
  import { telemetryAPI } from "@/services/telemetry-service"
  import { getTimeContext, selectionToQueryRangeMs } from "@/contexts/time-context.svelte"
  import { formatTimestamp } from "@/utils/time"
  import { compareByStringField, compareByTimestampField } from "@/utils/compare"
  import { popNavState, stashNavState } from "@/utils/nav-state"
  import type { TraceSummary, SearchResultEvent } from "@/types/api-types"
  import SignalHeader from "@/components/SignalHeader/SignalHeader.svelte"
  import { traceListStats } from "@/components/TraceList/trace-list-stats"

  // --- types (table) ---
  type SortColumn =
    | 'serviceName'
    | 'rootSpanName'
    | 'startTime'
    | 'spanCount'
    | 'errorCount'
    | 'exceptionCount'
  type SortDirection = 'asc' | 'desc'

  interface TracesPageNav {
    currentPage: number
    rowsPerPage: number
    sortColumn: SortColumn
    sortDirection: SortDirection
  }

  // --- row comparator for sort() ---
  /** Primary key by column + direction; tie-break on trace ID. */
  function compareTraceSummaries(
    a: TraceSummary,
    b: TraceSummary,
    col: SortColumn,
    dir: SortDirection
  ): number {
    const cmp =
      col === 'serviceName' ? compareByStringField(a, b, (t) => t.rootSpan?.serviceName) :
      col === 'rootSpanName' ? compareByStringField(a, b, (t) => t.rootSpan?.name) :
      col === 'startTime' ? compareByTimestampField(a, b, (t) => t.rootSpan?.startTime) :
      col === 'spanCount' ? a.spanCount - b.spanCount :
      col === 'errorCount' ? a.errorCount - b.errorCount :
      a.exceptionCount - b.exceptionCount

    return cmp !== 0
      ? dir === 'asc' ? cmp : -cmp
      : a.traceID.localeCompare(b.traceID)
  }

  // --- context ---
  let timeContext = getTimeContext()

  // --- state: API / list ---
  let traceSummaries = $state<TraceSummary[]>([])
  let loading = $state(true)
  let error = $state<string | null>(null)
  let mounted = $state(false)

  // --- state: sort ---
  let sortColumn = $state<SortColumn>('startTime')
  let sortDirection = $state<SortDirection>('desc')

  // --- state: pagination ---
  let currentPage = $state(1)
  let rowsPerPage = $state(25)
  let rowsPerPageOptions = [10, 25, 50, 100]
  let rowsPerPagePopoverOpen = $state(false)

  // --- state: selection ---
  let selectedTraceIDs = $state(new Set<string>())

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

  let totalPages = $derived(Math.ceil(sortedTraces.length / rowsPerPage))
  let startRow = $derived(sortedTraces.length === 0 ? 0 : (currentPage - 1) * rowsPerPage + 1)
  let endRow = $derived(Math.min(currentPage * rowsPerPage, sortedTraces.length))

  let hasTraceRows = $derived(traceSummaries.length > 0)
  let someSelected = $derived(selectedTraceIDs.size > 0)
  // --- derived: summary stats — full traceSummaries (not the current page) ---
  let listStats = $derived(traceListStats(traceSummaries))

  // --- effects ---
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
      error = err instanceof Error ? err.message : "Failed to fetch traces"
      console.error("Error fetching trace summaries:", err)
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
    stashNavState<TracesPageNav>('tracesPage', {
      currentPage,
      rowsPerPage,
      sortColumn,
      sortDirection,
    })
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
      error = err instanceof Error ? err.message : "Failed to delete traces"
      console.error("Error deleting traces:", err)
    }
  }

  // --- lifecycle ---
  onMount(async () => {
    const saved = popNavState<TracesPageNav>('tracesPage')
    if (saved) {
      currentPage = saved.currentPage
      rowsPerPage = saved.rowsPerPage
      sortColumn = saved.sortColumn
      sortDirection = saved.sortDirection
    }

    await fetchTraces()
    mounted = true
  })
</script>

<!-- TracesPage: list view — script order: imports → types → pure cmp → context → state → derived → effects → handlers → onMount -->
<div class="flex min-w-0 w-full flex-col overflow-y-auto py-6">
  <!-- 1. Header + search -->
  <SignalHeader 
    signal="traces"
    view="list"
    onRefresh={fetchTraces}
    onSearchResults={handleSearchResults}
  />
  
  {#if error}
    <div class="alert alert-error mb-4">
      <span>Error: {error}</span>
    </div>
  {/if}

  <div class="space-y-4">
    <!-- 2. Summary stats row -->
    <div
      class="flex min-w-0 flex-wrap items-center gap-x-2 gap-y-1 text-sm text-base-content/70 transition-opacity duration-300 ease-in-out {statsRowMuted ? 'opacity-[0.55]' : 'opacity-100'}"
      aria-busy={statsRowMuted}
    >
      {#if hasTraceRows}
        {@const s = listStats}
        <span class="font-medium text-base-content">
          {s.traces} trace{s.traces !== 1 ? 's' : ''}
        </span>
        <span class="text-base-content/35" aria-hidden="true">·</span>
        <span>{s.spans} span{s.spans !== 1 ? 's' : ''}</span>
        <span class="text-base-content/35" aria-hidden="true">·</span>
        <span>{s.services} service{s.services !== 1 ? 's' : ''}</span>
        <span class="text-base-content/35" aria-hidden="true">·</span>
        <span class={s.errors > 0 ? 'text-error/90' : ''}>
          {s.errors} error{s.errors !== 1 ? 's' : ''}
        </span>
        <span class="text-base-content/35" aria-hidden="true">·</span>
        <span class={s.exceptions > 0 ? 'text-warning/90' : ''}>
          {s.exceptions} exception{s.exceptions !== 1 ? 's' : ''}
        </span>
        <div class="ml-auto">
          <button
            type="button"
            class="btn btn-ghost btn-sm gap-1.5 text-base-content/50 hover:text-error"
            onclick={handleDelete}
          >
            <svg class="w-4 h-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-linecap="round" stroke-width="1.5">
              <path d="m19.5 5.5l-.62 10.025c-.158 2.561-.237 3.842-.88 4.763a4 4 0 0 1-1.2 1.128c-.957.584-2.24.584-4.806.584c-2.57 0-3.855 0-4.814-.585a4 4 0 0 1-1.2-1.13c-.642-.922-.72-2.205-.874-4.77L4.5 5.5M3 5.5h18m-4.944 0l-.683-1.408c-.453-.936-.68-1.403-1.071-1.695a2 2 0 0 0-.275-.172C13.594 2 13.074 2 12.035 2c-1.066 0-1.599 0-2.04.234a2 2 0 0 0-.278.18c-.395.303-.616.788-1.058 1.757L8.053 5.5m1.447 11v-6m5 6v-6" />
            </svg>
            {#if someSelected}
              Delete {selectedTraceIDs.size} trace{selectedTraceIDs.size !== 1 ? 's' : ''}
            {:else}
              Clear all
            {/if}
          </button>
        </div>
      {/if}
    </div>

    <!-- 3a. Loading (no rows yet) -->
    {#if loading && !hasTraceRows}
      <div
        class="rounded-xl border border-base-300/70 bg-base-100/80 px-4 py-12 text-center text-base-content/60 shadow-surface-sm backdrop-blur-sm"
      >
        Loading traces…
      </div>
    <!-- 3b. Empty state -->
    {:else if !loading && !hasTraceRows}
      <div
        class="rounded-xl border border-base-300/70 bg-base-100/80 px-4 py-12 text-center shadow-surface-sm backdrop-blur-sm"
      >
        <p class="text-base-content/60">No traces in this time range</p>
        <p class="mt-2 text-sm text-base-content/50">
          Send telemetry to the exporter or adjust the time range
        </p>
      </div>
    <!-- 3c. Table + pagination -->
    {:else}
    <div
      class="overflow-hidden rounded-xl border border-base-300/70 bg-base-100/80 shadow-surface-sm backdrop-blur-sm transition-opacity duration-200 {loading ? 'opacity-70' : 'opacity-100'}"
    >
      <div class="overflow-x-auto">
        <table class="w-full">
        <thead>
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
                        selectedTraceIDs = new Set(paginatedTraces.map(t => t.traceID))
                      }
                    }}
                    aria-label="Select all on this page"
                  />
                </th>
                <th class="table-header-cell--trace-id">
                  Trace ID
                </th>
                <th class="table-header-cell--after-trace-id">
                  Root Span
                </th>
                <th
                  class="table-header-cell table-header-cell--sortable table-header-cell--left group"
                  onclick={() => handleSort('rootSpanName')}
                  role="button"
                  tabindex="0"
                  onkeydown={(e) => e.key === 'Enter' && handleSort('rootSpanName')}
                >
                  <div class="flex items-center gap-2">
                    <span>Root Span Name</span>
                    <span class="w-4 h-4 flex items-center justify-center">
                      <svg
                        class="sort-indicator {sortColumn === 'rootSpanName'
                          ? 'sort-indicator--active'
                          : 'sort-indicator--inactive'} {sortColumn === 'rootSpanName' && sortDirection === 'asc'
                          ? 'sort-indicator--asc'
                          : ''}"
                        viewBox="0 0 24 24"
                      >
                        <path d="M12 18.502v-13.5m6 8s-4.419 6-6 6s-6-6-6-6" />
                      </svg>
                    </span>
                  </div>
                </th>
                <th
                  class="table-header-cell table-header-cell--sortable table-header-cell--left group"
                  onclick={() => handleSort('serviceName')}
                  role="button"
                  tabindex="0"
                  onkeydown={(e) => e.key === 'Enter' && handleSort('serviceName')}
                >
                  <div class="flex items-center gap-2">
                    <span>Service Name</span>
                    <span class="w-4 h-4 flex items-center justify-center">
                      <svg
                        class="sort-indicator {sortColumn === 'serviceName'
                          ? 'sort-indicator--active'
                          : 'sort-indicator--inactive'} {sortColumn === 'serviceName' && sortDirection === 'asc'
                          ? 'sort-indicator--asc'
                          : ''}"
                        viewBox="0 0 24 24"
                      >
                        <path d="M12 18.502v-13.5m6 8s-4.419 6-6 6s-6-6-6-6" />
                      </svg>
                    </span>
                  </div>
                </th>
                <th
                  class="table-header-cell table-header-cell--sortable table-header-cell--left group"
                  onclick={() => handleSort('startTime')}
                  role="button"
                  tabindex="0"
                  onkeydown={(e) => e.key === 'Enter' && handleSort('startTime')}
                >
                  <div class="flex items-center gap-2">
                    <span>Start Time</span>
                    <span class="w-4 h-4 flex items-center justify-center">
                      <svg
                        class="sort-indicator {sortColumn === 'startTime'
                          ? 'sort-indicator--active'
                          : 'sort-indicator--inactive'} {sortColumn === 'startTime' && sortDirection === 'asc'
                          ? 'sort-indicator--asc'
                          : ''}"
                        viewBox="0 0 24 24"
                      >
                        <path d="M12 18.502v-13.5m6 8s-4.419 6-6 6s-6-6-6-6" />
                      </svg>
                    </span>
                  </div>
                </th>
                <th
                  class="table-header-cell table-header-cell--sortable table-header-cell--center group"
                  onclick={() => handleSort('spanCount')}
                  role="button"
                  tabindex="0"
                  onkeydown={(e) => e.key === 'Enter' && handleSort('spanCount')}
                >
                  <div class="flex items-center justify-center gap-2">
                    <span>Spans</span>
                    <span class="w-4 h-4 flex items-center justify-center">
                      <svg
                        class="sort-indicator {sortColumn === 'spanCount'
                          ? 'sort-indicator--active'
                          : 'sort-indicator--inactive'} {sortColumn === 'spanCount' && sortDirection === 'asc'
                          ? 'sort-indicator--asc'
                          : ''}"
                        viewBox="0 0 24 24"
                      >
                        <path d="M12 18.502v-13.5m6 8s-4.419 6-6 6s-6-6-6-6" />
                      </svg>
                    </span>
                  </div>
                </th>
                <th
                  class="table-header-cell table-header-cell--sortable table-header-cell--center group"
                  onclick={() => handleSort('errorCount')}
                  role="button"
                  tabindex="0"
                  onkeydown={(e) => e.key === 'Enter' && handleSort('errorCount')}
                >
                  <div class="flex items-center justify-center gap-2">
                    <span>Errors</span>
                    <span class="w-4 h-4 flex items-center justify-center">
                      <svg
                        class="sort-indicator {sortColumn === 'errorCount'
                          ? 'sort-indicator--active'
                          : 'sort-indicator--inactive'} {sortColumn === 'errorCount' && sortDirection === 'asc'
                          ? 'sort-indicator--asc'
                          : ''}"
                        viewBox="0 0 24 24"
                      >
                        <path d="M12 18.502v-13.5m6 8s-4.419 6-6 6s-6-6-6-6" />
                      </svg>
                    </span>
                  </div>
                </th>
                <th
                  class="table-header-cell table-header-cell--sortable table-header-cell--center group"
                  onclick={() => handleSort('exceptionCount')}
                  role="button"
                  tabindex="0"
                  onkeydown={(e) => e.key === 'Enter' && handleSort('exceptionCount')}
                >
                  <div class="flex items-center justify-center gap-2">
                    <span>Exceptions</span>
                    <span class="w-4 h-4 flex items-center justify-center">
                      <svg
                        class="sort-indicator {sortColumn === 'exceptionCount'
                          ? 'sort-indicator--active'
                          : 'sort-indicator--inactive'} {sortColumn === 'exceptionCount' && sortDirection === 'asc'
                          ? 'sort-indicator--asc'
                          : ''}"
                        viewBox="0 0 24 24"
                      >
                        <path d="M12 18.502v-13.5m6 8s-4.419 6-6 6s-6-6-6-6" />
                      </svg>
                    </span>
                  </div>
                </th>
              </tr>
            </thead>
              <!-- Table Body -->
              <tbody class="divide-y divide-base-300">
                  {#each paginatedTraces as trace}
                  <tr 
                    class="table-row cursor-pointer hover:bg-base-200 transition-colors {selectedTraceIDs.has(trace.traceID) ? 'bg-primary/5' : ''}"
                    onclick={() => navigateToTrace(trace.traceID)}
                    role="button"
                    tabindex="0"
                    onkeydown={(e) => e.key === 'Enter' && navigateToTrace(trace.traceID)}
                  >
                    <td
                      class="table-cell--checkbox"
                      onclick={(e) => e.stopPropagation()}
                      onkeydown={(e) => e.stopPropagation()}
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
                    <td class="table-cell--trace-id">
                      {trace.traceID}
                    </td>
                    <td class="table-cell--has-root">
                      {#if trace.rootSpan}
                        <span class="inline-flex items-center justify-center w-6 h-6 rounded-full bg-success/20 text-success">
                          <svg class="w-4 h-4" viewBox="0 0 24 24">
                            <path d="m5 14l3.5 3.5L19 6.5" />
                          </svg>
                        </span>
                      {:else}
                        <span class="inline-flex items-center justify-center w-6 h-6 rounded-full bg-error/20 text-error">
                          <svg class="w-4 h-4" viewBox="0 0 24 24">
                            <path d="M18 6L6 18m12 0L6 6" />
                          </svg>
                        </span>
                      {/if}
                    </td>
                    <td class="table-cell">
                      {#if trace.rootSpan?.name}
                        {trace.rootSpan.name}
                      {:else}
                        <span class="text-base-content/50 italic">—</span>
                      {/if}
                    </td>
                    <td class="table-cell">
                      {#if trace.rootSpan?.serviceName}
                        {trace.rootSpan.serviceName}
                      {:else}
                        <span class="text-base-content/50 italic">—</span>
                      {/if}
                    </td>
                    <td class="table-cell text-base-content/80">
                      {#if trace.rootSpan}
                        {formatTimestamp(trace.rootSpan.startTime, timeContext.timezone, 'milliseconds')}
                      {:else}
                        <span class="text-base-content/50 italic">—</span>
                      {/if}
                    </td>
                    <td class="table-cell--count">
                      {trace.spanCount}
                    </td>
                    <td class="table-cell--count">
                      {#if trace.errorCount > 0}
                        <span
                          class="inline-flex items-center justify-center rounded-full bg-error/20 px-2 py-0.5 font-medium text-error"
                        >
                          {trace.errorCount}
                        </span>
                      {:else}
                        <span class="text-base-content/50">0</span>
                      {/if}
                    </td>
                    <td class="table-cell--count">
                      {#if trace.exceptionCount > 0}
                        <span
                          class="inline-flex items-center justify-center rounded-full bg-warning/20 px-2 py-0.5 font-medium text-warning"
                        >
                          {trace.exceptionCount}
                        </span>
                      {:else}
                        <span class="text-base-content/50">0</span>
                      {/if}
                    </td>
                  </tr>
                  {/each}
              </tbody>
              </table>
            </div>

            <!-- Pagination Controls -->
            {#if sortedTraces.length > 0}
              <div class="pagination-controls">
              <!-- Rows per page selector -->
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

              <!-- Current range and total -->
              <div class="pagination-range">
                {startRow}–{endRow} of {sortedTraces.length}
              </div>

              <!-- Navigation arrows -->
              <div class="pagination-nav">
                <button
                  class="pagination-nav-button"
                  disabled={currentPage === 1}
                  onclick={() => goToPage(currentPage - 1)}
                  aria-label="Previous page"
                >
                  <svg class="w-5 h-5" viewBox="0 0 24 24">
                    <path d="M15 19l-7-7 7-7" />
                  </svg>
                </button>
                <button
                  class="pagination-nav-button"
                  disabled={currentPage === totalPages}
                  onclick={() => goToPage(currentPage + 1)}
                  aria-label="Next page"
                >
                  <svg class="w-5 h-5" viewBox="0 0 24 24">
                    <path d="M9 5l7 7-7 7" />
                  </svg>
                </button>
              </div>
              </div>
            {/if}
          </div>
    {/if}
        </div>

        <!-- Rows per page popover -->
        <div id="rows-per-page-popover" class="popover rows-per-page-popover" popover="auto">
          {#each rowsPerPageOptions as option}
            <button
              class="pagination-popover-option {option === rowsPerPage ? 'pagination-popover-option--selected' : ''}"
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
  .rows-per-page-popover {
    /* Layout & Positioning */
    @apply dropdown-content;
    @apply px-0 py-1 mx-0 my-2;
    position-anchor: --rows-per-page-anchor;
    top: anchor(--rows-per-page-anchor bottom);
    left: anchor(--rows-per-page-anchor left);

    /* Visual Styling */
    @apply bg-base-100 rounded-md shadow-lg;
    @apply border border-base-300 text-base-content;
    @apply min-w-16;
  }
</style>