<script lang="ts">
  import { onMount } from "svelte"
  import { telemetryAPI } from "../services/telemetry-service"
  import type { TraceSummary } from "../types/api-types"
  import type { TraceFilters } from "../types/filter-types"
  import PageHeader from "../components/PageHeader.svelte"

  let traceSummaries: TraceSummary[] = []
  let loading = true
  let error: string | null = null

  // Filter state
  let filters: TraceFilters = {
    search: "",
    serviceName: [],
    timeRange: {
      start: "",
      end: ""
    },
    attributes: []
  }
  let timezone = 'UTC'

  // Handle filter changes from PageHeader
  function handleFiltersChange(newFilters: TraceFilters) {
    filters = newFilters
    // TODO: Apply filters to API call
  }

  function handleTimezoneChange(newTimezone: string) {
    timezone = newTimezone
  }

  function handleRefresh() {
    // TODO: Implement refresh logic
    console.log('Refresh clicked')
  }

  onMount(async () => {
    try {
      loading = true
      traceSummaries = await telemetryAPI.getTraceSummaries()
    } catch (err) {
      error = err instanceof Error ? err.message : "Failed to fetch traces"
      console.error("Error fetching trace summaries:", err)
    } finally {
      loading = false
    }
  })
</script>

<!-- TracesPage.svelte - Traces list and visualization page -->
<div class="flex flex-col w-full overflow-y-auto p-8">
  <!-- Page Header -->
  <PageHeader 
    title="Traces"
    {filters}
    onRefresh={handleRefresh}
    onFiltersChange={handleFiltersChange}
    onTimezoneChange={handleTimezoneChange}
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
          
          <!-- Trace summaries list -->
          <div class="space-y-2">
            {#each traceSummaries as trace}
              <div class="card bg-base-200 p-4">
                <div class="flex justify-between items-start">
                  <div>
                    <h3 class="font-semibold">{trace.rootSpan?.name || "Unknown Operation"}</h3>
                    <p class="text-sm text-base-content/70">Service: {trace.rootSpan?.serviceName || "Unknown Service"}</p>
                    <p class="text-sm text-base-content/70">Trace ID: {trace.traceID}</p>
                  </div>
                  <div class="text-right text-sm text-base-content/70">
                    <p>Spans: {trace.spanCount}</p>
                    {#if trace.rootSpan}
                      <p>Start: {trace.rootSpan.startTime.toString()}</p>
                    {/if}
                  </div>
                </div>
              </div>
            {/each}
          </div>

          <!-- Raw JSON for debugging -->
          <details class="mt-8">
            <summary class="cursor-pointer text-sm font-medium">Show Raw JSON</summary>
            <pre class="mt-2 p-4 bg-base-200 rounded text-xs overflow-auto">{JSON.stringify(traceSummaries, (key, value) => 
              typeof value === 'bigint' ? value.toString() : value, 2)}</pre>
          </details>
        </div>
      {/if}
</div>