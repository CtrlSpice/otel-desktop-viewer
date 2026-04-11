<script lang="ts">
  import { onMount } from 'svelte'
  import { router } from 'tinro5'
  import { telemetryAPI } from '@/services/telemetry-service'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import SignalToolbar from '@/components/SignalToolbar/SignalToolbar.svelte'
  import SearchEditor from '@/components/SignalToolbar/search/SearchEditor.svelte'
  import DateTimeFilter from '@/components/SignalToolbar/datetime/DateTimeFilter.svelte'
  import { formatTimestamp } from '@/utils/time'
  import type { MetricData, SearchResultEvent } from '@/types/api-types'

  let timeContext = getTimeContext()
  let metrics = $state<MetricData[]>([])
  let loading = $state(true)
  let error = $state<string | null>(null)
  let mounted = $state(false)
  let searchError = $state<string | null>(null)

  async function fetchMetrics() {
    try {
      loading = true
      error = null
      let startTime = timeContext.selection.start
      let endTime = timeContext.selection.end
      if (timeContext.selection.type === 'preset') {
        const duration = timeContext.selection.end - timeContext.selection.start
        endTime = Date.now()
        startTime = endTime - duration
      }
      metrics = await telemetryAPI.getMetrics(startTime, endTime, undefined)
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load metrics'
    } finally {
      loading = false
    }
  }

  $effect(() => {
    let _ = timeContext.selection
    if (mounted) {
      fetchMetrics()
    }
  })

  onMount(async () => {
    await fetchMetrics()
    mounted = true
  })

  function handleSearchResults(event: SearchResultEvent) {
    if (event.signal === 'metrics' && event.view === 'list') {
      loading = false
      error = null
      metrics = event.results
    }
  }
</script>

{#snippet toolbarTimeRange()}
  <DateTimeFilter />
{/snippet}

<!-- MetricsPage.svelte - Metrics visualization page -->
<div
  class="flex min-w-0 w-full flex-col gap-[var(--layout-gap)] overflow-y-auto pb-6 pt-0"
>
  <div class="page-toolbar-block">
    <SignalToolbar
      signal="metrics"
      view="list"
      onRefresh={fetchMetrics}
      trailingFilters={[toolbarTimeRange]}
      {searchError}

    >
      <SearchEditor
        signal="metrics"
        view="list"
        inToolbar
        onSearchResults={handleSearchResults}
        onSearchError={(err) => (searchError = err)}
      />
    </SignalToolbar>
  </div>

  {#if loading}
    <div class="flex justify-center items-center py-12">
      <span class="loading loading-spinner loading-lg"></span>
    </div>
  {:else if error}
    <div class="alert alert-error">
      <span>Error: {error}</span>
    </div>
  {:else if metrics.length === 0}
    <div class="text-center py-12">
      <p class="text-base-content/60 text-lg">No metrics data available</p>
      <p class="text-base-content/50 mt-2 text-sm">
        Configure your OTLP exporter and send some metrics to see them here
      </p>
    </div>
  {:else}
    <div class="space-y-[var(--layout-gap)]">
      {#each metrics as metric}
        <div class="bg-base-200 border border-base-300 rounded-lg p-6">
          <div class="flex items-center justify-between mb-4">
            <h3 class="text-lg font-semibold">
              <button
                type="button"
                class="link link-hover text-left font-semibold"
                onclick={() =>
                  router.goto(`/metrics/${encodeURIComponent(metric.name)}`)}
              >
                {metric.name}
              </button>
            </h3>
            {#if metric.datapoints.length > 0}
              <span class="badge badge-secondary badge-outline"
                >{metric.datapoints[0].metricType}</span
              >
            {/if}
          </div>

          <div class="mb-4 grid grid-cols-1 gap-4 min-[700px]:grid-cols-2">
            <div>
              <p class="text-sm text-base-content/70">Description</p>
              <p class="text-sm">{metric.description || 'No description'}</p>
            </div>
            <div>
              <p class="text-sm text-base-content/70">Unit</p>
              <p class="text-sm">{metric.unit || 'No unit'}</p>
            </div>
          </div>

          <div class="mb-4">
            <p class="text-sm text-base-content/70 mb-2">
              Data Points ({metric.datapoints.length})
            </p>
            <div class="max-h-32 overflow-y-auto">
              {#if metric.datapoints.length > 0}
                {#each metric.datapoints.slice(0, 5) as dataPoint}
                  <div class="text-xs bg-base-100 p-2 rounded mb-1">
                    <div class="flex justify-between">
                      <span>
                        {#if dataPoint.metricType === 'Gauge' || dataPoint.metricType === 'Sum'}
                          Value: {dataPoint.doubleValue ??
                            dataPoint.intValue ??
                            '-'}
                        {:else}
                          Count: {dataPoint.count}
                        {/if}
                      </span>
                      <span
                        >{formatTimestamp(
                          dataPoint.timestamp,
                          timeContext.timezone,
                          'nanoseconds'
                        )}</span
                      >
                    </div>
                  </div>
                {/each}
                {#if metric.datapoints.length > 5}
                  <p class="text-xs text-base-content/50 text-center mt-2">
                    ... and {metric.datapoints.length - 5} more
                  </p>
                {/if}
              {:else}
                <p class="text-xs text-base-content/50">
                  No data points available
                </p>
              {/if}
            </div>
          </div>
        </div>
      {/each}
    </div>

    <!-- Raw JSON for debugging -->
    <details class="mt-8">
      <summary class="cursor-pointer text-sm text-base-content/60"
        >Show raw JSON</summary
      >
      <pre
        class="mt-2 p-4 bg-base-100 border border-base-300 rounded text-xs overflow-auto max-h-64"><code
          >{JSON.stringify(
            metrics,
            (key, value) =>
              typeof value === 'bigint' ? value.toString() : value,
            2
          )}</code
        ></pre>
    </details>
  {/if}
</div>
