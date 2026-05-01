<script module lang="ts">
  import type { LogData } from '@/types/api-types'
  import {
    compareByStringField,
    compareByTimestampField,
  } from '@/utils/compare'
  import { getServiceName } from '@/utils/resource'

  // --- Sort ---

  export type LogSortColumn = 'timestamp' | 'severity' | 'service'
  export type LogSortDirection = 'asc' | 'desc'

  function compareLogs(
    a: LogData,
    b: LogData,
    col: LogSortColumn,
    dir: LogSortDirection
  ): number {
    const cmp =
      col === 'timestamp'
        ? compareByTimestampField(a, b, l => l.timestamp)
        : col === 'severity'
          ? a.severityNumber - b.severityNumber
          : compareByStringField(a, b, l => getServiceName(l.resource))

    return cmp !== 0 ? (dir === 'asc' ? cmp : -cmp) : a.id.localeCompare(b.id)
  }

  // --- Severity helpers ---

  type SeverityBand = 'trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal'

  export function severityBand(severityNumber: number): SeverityBand {
    if (severityNumber <= 4) return 'trace'
    if (severityNumber <= 8) return 'debug'
    if (severityNumber <= 12) return 'info'
    if (severityNumber <= 16) return 'warn'
    if (severityNumber <= 20) return 'error'
    return 'fatal'
  }

  const BADGE_CLASS: Record<SeverityBand, string> = {
    trace: 'badge badge-xs badge-soft badge-neutral',
    debug: 'badge badge-xs badge-soft badge-info',
    info: 'badge badge-xs badge-soft badge-success',
    warn: 'badge badge-xs badge-soft badge-warning',
    error: 'badge badge-xs badge-soft badge-error',
    fatal: 'badge badge-xs badge-soft badge-error',
  }

  const BORDER_CLASS: Record<SeverityBand, string> = {
    trace: 'border-l-neutral/40',
    debug: 'border-l-info/40',
    info: 'border-l-success/40',
    warn: 'border-l-warning/40',
    error: 'border-l-error/40',
    fatal: 'border-l-error',
  }

  export function severityBadgeClass(severityNumber: number): string {
    return BADGE_CLASS[severityBand(severityNumber)]
  }

  export function severityBorderClass(severityNumber: number): string {
    return BORDER_CLASS[severityBand(severityNumber)]
  }

  export function severityLabel(
    severityText: string,
    severityNumber: number
  ): string {
    return severityText || severityBand(severityNumber).toUpperCase()
  }

  const SORT_OPTIONS = [
    { value: 'timestamp', label: 'Timestamp' },
    { value: 'severity', label: 'Severity' },
    { value: 'service', label: 'Service' },
  ]
</script>

<script lang="ts">
  import { onMount } from 'svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import {
    getTimeContext,
    selectionToQueryRangeMs,
  } from '@/contexts/time-context.svelte'
  import type { SearchResultEvent } from '@/types/api-types'
  import type { SearchEditorAPI } from '@/components/SignalToolbar/search/search-editor-api'
  import SignalListDrawer from '@/components/SignalListDrawer.svelte'
  import DrawerSearchPanel from '@/components/DrawerSearchPanel.svelte'
  import LogCard from '@/components/LogCard.svelte'
  import LogDetailPanel from '@/components/LogDetails/LogDetailPanel.svelte'
  import { TrashIcon } from '@/icons'

  // --- context ---
  let timeContext = getTimeContext()

  // --- state: API / list ---
  let logs = $state<LogData[]>([])
  let loading = $state(true)
  let error = $state<string | null>(null)
  let mounted = $state(false)

  // --- state: sort ---
  let sortColumn = $state<LogSortColumn>('timestamp')
  let sortDirection = $state<LogSortDirection>('desc')

  // --- state: selection ---
  let selectedLogId = $state<string | null>(null)

  // --- state: polling / refresh ---
  let searchEditorApi = $state<SearchEditorAPI | null>(null)
  let baselineLogCount = $state(0)
  let polledLogCount = $state(0)
  const POLL_INTERVAL_MS = 3000

  // --- derived ---
  let sortedLogs = $derived.by(() => {
    const col = sortColumn
    const dir = sortDirection
    const rows = [...logs]
    rows.sort((a, b) => compareLogs(a, b, col, dir))
    return rows
  })

  let hasLogRows = $derived(logs.length > 0)

  let selectedLog = $derived(
    selectedLogId ? sortedLogs.find(l => l.id === selectedLogId) : undefined
  )

  let pendingNewLogCount = $derived.by(() => {
    const delta = polledLogCount - baselineLogCount
    return delta > 0 ? delta : 0
  })

  let refreshPulse = $derived(pendingNewLogCount > 0)

  // --- effects ---
  $effect(() => {
    if (!selectedLogId || !sortedLogs.some(l => l.id === selectedLogId)) {
      const first = sortedLogs[0]
      selectedLogId = first?.id ?? null
    }
  })

  $effect(() => {
    void timeContext.selection
    if (mounted) {
      fetchLogs()
    }
  })

  $effect(() => {
    if (!mounted) return
    const id = setInterval(async () => {
      try {
        const s = await telemetryAPI.getStats()
        polledLogCount = s.logs.logCount
      } catch {
        /* polling failures are silent */
      }
    }, POLL_INTERVAL_MS)
    return () => clearInterval(id)
  })

  // --- handlers ---
  function handleSortChange(value: string, direction: 'asc' | 'desc') {
    sortColumn = value as LogSortColumn
    sortDirection = direction
  }

  function selectLog(logId: string) {
    selectedLogId = logId
  }

  async function fetchLogs() {
    try {
      loading = true
      error = null
      const { start: startTime, end: endTime } = selectionToQueryRangeMs(
        timeContext.selection,
        Date.now()
      )
      logs = await telemetryAPI.searchLogs(startTime, endTime, undefined)
      const s = await telemetryAPI.getStats()
      baselineLogCount = s.logs.logCount
      polledLogCount = s.logs.logCount
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to load logs'
    } finally {
      loading = false
    }
  }

  function handleRefresh() {
    searchEditorApi?.clear()
    fetchLogs()
  }

  function handleSearchResults(event: SearchResultEvent) {
    if (event.signal === 'logs') {
      loading = false
      error = null
      logs = event.results
    }
  }

  async function handleDeleteLog(logId: string) {
    try {
      await telemetryAPI.deleteLogByID(logId)
      if (selectedLogId === logId) {
        selectedLogId = null
      }
      await fetchLogs()
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to delete log'
    }
  }

  async function handleDeleteAllLogs() {
    try {
      await telemetryAPI.clearLogs()
      selectedLogId = null
      await fetchLogs()
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to delete logs'
    }
  }

  // --- lifecycle ---
  onMount(async () => {
    await fetchLogs()
    mounted = true
  })
</script>

<div class="logs-page">
  <SignalListDrawer
    items={sortedLogs}
    selectedId={selectedLogId}
    drawerId="log-drawer"
    label="Logs"
    count={sortedLogs.length}
    storageKey="log-drawer"
    onSelect={selectLog}
    onRefresh={handleRefresh}
    {refreshPulse}
    itemKey={l => l.id}
  >
    {#snippet refreshAside()}
      {#if pendingNewLogCount > 0}
        <span class="signal-drawer__refresh-aside-pill">
          +{pendingNewLogCount.toLocaleString()}
          log{pendingNewLogCount !== 1 ? 's' : ''}
        </span>
      {/if}
    {/snippet}

    {#snippet drawerChrome()}
      <DrawerSearchPanel
        segment="chrome"
        signal="logs"
        sortOptions={SORT_OPTIONS}
        sortValue={sortColumn}
        {sortDirection}
        onSortChange={handleSortChange}
        onRefresh={handleRefresh}
        {refreshPulse}
      >
        {#snippet refreshAside()}
          {#if pendingNewLogCount > 0}
            <span class="signal-drawer__refresh-aside-pill">
              +{pendingNewLogCount.toLocaleString()}
              log{pendingNewLogCount !== 1 ? 's' : ''}
            </span>
          {/if}
        {/snippet}
      </DrawerSearchPanel>
    {/snippet}

    {#snippet drawerSearch()}
      <DrawerSearchPanel
        segment="search"
        signal="logs"
        sortOptions={SORT_OPTIONS}
        sortValue={sortColumn}
        {sortDirection}
        onSortChange={handleSortChange}
        onRefresh={handleRefresh}
        {refreshPulse}
        onSearchResults={handleSearchResults}
        onSearchReady={api => (searchEditorApi = api)}
      />
    {/snippet}

    {#snippet itemSnippet(log, selected)}
      <LogCard {log} {selected} onclick={selectLog} />
    {/snippet}

    {#snippet footer()}
      <div class="flex items-center justify-between">
        <span class="text-xs tabular-nums text-base-content/50">
          {sortedLogs.length} log{sortedLogs.length !== 1 ? 's' : ''}
        </span>
        <button
          type="button"
          class="btn btn-ghost btn-xs text-error"
          onclick={handleDeleteAllLogs}
          aria-label="Delete all logs"
        >
          <TrashIcon class="h-3 w-3" aria-hidden="true" />
          Delete all
        </button>
      </div>
    {/snippet}

    {#snippet children()}
      <div class="logs-content">
        <div class="logs-content__body">
          {#if error}
            <div class="alert alert-error">
              <span>Error: {error}</span>
            </div>
          {:else if loading && !hasLogRows}
            <div class="logs-empty">Loading logs…</div>
          {:else if !loading && !hasLogRows}
            <div class="logs-empty">
              <p class="text-base-content/60">No logs in this time range</p>
              <p class="mt-2 text-sm text-base-content/50">
                Send telemetry to the exporter or adjust the time range
              </p>
            </div>
          {:else}
            <div class="logs-detail">
              <LogDetailPanel log={selectedLog} onDelete={handleDeleteLog} />
            </div>
          {/if}
        </div>
      </div>
    {/snippet}
  </SignalListDrawer>
</div>

<style lang="postcss">
  @reference "../app.css";

  .logs-page {
    @apply flex min-h-0 min-w-0 w-full flex-1;
  }

  .logs-content {
    @apply relative flex min-h-0 min-w-0 flex-1 flex-col;
  }

  .logs-content__body {
    @apply flex min-h-0 min-w-0 flex-1 flex-col p-[var(--layout-gap)];
  }

  .logs-detail {
    @apply flex-1 min-h-0 min-w-0 overflow-hidden rounded-xl border border-base-300/70 bg-base-100/80 shadow-surface-sm backdrop-blur-sm;
  }

  .logs-empty {
    @apply rounded-xl border border-base-300/70 bg-base-100/80 px-4 py-12 text-center text-base-content/60 shadow-surface-sm backdrop-blur-sm;
  }
</style>
