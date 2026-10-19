<script lang="ts">
  import { onDestroy, tick, untrack, type Snippet } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { LayoutThreeColumnIcon } from '@hugeicons/core-free-icons'
  import type {
    LogTableRow,
    LogErrorNavigation,
    LogErrorTarget,
  } from '@/types/api-types'
  import PaneHeader from '@/components/shared/PaneHeader.svelte'
  import ErrorNavigationBadge from '@/components/shared/ErrorNavigationBadge.svelte'
  import TableColumnHeading from '@/components/shared/TableColumnHeading.svelte'
  import TableValueCell from '@/components/shared/TableValueCell.svelte'
  import Identifier from '@/components/shared/Identifier.svelte'
  import type { FieldDefinition } from '@/search/model'
  import ValueSyntax from '@/components/shared/ValueSyntax.svelte'
  import { compactValueTokens } from '@/components/shared/detail-value'
  import ScrollToTopButton from '@/components/shared/ScrollToTopButton.svelte'
  import type { DetailFieldTarget } from '@/components/shared/utils/reveal-detail-field'
  import {
    flex,
    fixed,
    fitWidths,
    reconcileWidths,
    resizeBar,
    barPositions,
    startColumnResize,
    type ColumnWidths,
  } from '@/components/shared/utils/column-resize'
  import type { DragHandle } from '@/components/shared/utils/drag'
  import {
    createPopoverID,
    setupAnchorPopover,
  } from '@/components/shared/utils/anchor-popover'
  import {
    keyDeltaFor,
    resolveNextPos,
    escapeForSelector,
  } from '@/components/shared/utils/table-keyboard-nav'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import {
    formatTimestamp,
    formatTimestampParts,
    formatTimezoneLabel,
  } from '@/utils/time'
  import { severityLabel } from './log-severity'
  import LogSeverity from './LogSeverity.svelte'
  import {
    LOG_COLUMN_SOURCES,
    logColumnOptions,
    logColumnLabel,
    logColumnCell,
    type LogColumnOption,
    LOG_SEVERITY_MIN_WIDTH,
    loadLogColumns,
    type LogSortColumn,
    type LogSortDirection,
  } from './log-table'

  let {
    logs,
    total,
    pageNumber = 1,
    selectedID,
    sortColumn,
    sortDirection,
    loading = false,
    onSelect,
    onInspect,
    onSort,
    filters,
    errorNavigation = null,
    errorNavigationError = '',
    errorNavigationLoading = false,
    onSelectError,
    fields = [],
    selectedColumns = $bindable(loadLogColumns()),
    onColumnsChange,
  }: {
    logs: LogTableRow[]
    total: number
    pageNumber?: number
    selectedID: string | null
    sortColumn: LogSortColumn
    sortDirection: LogSortDirection
    loading?: boolean
    onSelect: (id: string, keyboard?: boolean) => void
    onInspect: (id: string, target: DetailFieldTarget) => void
    onSort: (column: LogSortColumn, direction: LogSortDirection) => void
    filters?: Snippet
    errorNavigation?: LogErrorNavigation | null
    errorNavigationError?: string
    errorNavigationLoading?: boolean
    onSelectError: (target: LogErrorTarget) => void
    fields?: FieldDefinition[]
    selectedColumns?: string[]
    onColumnsChange?: () => void
  } = $props()
  const time = getTimeContext()
  let table = $state<HTMLDivElement>()
  $effect.pre(() => {
    if (!errorNavigationLoading) return
    const button = document.activeElement
    if (
      !(button instanceof HTMLButtonElement) ||
      !table?.contains(button) ||
      !button.closest('[aria-label="Error navigation"]')
    )
      return
    // Disabling the focused button blurs it. Restore it only if the user
    // has not moved focus elsewhere while navigation was loading.
    let moved = false
    const onFocus = () => (moved = true)
    document.addEventListener('focusin', onFocus)
    return () => {
      document.removeEventListener('focusin', onFocus)
      void tick().then(() => {
        if (
          !moved &&
          button.isConnected &&
          !button.disabled &&
          document.activeElement === document.body
        )
          button.focus({ preventScroll: true })
      })
    }
  })
  let timestampTimezone = $derived(
    logs.length
      ? [
          ...new Set(
            logs.map(
              log =>
                formatTimestampParts(log.timestamp, time.tz, 'milliseconds')
                  .unit
            )
          ),
        ].join(' / ')
      : formatTimezoneLabel(time.tz)
  )
  let options = $derived(logColumnOptions(fields, selectedColumns))
  let columns = $derived(
    options.filter(column => selectedColumns.includes(column.id))
  )
  let severityWidth = $state(LOG_SEVERITY_MIN_WIDTH)
  let specs = $derived(
    columns.map(column =>
      column.id === 'severity'
        ? fixed(column.id, severityWidth)
        : flex(column.id, column.min, column.id === 'body' ? 4 : 1)
    )
  )
  let viewport = $state<HTMLDivElement>()
  let width = $state(800)
  let scrollLeft = $state(0)
  const storedWidths = loadWidths()
  let resized = Object.keys(storedWidths).length > 0
  let measured = false
  let activeResize = $state<string | null>(null)
  let drag: DragHandle | null = null

  function isStoredWidths(value: unknown): value is object {
    return value !== null && typeof value === 'object' && !Array.isArray(value)
  }

  function isPositiveWidth(value: unknown): value is number {
    return typeof value === 'number' && Number.isFinite(value) && value > 0
  }

  function loadWidths(): ColumnWidths {
    try {
      const value: unknown = JSON.parse(
        localStorage.getItem('logs-column-widths') ?? 'null'
      )
      if (isStoredWidths(value))
        return Object.fromEntries(
          Object.entries(value).filter(([, size]) => isPositiveWidth(size))
        )
    } catch {
      /* Retain default sizes. */
    }
    return {}
  }
  function defaultWidths(size: number) {
    const widths = Object.fromEntries(
      specs.map(column => [column.id, column.min])
    )
    const stretch =
      specs.find(column => column.id === 'body') ??
      specs.findLast(column => column.flex > 0)
    if (stretch)
      widths[stretch.id] += Math.max(
        0,
        size - specs.reduce((sum, column) => sum + column.min, 0)
      )
    return widths
  }
  let widths = $state<ColumnWidths>(untrack(() => defaultWidths(width)))
  let template = $derived(
    columns.map(column => `${widths[column.id]}px`).join(' ')
  )
  let gridWidth = $derived(
    columns.reduce((sum, column) => sum + widths[column.id], 0)
  )
  let bars = $derived(barPositions(specs, widths))
  let pinnedSeverityWidth = $derived(
    columns.some(column => column.id === 'severity') ? widths.severity : 0
  )
  let pinnedLeftWidth = $derived(
    pinnedSeverityWidth +
      (columns.some(column => column.id === 'timestamp') ? widths.timestamp : 0)
  )
  let pinnedBodyWidth = $derived(
    columns.some(column => column.id === 'body') ? widths.body : 0
  )
  let middleWidth = $derived(
    Math.max(
      0,
      ...columns
        .filter(
          column => !['severity', 'timestamp', 'body'].includes(column.id)
        )
        .map(column => widths[column.id])
    )
  )
  let pinColumns = $derived(
    pinnedLeftWidth + pinnedBodyWidth + middleWidth <= width
  )
  let paintedBars = $derived(
    bars.flatMap(bar => {
      const beforeBody =
        columns[columns.findIndex(column => column.id === bar.id) + 1]?.id ===
        'body'
      const left = !pinColumns
        ? bar.left - scrollLeft
        : bar.id === 'timestamp'
          ? pinnedLeftWidth
          : beforeBody
            ? width - pinnedBodyWidth
            : bar.left - scrollLeft
      return left >= (pinColumns ? pinnedLeftWidth : 0) &&
        left <= width - (pinColumns ? pinnedBodyWidth : 0)
        ? [{ ...bar, left }]
        : []
    })
  )

  $effect(() => {
    if (!viewport) return
    const observer = new ResizeObserver(entries => {
      width = entries[0]!.contentRect.width
      if (!measured) {
        widths = reconcileWidths(
          specs,
          { ...defaultWidths(width), ...storedWidths },
          width,
          'body'
        )
        measured = true
        return
      }
      if (!activeResize)
        widths = resized
          ? fitWidths(specs, widths, width, 'body')
          : defaultWidths(width)
    })
    observer.observe(viewport)
    return () => observer.disconnect()
  })
  $effect(() => {
    void logs
    void columns
    if (!viewport) return
    const table = viewport
    let cancelled = false
    void tick().then(() => {
      if (cancelled) return
      let required = LOG_SEVERITY_MIN_WIDTH
      for (const value of table.querySelectorAll<HTMLElement>(
        '.log-severity'
      )) {
        if (value.getBoundingClientRect().width === 0) continue
        const label = value.querySelector<HTMLElement>('.log-severity__label')!
        const circle = value.querySelector<HTMLElement>('.number-circle')!
        const inset = getComputedStyle(value.closest('.table-value-cell')!)
        required = Math.max(
          required,
          Math.ceil(
            label.scrollWidth +
              circle.getBoundingClientRect().width +
              parseFloat(getComputedStyle(value).columnGap) +
              parseFloat(inset.paddingLeft) +
              parseFloat(inset.paddingRight) +
              2
          )
        )
      }
      required = Math.min(required, LOG_SEVERITY_MIN_WIDTH * 2)
      if (required === severityWidth) return
      severityWidth = required
      widths = reconcileWidths(specs, widths, width, 'body')
    })
    return () => {
      cancelled = true
    }
  })
  function saveWidths() {
    resized = true
    try {
      localStorage.setItem(
        'logs-column-widths',
        JSON.stringify({ ...loadWidths(), ...widths })
      )
    } catch {
      /* Keep session sizes. */
    }
  }
  function startResize(id: string, event: PointerEvent) {
    if (event.button !== 0 || activeResize) return
    activeResize = id
    drag = startColumnResize(
      specs,
      widths,
      id,
      event,
      next => (widths = next),
      () => {
        activeResize = null
        drag = null
        saveWidths()
      }
    )
  }
  function resizeKey(id: string, event: KeyboardEvent) {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const delta =
      event.key === 'Home'
        ? -gridWidth
        : event.key === 'End'
          ? gridWidth
          : (event.key === 'ArrowLeft' ? -1 : 1) * (event.shiftKey ? 64 : 16)
    widths = resizeBar(specs, widths, id, delta)
    saveWidths()
  }
  onDestroy(() => drag?.cancel())

  const popoverID = createPopoverID('log-columns')
  let trigger = $state<HTMLButtonElement>()
  let popover = $state<HTMLDivElement>()
  let columnSearch = $state('')
  let pickerOpen = $state(false)
  $effect(() => {
    if (!trigger || !popover) return
    return setupAnchorPopover({
      trigger,
      popover,
      anchor: 'below-end',
      onOpenChange: open => (pickerOpen = open),
    })
  })
  function toggleColumn(id: string, checked: boolean) {
    drag?.cancel()
    const next = options.filter(column =>
      column.id === id ? checked : selectedColumns.includes(column.id)
    )
    selectedColumns = next.map(column => column.id)
    const nextSpecs = next.map(column =>
      column.id === 'severity'
        ? fixed(column.id, severityWidth)
        : flex(column.id, column.min, column.id === 'body' ? 4 : 1)
    )
    widths = reconcileWidths(
      nextSpecs,
      {
        ...Object.fromEntries(nextSpecs.map(column => [column.id, column.min])),
        ...loadWidths(),
        ...widths,
      },
      width,
      'body'
    )
    saveWidths()
    try {
      localStorage.setItem(
        'logs-table-columns',
        JSON.stringify(selectedColumns)
      )
    } catch {
      /* Keep session columns. */
    }
    onColumnsChange?.()
  }

  let scrolled = $state(false)
  let lastSelection: string | null = null
  let lastPage = 1
  $effect(() => {
    if (!viewport || pageNumber === lastPage) return
    lastPage = pageNumber
    viewport.scrollTop = 0
  })
  $effect(() => {
    if (!viewport || !selectedID || selectedID === lastSelection) return
    const index = logs.findIndex(log => log.logRef === selectedID)
    if (index < 0) return
    lastSelection = selectedID
    const id = selectedID
    void tick().then(() => revealRow(id))
  })
  function revealRow(id: string) {
    const row = viewport?.querySelector<HTMLElement>(
      `[data-log-ref="${escapeForSelector(id)}"]`
    )
    if (!viewport || !row) return
    const bounds = viewport.getBoundingClientRect()
    const rect = row.getBoundingClientRect()
    const headerHeight =
      viewport.querySelector('.log-table__header')?.getBoundingClientRect()
        .height ?? 0
    if (rect.top < bounds.top + headerHeight)
      viewport.scrollTop += rect.top - bounds.top - headerHeight
    else if (rect.bottom > bounds.bottom)
      viewport.scrollTop += rect.bottom - bounds.bottom
  }
  async function navigate(event: KeyboardEvent) {
    if (event.ctrlKey || event.metaKey || event.altKey || event.shiftKey) return
    const delta = keyDeltaFor(event.key)
    if (!delta || !logs.length) return
    event.preventDefault()
    const target =
      event.target instanceof Element
        ? event.target.closest<HTMLElement>('[data-log-ref]')
        : null
    const current = logs.findIndex(
      log => log.logRef === (target?.dataset.logRef ?? selectedID)
    )
    const index = resolveNextPos(delta, Math.max(0, current), logs.length - 1)
    const id = logs[index]!.logRef
    onSelect(id, true)
    await tick()
    revealRow(id)
    viewport
      ?.querySelector<HTMLElement>(`[data-log-ref="${escapeForSelector(id)}"]`)
      ?.focus({ preventScroll: true })
  }
  function text(log: LogTableRow, column: LogColumnOption) {
    const id = column.id
    if (column.projection)
      return (
        logColumnCell(log, column)?.text ??
        (loading ? 'Loading…' : 'Unavailable')
      )
    if (id === 'timestamp')
      return formatTimestamp(log.timestamp, time.tz, 'milliseconds')
    if (id === 'severity')
      return `${severityLabel(log.severityText, log.severityNumber)} (${log.severityNumber})`
    if (id === 'service') return log.serviceName || '—'
    if (id === 'name') return log.eventName || '—'
    return log.bodyPreview
  }
</script>

{#snippet columnHeading(column: LogColumnOption)}
  <TableColumnHeading
    label={column.id === 'timestamp'
      ? `${column.key} (${timestampTimezone})`
      : column.key || '(empty key)'}
    source={column.source}
    signal="Log"
  />
  {#if sortColumn === column.sort}<span
      class="log-table__sort-direction"
      aria-hidden="true">{sortDirection === 'asc' ? '↑' : '↓'}</span
    >{/if}
{/snippet}

<div class="log-table" class:log-table--pinned={pinColumns} bind:this={table}>
  <PaneHeader pill mode="title" title="Logs" ariaLabel="Log table summary">
    {#snippet badge()}<span class="badge-count"
        >{total.toLocaleString()} {total === 1 ? 'record' : 'records'}</span
      >
      {#if errorNavigation}<ErrorNavigationBadge
          count={errorNavigation.count}
          busy={errorNavigationLoading}
          index={errorNavigation.index}
          previous={errorNavigation.previous !== null}
          next={errorNavigation.next !== null}
          onPrevious={() => {
            if (errorNavigation?.previous)
              onSelectError(errorNavigation.previous)
          }}
          onNext={() => {
            if (errorNavigation?.next) onSelectError(errorNavigation.next)
          }}
        />{/if}
      {#if errorNavigationError}<span
          class="badge badge-xs badge-error"
          role="status"
          title={errorNavigationError}>Errors unavailable</span
        >{/if}
    {/snippet}
    {#snippet right()}
      {@render filters?.()}
      <button
        bind:this={trigger}
        type="button"
        class="btn btn-xs chart-header-control tooltip tooltip-left"
        aria-label="Columns"
        data-tip="Select columns"
        aria-expanded={pickerOpen}
        popovertarget={popoverID}
      >
        <HugeiconsIcon
          icon={LayoutThreeColumnIcon}
          size={16}
          strokeWidth={1.5}
          aria-hidden="true"
        />
      </button>
    {/snippet}
  </PaneHeader>
  <div
    id={popoverID}
    bind:this={popover}
    popover="auto"
    role="group"
    aria-label="Log columns"
    class="anchor-popover anchor-popover--anchored log-table__columns"
  >
    <input
      class="typed-field typed-field--pill mb-2 w-full"
      type="search"
      aria-label="Find columns"
      placeholder="Find columns…"
      bind:value={columnSearch}
    />
    {#each LOG_COLUMN_SOURCES as source}
      {#each ['field', 'attribute'] as kind}
        {@const group = options.filter(
          column =>
            column.target.source === source &&
            column.target.kind === kind &&
            logColumnLabel(column)
              .toLowerCase()
              .includes(columnSearch.trim().toLowerCase())
        )}
        {#if group.length}
          <fieldset class="min-w-0">
            <legend class="px-2 py-1 text-xs font-medium capitalize"
              >{source} {kind === 'field' ? 'fields' : 'attributes'}</legend
            >
            {#each group as column (column.id)}
              <label
                class="item-state flex items-center gap-2 rounded-lg px-2 py-1.5 text-sm"
              >
                <input
                  type="checkbox"
                  class="checkbox checkbox-xs"
                  aria-label={logColumnLabel(column)}
                  checked={selectedColumns.includes(column.id)}
                  disabled={selectedColumns.length === 1 &&
                    selectedColumns.includes(column.id)}
                  onchange={event =>
                    toggleColumn(column.id, event.currentTarget.checked)}
                />
                <span class="min-w-0 flex-1 break-words"
                  >{column.key || '(empty key)'}</span
                >
              </label>
            {/each}
          </fieldset>
        {/if}
      {/each}
    {/each}
    {#if !options.some(column => logColumnLabel(column)
        .toLowerCase()
        .includes(columnSearch.trim().toLowerCase()))}
      <p class="p-2 text-sm text-base-content/70">No matching columns.</p>
    {/if}
  </div>
  <div class="log-table__frame">
    {#each paintedBars as bar (bar.id)}
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
      <div
        class="col-resize-bar col-resize-bar--guide"
        class:col-resize-bar--active={activeResize === bar.id}
        style:left="{bar.left}px"
        role="separator"
        aria-label={`Resize ${options.find(column => column.id === bar.id)?.key} column`}
        aria-orientation="vertical"
        aria-valuenow={Math.round(widths[bar.id] ?? 0)}
        aria-valuemin={Math.round(
          specs.find(spec => spec.id === bar.id)?.min ?? 0
        )}
        aria-valuemax={Math.round(gridWidth)}
        tabindex="0"
        onpointerdown={event => startResize(bar.id, event)}
        onkeydown={event => resizeKey(bar.id, event)}
        ondblclick={() => {
          widths = defaultWidths(width)
          saveWidths()
        }}
      >
        <div class="col-resize-bar__line"></div>
      </div>
    {/each}
    <div
      class="log-table__scroll"
      bind:this={viewport}
      onscroll={event => {
        scrolled = event.currentTarget.scrollTop > 1
        scrollLeft = event.currentTarget.scrollLeft
      }}
    >
      <div
        class="log-table__grid"
        style:width="{gridWidth}px"
        style:--log-columns={template}
        style:--log-severity-width="{pinnedSeverityWidth}px"
      >
        <div
          class="log-table__grid-body"
          role="grid"
          tabindex="-1"
          aria-label="Logs"
          aria-rowcount={logs.length + 1}
          aria-colcount={columns.length}
          aria-busy={loading}
          onkeydown={navigate}
        >
          <div class="log-table__header" role="row" aria-rowindex={1}>
            {#each columns as column (column.id)}
              {@const sort = column.sort}
              <div
                role="columnheader"
                aria-sort={!column.sort
                  ? undefined
                  : sortColumn === column.sort
                    ? sortDirection === 'asc'
                      ? 'ascending'
                      : 'descending'
                    : 'none'}
                class="log-table__heading"
                class:log-table__pin-left={column.id === 'timestamp'}
                class:log-table__pin-severity={column.id === 'severity'}
                class:log-table__pin-body={column.id === 'body'}
              >
                {#if sort}<button
                    type="button"
                    class="log-table__sort"
                    aria-label={`Sort by ${column.key}`}
                    onclick={() =>
                      onSort(
                        sort,
                        sortColumn === column.sort && sortDirection === 'desc'
                          ? 'asc'
                          : 'desc'
                      )}
                  >
                    {@render columnHeading(column)}
                  </button>{:else}{@render columnHeading(column)}{/if}
              </div>
            {/each}
          </div>
          {#if logs.length}
            {#each logs as log, index (log.logRef)}
              <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
              <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
              <div
                class="log-table__row table-row"
                role="row"
                aria-rowindex={index + 2}
                aria-selected={selectedID === log.logRef}
                data-log-ref={log.logRef}
                tabindex={selectedID === log.logRef ? 0 : -1}
                onclick={event => {
                  if (window.getSelection()?.isCollapsed === false) return
                  onSelect(log.logRef)
                  event.currentTarget.focus({ preventScroll: true })
                }}
                onkeydown={event => {
                  if (
                    event.target === event.currentTarget &&
                    (event.key === 'Enter' || event.key === ' ')
                  ) {
                    event.preventDefault()
                    onSelect(log.logRef)
                  }
                }}
              >
                {#each columns as column (column.id)}
                  {@const cell = column.projection
                    ? logColumnCell(log, column)
                    : null}
                  <div
                    role="gridcell"
                    class="log-table__cell"
                    class:log-table__cell--attribute={column.target.kind ===
                      'attribute'}
                    class:log-table__cell--body={column.id === 'body'}
                    class:log-table__pin-left={column.id === 'timestamp'}
                    class:log-table__pin-severity={column.id === 'severity'}
                    class:log-table__pin-body={column.id === 'body'}
                  >
                    <TableValueCell
                      label={`${column.source.toLowerCase()}${column.projection ? ` ${column.target.kind}` : ''}: ${column.key || '(empty key)'}`}
                      text={text(log, column)}
                      numeric={cell?.numeric}
                      onInspect={() => onInspect(log.logRef, column.target)}
                    >
                      {#snippet children()}{#if column.id === 'severity'}<LogSeverity
                            number={log.severityNumber}
                            text={log.severityText}
                          />{:else if column.id === 'timestamp'}{formatTimestampParts(
                            log.timestamp,
                            time.tz,
                            'milliseconds'
                          )
                            .value}{:else if column.id === 'body' && (log.bodyType === 'map' || log.bodyType === 'array')}<ValueSyntax
                            tokens={compactValueTokens(log.bodyPreview)}
                          />{:else if column.identifier && cell?.present}<Identifier
                            value={cell.text}
                          />{:else}{text(log, column)}{/if}{/snippet}
                    </TableValueCell>
                  </div>
                {/each}
              </div>
            {/each}
          {:else}<div class="log-table__empty" role="status">
              {loading
                ? 'Loading logs…'
                : 'No logs match this search and time range.'}
            </div>{/if}
        </div>
      </div>
    </div>
    <ScrollToTopButton
      visible={scrolled}
      label="Scroll logs to top"
      onclick={() => {
        viewport?.scrollTo({
          top: 0,
          behavior: window.matchMedia('(prefers-reduced-motion: reduce)')
            .matches
            ? 'instant'
            : 'smooth',
        })
      }}
    />
  </div>
</div>

<style lang="postcss">
  @reference "../../app.css";
  .log-table {
    --table-cell-inline-padding: 1rem;
    --table-header-h: 2.5rem;
    @apply flex h-full min-h-0 min-w-0 flex-col;
  }
  .log-table__frame {
    @apply relative isolate flex min-h-0 flex-1 flex-col overflow-hidden;
  }
  .log-table__scroll {
    @apply min-h-0 flex-1 overflow-auto;
    scrollbar-width: thin;
  }
  .log-table__grid {
    min-height: 100%;
  }
  .log-table__header,
  .log-table__row {
    display: grid;
    grid-template-columns: var(--log-columns);
  }
  .log-table__header {
    @apply sticky top-0 z-20 h-10 bg-base-100;
  }
  .log-table__heading {
    @apply relative flex min-w-0 items-center gap-2 py-2 text-left text-sm font-semibold;
    padding-inline: var(--table-cell-inline-padding);
    color: var(--table-heading-color);
  }
  .log-table__sort {
    @apply relative block min-w-0 flex-1 cursor-pointer rounded-sm pr-3 text-left;
  }
  .log-table__sort-direction {
    @apply absolute right-0 top-1/2 -translate-y-1/2 text-xs;
  }
  .log-table__row {
    --table-row-h: 2.5rem;
    @apply h-10 cursor-pointer items-center rounded-xl text-sm;
  }
  .log-table__cell {
    @apply flex min-w-0 self-stretch items-center;
  }
  .log-table__cell--body {
    --table-cell-inline-end: 0.5rem;
  }
  .log-table__cell--attribute {
    @apply text-base-content/60;
  }
  .log-table__cell > :global(.table-value-cell) {
    @apply w-full;
  }
  .log-table--pinned .log-table__pin-left,
  .log-table--pinned .log-table__pin-body,
  .log-table--pinned .log-table__pin-severity {
    @apply sticky z-10 bg-base-100;
    background-image: linear-gradient(
      var(--item-state-bg, transparent),
      var(--item-state-bg, transparent)
    );
  }
  .log-table--pinned .log-table__pin-left {
    left: var(--log-severity-width);
    z-index: 11;
  }
  .log-table--pinned .log-table__pin-body {
    right: 0;
  }
  .log-table--pinned .log-table__pin-severity {
    left: 0;
    z-index: 12;
  }
  .log-table__row > :first-child {
    border-top-left-radius: 0.75rem;
    border-bottom-left-radius: 0.75rem;
  }
  .log-table__row > :last-child {
    border-top-right-radius: 0.75rem;
    border-bottom-right-radius: 0.75rem;
  }
  .log-table__columns {
    @apply w-80 max-w-[calc(100vw-1.5rem)] p-2;
    max-height: min(70vh, 32rem);
    overflow-y: auto;
  }
  .log-table__empty {
    @apply p-8 text-center text-sm;
    color: var(--color-subtle);
  }
</style>
