<script lang="ts">
  import { onMount } from "svelte"
  import { telemetryAPI } from "../services/telemetry-service"
  import type { MetricData } from "../types/api-types"

  let metrics: MetricData[] = []
  let loading = true
  let error: string | null = null

  onMount(async () => {
    try {
      metrics = await telemetryAPI.getMetrics()
    } catch (err) {
      error = err instanceof Error ? err.message : "Failed to load metrics"
    } finally {
      loading = false
    }
  })
</script>

<!-- MetricsPage.svelte - Metrics visualization page -->
<div class="max-w-6xl mx-auto px-6 py-12">
  <div class="mb-8">
    <h1 class="text-3xl font-bold mb-2">Metrics</h1>
    <p class="text-base-content/70">
      View and analyze your OpenTelemetry metrics data
    </p>
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
      <p class="text-base-content/50 text-sm mt-2">
        Configure your OTLP exporter and send some metrics to see them here
      </p>
    </div>
  {:else}
    <div class="space-y-6">
      {#each metrics as metric}
        <div class="bg-base-200 border border-base-300 rounded-lg p-6">
          <div class="flex items-center justify-between mb-4">
            <h3 class="text-lg font-semibold">{metric.name}</h3>
            <span class="badge badge-outline">{metric.dataPoints.type}</span>
          </div>
          
          <div class="grid md:grid-cols-2 gap-4 mb-4">
            <div>
              <p class="text-sm text-base-content/70">Description</p>
              <p class="text-sm">{metric.description || "No description"}</p>
            </div>
            <div>
              <p class="text-sm text-base-content/70">Unit</p>
              <p class="text-sm">{metric.unit || "No unit"}</p>
            </div>
          </div>

          <div class="mb-4">
            <p class="text-sm text-base-content/70 mb-2">Data Points ({Array.isArray(metric.dataPoints) ? metric.dataPoints.length : 0})</p>
            <div class="max-h-32 overflow-y-auto">
              {#if Array.isArray(metric.dataPoints)}
                {#each metric.dataPoints.slice(0, 5) as dataPoint}
                  <div class="text-xs bg-base-100 p-2 rounded mb-1">
                    <div class="flex justify-between">
                      <span>Value: {dataPoint.value}</span>
                      <span>{dataPoint.timestamp.toLocal()}</span>
                    </div>
                  </div>
                {/each}
                {#if metric.dataPoints.length > 5}
                  <p class="text-xs text-base-content/50 text-center mt-2">
                    ... and {metric.dataPoints.length - 5} more
                  </p>
                {/if}
              {:else}
                <p class="text-xs text-base-content/50">No data points available</p>
              {/if}
            </div>
          </div>
        </div>
      {/each}
    </div>

    <!-- Raw JSON for debugging -->
    <details class="mt-8">
      <summary class="cursor-pointer text-sm text-base-content/60">Show raw JSON</summary>
      <pre class="mt-2 p-4 bg-base-100 border border-base-300 rounded text-xs overflow-auto max-h-64"><code>{JSON.stringify(metrics, (key, value) => typeof value === 'bigint' ? value.toString() : value, 2)}</code></pre>
    </details>
  {/if}
</div>
