<script lang="ts">
  import { onMount } from 'svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import SignalToolbar from '@/components/SignalToolbar/SignalToolbar.svelte'
  import SearchEditor from '@/components/SignalToolbar/search/SearchEditor.svelte'
  import DateTimeFilter from '@/components/SignalToolbar/datetime/DateTimeFilter.svelte'
  import { formatTimestamp } from '@/utils/time'
  import type { LogData, SearchResultEvent } from '@/types/api-types'

  let timeContext = getTimeContext()
  let logs = $state<LogData[]>([])
  let loading = $state(true)
  let error = $state<string | null>(null)
  let mounted = $state(false)

  async function fetchLogs() {
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
      logs = await telemetryAPI.searchLogs(startTime, endTime, undefined)
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load logs'
    } finally {
      loading = false
    }
  }

  $effect(() => {
    let _ = timeContext.selection
    if (mounted) {
      fetchLogs()
    }
  })

  onMount(async () => {
    await fetchLogs()
    mounted = true
  })

  function handleSearchResults(event: SearchResultEvent) {
    if (event.signal === 'logs' && event.view === 'list') {
      loading = false
      error = null
      logs = event.results
    }
  }
</script>

{#snippet toolbarTimeRange()}
  <DateTimeFilter />
{/snippet}

<!-- LogsPage.svelte - Logs viewing page -->
<div
  class="flex min-w-0 w-full flex-col gap-[var(--layout-gap)] overflow-y-auto pb-6 pt-0"
>
  <SignalToolbar
    signal="logs"
    view="list"
    onRefresh={fetchLogs}
    trailingFilters={[toolbarTimeRange]}
  />
  <SearchEditor
    signal="logs"
    view="list"
    inToolbar
    onSearchResults={handleSearchResults}
  />

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
      <p class="text-base-content/50 mt-2 text-sm">
        Configure your OTLP exporter and send some logs to see them here
      </p>
    </div>
  {:else}
    <div class="space-y-[var(--layout-gap)]">
      {#each logs as log}
        <div class="bg-base-200 border border-base-300 rounded-lg p-4">
          <div class="flex items-start justify-between mb-3">
            <div class="flex-1">
              <div class="flex items-center gap-3 mb-2">
                <span class="badge badge-secondary badge-outline text-xs"
                  >{log.severityText}</span
                >
                <span class="text-sm text-base-content/70"
                  >{formatTimestamp(
                    log.timestamp,
                    timeContext.timezone,
                    'nanoseconds'
                  )}</span
                >
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

          {#if log.attributes && log.attributes.length > 0}
            <details class="mt-3">
              <summary class="cursor-pointer text-xs text-base-content/60"
                >Attributes ({log.attributes.length})</summary
              >
              <div class="mt-2 p-2 bg-base-100 rounded text-xs">
                {#each log.attributes as attr}
                  <div class="flex justify-between mb-1">
                    <span class="font-mono">{attr.key}:</span>
                    <span class="font-mono">{attr.value}</span>
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
      <summary class="cursor-pointer text-sm text-base-content/60"
        >Show raw JSON</summary
      >
      <pre
        class="mt-2 p-4 bg-base-100 border border-base-300 rounded text-xs overflow-auto max-h-64"><code
          >{JSON.stringify(
            logs,
            (key, value) =>
              typeof value === 'bigint' ? value.toString() : value,
            2
          )}</code
        ></pre>
    </details>
  {/if}
</div>
