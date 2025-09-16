<script lang="ts">
  import { onMount } from "svelte"
  import { telemetryAPI } from "../services/telemetry-service"
  import type { LogData } from "../types/api-types"

  let logs: LogData[] = []
  let loading = true
  let error: string | null = null

  onMount(async () => {
    try {
      logs = await telemetryAPI.getLogs()
    } catch (err) {
      error = err instanceof Error ? err.message : "Failed to load logs"
    } finally {
      loading = false
    }
  })
</script>

<!-- LogsPage.svelte - Logs viewing page -->
<div class="max-w-6xl mx-auto px-6 py-12">
  <div class="mb-8">
    <h1 class="text-3xl font-bold mb-2">Logs</h1>
    <p class="text-base-content/70">
      View and search your OpenTelemetry logs
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
  {:else if logs.length === 0}
    <div class="text-center py-12">
      <p class="text-base-content/60 text-lg">No logs available</p>
      <p class="text-base-content/50 text-sm mt-2">
        Configure your OTLP exporter and send some logs to see them here
      </p>
    </div>
  {:else}
    <div class="space-y-4">
      {#each logs as log}
        <div class="bg-base-200 border border-base-300 rounded-lg p-4">
          <div class="flex items-start justify-between mb-3">
            <div class="flex-1">
              <div class="flex items-center gap-3 mb-2">
                <span class="badge badge-outline text-xs">{log.severityText}</span>
                <span class="text-sm text-base-content/70">{log.timestamp.toLocal()}</span>
                {#if log.traceID}
                  <span class="text-xs text-primary">Trace: {log.traceID}</span>
                {/if}
                {#if log.spanID}
                  <span class="text-xs text-secondary">Span: {log.spanID}</span>
                {/if}
              </div>
              <p class="text-sm font-medium mb-2">{log.body}</p>
            </div>
          </div>
          
          {#if log.attributes && Object.keys(log.attributes).length > 0}
            <details class="mt-3">
              <summary class="cursor-pointer text-xs text-base-content/60">Attributes ({Object.keys(log.attributes).length})</summary>
              <div class="mt-2 p-2 bg-base-100 rounded text-xs">
                {#each Object.entries(log.attributes) as [key, value]}
                  <div class="flex justify-between mb-1">
                    <span class="font-mono">{key}:</span>
                    <span class="font-mono">{value}</span>
                  </div>
                {/each}
              </div>
            </details>
          {/if}
        </div>
      {/each}
    </div>

    <!-- Raw JSON for debugging -->
    <details class="mt-8">
      <summary class="cursor-pointer text-sm text-base-content/60">Show raw JSON</summary>
      <pre class="mt-2 p-4 bg-base-100 border border-base-300 rounded text-xs overflow-auto max-h-64"><code>{JSON.stringify(logs, (key, value) => typeof value === 'bigint' ? value.toString() : value, 2)}</code></pre>
    </details>
  {/if}
</div>
