<script lang="ts">
  import { router } from 'tinro5'
  import { telemetryAPI } from '@/services/telemetry-service'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import SignalToolbar from '@/components/SignalToolbar/SignalToolbar.svelte'
  import SearchEditor from '@/components/SignalToolbar/search/SearchEditor.svelte'
  import DateTimeFilter from '@/components/SignalToolbar/datetime/DateTimeFilter.svelte'
  import { formatTimestamp } from '@/utils/time'
  import type { MetricData, MetricStats } from '@/types/api-types'
  import type { SearchEditorAPI } from '@/components/SignalToolbar/search/search-editor-api'

  let timeContext = getTimeContext()
  let metricName = $state('')
  let metrics = $state<MetricData[]>([])
  let loading = $state(true)
  let error = $state<string | null>(null)
  let searchError = $state<string | null>(null)

  let searchEditorApi = $state<SearchEditorAPI | null>(null)
  let baselineDataPointCount = $state(0)
  let polledDataPointCount = $state(0)
  const POLL_INTERVAL_MS = 3000

  let refreshIndicatorText = $derived.by(() => {
    const delta = polledDataPointCount - baselineDataPointCount
    if (delta <= 0) return ''
    return `+${delta} data point${delta !== 1 ? 's' : ''}`
  })

  $effect(() => {
    let unsubscribe = router.subscribe(route => {
      let match = route.path.match(/^\/metrics\/(.+)$/)
      if (match && match[1]) {
        try {
          metricName = decodeURIComponent(match[1])
          error = null
        } catch {
          metricName = ''
          error = 'Invalid metric name in URL'
          loading = false
        }
      } else {
        metricName = ''
        error = 'No metric name provided'
        loading = false
      }
    })
    return unsubscribe
  })

  let loadSeq = 0

  async function fetchMetricDetail() {
    if (!metricName) return
    const seq = ++loadSeq
    loading = true
    error = null
    try {
      let startTime = timeContext.selection.start
      let endTime = timeContext.selection.end
      if (timeContext.selection.type === 'preset') {
        const duration = timeContext.selection.end - timeContext.selection.start
        endTime = Date.now()
        startTime = endTime - duration
      }
      let all = await telemetryAPI.getMetrics(startTime, endTime, undefined)
      if (seq !== loadSeq) return
      metrics = all.filter(m => m.name === metricName)
      if (metrics.length === 0) {
        error = null
      }
      const s = await telemetryAPI.getStats()
      baselineDataPointCount = s.metrics.dataPointCount
      polledDataPointCount = s.metrics.dataPointCount
    } catch (err) {
      if (seq !== loadSeq) return
      error = err instanceof Error ? err.message : 'Failed to load metrics'
    } finally {
      if (seq === loadSeq) {
        loading = false
      }
    }
  }

  $effect(() => {
    if (!metricName) return
    let _ = timeContext.selection
    void fetchMetricDetail()
  })

  function handleBack() {
    if (history.length > 1) {
      history.back()
    } else {
      router.goto('/metrics')
    }
  }

  function handleRefresh() {
    searchEditorApi?.clear()
    fetchMetricDetail()
  }

  $effect(() => {
    if (!metricName) return
    const id = setInterval(async () => {
      try {
        const s = await telemetryAPI.getStats()
        polledDataPointCount = s.metrics.dataPointCount
      } catch {
        /* polling failures are silent */
      }
    }, POLL_INTERVAL_MS)
    return () => clearInterval(id)
  })
</script>

{#snippet toolbarTimeRange()}
  <DateTimeFilter />
{/snippet}

<!-- MetricsDetailPage.svelte — stub; mirrors MetricsPage for a single metric from the URL -->
<div
  class="flex min-w-0 w-full flex-col gap-[var(--layout-gap)] overflow-y-auto pb-6 pt-0"
>
  <div class="page-toolbar-block">
    <SignalToolbar
      signal="metrics"
      view="detail"
      metricName={metricName || 'Metric'}
      onBack={handleBack}
      onRefresh={handleRefresh}
      trailingFilters={[toolbarTimeRange]}
      {searchError}
      {refreshIndicatorText}
    >
      <SearchEditor
        signal="metrics"
        view="detail"
        inToolbar
        onSearchError={err => (searchError = err)}
        onReady={api => (searchEditorApi = api)}
      />
    </SignalToolbar>
  </div>

  {#if loading}
    <div class="flex items-center justify-center py-12">
      <span class="loading loading-spinner loading-lg"></span>
    </div>
  {:else if error}
    <div class="alert alert-error">
      <span>Error: {error}</span>
    </div>
  {:else if metrics.length === 0}
    <div class="py-12 text-center">
      <p class="text-lg text-base-content/60">Metric not found</p>
      <p class="mt-2 text-sm text-base-content/50">
        No metric named <span class="font-mono">{metricName}</span> in the current
        data
      </p>
    </div>
  {:else}
    <div class="flex flex-col gap-[var(--layout-gap)]">
      <div class="space-y-[var(--layout-gap)]">
        {#each metrics as metric}
          <div class="rounded-lg border border-base-300 bg-base-200 p-6">
            <div class="mb-4 flex items-center justify-between">
              <h3 class="text-lg font-semibold">{metric.name}</h3>
              {#if metric.datapoints.length > 0}
                <span class="badge badge-sm text-xs badge-secondary badge-outline"
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
              <p class="mb-2 text-sm text-base-content/70">
                Data Points ({metric.datapoints.length})
              </p>
              <div class="max-h-32 overflow-y-auto">
                {#if metric.datapoints.length > 0}
                  {#each metric.datapoints.slice(0, 5) as dataPoint}
                    <div class="mb-1 rounded bg-base-100 p-2 text-xs">
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
                    <p class="mt-2 text-center text-xs text-base-content/50">
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

      <details>
        <summary class="cursor-pointer text-sm text-base-content/60"
          >Show raw JSON</summary
        >
        <pre
          class="mt-2 max-h-64 overflow-auto rounded border border-base-300 bg-base-100 p-4 text-xs"><code
            >{JSON.stringify(
              metrics,
              (key, value) =>
                typeof value === 'bigint' ? value.toString() : value,
              2
            )}</code
          ></pre>
      </details>
    </div>
  {/if}
</div>
