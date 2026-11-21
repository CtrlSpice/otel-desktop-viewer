<script module lang="ts">
  import type { LogSummary } from '@/types/api-types'
  import {
    compareByStringField,
    compareByTimestampField,
  } from '@/utils/compare'

  import type {
    LogSortColumn,
    LogSortDirection,
  } from '@/features/logs/log-table'
  export type {
    LogSortColumn,
    LogSortDirection,
  } from '@/features/logs/log-table'

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
  import { untrack } from 'svelte'
  import { createRead } from '@/services/reads'
  import { createPageReadGroup } from '@/contexts/page-read-group.svelte'
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
  import PageLayout from '@/components/shared/Layout/PageLayout.svelte'
  import DrawerSearchPanel from '@/components/shared/Drawer/DrawerSearchPanel.svelte'
  import LogTable from '@/features/logs/LogTable.svelte'
  import { loadLogColumns, logColumnRequests } from '@/features/logs/log-table'
  import LogFilters from '@/features/logs/LogFilters.svelte'
  import type { FieldDefinition, ParsedSearchRequest } from '@/search/model'
  import type { DetailFieldTarget } from '@/components/shared/Detail/reveal-detail-field'
  import { detailPanel } from '@/state/detail-panel.svelte'
  import LogDetailPanel from '@/features/logs/LogDetailView.svelte'
  import type { DetailViewMode } from '@/components/shared/Detail/DetailViewTabs.svelte'
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
  const readGroup = createPageReadGroup(handleRefresh)
  const statsRead = createRead<
    Awaited<ReturnType<typeof telemetryAPI.getStats>>
  >({ group: null })
  const pagination = createRead<
    Awaited<ReturnType<typeof telemetryAPI.searchLogPage>>
  >({ errorMessage: 'Failed to load logs' })
  const navigationRead = createRead<LogErrorNavigation>({
    errorMessage: 'Could not load error navigation',
  })

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
  let total = $state<number | null>(null)
  let listVersion = $state(0)
  let errorNavigation = $state<LogErrorNavigation | null>(null)
  const errorNavigationError = $derived(navigationRead.error ?? '')
  const errorNavigationLoading = $derived(navigationRead.pending)
  let requestedPage = 1
  const pageLoading = $derived(pagination.pending)
  const pageRequestKey = $derived(pagination.requestKey)
  const pageError = $derived(pagination.error)
  let lastTimeSelection = untrack(() => timeContext.selection)
  let queryWindow: ReturnType<typeof selectionToQueryRangeNs> | null = null
  let services = $state<string[]>([])
  type Reading = {
    window: ReturnType<typeof selectionToQueryRangeNs>
    query: ParsedSearchRequest | null
    text: string
    sort: { field: LogSortColumn; direction: LogSortDirection }
    columns: ReturnType<typeof logColumnRequests>
  }
  let displayed = $state<Reading | null>(null)
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
    fetchList: async (signal): Promise<LogTableRow[]> => {
      statsRead.cancel()
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
          size: pageSize,
          query: submittedQuery,
          text: submittedText,
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
          signal
        )
      let result = await fetchPage(request.number)
      signal.throwIfAborted()
      const lastPage = Math.max(1, Math.ceil(result.total / request.size))
      if (request.number > lastPage) {
        request.number = lastPage
        result = await fetchPage(lastPage)
      }
      signal.throwIfAborted()
      pagination.clear()
      errorNavigation = null
      displayed = {
        window: { startTime: request.startTime, endTime: request.endTime },
        query: request.query,
        text: request.text,
        sort: request.sort,
        columns: request.columns,
      }
      total = result.total
      pageNumber = request.number
      pageSize = request.size
      requestedPage = request.number
      listVersion++
      // Stats are only a freshness indicator; a failed read must not hide a page.
      void statsRead.load(
        signal => telemetryAPI.getStats(signal),
        stats => {
          baselineLogCount = stats.logs.logCount
          polledLogCount = stats.logs.logCount
        }
      )
      return result.logs
    },
    statsPending: () => statsRead.pending,
    refreshStats: () =>
      statsRead.load(
        signal => telemetryAPI.getStats(signal),
        stats => {
          polledLogCount = stats.logs.logCount
        }
      ),
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
    fetch: (id, signal) => telemetryAPI.getLog(id, signal),
    keysEqual: (a, b) => a === b,
    fallbackErrorMessage: 'Failed to load log details',
  })

  let displayError = $derived(page.error ?? pageError ?? actionError)
  let listLoading = $derived(page.loading || pageLoading)
  let selectedLog = $derived(
    page.selectedID ? (detailFetcher.data ?? undefined) : undefined
  )
  let keyboardDetailID: string | null = null

  $effect(() => {
    const names = page.items.map(log => log.serviceName).filter(Boolean)
    services = [...new Set([...untrack(() => services), ...names])]
  })

  $effect(() => {
    const id = page.selectedID
    untrack(() => {
      detailFetcher.select(id, keyboardDetailID === id)
      keyboardDetailID = null
    })
  })

  $effect(() => {
    void listVersion
    const selected = page.selectedID
    const mounted = page.mounted
    const reading = displayed
    navigationRead.clearError()
    if (!mounted || !reading) {
      return
    }
    const request = untrack(() => ({
      window: reading.window,
      query: reading.query,
      sort: reading.sort,
    }))
    void navigationRead.load(
      signal =>
        telemetryAPI.getLogErrorNavigation(
          request.window.startTime,
          request.window.endTime,
          selected,
          request.query?.predicate ?? undefined,
          request.query?.limit ?? undefined,
          request.sort,
          signal
        ),
      result => {
        errorNavigation = result
      },
      {
        onError: () => {
          errorNavigation = null
        },
      }
    )
    return () => navigationRead.cancel()
  })

  function selectError(target: LogErrorTarget) {
    const number = Math.floor(target.offset / pageSize) + 1
    selectLog(target.logRef, true)
    if (number !== pageNumber) void fetchDisplayedPage(number, pageSize)
  }

  function selectLog(logRef: string, keyboard = false) {
    keyboardDetailID = keyboard ? logRef : null
    inspection = null
    page.selectItem(logRef, keyboard ? 'replace' : 'push')
  }

  function inspectLog(id: string, target: DetailFieldTarget) {
    keyboardDetailID = null
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
    if (number === pageNumber) return
    void fetchDisplayedPage(number, pageSize)
  }

  function changePageSize(size: number) {
    void fetchDisplayedPage(1, size)
  }

  async function fetchDisplayedPage(number: number, size: number) {
    const reading = displayed
    if (!reading) return
    await pagination.load(
      async signal => {
        const fetchPage = (number: number) =>
          telemetryAPI.searchLogPage(
            reading.window.startTime,
            reading.window.endTime,
            size,
            (number - 1) * size,
            reading.query?.predicate ?? undefined,
            reading.query?.limit ?? undefined,
            reading.sort,
            reading.columns,
            signal
          )
        let result = await fetchPage(number)
        signal.throwIfAborted()
        const lastPage = Math.max(1, Math.ceil(result.total / size))
        if (number > lastPage) {
          number = lastPage
          result = await fetchPage(number)
        }
        return result
      },
      result => {
        total = result.total
        pageNumber = number
        pageSize = size
        if (
          reading.text === submittedText &&
          reading.sort.field === page.sortColumn &&
          reading.sort.direction === page.sortDirection &&
          reading.window.startTime === queryWindow?.startTime &&
          reading.window.endTime === queryWindow?.endTime
        ) {
          requestedPage = number
        }
        page.publishItems(result.logs)
        listVersion++
      }
    )
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
  mainTitle="Logs"
  hideDrawer
  mainSurface="base"
  items={page.sortedItems}
  selectedID={page.selectedID}
  drawerID="signal-drawer"
  drawerLabel="Filters"
  onRefresh={() => void readGroup.refresh()}
  refreshPulse={page.refreshPulse}
  refreshAsideTip={page.refreshAsideTip}
  loading={listLoading}
  loadingRequestKey={`${page.requestKey}:${pageRequestKey}:${detailFetcher.requestKey}`}
  loadingMessage={detailFetcher.loading
    ? 'Loading selected log…'
    : listLoading
      ? 'Loading logs…'
      : undefined}
  itemKey={l => l.logRef}
>
  {#snippet drawerSearch()}
    <DrawerSearchPanel
      segment="search"
      signal="logs"
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
          <a class="link" href="/#{INGESTION_ISSUES_ID}" onclick={onViewIssue}>
            View issue
          </a>
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
        sortColumn={displayed?.sort.field ?? page.sortColumn}
        sortDirection={displayed?.sort.direction ?? page.sortDirection}
        loading={listLoading}
        loadingRequestKey={`${page.requestKey}:${pageRequestKey}`}
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
            appliedQuery={displayed?.text ?? ''}
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
      loading={listLoading}
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
    <LogDetailPanel
      loading={detailFetcher.loading}
      error={page.selectedID ? (detailFetcher.error ?? '') : ''}
      log={selectedLog}
      bind:detailView
      fieldTarget={inspection?.id === selectedLog?.logRef
        ? inspection?.target
        : undefined}
    >
      {#snippet footer()}
        {#if selectedLog}
          {@const logRef = selectedLog.logRef}
          <SignalFooter label="log" onDelete={() => handleDeleteLog(logRef)} />
        {/if}
      {/snippet}
    </LogDetailPanel>
  {/snippet}
</PageLayout>

<style lang="postcss">
  @reference "../../app.css";

  .logs-page__placeholder {
    @apply m-[var(--layout-gap)];
  }

  .logs-page__table {
    @apply min-h-0 min-w-0 flex-1;
  }
</style>
