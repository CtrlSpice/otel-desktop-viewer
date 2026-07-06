<script module lang="ts">
  import type { LogSummary } from '@/types/api-types'
  import {
    compareByStringField,
    compareByTimestampField,
  } from '@/utils/compare'

  // --- Sort ---

  export type LogSortColumn =
    | 'timestamp'
    | 'severity'
    | 'service'
    | 'body'
  export type LogSortDirection = 'asc' | 'desc'

  // The list page now operates on LogSummary (the card-shaped
  // projection); full LogData for the detail pane is fetched on
  // demand. serviceName is denormalized onto the summary, so we
  // sort against it directly instead of digging through resource
  // attributes.
  function compareLogs(
    a: LogSummary,
    b: LogSummary,
    col: LogSortColumn,
    dir: LogSortDirection
  ): number {
    const cmp =
      col === 'timestamp'
        ? compareByTimestampField(a, b, l => l.timestamp)
        : col === 'severity'
          ? a.severityNumber - b.severityNumber
          : col === 'body'
            ? compareByStringField(a, b, l => l.bodyPreview)
            : compareByStringField(a, b, l => l.serviceName)

    return cmp !== 0 ? (dir === 'asc' ? cmp : -cmp) : a.id.localeCompare(b.id)
  }

  const SORT_OPTIONS = [
    { value: 'timestamp', label: 'Timestamp' },
    { value: 'body', label: 'Body' },
    { value: 'service', label: 'Service Name' },
    { value: 'severity', label: 'Severity' },
  ]
</script>

<script lang="ts">
  import { onMount } from 'svelte'
  import { router } from 'tinro5'
  import { telemetryAPI } from '@/services/telemetry-service'
  import {
    getTimeContext,
    selectionToQueryRangeMs,
  } from '@/contexts/time-context.svelte'
  import { signalIdFromPath, navigateToItem } from '@/utils/url-state'
  import type { LogData, SearchResultEvent } from '@/types/api-types'
  import { createDebouncedDetailFetcher } from '@/components/shared/utils/debounced-detail-fetcher.svelte'
  import type { SearchEditorAPI } from '@/components/shared/Search/search-editor-api'
  import PageLayout from '@/components/shared/PageLayout.svelte'
  import DrawerSearchPanel from '@/components/shared/Drawer/DrawerSearchPanel.svelte'
  import LogCard from '@/components/logs/LogCard.svelte'
  import LogDetailPanel from '@/components/logs/LogDetailView.svelte'
  import SignalFooter from '@/components/shared/SignalFooter.svelte'
  import { TrashIcon } from '@/icons'

  // --- context ---
  let timeContext = getTimeContext()

  // --- URL is the source of truth for the selected log (`/logs/<id>`) ---
  let currentPath = $state(router.path ?? '/')
  $effect(() => {
    const unsubscribe = router.subscribe(route => {
      currentPath = route.path
    })
    return unsubscribe
  })

  // --- state: API / list ---
  let logs = $state<LogSummary[]>([])
  let loading = $state(true)
  let error = $state<string | null>(null)
  let mounted = $state(false)

  // --- state: sort ---
  let sortColumn = $state<LogSortColumn>('timestamp')
  let sortDirection = $state<LogSortDirection>('desc')

  // --- selection (derived from URL) ---
  //
  // selectedLogId is the user's pick from the list (the LogSummary
  // `id`), read from the route path. The detail fetcher round-trips to
  // getLog(id) for the full LogData on demand, with a debounce that keeps
  // held-arrow keyboard nav from firing a request per row. Detail loading/
  // error state lives on the fetcher object, not on the page.
  let selectedLogId = $derived(signalIdFromPath('logs', currentPath))
  const detailFetcher = createDebouncedDetailFetcher<string, LogData>({
    fetch: id => telemetryAPI.getLog(id),
    keysEqual: (a, b) => a === b,
    fallbackErrorMessage: 'Failed to load log details',
  })

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

  // The current selection from the list-side perspective: a
  // LogSummary, used for footer/nav rendering that doesn't need the
  // full body/attributes. The full LogData for the detail panel
  // lives in selectedLogDetail (fetched in an effect below).
  let selectedSummary = $derived(
    selectedLogId ? sortedLogs.find(l => l.id === selectedLogId) : undefined
  )

  let pendingNewLogCount = $derived.by(() => {
    const delta = polledLogCount - baselineLogCount
    return delta > 0 ? delta : 0
  })

  let refreshPulse = $derived(pendingNewLogCount > 0)

  let refreshAsideTip = $derived(
    pendingNewLogCount > 0
      ? `+${pendingNewLogCount.toLocaleString()} log${pendingNewLogCount !== 1 ? 's' : ''}`
      : ''
  )

  // --- effects ---
  let lastValidIndex = $state(0)

  // Guarded behind mounted + !loading so a URL-provided id (shared link) is
  // never replaced before the list has finished fetching.
  $effect(() => {
    if (!mounted || loading) return
    const id = selectedLogId
    const idx = id ? sortedLogs.findIndex(l => l.id === id) : -1
    if (idx >= 0) {
      lastValidIndex = idx
    } else if (sortedLogs.length > 0) {
      const fallback = sortedLogs[Math.min(lastValidIndex, sortedLogs.length - 1)]
      if (fallback) navigateToItem('logs', fallback.id, { replace: true })
    } else if (id) {
      navigateToItem('logs', null, { replace: true })
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

  // Pipe selection into the detail fetcher. The fetcher's $effect
  // handles the debounce + race-guarded round-trip and exposes
  // loading/error/data as reactive reads. Single source of truth
  // remains selectedLogId; the fetcher just mirrors it.
  $effect(() => {
    detailFetcher.key = selectedLogId
  })

  // --- handlers ---
  function handleSortChange(value: string, direction: 'asc' | 'desc') {
    sortColumn = value as LogSortColumn
    sortDirection = direction
  }

  function selectLog(logId: string) {
    // Explicit click is navigational: push so back returns to the prior log.
    navigateToItem('logs', logId, { replace: false })
  }

  // --- nav: walk sortedLogs ---

  let selectedIndex = $derived(
    selectedLogId ? sortedLogs.findIndex(l => l.id === selectedLogId) : -1
  )

  function selectByOffset(delta: number) {
    if (selectedIndex < 0 || sortedLogs.length === 0) return
    const target = Math.max(
      0,
      Math.min(sortedLogs.length - 1, selectedIndex + delta)
    )
    if (target === selectedIndex) return
    const next = sortedLogs[target]
    if (next) navigateToItem('logs', next.id, { replace: true })
  }

  function selectFirst() {
    const first = sortedLogs[0]
    if (first) navigateToItem('logs', first.id, { replace: true })
  }

  function selectLast() {
    const last = sortedLogs[sortedLogs.length - 1]
    if (last) navigateToItem('logs', last.id, { replace: true })
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
        navigateToItem('logs', null, { replace: true })
      }
      await fetchLogs()
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to delete log'
    }
  }

  async function handleDeleteAllLogs() {
    try {
      await telemetryAPI.clearLogs()
      navigateToItem('logs', null, { replace: true })
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
  <PageLayout
    items={sortedLogs}
    selectedId={selectedLogId}
    drawerId="signal-drawer"
    drawerLabel="Logs"
    onSelect={selectLog}
    onRefresh={handleRefresh}
    {refreshPulse}
    {refreshAsideTip}
    {loading}
    itemKey={l => l.id}
  >
    {#snippet drawerChromeToolbar()}
      <DrawerSearchPanel
        segment="toolbar"
        signal="logs"
        sortOptions={SORT_OPTIONS}
        sortValue={sortColumn}
        {sortDirection}
        onSortChange={handleSortChange}
      />
    {/snippet}

    {#snippet drawerSearch()}
      <DrawerSearchPanel
        segment="search"
        signal="logs"
        sortOptions={SORT_OPTIONS}
        sortValue={sortColumn}
        {sortDirection}
        onSortChange={handleSortChange}
        onSearchResults={handleSearchResults}
        onSearchReady={api => (searchEditorApi = api)}
      />
    {/snippet}

    {#snippet itemSnippet(log, selected)}
      <LogCard {log} {selected} onclick={selectLog} />
    {/snippet}

    {#snippet drawerFooter()}
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

    {#snippet main()}
      {#if error}
          <div class="logs-page__placeholder alert alert-error">
            <span>Error: {error}</span>
          </div>
        {:else if loading && !hasLogRows}
          <div class="logs-page__placeholder logs-empty">Loading logs…</div>
        {:else if !loading && !hasLogRows}
          <div class="logs-page__placeholder logs-empty">
            <p class="text-rp-subtle">No logs in this time range</p>
            <p class="mt-2 text-sm text-rp-muted">
              Send telemetry to the exporter or adjust the time range
            </p>
          </div>
        {:else if detailFetcher.loading && !detailFetcher.data}
          <div class="logs-page__placeholder logs-empty">
            Loading log details…
          </div>
        {:else if detailFetcher.error}
          <div class="logs-page__placeholder alert alert-error">
            <span>Error: {detailFetcher.error}</span>
          </div>
        {:else}
          <LogDetailPanel log={detailFetcher.data ?? undefined} />
      {/if}
    {/snippet}

    {#snippet pageFooter()}
      <SignalFooter
        index={selectedIndex}
        total={sortedLogs.length}
        label="log"
        onFirst={selectFirst}
        onPrev={() => selectByOffset(-1)}
        onNext={() => selectByOffset(1)}
        onLast={selectLast}
        onDelete={selectedSummary
          ? () => handleDeleteLog(selectedSummary.id)
          : undefined}
      />
    {/snippet}
  </PageLayout>
</div>

<style lang="postcss">
  @reference "../app.css";

  .logs-page {
    @apply flex min-h-0 min-w-0 w-full flex-1;
  }

  .logs-page__placeholder {
    @apply m-[var(--layout-gap)];
  }

  .logs-empty {
    @apply px-4 py-12 text-center;
    color: var(--color-subtle);
  }
</style>
