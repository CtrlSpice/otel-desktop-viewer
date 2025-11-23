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

  function handleSort(column: SortColumn) {
    if (sortColumn === column) {
      // Toggle direction if clicking the same column
      sortDirection = sortDirection === 'asc' ? 'desc' : 'asc'
    } else {
      // New column, start with ascending
      sortColumn = column
      sortDirection = 'asc'
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
          <div class="overflow-x-auto rounded-lg border border-base-300 bg-base-100">
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
                        {#if sortColumn === 'serviceName'}
                          {#if sortDirection === 'asc'}
                            <svg class="w-4 h-4 text-primary" fill="currentColor" viewBox="0 0 20 20">
                              <path fill-rule="evenodd" d="M14.707 12.707a1 1 0 01-1.414 0L10 9.414l-3.293 3.293a1 1 0 01-1.414-1.414l4-4a1 1 0 011.414 0l4 4a1 1 0 010 1.414z" clip-rule="evenodd" />
                            </svg>
                          {:else}
                            <svg class="w-4 h-4 text-primary" fill="currentColor" viewBox="0 0 20 20">
                              <path fill-rule="evenodd" d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clip-rule="evenodd" />
                            </svg>
                          {/if}
                        {:else}
                          <svg class="w-4 h-4 text-base-content/40 opacity-0 group-hover:opacity-100 transition-opacity" fill="currentColor" viewBox="0 0 20 20">
                            <path fill-rule="evenodd" d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clip-rule="evenodd" />
                          </svg>
                        {/if}
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
                        {#if sortColumn === 'rootSpanName'}
                          {#if sortDirection === 'asc'}
                            <svg class="w-4 h-4 text-primary" fill="currentColor" viewBox="0 0 20 20">
                              <path fill-rule="evenodd" d="M14.707 12.707a1 1 0 01-1.414 0L10 9.414l-3.293 3.293a1 1 0 01-1.414-1.414l4-4a1 1 0 011.414 0l4 4a1 1 0 010 1.414z" clip-rule="evenodd" />
                            </svg>
                          {:else}
                            <svg class="w-4 h-4 text-primary" fill="currentColor" viewBox="0 0 20 20">
                              <path fill-rule="evenodd" d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clip-rule="evenodd" />
                            </svg>
                          {/if}
                        {:else}
                          <svg class="w-4 h-4 text-base-content/40 opacity-0 group-hover:opacity-100 transition-opacity" fill="currentColor" viewBox="0 0 20 20">
                            <path fill-rule="evenodd" d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clip-rule="evenodd" />
                          </svg>
                        {/if}
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
                        {#if sortColumn === 'startTime'}
                          {#if sortDirection === 'asc'}
                            <svg class="w-4 h-4 text-primary" fill="currentColor" viewBox="0 0 20 20">
                              <path fill-rule="evenodd" d="M14.707 12.707a1 1 0 01-1.414 0L10 9.414l-3.293 3.293a1 1 0 01-1.414-1.414l4-4a1 1 0 011.414 0l4 4a1 1 0 010 1.414z" clip-rule="evenodd" />
                            </svg>
                          {:else}
                            <svg class="w-4 h-4 text-primary" fill="currentColor" viewBox="0 0 20 20">
                              <path fill-rule="evenodd" d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clip-rule="evenodd" />
                            </svg>
                          {/if}
                        {:else}
                          <svg class="w-4 h-4 text-base-content/40 opacity-0 group-hover:opacity-100 transition-opacity" fill="currentColor" viewBox="0 0 20 20">
                            <path fill-rule="evenodd" d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clip-rule="evenodd" />
                          </svg>
                        {/if}
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
                        {#if sortColumn === 'spanCount'}
                          {#if sortDirection === 'asc'}
                            <svg class="w-4 h-4 text-primary" fill="currentColor" viewBox="0 0 20 20">
                              <path fill-rule="evenodd" d="M14.707 12.707a1 1 0 01-1.414 0L10 9.414l-3.293 3.293a1 1 0 01-1.414-1.414l4-4a1 1 0 011.414 0l4 4a1 1 0 010 1.414z" clip-rule="evenodd" />
                            </svg>
                          {:else}
                            <svg class="w-4 h-4 text-primary" fill="currentColor" viewBox="0 0 20 20">
                              <path fill-rule="evenodd" d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clip-rule="evenodd" />
                            </svg>
                          {/if}
                        {:else}
                          <svg class="w-4 h-4 text-base-content/40 opacity-0 group-hover:opacity-100 transition-opacity" fill="currentColor" viewBox="0 0 20 20">
                            <path fill-rule="evenodd" d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clip-rule="evenodd" />
                          </svg>
                        {/if}
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
                        {#if sortColumn === 'errorCount'}
                          {#if sortDirection === 'asc'}
                            <svg class="w-4 h-4 text-primary" fill="currentColor" viewBox="0 0 20 20">
                              <path fill-rule="evenodd" d="M14.707 12.707a1 1 0 01-1.414 0L10 9.414l-3.293 3.293a1 1 0 01-1.414-1.414l4-4a1 1 0 011.414 0l4 4a1 1 0 010 1.414z" clip-rule="evenodd" />
                            </svg>
                          {:else}
                            <svg class="w-4 h-4 text-primary" fill="currentColor" viewBox="0 0 20 20">
                              <path fill-rule="evenodd" d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clip-rule="evenodd" />
                            </svg>
                          {/if}
                        {:else}
                          <svg class="w-4 h-4 text-base-content/40 opacity-0 group-hover:opacity-100 transition-opacity" fill="currentColor" viewBox="0 0 20 20">
                            <path fill-rule="evenodd" d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clip-rule="evenodd" />
                          </svg>
                        {/if}
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
                        {#if sortColumn === 'exceptionCount'}
                          {#if sortDirection === 'asc'}
                            <svg class="w-4 h-4 text-primary" fill="currentColor" viewBox="0 0 20 20">
                              <path fill-rule="evenodd" d="M14.707 12.707a1 1 0 01-1.414 0L10 9.414l-3.293 3.293a1 1 0 01-1.414-1.414l4-4a1 1 0 011.414 0l4 4a1 1 0 010 1.414z" clip-rule="evenodd" />
                            </svg>
                          {:else}
                            <svg class="w-4 h-4 text-primary" fill="currentColor" viewBox="0 0 20 20">
                              <path fill-rule="evenodd" d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clip-rule="evenodd" />
                            </svg>
                          {/if}
                        {:else}
                          <svg class="w-4 h-4 text-base-content/40 opacity-0 group-hover:opacity-100 transition-opacity" fill="currentColor" viewBox="0 0 20 20">
                            <path fill-rule="evenodd" d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z" clip-rule="evenodd" />
                          </svg>
                        {/if}
                      </span>
                      <span>Exceptions</span>
                    </div>
                  </th>
                </tr>
              </thead>
              <!-- Table Body -->
              <tbody class="divide-y divide-base-300">
                {#each sortedTraces as trace}
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
                          <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                            <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
                          </svg>
                        </span>
                      {:else}
                        <span class="inline-flex items-center justify-center w-6 h-6 rounded-full bg-base-300 text-base-content/50">
                          <svg class="w-4 h-4" fill="currentColor" viewBox="0 0 20 20">
                            <path fill-rule="evenodd" d="M4.293 4.293a1 1 0 011.414 0L10 8.586l4.293-4.293a1 1 0 111.414 1.414L11.414 10l4.293 4.293a1 1 0 01-1.414 1.414L10 11.414l-4.293 4.293a1 1 0 01-1.414-1.414L8.586 10 4.293 5.707a1 1 0 010-1.414z" clip-rule="evenodd" />
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
        </div>
      {/if}
</div>