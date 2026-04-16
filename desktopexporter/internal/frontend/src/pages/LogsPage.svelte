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

    return cmp !== 0
      ? dir === 'asc'
        ? cmp
        : -cmp
      : a.id.localeCompare(b.id)
  }

  // --- Table state (localStorage persistence) ---

  const LOG_TABLE_STORAGE_KEY = 'otel-desktop-viewer:log-list-table-state-v1'

  interface LogListTableState {
    sortColumn: LogSortColumn
    sortDirection: LogSortDirection
    rowsPerPage: number
  }

  const LOG_TABLE_DEFAULTS: LogListTableState = {
    sortColumn: 'timestamp',
    sortDirection: 'desc',
    rowsPerPage: 25,
  }

  const VALID_LOG_SORT_COLUMNS: ReadonlySet<string> = new Set<LogSortColumn>([
    'timestamp',
    'severity',
    'service',
  ])

  const VALID_ROWS_PER_PAGE: ReadonlySet<number> = new Set([10, 25, 50, 100])

  function loadLogListTableState(): LogListTableState {
    if (typeof localStorage === 'undefined') return { ...LOG_TABLE_DEFAULTS }
    try {
      const raw = localStorage.getItem(LOG_TABLE_STORAGE_KEY)
      if (!raw) return { ...LOG_TABLE_DEFAULTS }
      const o = JSON.parse(raw) as Partial<LogListTableState>
      return {
        sortColumn: VALID_LOG_SORT_COLUMNS.has(o.sortColumn ?? '')
          ? (o.sortColumn as LogSortColumn)
          : LOG_TABLE_DEFAULTS.sortColumn,
        sortDirection:
          o.sortDirection === 'asc' || o.sortDirection === 'desc'
            ? o.sortDirection
            : LOG_TABLE_DEFAULTS.sortDirection,
        rowsPerPage: VALID_ROWS_PER_PAGE.has(o.rowsPerPage ?? -1)
          ? o.rowsPerPage!
          : LOG_TABLE_DEFAULTS.rowsPerPage,
      }
    } catch {
      return { ...LOG_TABLE_DEFAULTS }
    }
  }

  function saveLogListTableState(state: LogListTableState): void {
    if (typeof localStorage === 'undefined') return
    localStorage.setItem(LOG_TABLE_STORAGE_KEY, JSON.stringify(state))
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
    trace: 'badge badge-sm text-xs badge-soft badge-neutral',
    debug: 'badge badge-sm text-xs badge-soft badge-info',
    info: 'badge badge-sm text-xs badge-soft badge-success',
    warn: 'badge badge-sm text-xs badge-soft badge-warning',
    error: 'badge badge-sm text-xs badge-soft badge-error',
    fatal: 'badge badge-sm text-xs badge-error',
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

  export function severityLabel(severityText: string, severityNumber: number): string {
    return severityText || severityBand(severityNumber).toUpperCase()
  }
</script>

<script lang="ts">
  import { onMount, untrack } from 'svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import {
    getTimeContext,
    selectionToQueryRangeMs,
  } from '@/contexts/time-context.svelte'
  import { formatTimestamp } from '@/utils/time'
  import type { SearchResultEvent } from '@/types/api-types'
  import type { SearchEditorAPI } from '@/components/SignalToolbar/search/search-editor-api'
  import SignalToolbar from '@/components/SignalToolbar/SignalToolbar.svelte'
  import SearchEditor from '@/components/SignalToolbar/search/SearchEditor.svelte'
  import DateTimeFilter from '@/components/SignalToolbar/datetime/DateTimeFilter.svelte'
  import ResizablePanels from '@/components/ResizablePanels.svelte'
  import LogDetailPanel from '@/components/LogDetails/LogDetailPanel.svelte'
  import {
    ArrowDownIcon,
    ArrowLeftDoubleIcon,
    ArrowLeftIcon,
    ArrowRightDoubleIcon,
    ArrowRightIcon,
    TrashIcon,
  } from '@/icons'
  import {
    fixed, flex,
    computeInitialWidths,
    redistributeWidths,
    computeBarPositions,
    startColumnResize,
  } from '@/utils/column-resize'
  import { tableNav } from '@/utils/table-keyboard-nav'

  // --- context ---
  let timeContext = getTimeContext()

  // --- state: API / list ---
  let logs = $state<LogData[]>([])
  let loading = $state(true)
  let error = $state<string | null>(null)
  let mounted = $state(false)
  let searchError = $state<string | null>(null)

  // --- state: sort + pagination (persisted via localStorage) ---
  const savedTableState = loadLogListTableState()
  let sortColumn = $state<LogSortColumn>(savedTableState.sortColumn)
  let sortDirection = $state<LogSortDirection>(savedTableState.sortDirection)
  let currentPage = $state(1)
  let rowsPerPage = $state(savedTableState.rowsPerPage)
  let rowsPerPageOptions = [10, 25, 50, 100]
  let rowsPerPagePopoverOpen = $state(false)

  // --- state: selection ---
  let selectedLogId = $state<string | null>(null)

  // --- state: polling / refresh indicator ---
  let searchEditorApi = $state<SearchEditorAPI | null>(null)
  let baselineLogCount = $state(0)
  let polledLogCount = $state(0)
  const POLL_INTERVAL_MS = 3000

  // --- state: column resize ---
  const logCols = [
    flex('timestamp', 120, 2),
    flex('severity', 70, 1),
    flex('service', 100, 2),
    flex('body', 120, 5),
  ]

  let activeResizeCol = $state<number | null>(null)
  let logContainerEl = $state<HTMLDivElement | null>(null)
  let colWidths = $state(logCols.map(d => d.min))

  let barPositions = $derived(computeBarPositions(logCols, colWidths))

  $effect(() => {
    if (!logContainerEl) return
    untrack(() => {
      colWidths = computeInitialWidths(logCols, logContainerEl!.clientWidth)
    })
    const ro = new ResizeObserver(entries => {
      const w = entries[0]?.contentRect.width
      if (w && activeResizeCol === null) {
        colWidths = redistributeWidths(logCols, colWidths, w)
      }
    })
    ro.observe(logContainerEl)
    return () => ro.disconnect()
  })

  function handleStartResize(colIndex: number, e: PointerEvent) {
    activeResizeCol = colIndex
    startColumnResize(
      logCols,
      () => colWidths, colIndex, e,
      next => { colWidths = next },
      () => { activeResizeCol = null }
    )
  }

  // --- derived: table rows ---
  let sortedLogs = $derived.by(() => {
    const col = sortColumn
    const dir = sortDirection
    const rows = [...logs]
    rows.sort((a, b) => compareLogs(a, b, col, dir))
    return rows
  })

  let paginatedLogs = $derived.by(() => {
    const start = (currentPage - 1) * rowsPerPage
    const end = start + rowsPerPage
    return sortedLogs.slice(start, end)
  })

  let totalPages = $derived(Math.ceil(sortedLogs.length / rowsPerPage))
  let startRow = $derived(
    sortedLogs.length === 0 ? 0 : (currentPage - 1) * rowsPerPage + 1
  )
  let endRow = $derived(
    Math.min(currentPage * rowsPerPage, sortedLogs.length)
  )

  let hasLogRows = $derived(logs.length > 0)

  let selectedLog = $derived(
    selectedLogId ? sortedLogs.find(l => l.id === selectedLogId) : undefined
  )

  let refreshIndicatorText = $derived.by(() => {
    const delta = polledLogCount - baselineLogCount
    if (delta <= 0) return ''
    return `+${delta} log${delta !== 1 ? 's' : ''}`
  })

  // --- effects ---
  $effect(() => {
    saveLogListTableState({ sortColumn, sortDirection, rowsPerPage })
  })

  $effect(() => {
    const first = paginatedLogs[0]
    if (!selectedLogId || !sortedLogs.some(l => l.id === selectedLogId)) {
      selectedLogId = first?.id ?? null
    }
  })

  $effect(() => {
    const popover = document.getElementById('log-rows-per-page-popover')
    if (popover) {
      const handleToggle = () => {
        rowsPerPagePopoverOpen = popover.matches(':popover-open')
      }
      popover.addEventListener('toggle', handleToggle)
      return () => popover.removeEventListener('toggle', handleToggle)
    }
  })

  $effect(() => {
    const n = sortedLogs.length
    const pages = Math.max(1, Math.ceil(n / rowsPerPage))
    if (n > 0 && currentPage > pages) {
      currentPage = pages
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
  function handleSort(column: LogSortColumn) {
    if (sortColumn === column) {
      sortDirection = sortDirection === 'asc' ? 'desc' : 'asc'
    } else {
      sortColumn = column
      sortDirection = 'asc'
    }
    currentPage = 1
  }

  function handleRowsPerPageChange(newRowsPerPage: number) {
    rowsPerPage = newRowsPerPage
    currentPage = 1
  }

  function goToPage(page: number) {
    if (page >= 1 && page <= totalPages) {
      currentPage = page
    }
  }

  function selectLog(logId: string) {
    selectedLogId = selectedLogId === logId ? null : logId
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
    if (event.signal === 'logs' && event.view === 'list') {
      loading = false
      error = null
      logs = event.results
    }
  }

  async function handleDeleteLog(logId: string) {
    const idx = paginatedLogs.findIndex(l => l.id === logId)
    const nextIdx = idx < paginatedLogs.length - 1 ? idx + 1 : idx - 1
    const nextId = nextIdx >= 0 ? paginatedLogs[nextIdx]?.id ?? null : null
    try {
      await telemetryAPI.deleteLogByID(logId)
      selectedLogId = nextId
      await fetchLogs()
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to delete log'
    }
  }

  async function handleDeleteAllLogs() {
    try {
      await telemetryAPI.clearLogs()
      selectedLogId = null
      currentPage = 1
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

{#snippet toolbarTimeRange()}
  <DateTimeFilter />
{/snippet}

<div
  class="flex min-h-0 min-w-0 w-full flex-1 flex-col gap-[var(--layout-gap)] pt-0"
>
  <div class="page-toolbar-block">
    <SignalToolbar
      signal="logs"
      view="list"
      onRefresh={handleRefresh}
      trailingFilters={[toolbarTimeRange]}
      {searchError}
      {refreshIndicatorText}
    >
      <SearchEditor
        signal="logs"
        view="list"
        inToolbar
        onSearchResults={handleSearchResults}
        onSearchError={err => (searchError = err)}
        onReady={api => (searchEditorApi = api)}
      />
    </SignalToolbar>
  </div>

  {#if error}
    <div class="alert alert-error">
      <span>Error: {error}</span>
    </div>
  {/if}

  <div class="flex min-h-0 flex-1 flex-col gap-[var(--layout-gap)]">
    {#if loading && !hasLogRows}
      <div
        class="rounded-xl border border-base-300/70 bg-base-100/80 px-4 py-12 text-center text-base-content/60 shadow-surface-sm backdrop-blur-sm"
      >
        Loading logs…
      </div>
    {:else if !loading && !hasLogRows}
      <div
        class="rounded-xl border border-base-300/70 bg-base-100/80 px-4 py-12 text-center shadow-surface-sm backdrop-blur-sm"
      >
        <p class="text-base-content/60">No logs in this time range</p>
        <p class="mt-2 text-sm text-base-content/50">
          Send telemetry to the exporter or adjust the time range
        </p>
      </div>
    {:else}
      <div class="min-h-0 flex-1">
        <ResizablePanels
          defaultLeftWidth={0.65}
          minLeftWidth={0.3}
          minRightWidth={0.2}
          storageKey="log-detail-panels"
        >
          {#snippet leftPanel()}
            <div
              class="flex h-full flex-col overflow-hidden rounded-xl border border-base-300/70 bg-base-100/80 shadow-surface-sm backdrop-blur-sm transition-opacity duration-200 {loading
                ? 'opacity-70'
                : 'opacity-100'}"
            >
              <div class="flex-1 min-h-0 overflow-x-auto overflow-y-auto" bind:this={logContainerEl}>
                <div class="col-resize-context">
                <table
                  class="log-list-table table table-fixed table-sm w-full border-collapse"
                  use:tableNav={{
                    rowIdAttr: 'log-id',
                    onSelect: id => { selectedLogId = id },
                    onActivate: id => selectLog(id),
                  }}
                >
                  <colgroup>
                    {#each colWidths as w}
                      <col style:width="{w}px" />
                    {/each}
                  </colgroup>
                  <thead class="sticky top-0 z-10 table-header-surface">
                    <tr class="table-header-row">
                      <th
                        class="table-header-cell table-header-cell--sortable table-header-cell--left group"
                        onclick={() => handleSort('timestamp')}
                        role="button"
                        tabindex="0"
                        onkeydown={e => e.key === 'Enter' && handleSort('timestamp')}
                      >
                        <div class="table-header-sort">
                          <span class="table-header-sort__label">Timestamp</span>
                          <span class="table-header-sort__indicator">
                            <ArrowDownIcon
                              class="sort-indicator {sortColumn === 'timestamp'
                                ? 'sort-indicator--active'
                                : 'sort-indicator--inactive'} {sortColumn ===
                                'timestamp' && sortDirection === 'asc'
                                ? 'sort-indicator--asc'
                                : ''}"
                              aria-hidden="true"
                            />
                          </span>
                        </div>
                      </th>
                      <th
                        class="table-header-cell table-header-cell--sortable table-header-cell--left group"
                        onclick={() => handleSort('severity')}
                        role="button"
                        tabindex="0"
                        onkeydown={e => e.key === 'Enter' && handleSort('severity')}
                      >
                        <div class="table-header-sort">
                          <span class="table-header-sort__label">Severity</span>
                          <span class="table-header-sort__indicator">
                            <ArrowDownIcon
                              class="sort-indicator {sortColumn === 'severity'
                                ? 'sort-indicator--active'
                                : 'sort-indicator--inactive'} {sortColumn ===
                                'severity' && sortDirection === 'asc'
                                ? 'sort-indicator--asc'
                                : ''}"
                              aria-hidden="true"
                            />
                          </span>
                        </div>
                      </th>
                      <th
                        class="table-header-cell table-header-cell--sortable table-header-cell--left group"
                        onclick={() => handleSort('service')}
                        role="button"
                        tabindex="0"
                        onkeydown={e => e.key === 'Enter' && handleSort('service')}
                      >
                        <div class="table-header-sort">
                          <span class="table-header-sort__label">Service</span>
                          <span class="table-header-sort__indicator">
                            <ArrowDownIcon
                              class="sort-indicator {sortColumn === 'service'
                                ? 'sort-indicator--active'
                                : 'sort-indicator--inactive'} {sortColumn ===
                                'service' && sortDirection === 'asc'
                                ? 'sort-indicator--asc'
                                : ''}"
                              aria-hidden="true"
                            />
                          </span>
                        </div>
                      </th>
                      <th class="table-header-cell table-header-cell--left">Body</th>
                    </tr>
                  </thead>
                  <tbody class="table-body-surface">
                    {#each paginatedLogs as log (log.id)}
                      {@const selected = selectedLogId === log.id}
                      {@const service = getServiceName(log.resource) ?? ''}
                      <tr
                        class="log-row cursor-pointer transition-colors border-l-2 {severityBorderClass(log.severityNumber)} {selected ? 'table-row--selected' : 'hover:bg-base-200'}"
                        data-log-id={log.id}
                        onclick={() => selectLog(log.id)}
                        role="button"
                        tabindex="0"
                      >
                        <td
                          class="log-cell truncate text-base-content/80 tabular-nums"
                          title={formatTimestamp(log.timestamp, timeContext.timezone, 'nanoseconds')}
                        >
                          {formatTimestamp(log.timestamp, timeContext.timezone, 'milliseconds')}
                        </td>
                        <td class="log-cell">
                          <span class={severityBadgeClass(log.severityNumber)}>
                            {severityLabel(log.severityText, log.severityNumber)}
                          </span>
                        </td>
                        <td class="log-cell truncate" title={service}>
                          {service || '—'}
                        </td>
                        <td class="log-cell text-base-content/80 truncate" title={log.body}>
                          {log.body}
                          {#if log.bodyType}
                            <span class="badge-type ml-1">{log.bodyType}</span>
                          {/if}
                        </td>
                      </tr>
                    {/each}
                  </tbody>
                </table>
                {#each barPositions as bar}
                  <div
                    class="col-resize-bar col-resize-bar--guide"
                    class:col-resize-bar--active={activeResizeCol === bar.index}
                    style:left="{bar.left}px"
                    role="separator"
                    aria-orientation="vertical"
                    aria-label="Resize {logCols[bar.index].id} column"
                    onpointerdown={e => handleStartResize(bar.index, e)}
                  >
                    <div class="col-resize-bar__line"></div>
                  </div>
                {/each}
                </div>
              </div>

              <!-- Pagination -->
              {#if sortedLogs.length > 0}
                <div class="pagination-controls">
                  <div class="pagination-rows-selector">
                    <span class="pagination-label">Rows per page:</span>
                    <button
                      class="pagination-rows-button"
                      popovertarget="log-rows-per-page-popover"
                      style="anchor-name: --log-rows-per-page-anchor"
                    >
                      <span>{rowsPerPage}</span>
                      <svg
                        class="w-3 h-3 popover-indicator {rowsPerPagePopoverOpen
                          ? 'popover-indicator--open'
                          : ''}"
                        viewBox="0 0 24 24"
                      >
                        <path d="M18 9s-4.419 6-6 6s-6-6-6-6" />
                      </svg>
                    </button>
                  </div>

                  <div class="pagination-controls__center">
                    <div
                      class="flex min-w-0 flex-nowrap items-center justify-center gap-1.5"
                    >
                      <button
                        type="button"
                        class="btn btn-ghost btn-sm btn-circle"
                        disabled={currentPage === 1}
                        onclick={() => goToPage(1)}
                        aria-label="First page"
                      >
                        <ArrowLeftDoubleIcon class="h-4 w-4" aria-hidden="true" />
                      </button>
                      <button
                        type="button"
                        class="btn btn-ghost btn-sm btn-circle"
                        disabled={currentPage === 1}
                        onclick={() => goToPage(currentPage - 1)}
                        aria-label="Previous page"
                      >
                        <ArrowLeftIcon class="h-4 w-4" aria-hidden="true" />
                      </button>
                      <div
                        class="flex min-h-8 min-w-[10rem] items-center justify-center rounded-lg bg-base-200/50 px-3 text-sm tabular-nums text-base-content/70"
                      >
                        {startRow}–{endRow} of {sortedLogs.length} logs
                      </div>
                      <button
                        type="button"
                        class="btn btn-ghost btn-sm btn-circle"
                        disabled={currentPage === totalPages}
                        onclick={() => goToPage(currentPage + 1)}
                        aria-label="Next page"
                      >
                        <ArrowRightIcon class="h-4 w-4" aria-hidden="true" />
                      </button>
                      <button
                        type="button"
                        class="btn btn-ghost btn-sm btn-circle"
                        disabled={currentPage === totalPages}
                        onclick={() => goToPage(totalPages)}
                        aria-label="Last page"
                      >
                        <ArrowRightDoubleIcon class="h-4 w-4" aria-hidden="true" />
                      </button>
                    </div>
                  </div>

                  {#if hasLogRows}
                    <div class="pagination-controls__actions">
                      <button
                        type="button"
                        class="btn btn-ghost btn-sm text-error"
                        onclick={handleDeleteAllLogs}
                        aria-label="Delete all logs"
                      >
                        <TrashIcon class="h-3.5 w-3.5" aria-hidden="true" />
                        Delete all logs
                      </button>
                    </div>
                  {/if}
                </div>
              {/if}
            </div>
          {/snippet}
          {#snippet rightPanel()}
            <div
              class="h-full overflow-hidden rounded-xl border border-base-300/70 bg-base-100/80 shadow-surface-sm backdrop-blur-sm"
            >
              <LogDetailPanel log={selectedLog} onDelete={handleDeleteLog} />
            </div>
          {/snippet}
        </ResizablePanels>
      </div>
    {/if}
  </div>

  <!-- Rows per page popover -->
  <div
    id="log-rows-per-page-popover"
    class="popover dropdown-content log-rows-per-page-popover"
    popover="auto"
  >
    {#each rowsPerPageOptions as option}
      <button
        class="pagination-popover-option {option === rowsPerPage
          ? 'pagination-popover-option--selected'
          : ''}"
        onclick={() => {
          handleRowsPerPageChange(option)
          document.getElementById('log-rows-per-page-popover')?.hidePopover()
        }}
      >
        {#if option === rowsPerPage}
          <svg class="w-4 h-4 text-primary" viewBox="0 0 24 24">
            <path d="m5 14l3.5 3.5L19 6.5" />
          </svg>
        {:else}
          <span class="w-4 h-4"></span>
        {/if}
        <span>{option}</span>
      </button>
    {/each}
  </div>
</div>

<style lang="postcss">
  @reference "../app.css";

  .log-row {
    border-left-width: 2px;
  }

  .log-cell {
    @apply px-4 py-2 align-middle text-sm;
  }

  .log-rows-per-page-popover {
    @apply fixed z-50 px-0 py-1 mx-0 mb-2;
    position-anchor: --log-rows-per-page-anchor;
    bottom: anchor(--log-rows-per-page-anchor top);
    top: auto;
    left: anchor(--log-rows-per-page-anchor left);
    right: auto;
    position-try-fallbacks: flip-block;
    @apply bg-base-100 rounded-md shadow-lg;
    @apply border border-base-300 text-base-content;
    @apply min-w-16;
  }
</style>
