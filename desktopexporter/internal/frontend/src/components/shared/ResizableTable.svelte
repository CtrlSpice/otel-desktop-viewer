<script module lang="ts">
  export type ResizableTableColumn = {
    id: string
    label: string
    min: number
    width: number
    fixed?: boolean
  }
  export type ResizableTableLayout = {
    pinned: Record<string, number | undefined>
    viewportWidth: number
  }
</script>

<script lang="ts">
  import { onDestroy, untrack, type Snippet } from 'svelte'
  import {
    flex,
    fixed,
    fitWidths,
    reconcileWidths,
    resizeBar,
    barPositions,
    startColumnResize,
    type ColumnWidths,
  } from './utils/column-resize'
  import type { DragHandle } from './utils/drag'
  import { resizeRowHover } from './utils/resize-row-hover'

  let {
    columns,
    storageKey,
    caption,
    header,
    children,
    footer,
    pinnedCount = 0,
  }: {
    columns: ResizableTableColumn[]
    storageKey: string
    caption: string
    header: Snippet<[ResizableTableLayout]>
    children: Snippet<[ResizableTableLayout]>
    footer?: Snippet<[ResizableTableLayout]>
    pinnedCount?: number
  } = $props()
  let viewportWidth = $state(0)
  let viewportHeight = $state(0)
  let tableHeight = $state(0)
  let scrollTop = $state(0)
  const handleHeight = $derived(
    Math.min(viewportHeight, Math.max(0, tableHeight - scrollTop))
  )
  let scrollLeft = $state(0)
  let current = $state<ColumnWidths>({})
  let active = $state<string | null>(null)
  let drag: DragHandle | null = null
  const specs = $derived(
    columns.map(column =>
      column.fixed
        ? fixed(column.id, column.width)
        : flex(column.id, column.min, 1)
    )
  )
  const defaults = $derived(
    Object.fromEntries(columns.map(column => [column.id, column.width]))
  )
  const baseWidth = $derived(
    Math.max(
      columns.length > pinnedCount ? viewportWidth : 0,
      columns.reduce(
        (sum, column) => sum + (column.fixed ? column.width : column.min),
        0
      )
    )
  )
  const targetWidth = $derived(
    Math.max(
      baseWidth,
      columns.reduce(
        (sum, column) =>
          sum +
          (column.fixed ? column.width : (current[column.id] ?? column.min)),
        0
      )
    )
  )
  const widths = $derived(
    reconcileWidths(specs, { ...defaults, ...current }, targetWidth)
  )
  const tableWidth = $derived(
    columns.reduce((sum, column) => sum + widths[column.id], 0)
  )
  const pinnedWidth = $derived(
    columns
      .slice(0, pinnedCount)
      .reduce((sum, column) => sum + widths[column.id], 0)
  )
  const widestUnpinned = $derived(
    Math.max(0, ...columns.slice(pinnedCount).map(column => widths[column.id]))
  )
  // Match Logs: leave room for a complete scrolling column on narrow panes.
  const pinColumns = $derived(
    pinnedCount > 0 && pinnedWidth + widestUnpinned <= viewportWidth
  )
  const layout = $derived.by<ResizableTableLayout>(() => {
    let left = 0
    const pinned: Record<string, number> = {}
    if (pinColumns) {
      for (const column of columns.slice(0, pinnedCount)) {
        pinned[column.id] = left
        left += widths[column.id]
      }
    }
    return { pinned, viewportWidth }
  })
  const bars = $derived(
    barPositions(specs, widths, viewportWidth)
      .map(bar => ({
        ...bar,
        position:
          layout.pinned[bar.id] !== undefined
            ? bar.left
            : bar.left - scrollLeft,
      }))
      .filter(
        bar =>
          bar.position > 0 &&
          bar.position < viewportWidth &&
          (layout.pinned[bar.id] !== undefined ||
            bar.position > (pinColumns ? pinnedWidth : 0))
      )
  )

  $effect(() => {
    const key = storageKey
    current = {}
    try {
      const stored: unknown = JSON.parse(localStorage.getItem(key) ?? 'null')
      if (stored && typeof stored === 'object' && !Array.isArray(stored)) {
        current = Object.fromEntries(
          Object.entries(stored).filter(
            ([, value]) =>
              typeof value === 'number' && Number.isFinite(value) && value > 0
          )
        )
      }
    } catch {
      /* Retain defaults when browser storage is unavailable. */
    }
  })

  $effect(() => {
    void specs
    return () => drag?.cancel()
  })

  function save() {
    const saved = { ...current, ...widths }
    try {
      localStorage.setItem(storageKey, JSON.stringify(saved))
    } catch {
      /* Session widths remain usable. */
    }
  }

  function start(id: string, event: PointerEvent) {
    if (event.button !== 0 || active) return
    active = id
    drag = startColumnResize(
      specs,
      widths,
      id,
      event,
      next => {
        current = { ...current, ...next }
      },
      () => {
        active = null
        drag = null
        save()
      },
      viewportWidth
    )
  }

  function keydown(id: string, event: KeyboardEvent) {
    if (!['ArrowLeft', 'ArrowRight', 'Home', 'End'].includes(event.key)) return
    event.preventDefault()
    const delta =
      event.key === 'Home'
        ? -tableWidth
        : event.key === 'End'
          ? tableWidth
          : (event.key === 'ArrowLeft' ? -1 : 1) * (event.shiftKey ? 64 : 16)
    current = {
      ...current,
      ...resizeBar(specs, widths, id, delta, viewportWidth),
    }
    save()
  }

  function reset() {
    current = {
      ...current,
      ...fitWidths(specs, defaults, baseWidth),
    }
    save()
  }

  onDestroy(() => untrack(() => drag?.cancel()))
</script>

<div class="resizable-table">
  {#each bars as bar (bar.id)}
    <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <div
      class="col-resize-bar col-resize-bar--column"
      use:resizeRowHover
      class:col-resize-bar--active={active === bar.id}
      style:left="{bar.position}px"
      style:height="{handleHeight}px"
      role="separator"
      aria-orientation="vertical"
      aria-label={`Resize ${columns.find(column => column.id === bar.id)?.label} column`}
      aria-valuenow={Math.round(bar.left)}
      aria-valuemin={Math.round(bar.min)}
      aria-valuemax={Math.round(bar.max)}
      tabindex="0"
      onpointerdown={event => start(bar.id, event)}
      onkeydown={event => keydown(bar.id, event)}
      ondblclick={reset}
    >
      <div class="col-resize-bar__line"></div>
    </div>
  {/each}
  <!-- Keyboard users must be able to focus and scroll this overflow region. -->
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <div
    class="resizable-table__scroll"
    bind:clientWidth={viewportWidth}
    bind:clientHeight={viewportHeight}
    onscroll={event => {
      scrollLeft = event.currentTarget.scrollLeft
      scrollTop = event.currentTarget.scrollTop
    }}
    tabindex="0"
    role="region"
    aria-label={`${caption} table viewport`}
  >
    <table
      class="table resizable-table__table"
      style:width="{tableWidth}px"
      bind:offsetHeight={tableHeight}
    >
      <caption class="sr-only">{caption}</caption>
      <colgroup
        >{#each columns as column (column.id)}<col
            style:width="{widths[column.id]}px"
          />{/each}</colgroup
      >
      <thead>{@render header(layout)}</thead>
      <tbody>{@render children(layout)}</tbody>
      {#if footer}<tfoot>{@render footer(layout)}</tfoot>{/if}
    </table>
  </div>
</div>

<style lang="postcss">
  @reference "../../app.css";
  .resizable-table {
    @apply relative flex min-h-0 min-w-0 flex-1 overflow-hidden;
  }
  .resizable-table__scroll {
    @apply min-h-0 min-w-0 flex-1 overflow-auto;
  }
  .resizable-table__table {
    --item-idle-bg: var(--color-base-100);
    table-layout: fixed;
  }
  .resizable-table__table thead {
    @apply sticky top-0 z-10 h-10 bg-base-100;
  }
  .resizable-table__table :global(th[scope='col']) {
    @apply font-normal;
    color: var(--color-subtle);
  }
  .resizable-table__table :global(.table-cell) {
    overflow: hidden;
  }
  .resizable-table__table :global(.table-cell--pinned) {
    position: sticky;
    z-index: 2;
    background-color: var(--item-state-bg, var(--color-base-100));
  }

  .resizable-table__table tfoot {
    position: relative;
  }

  .resizable-table__table tfoot:has(> :global(tr))::before {
    content: '';
    position: absolute;
    inset-inline: 0;
    top: 0;
    height: 1px;
    background-color: var(--guide-dot-color);
    opacity: var(--guide-dot-opacity);
    pointer-events: none;
    z-index: 3;
  }
</style>
