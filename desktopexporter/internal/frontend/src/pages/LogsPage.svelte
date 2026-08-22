<script module lang="ts">
  import type { LogSummary } from '@/types/api-types'
  import {
    compareByStringField,
    compareByTimestampField,
  } from '@/utils/compare'

  // --- Sort ---

  export type LogSortColumn = 'timestamp' | 'severity' | 'service' | 'body'
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
  import { telemetryAPI } from '@/services/telemetry-service'
  import {
    getTimeContext,
    selectionToQueryRangeMs,
  } from '@/contexts/time-context.svelte'
  import { navigateToItem } from '@/route'
  import type { LogData } from '@/types/api-types'
  import { createSignalListPage } from '@/contexts/signal-list-page.svelte'
  import { createDebouncedDetailFetcher } from '@/components/shared/utils/debounced-detail-fetcher.svelte'
  import PageLayout from '@/components/shared/PageLayout.svelte'
  import DrawerSearchPanel from '@/components/shared/Drawer/DrawerSearchPanel.svelte'
  import SignalDrawerFooter from '@/components/shared/Drawer/SignalDrawerFooter.svelte'
  import LogCard from '@/components/logs/LogCard.svelte'
  import LogDetailPanel from '@/components/logs/LogDetailView.svelte'
  import SignalFooter from '@/components/shared/SignalFooter.svelte'

  let timeContext = getTimeContext()

  let baselineLogCount = $state(0)
  let polledLogCount = $state(0)
  let actionError = $state<string | null>(null)

  const page = createSignalListPage<LogSummary>({
    signal: 'logs',
    getItemID: log => log.id,
    initialSort: { column: 'timestamp', direction: 'desc' },
    compare: (a, b, col, dir) =>
      compareLogs(a, b, col as LogSortColumn, dir as LogSortDirection),
    fetchList: async () => {
      const { start: startTime, end: endTime } = selectionToQueryRangeMs(
        timeContext.selection,
        Date.now()
      )
      const results = await telemetryAPI.searchLogs(
        startTime,
        endTime,
        undefined
      )
      const s = await telemetryAPI.getStats()
      baselineLogCount = s.logs.logCount
      polledLogCount = s.logs.logCount
      return results
    },
    pollStats: async () => {
      const s = await telemetryAPI.getStats()
      polledLogCount = s.logs.logCount
    },
    refreshFromStats: () => {
      const delta = polledLogCount - baselineLogCount
      const pending = delta > 0 ? delta : 0
      return {
        pulse: pending > 0,
        tip:
          pending > 0
            ? `+${pending.toLocaleString()} log${pending !== 1 ? 's' : ''}`
            : '',
      }
    },
  })

  // selectedLogID is the user's pick from the list (the LogSummary `id`),
  // read from the route path. The detail fetcher round-trips to getLog(id) for
  // the full LogData on demand, with a debounce that keeps held-arrow keyboard
  // nav from firing a request per row.
  const detailFetcher = createDebouncedDetailFetcher<string, LogData>({
    fetch: id => telemetryAPI.getLog(id),
    keysEqual: (a, b) => a === b,
    fallbackErrorMessage: 'Failed to load log details',
  })

  let hasLogRows = $derived(page.items.length > 0)
  let displayError = $derived(page.error ?? actionError)

  $effect(() => {
    detailFetcher.key = page.selectedID
  })

  function selectLog(logID: string) {
    page.selectItem(logID)
  }

  async function handleDeleteLog(logID: string) {
    actionError = null
    try {
      await telemetryAPI.deleteLogByID(logID)
      if (page.selectedID === logID) {
        navigateToItem('logs', null, 'replace')
      }
      await page.runListFetch()
    } catch (err) {
      actionError = err instanceof Error ? err.message : 'Failed to delete log'
    }
  }

  async function handleDeleteAllLogs() {
    actionError = null
    try {
      await telemetryAPI.clearLogs()
      navigateToItem('logs', null, 'replace')
      await page.runListFetch()
    } catch (err) {
      actionError = err instanceof Error ? err.message : 'Failed to delete logs'
    }
  }
</script>

<div class="logs-page">
  <PageLayout
    items={page.sortedItems}
    selectedID={page.selectedID}
    drawerID="signal-drawer"
    drawerLabel="Logs"
    onRefresh={page.handleRefresh}
    refreshPulse={page.refreshPulse}
    refreshAsideTip={page.refreshAsideTip}
    loading={page.loading}
    itemKey={l => l.id}
  >
    {#snippet drawerChromeToolbar()}
      <DrawerSearchPanel
        segment="toolbar"
        signal="logs"
        sortOptions={SORT_OPTIONS}
        sortValue={page.sortColumn}
        sortDirection={page.sortDirection}
        onSortChange={page.handleSortChange}
      />
    {/snippet}

    {#snippet drawerSearch()}
      <DrawerSearchPanel
        segment="search"
        signal="logs"
        sortOptions={SORT_OPTIONS}
        sortValue={page.sortColumn}
        sortDirection={page.sortDirection}
        onSortChange={page.handleSortChange}
        onSearchResults={page.handleSearchResults}
        onSearchReady={api => (page.searchEditorApi = api)}
      />
    {/snippet}

    {#snippet itemSnippet(log, selected)}
      <LogCard {log} {selected} onclick={selectLog} />
    {/snippet}

    {#snippet drawerFooter()}
      <SignalDrawerFooter
        count={page.sortedItems.length}
        label="log"
        onDeleteAll={handleDeleteAllLogs}
      />
    {/snippet}

    {#snippet main()}
      {#if displayError}
        <div class="logs-page__placeholder alert alert-error">
          <span>Error: {displayError}</span>
        </div>
      {:else if page.loading && !hasLogRows}
        <div class="logs-page__placeholder logs-empty">Loading logs…</div>
      {:else if !page.loading && !hasLogRows}
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
        index={page.selectedIndex}
        total={page.sortedItems.length}
        label="log"
        onFirst={page.selectFirst}
        onPrev={() => page.selectByOffset(-1)}
        onNext={() => page.selectByOffset(1)}
        onLast={page.selectLast}
        onDelete={page.selectedSummary
          ? () => handleDeleteLog(page.selectedSummary!.id)
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
