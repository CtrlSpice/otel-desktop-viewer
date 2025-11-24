<script lang="ts">
  import { onMount } from "svelte"
  import { telemetryAPI } from "@/services/telemetry-service"
  import { getTimeContext } from "@/contexts/time-context.svelte"
  import type { TraceSummary, SearchResultEvent } from "@/types/api-types"
  import PageHeader from "@/components/PageHeader/PageHeader.svelte"

  // Create time context for this page
  let timeContext = getTimeContext();

  let traceSummaries = $state<TraceSummary[]>([])
  let loading = $state(true)
  let error = $state<string | null>(null)

  // Sorting state
  type SortColumn = 'serviceName' | 'rootSpanName' | 'startTime' | 'spanCount' | 'errorCount' | 'exceptionCount'
  type SortDirection = 'asc' | 'desc'
  let sortColumn = $state<SortColumn>('startTime')
  let sortDirection = $state<SortDirection>('desc')

  // Pagination state
  let currentPage = $state(1)
  let rowsPerPage = $state(25)
  let rowsPerPageOptions = [10, 25, 50, 100]
  let rowsPerPagePopoverOpen = $state(false)

  // Track rows per page popover state
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

  // Sorted traces
  let sortedTraces = $derived([...traceSummaries].sort((a, b) => {
        let comparison = 0

        switch (sortColumn) {
          case 'serviceName':
            let aService = a.rootSpan?.serviceName || ''
            let bService = b.rootSpan?.serviceName || ''
            comparison = aService.localeCompare(bService)
            break
          case 'rootSpanName':
            let aName = a.rootSpan?.name || ''
            let bName = b.rootSpan?.name || ''
            comparison = aName.localeCompare(bName)
            break
          case 'startTime':
            let aTime = a.rootSpan?.startTime.nanoseconds || BigInt(0)
            let bTime = b.rootSpan?.startTime.nanoseconds || BigInt(0)
            if (aTime < bTime) comparison = -1
            else if (aTime > bTime) comparison = 1
            else comparison = 0
            break
          case 'spanCount':
            comparison = a.spanCount - b.spanCount
            break
          case 'errorCount':
            comparison = a.errorCount - b.errorCount
            break
          case 'exceptionCount':
            comparison = a.exceptionCount - b.exceptionCount
            break
        }

        return sortDirection === 'asc' ? comparison : -comparison
      })
  )

  // Paginated traces
  let paginatedTraces = $derived.by(() => {
    let start = (currentPage - 1) * rowsPerPage
    let end = start + rowsPerPage
    return sortedTraces.slice(start, end)
  })

  // Pagination calculations
  let totalPages = $derived(Math.ceil(sortedTraces.length / rowsPerPage))
  let startRow = $derived((currentPage - 1) * rowsPerPage + 1)
  let endRow = $derived(Math.min(currentPage * rowsPerPage, sortedTraces.length))

  function handleSort(column: SortColumn) {
    if (sortColumn === column) {
      // Toggle direction if clicking the same column
      sortDirection = sortDirection === 'asc' ? 'desc' : 'asc'
    } else {
      // New column, start with ascending
      sortColumn = column
      sortDirection = 'asc'
    }
    // Reset to first page when sorting changes
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

  function handleRefresh() {
    // TODO: Implement refresh logic
    console.log('Refresh clicked')
  }

  function handleSearchResults(event: SearchResultEvent) {
    // Type narrowing with discriminated union
    if (event.signal === 'traces' && event.view === 'list') {
      loading = false
      error = null
      traceSummaries = event.results
    }
  }

  onMount(async () => {
    try {
      loading = true
      // Use time context for initial load
      let startTime = timeContext.selection.start;
      let endTime = timeContext.selection.end;
      
      // For presets, calculate fresh time range based on current time
      if (timeContext.selection.type === 'preset') {
        const duration = timeContext.selection.end - timeContext.selection.start;
        endTime = Date.now();
        startTime = endTime - duration;
      }
      
      traceSummaries = await telemetryAPI.searchTraces(startTime, endTime)
    } catch (err) {
      error = err instanceof Error ? err.message : "Failed to fetch traces"
      console.error("Error fetching trace summaries:", err)
    } finally {
      loading = false
    }
  })
</script>

<!-- TracesPage.svelte - Traces list and visualization page -->
<div class="flex flex-col w-full overflow-y-auto p-4">
  <!-- Page Header -->
  <PageHeader 
    signal="traces"
    view="list"
    onRefresh={handleRefresh}
    onSearchResults={handleSearchResults}
  />
      {#if loading}
        <div class="flex justify-center items-center py-8">
          <span class="loading loading-spinner loading-lg"></span>
          <span class="ml-4">Loading traces...</span>
        </div>
      {:else if error}
        <div class="alert alert-error">
          <span>Error: {error}</span>
        </div>
      {:else if traceSummaries.length === 0}
        <div class="text-center py-8">
          <p class="text-base-content/60">No traces found</p>
          <p class="text-sm text-base-content/50 mt-2">
            Send some telemetry data to see traces here
          </p>
        </div>
      {:else}
        <div class="space-y-4">
          <p class="text-base-content/80">
            Found {traceSummaries.length} trace(s):
          </p>
          
          <!-- Material Design 2 Data Table -->
          <div class="rounded-lg border border-base-300 bg-base-100 overflow-hidden">
            <div class="overflow-x-auto">
              <table class="w-full">
              <!-- Table Header -->
              <thead>
                <tr class="border-b border-base-300 bg-base-200">
                  <th
                    class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-base-content/70 cursor-pointer select-none group"
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
                    class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-base-content/70 cursor-pointer select-none group"
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
                    class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-base-content/70 cursor-pointer select-none group"
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
                  <th class="px-4 py-3 text-left text-xs font-semibold uppercase tracking-wider text-base-content/70">
                    Trace ID
                  </th>
                  <th class="px-4 py-3 text-center text-xs font-semibold uppercase tracking-wider text-base-content/70">
                    Has Root Span
                  </th>
                  <th
                    class="pl-2 pr-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-base-content/70 cursor-pointer select-none group"
                    onclick={() => handleSort('spanCount')}
                    role="button"
                    tabindex="0"
                    onkeydown={(e) => e.key === 'Enter' && handleSort('spanCount')}
                  >
                    <div class="flex items-center justify-end gap-2">
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
                      <span>Spans</span>
                    </div>
                  </th>
                  <th
                    class="pl-2 pr-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-base-content/70 cursor-pointer select-none group"
                    onclick={() => handleSort('errorCount')}
                    role="button"
                    tabindex="0"
                    onkeydown={(e) => e.key === 'Enter' && handleSort('errorCount')}
                  >
                    <div class="flex items-center justify-end gap-2">
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
                      <span>Errors</span>
                    </div>
                  </th>
                  <th
                    class="pl-2 pr-4 py-3 text-right text-xs font-semibold uppercase tracking-wider text-base-content/70 cursor-pointer select-none group"
                    onclick={() => handleSort('exceptionCount')}
                    role="button"
                    tabindex="0"
                    onkeydown={(e) => e.key === 'Enter' && handleSort('exceptionCount')}
                  >
                    <div class="flex items-center justify-end gap-2">
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
                      <span>Exceptions</span>
                    </div>
                  </th>
                </tr>
              </thead>
              <!-- Table Body -->
              <tbody class="divide-y divide-base-300">
                {#each paginatedTraces as trace}
                  <tr class="hover:bg-base-200 transition-colors duration-150">
                    <td class="px-4 py-4 text-sm text-base-content">
                      {#if trace.rootSpan?.serviceName}
                        {trace.rootSpan.serviceName}
                      {:else}
                        <span class="text-base-content/50 italic">—</span>
                      {/if}
                    </td>
                    <td class="px-4 py-4 text-sm text-base-content">
                      {#if trace.rootSpan?.name}
                        {trace.rootSpan.name}
                      {:else}
                        <span class="text-base-content/50 italic">—</span>
                      {/if}
                    </td>
                    <td class="px-4 py-4 text-sm text-base-content/80">
                      {#if trace.rootSpan}
                        {trace.rootSpan.startTime.toLocal('milliseconds')}
                      {:else}
                        <span class="text-base-content/50 italic">—</span>
                      {/if}
                    </td>
                    <td class="px-4 py-4 text-sm font-mono text-base-content">
                      {trace.traceID}
                    </td>
                    <td class="px-4 py-4 text-center">
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
                    <td class="pl-2 pr-4 py-4 text-sm text-right text-base-content">
                      {trace.spanCount}
                    </td>
                    <td class="pl-2 pr-4 py-4 text-sm text-right">
                      {#if trace.errorCount > 0}
                        <span class="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium bg-error/20 text-error">
                          {trace.errorCount}
                        </span>
                      {:else}
                        <span class="text-base-content/50">0</span>
                      {/if}
                    </td>
                    <td class="pl-2 pr-4 py-4 text-sm text-right">
                      {#if trace.exceptionCount > 0}
                        <span class="inline-flex items-center px-2 py-1 rounded-full text-xs font-medium bg-warning/20 text-warning">
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
              <div class="flex items-center justify-between px-4 py-3 bg-base-100 border-t border-base-300">
              <!-- Rows per page selector -->
              <div class="flex items-center gap-3">
                <span class="text-sm text-base-content/70">Rows per page:</span>
                <button
                  class="btn btn-sm btn-ghost min-w-16 justify-between bg-base-100 border border-base-300 hover:bg-base-200"
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
              <div class="text-sm text-base-content/70">
                {startRow}–{endRow} of {sortedTraces.length}
              </div>

              <!-- Navigation arrows -->
              <div class="flex items-center gap-1">
                <button
                  class="btn btn-sm btn-ghost btn-square disabled:opacity-30 disabled:cursor-not-allowed hover:bg-base-200"
                  disabled={currentPage === 1}
                  onclick={() => goToPage(currentPage - 1)}
                  aria-label="Previous page"
                >
                  <svg class="w-5 h-5" viewBox="0 0 24 24">
                    <path d="M15 19l-7-7 7-7" />
                  </svg>
                </button>
                <button
                  class="btn btn-sm btn-ghost btn-square disabled:opacity-30 disabled:cursor-not-allowed hover:bg-base-200"
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
        </div>

        <!-- Rows per page popover -->
        <div id="rows-per-page-popover" class="popover rows-per-page-popover" popover="auto">
          {#each rowsPerPageOptions as option}
            <button
              class="w-full text-left px-3 py-2 text-sm hover:bg-base-200 flex items-center gap-2 {option === rowsPerPage ? 'bg-base-200' : ''}"
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
      {/if}
</div>

<style>
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