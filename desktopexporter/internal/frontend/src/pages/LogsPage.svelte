<script module lang="ts">
  import type { LogSummary } from '@/types/api-types'
  import {
    compareByStringField,
    compareByTimestampField,
  } from '@/utils/compare'

  import type {
    LogSortColumn,
    LogSortDirection,
  } from '@/components/logs/log-table'
  export type {
    LogSortColumn,
    LogSortDirection,
  } from '@/components/logs/log-table'

  // Summaries include serviceName, so sorting does not require full log data.
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
            : col === 'name'
              ? compareByStringField(a, b, l => l.eventName ?? '')
              : compareByStringField(a, b, l => l.serviceName)

    return cmp !== 0
      ? dir === 'asc'
        ? cmp
        : -cmp
      : a.logRef.localeCompare(b.logRef)
  }
</script>

<script lang="ts">
  import { onDestroy, untrack } from 'svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import { confirmDeleteAll } from '@/utils/confirm-delete'
  import {
    getTimeContext,
    selectionToQueryRangeNs,
  } from '@/contexts/time-context.svelte'
  import { navigateToItem } from '@/route'
  import type {
    LogData,
    LogTableRow,
    LogErrorNavigation,
    LogErrorTarget,
  } from '@/types/api-types'
  import { createSignalListPage } from '@/contexts/signal-list-page.svelte'
  import { createDebouncedDetailFetcher } from '@/components/shared/utils/debounced-detail-fetcher.svelte'
  import PageLayout from '@/components/shared/PageLayout.svelte'
  import DrawerSearchPanel from '@/components/shared/Drawer/DrawerSearchPanel.svelte'
  import DateTimeFilter from '@/components/shared/Toolbar/DateTimeFilter.svelte'
  import LogTable from '@/components/logs/LogTable.svelte'
  import {
    loadLogColumns,
    logColumnRequests,
  } from '@/components/logs/log-table'
  import LogFilters from '@/components/logs/LogFilters.svelte'
  import type { FieldDefinition, ParsedSearchRequest } from '@/search/model'
  import type { DetailFieldTarget } from '@/components/shared/utils/reveal-detail-field'
  import { detailPanel } from '@/state/detail-panel.svelte'
  import LogDetailPanel from '@/components/logs/LogDetailView.svelte'
  import type { DetailViewMode } from '@/components/shared/DetailViewTabs.svelte'
  import Pagination from '@/components/shared/Pagination.svelte'
  import SignalFooter from '@/components/shared/SignalFooter.svelte'
  import { INGESTION_ISSUES_ID } from '@/components/shared/IngestionIssues.svelte'
  import type { ImportFailure } from '@/types/import-types'

  let {
    importFailure = null,
    onViewIssue,
  }: {
    importFailure?: ImportFailure | null
    onViewIssue?: (event: MouseEvent) => void
  } = $props()

  let timeContext = getTimeContext()

  let baselineLogCount = $state(0)
  let polledLogCount = $state(0)
  let actionError = $state<string | null>(null)
  let detailView = $state<DetailViewMode>('fields')
  let query = $state('')
  let fields = $state<FieldDefinition[]>([])
  let selectedColumns = $state(loadLogColumns())
  let selectionMode = $state(false)
  let checkedIDs = $state<string[]>([])
  let deletingSelected = $state(false)
  let submittedQuery: ParsedSearchRequest | null = null
  let submittedText = $state('')
  let pageNumber = $state(1)
  let pageSize = $state(100)
  let total = $state(0)
  let listVersion = $state(0)
  let errorNavigation = $state<LogErrorNavigation | null>(null)
  let errorNavigationError = $state('')
  let errorNavigationLoading = $state(false)
  let requestedPage = 1
  let requestedSize = 100
  let fetchVersion = 0
  let listRequest: AbortController | undefined
  onDestroy(() => listRequest?.abort())
  let lastTimeSelection = untrack(() => timeContext.selection)
  let queryWindow: ReturnType<typeof selectionToQueryRangeNs> | null = null
  let services = $state<string[]>([])
  let inspection = $state<{ id: string; target: DetailFieldTarget } | null>(
    null
  )

  const page = createSignalListPage<LogTableRow, LogSortColumn>({
    signal: 'logs',
    getItemID: log => log.logRef,
    initialSort: { column: 'severity', direction: 'desc' },
    preserveMissingSelection: true,
    serverSorted: true,
    compare: compareLogs,
    fetchList: async (): Promise<LogTableRow[]> => {
      const version = ++fetchVersion
      listRequest?.abort()
      const controller = new AbortController()
      listRequest = controller
      const request = untrack(() => {
        if (lastTimeSelection !== timeContext.selection) {
          requestedPage = 1
          queryWindow = null
          lastTimeSelection = timeContext.selection
        }
        queryWindow ??= selectionToQueryRangeNs(
          timeContext.selection,
          Date.now()
        )
        return {
          ...queryWindow,
          number: requestedPage,
          size: requestedSize,
          query: submittedQuery,
          sort: { field: page.sortColumn, direction: page.sortDirection },
          columns: logColumnRequests(selectedColumns),
        }
      })
      const fetchPage = (number: number) =>
        telemetryAPI.searchLogPage(
          request.startTime,
          request.endTime,
          request.size,
          (number - 1) * request.size,
          request.query?.predicate ?? undefined,
          request.query?.limit ?? undefined,
          request.sort,
          request.columns,
          controller.signal
        )
      let result = await fetchPage(request.number)
      if (version !== fetchVersion) return result.logs
      const lastPage = Math.max(1, Math.ceil(result.total / request.size))
      if (request.number > lastPage) {
        request.number = lastPage
        result = await fetchPage(lastPage)
      }
      if (version !== fetchVersion) return result.logs
      total = result.total
      pageNumber = request.number
      pageSize = request.size
      requestedPage = request.number
      listVersion++
      // Stats are only a freshness indicator; a failed poll must not hide a page.
      void telemetryAPI
        .getStats()
        .then(stats => {
          if (version !== fetchVersion) return
          baselineLogCount = stats.logs.logCount
          polledLogCount = stats.logs.logCount
        })
        .catch(() => {})
      return result.logs
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

  // Debouncing avoids a detail request for every row during keyboard navigation.
  const detailFetcher = createDebouncedDetailFetcher<string, LogData>({
    fetch: id => telemetryAPI.getLog(id),
    keysEqual: (a, b) => a === b,
    fallbackErrorMessage: 'Failed to load log details',
  })

  let displayError = $derived(page.error ?? actionError)
  let selectedLog = $derived(
    detailFetcher.data?.logRef === page.selectedID
      ? detailFetcher.data
      : undefined
  )

  $effect(() => {
    const names = page.items.map(log => log.serviceName).filter(Boolean)
    services = [...new Set([...untrack(() => services), ...names])]
  })

  $effect(() => {
    detailFetcher.key = page.selectedID
  })

  $effect(() => {
    void listVersion
    const selected = page.selectedID
    const loading = page.loading
    const mounted = page.mounted
    errorNavigationError = ''
    if (!mounted || loading) {
      errorNavigationLoading = loading
      return
    }
    const controller = new AbortController()
    const request = untrack(() => ({
      window: queryWindow,
      query: submittedQuery,
      sort: { field: page.sortColumn, direction: page.sortDirection },
    }))
    if (!request.window) return
    errorNavigationLoading = true
    void telemetryAPI
      .getLogErrorNavigation(
        request.window.startTime,
        request.window.endTime,
        selected,
        request.query?.predicate ?? undefined,
        request.query?.limit ?? undefined,
        request.sort,
        controller.signal
      )
      .then(result => {
        if (!controller.signal.aborted) errorNavigation = result
      })
      .catch(error => {
        if (!controller.signal.aborted) {
          errorNavigation = null
          errorNavigationError =
            error instanceof Error
              ? error.message
              : 'Could not load error navigation'
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) errorNavigationLoading = false
      })
    return () => controller.abort()
  })

  function selectError(target: LogErrorTarget) {
    const number = Math.floor(target.offset / requestedSize) + 1
    requestedPage = number
    selectLog(target.logRef, true)
    if (number !== pageNumber) void page.runListFetch()
  }

  function selectLog(logRef: string, keyboard = false) {
    inspection = null
    page.selectItem(logRef, keyboard ? 'replace' : 'push')
  }

  function inspectLog(id: string, target: DetailFieldTarget) {
    inspection = { id, target: { ...target } }
    detailPanel.reveal()
    page.selectItem(id)
  }

  // The stable log ID does not retrigger the detail fetch when the list reloads.
  function handleRefresh() {
    queryWindow = null
    page.handleRefresh()
    detailFetcher.refresh()
  }

  function changePage(number: number) {
    if (number === pageNumber || page.loading) return
    requestedPage = number
    void page.runListFetch()
  }

  function changePageSize(size: number) {
    requestedSize = size
    requestedPage = 1
    void page.runListFetch()
  }

  function changeSort(column: LogSortColumn, direction: LogSortDirection) {
    requestedPage = 1
    page.handleSortChange(column, direction)
    void page.runListFetch()
  }

  async function handleDeleteLog(logRef: string) {
    actionError = null
    try {
      await telemetryAPI.deleteLogsByRefs(logRef)
      if (page.selectedID === logRef) {
        navigateToItem('logs', null, 'replace')
      }
      await page.runListFetch()
    } catch (err) {
      actionError = err instanceof Error ? err.message : 'Failed to delete log'
    }
  }

  async function handleDeleteAllLogs() {
    if (!confirmDeleteAll('log')) return
    actionError = null
    try {
      await telemetryAPI.clearLogs()
      navigateToItem('logs', null, 'replace')
      await page.runListFetch()
    } catch (err) {
      actionError = err instanceof Error ? err.message : 'Failed to delete logs'
    }
  }

  async function handleDeleteSelectedLogs() {
    if (!checkedIDs.length || deletingSelected || page.loading) return
    const ids = [...checkedIDs]
    deletingSelected = true
    actionError = null
    try {
      await telemetryAPI.deleteLogsByRefs(...ids)
      checkedIDs = []
      if (page.selectedID && ids.includes(page.selectedID))
        navigateToItem('logs', null, 'replace')
      await page.runListFetch()
    } catch (err) {
      actionError =
        err instanceof Error ? err.message : 'Failed to delete selected logs'
    } finally {
      deletingSelected = false
    }
  }
</script>

<PageLayout
  hideDrawer
  mainSurface="base"
  items={page.sortedItems}
  navigationCount={total}
  selectedID={page.selectedID}
  drawerID="signal-drawer"
  drawerLabel="Filters"
  onRefresh={handleRefresh}
  refreshPulse={page.refreshPulse}
  refreshAsideTip={page.refreshAsideTip}
  loading={page.loading}
  itemKey={l => l.logRef}
>
  {#snippet drawerChromeToolbar()}
    <DateTimeFilter />
  {/snippet}

  {#snippet drawerSearch()}
    <DrawerSearchPanel
      segment="search"
      signal="logs"
      sortValue={page.sortColumn}
      sortDirection={page.sortDirection}
      onSearchReady={api => (page.searchEditorApi = api)}
      onQueryChange={(text, availableFields) => {
        query = text
        fields = availableFields
      }}
      onQuerySubmit={text => {
        if (text !== submittedText) {
          requestedPage = 1
          queryWindow = null
          submittedText = text
        }
      }}
      onSearchRequest={request => {
        submittedQuery = request
        void page.runListFetch()
      }}
    />
  {/snippet}

  {#snippet main()}
    {#if displayError || importFailure}
      <div class="logs-page__placeholder alert alert-error">
        {#if displayError}
          <span>Error: {displayError}</span>
        {:else if importFailure}
          <span>Import issue in {importFailure.fileName}.</span>
          <a class="link" href="/#{INGESTION_ISSUES_ID}" onclick={onViewIssue}
            >View issue</a
          >
        {/if}
      </div>
    {/if}
    <div class="logs-page__table">
      <LogTable
        {total}
        {pageNumber}
        {pageSize}
        {errorNavigation}
        {errorNavigationError}
        {errorNavigationLoading}
        onSelectError={selectError}
        logs={page.sortedItems}
        selectedID={page.selectedID}
        sortColumn={page.sortColumn}
        sortDirection={page.sortDirection}
        loading={page.loading}
        onSelect={selectLog}
        onInspect={inspectLog}
        onSort={changeSort}
        {fields}
        bind:selectedColumns
        bind:selectionMode
        bind:checkedIDs
        selectionBusy={deletingSelected}
        onColumnsChange={() => void page.runListFetch()}
      >
        {#snippet filters()}<LogFilters
            {query}
            appliedQuery={submittedText}
            {fields}
            {services}
            editor={page.searchEditorApi}
          />{/snippet}
      </LogTable>
    </div>
    <Pagination
      page={pageNumber}
      {pageSize}
      {total}
      loading={page.loading}
      onPage={changePage}
      onPageSize={changePageSize}
      onDeleteAll={handleDeleteAllLogs}
      {selectionMode}
      selectedCount={checkedIDs.length}
      deleting={deletingSelected}
      onDeleteSelected={handleDeleteSelectedLogs}
    />
  {/snippet}

  {#snippet detail()}
    {#if page.selectedID && !selectedLog && !detailFetcher.error}
      <div class="logs-page__placeholder logs-empty">Loading log details…</div>
    {:else if page.selectedID && detailFetcher.error}
      <div class="logs-page__placeholder alert alert-error">
        <span>Error: {detailFetcher.error}</span>
      </div>
    {:else}
      <LogDetailPanel
        log={selectedLog}
        bind:detailView
        fieldTarget={inspection?.id === selectedLog?.logRef
          ? inspection?.target
          : undefined}
      >
        {#snippet footer()}
          {#if selectedLog}
            {@const logRef = selectedLog.logRef}
            <SignalFooter
              label="log"
              onDelete={() => handleDeleteLog(logRef)}
            />
          {/if}
        {/snippet}
      </LogDetailPanel>
    {/if}
  {/snippet}
</PageLayout>

<style lang="postcss">
  @reference "../app.css";

  .logs-page__placeholder {
    @apply m-[var(--layout-gap)];
  }

  .logs-page__table {
    @apply min-h-0 min-w-0 flex-1;
  }

  .logs-empty {
    @apply px-4 py-12 text-center;
    color: var(--color-subtle);
  }
</style>
