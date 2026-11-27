<script lang="ts">
  import type {
    WaterfallSpanNode as SpanNode,
    TraceLogSummary,
    TraceColumnOption,
  } from '@/types/api-types'
  import { getServiceName } from '@/utils/resource'
  import ExportButton from '@/components/shared/ExportButton.svelte'
  import { themeSignal } from '@/state/theme.svelte'
  import type { TimelineRecord } from './timeline-markers'
  import { buildWaterfallRows, getTraceBounds } from './waterfall-layout'
  import { onDestroy, tick, untrack } from 'svelte'
  import type { Snippet } from 'svelte'
  import VirtualList from '@humanspeak/svelte-virtual-list'
  import PaneHeader from '@/components/shared/Layout/WorkspaceHeader.svelte'
  import ErrorNavigationBadge from '@/components/shared/ErrorNavigationBadge.svelte'
  import SignalBadges from '@/components/shared/SignalBadges.svelte'
  import ScrollToTopButton from '@/components/shared/ScrollToTopButton.svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { UnfoldMoreIcon, UnfoldLessIcon } from '@hugeicons/core-free-icons'
  import WaterfallTimeAxisHeader, {
    waterfallTimeAxis,
  } from './WaterfallTimeAxisHeader.svelte'
  import WaterfallRow from './WaterfallRow.svelte'
  import StickyAncestorRow from './StickyAncestorRow.svelte'
  import { buildStickyTree, stickyAncestors } from './waterfall-sticky'
  import WaterfallColumnsPicker from './WaterfallColumnsPicker.svelte'
  import {
    columnID,
    columnLabel,
    columnOptions,
    loadColumns,
    columnMinimumWidth,
    saveColumns,
    type WaterfallColumn,
  } from './waterfall-columns'
  import {
    escapeForSelector,
    keyDeltaFor,
    resolveNextPos,
  } from '@/components/shared/Tables/table-keyboard-nav'
  import {
    computeSearchCollapsedParents,
    buildStructuralMaps,
    spanSubtree,
  } from './waterfall-tree'
  import { ancestorIdsOf, keyboardAnchorSpanID } from './waterfall-reveal'
  import {
    collapsedForTrace,
    setCollapsedForTrace,
  } from './waterfall-collapse-store'
  import {
    DURATION_GUTTER_REM,
    OUTSIDE_RECORD_SLOT_REM,
    TIMELINE_LEFT_INSET_REM,
    partitionTraceLogs,
  } from './timeline-markers'
  import { remToPx } from '@/state/panel-width'

  const WATERFALL_ROW_HEIGHT_PX = 40
  const GRID_PAGE_STEP = 8

  /** Walk ancestors via the parent map; true if any ancestor is in the collapsed set. */
  // Iterative with a visited set, because parentSpanID is reported data, not
  // verified structure: a salvaged trace (see cyclePoint) can make two spans
  // each other's ancestor, and this runs for every span on every render --
  // the recursive version overflowed the stack the moment a cycle trace
  // loaded, before any interaction at all.
  function hasCollapsedAncestor(
    id: string,
    parentOf: Map<string, string | null>,
    collapsed: ReadonlySet<string>
  ): boolean {
    const seen = new Set([id])
    let pid = parentOf.get(id) ?? null
    while (pid !== null && !seen.has(pid)) {
      if (collapsed.has(pid)) return true
      seen.add(pid)
      pid = parentOf.get(pid) ?? null
    }
    return false
  }

  function rowVisibilityMap(
    spans: readonly { spanData: { spanID: string } }[],
    parentBySpanID: Map<string, string | null>,
    collapsedParents: ReadonlySet<string>
  ): Map<string, boolean> {
    return new Map(
      spans.map(n => [
        n.spanData.spanID,
        !hasCollapsedAncestor(
          n.spanData.spanID,
          parentBySpanID,
          collapsedParents
        ),
      ])
    )
  }

  type Props = {
    spans: SpanNode[]
    /** Retained trace identity, including when no spans can be placed in the view. */
    traceID?: string
    logs?: TraceLogSummary[]
    columnCatalogue?: TraceColumnOption[]
    onColumnsChange?: (columns: WaterfallColumn[]) => void
    focusedSpanID?: string | null
    selectedSpanID: string | null
    searchActive?: boolean
    onSelectSpan: (spanID: string, focus?: string | null) => void
    onInspectColumn?: (spanID: string, column: WaterfallColumn) => void
    onSelectTimelineRecord: (spanID: string, record: TimelineRecord) => void
    footer?: Snippet
  }

  let {
    spans: traceSpans,
    traceID: providedTraceID,
    logs = [],
    columnCatalogue,
    onColumnsChange,
    selectedSpanID,
    focusedSpanID,
    searchActive = false,
    onSelectSpan,
    onInspectColumn,
    onSelectTimelineRecord,
    footer,
  }: Props = $props()

  let traceID = $derived(
    providedTraceID ?? traceSpans[0]?.spanData.traceID ?? ''
  )
  let subtree = $derived(
    focusedSpanID ? spanSubtree(traceSpans, focusedSpanID) : null
  )
  let spans = $derived(subtree?.spans ?? traceSpans)
  let collapseKey = $derived(
    subtree ? `${traceID}/${subtree.root.spanData.spanID}` : traceID
  )

  async function focusSubtree(id: string | null) {
    const selected = id ?? selectedSpanID ?? subtree?.root.spanData.spanID
    if (selected) {
      onSelectSpan(selected, id)
      await tick()
      await revealAndScrollToSpan(selected, false)
      await focusRowTr(selected)
    }
  }

  let bounds = $derived(getTraceBounds(spans))
  let logPartition = $derived(partitionTraceLogs(logs, traceSpans))
  let rows = $derived(
    buildWaterfallRows(
      spans,
      bounds,
      themeSignal.value,
      logPartition.bySpanID,
      subtree?.root.spanData.spanID
    )
  )
  let rowIndexBySpanID = $derived(
    new Map(rows.map((row, index) => [row.spanNode.spanData.spanID, index + 1]))
  )
  const countDigits = $derived(
    String(
      rows.reduce(
        (maximum, row) => Math.max(maximum, row.tree.childrenCount),
        1
      )
    ).length
  )

  let headerName = $derived.by(() => {
    if (spans.length === 0) return 'Trace'
    const root = spans.find(n => n.depth === 0) ?? spans[0]
    return root?.spanData.name?.trim() || 'Trace'
  })

  let headerService = $derived.by(() => {
    if (spans.length === 0) return ''
    const root = spans.find(n => n.depth === 0) ?? spans[0]
    return root ? (getServiceName(root.spanData.resource) ?? '') : ''
  })

  let errorSpans = $derived(
    spans.filter(node => node.spanData.statusCodeValue === 2)
  )
  let headerErrorCount = $derived(errorSpans.length)
  let selectedErrorIndex = $derived(
    errorSpans.findIndex(node => node.spanData.spanID === selectedSpanID)
  )

  let selectedSpanIndex = $derived(
    selectedSpanID === null
      ? -1
      : spans.findIndex(node => node.spanData.spanID === selectedSpanID)
  )

  let previousErrorSpanID = $derived.by(() => {
    if (selectedSpanIndex < 0) return null
    return (
      spans
        .slice(0, selectedSpanIndex)
        .findLast(node => node.spanData.statusCodeValue === 2)?.spanData
        .spanID ?? null
    )
  })

  let nextErrorSpanID = $derived.by(() => {
    const startIndex = selectedSpanIndex < 0 ? 0 : selectedSpanIndex + 1
    return (
      spans.slice(startIndex).find(node => node.spanData.statusCodeValue === 2)
        ?.spanData.spanID ?? null
    )
  })

  function selectPreviousError() {
    if (previousErrorSpanID) onSelectSpan(previousErrorSpanID)
  }

  function selectNextError() {
    if (nextErrorSpanID) onSelectSpan(nextErrorSpanID)
  }

  import {
    flex,
    fitWidths,
    reconcileWidths,
    resizeBar,
    barPositions as computeBarPositions,
    startColumnResize,
    type ColumnWidths,
  } from '@/components/shared/Tables/column-resize'
  import type { DragHandle } from '@/components/shared/utils/drag'
  import { resizeRowHover } from '@/components/shared/Tables/resize-row-hover'

  let selectedColumns = $state(loadColumns())
  let availableColumns = $derived(
    columnOptions(spans, selectedColumns, columnCatalogue)
  )
  $effect(() => onColumnsChange?.(selectedColumns))
  let wfCols = $derived([
    flex('span', 140, 2),
    ...selectedColumns.map(column =>
      flex(columnID(column), columnMinimumWidth(column), 1)
    ),
    flex('timeline', 240, 4),
  ])

  /* Widths key by column id, never position: the column set
   * is user-configurable, and a stored or derived width must land on
   * the column it belongs to no matter what was added or removed around
   * it. Stored widths from a different column set reconcile on load. */
  const COLUMN_WIDTHS_KEY = 'waterfall-column-widths'
  const COLUMN_RESIZE_STEP_PX = 16
  const COLUMN_RESIZE_LARGE_STEP_PX = 64

  function reconcileColumnWidths(
    containerPx: number,
    stored: ColumnWidths,
    specs = wfCols
  ): ColumnWidths {
    const width = Math.max(576, containerPx)
    const widths = {
      ...Object.fromEntries(specs.map(column => [column.id, column.min])),
      ...stored,
    }
    if (!Number.isFinite(stored.timeline)) {
      widths.timeline = Math.max(
        240,
        width -
          specs
            .filter(column => column.id !== 'timeline')
            .reduce(
              (sum, column) => sum + Math.max(column.min, widths[column.id]),
              0
            )
      )
    }
    return reconcileWidths(specs, widths, width)
  }

  function isStoredWidthsObject(value: unknown): value is object {
    return value !== null && typeof value === 'object'
  }

  function isFiniteWidth(value: unknown): value is number {
    return (
      typeof value === 'number' &&
      Number.isFinite(value) &&
      Math.abs(value) <= Number.MAX_SAFE_INTEGER
    )
  }

  function loadStoredWidths(): ColumnWidths {
    if (typeof localStorage === 'undefined') return {}
    try {
      const parsed: unknown = JSON.parse(
        localStorage.getItem(COLUMN_WIDTHS_KEY) ?? 'null'
      )
      if (!isStoredWidthsObject(parsed)) return {}
      const out: ColumnWidths = {}
      for (const [id, w] of Object.entries(parsed)) {
        if (isFiniteWidth(w)) out[id] = w
      }
      // The fixed service column used this key before configurable columns.
      const serviceID = columnID({ source: 'resource', key: 'service.name' })
      if (out[serviceID] === undefined && out.service !== undefined) {
        out[serviceID] = out.service
      }
      delete out.service
      return out
    } catch {
      return {}
    }
  }

  function saveColumnWidths() {
    if (typeof localStorage === 'undefined') return
    try {
      localStorage.setItem(
        COLUMN_WIDTHS_KEY,
        JSON.stringify({ ...loadStoredWidths(), ...colWidths })
      )
    } catch {
      // A full or blocked store costs the preference, not the drag.
    }
  }

  let activeResizeCol = $state<string | null>(null)
  let colWidths = $state<ColumnWidths>(
    untrack(() => reconcileColumnWidths(800, loadStoredWidths()))
  )
  let colDrag: DragHandle | null = null

  let spanColWidth = $derived(colWidths['span'] ?? 0)
  let optionalColumns = $derived(
    selectedColumns.map(column => ({
      ...column,
      id: columnID(column),
      width: colWidths[columnID(column)] ?? 100,
    }))
  )
  let gridWidth = $derived(
    wfCols.reduce(
      (width, column) => width + (colWidths[column.id] ?? column.min),
      0
    )
  )

  let barPositions = $derived(computeBarPositions(wfCols, colWidths))

  function resizeColumnLabel(id: string): string {
    const selected = selectedColumns.find(column => columnID(column) === id)
    return selected ? columnLabel(selected) : id
  }

  function setColumns(next: WaterfallColumn[]) {
    colDrag?.cancel()
    saveColumnWidths()
    selectedColumns = next
    saveColumns(next)
  }

  function resizeBarPercent(positionPx: number): number {
    if (gridWidth <= 0) return 0
    return Math.round((positionPx / gridWidth) * 100)
  }

  function handleStartResize(barId: string, e: PointerEvent) {
    if (activeResizeCol !== null) return
    activeResizeCol = barId
    colDrag = startColumnResize(
      wfCols,
      colWidths,
      barId,
      e,
      next => {
        colWidths = next
      },
      () => {
        activeResizeCol = null
        colDrag = null
        saveColumnWidths()
      }
    )
  }

  function handleResizeKeydown(barId: string, e: KeyboardEvent) {
    if (e.key === 'Home') {
      e.preventDefault()
      const containerPx = scrollContainerEl?.clientWidth || scrollContainerW
      colWidths = reconcileColumnWidths(containerPx, {})
      saveColumnWidths()
      return
    }

    if (e.key !== 'ArrowLeft' && e.key !== 'ArrowRight') return
    e.preventDefault()

    const amount = e.shiftKey
      ? COLUMN_RESIZE_LARGE_STEP_PX
      : COLUMN_RESIZE_STEP_PX
    const next = resizeBar(
      wfCols,
      colWidths,
      barId,
      e.key === 'ArrowLeft' ? -amount : amount
    )
    if (next === colWidths) return
    colWidths = next
    saveColumnWidths()
  }

  onDestroy(() => colDrag?.cancel())

  let scrollContainerEl = $state<HTMLDivElement | null>(null)
  let scrollContainerW = $state(800)

  $effect(() => {
    const container = scrollContainerEl
    if (!container) return
    untrack(() => {
      colWidths = reconcileColumnWidths(
        container.clientWidth,
        loadStoredWidths()
      )
    })
    const ro = new ResizeObserver(entries => {
      const w = entries[0]?.contentRect.width ?? 800
      scrollContainerW = w
      queueStickyUpdate()
      if (activeResizeCol === null) {
        colWidths = fitWidths(wfCols, colWidths, Math.max(576, w))
      }
    })
    ro.observe(container)
    return () => ro.disconnect()
  })

  // A changed column set keeps surviving widths attached to their identities.
  $effect(() => {
    const specs = wfCols
    untrack(() => {
      colDrag?.cancel()
      colWidths = reconcileColumnWidths(
        scrollContainerEl?.clientWidth || scrollContainerW,
        { ...loadStoredWidths(), ...colWidths },
        specs
      )
    })
  })

  const TICK_LABEL_SLOT_PX = 96
  const TICK_COUNT_STEPS = [1, 2, 4, 6]

  let timelineColPx = $derived(colWidths['timeline'] ?? 0)
  let timelinePlotPx = $derived(
    Math.max(
      0,
      timelineColPx -
        remToPx(TIMELINE_LEFT_INSET_REM) -
        remToPx(DURATION_GUTTER_REM) -
        remToPx(OUTSIDE_RECORD_SLOT_REM)
    )
  )
  let targetTickCount = $derived.by(() => {
    // Nice intervals can produce more ticks than requested. Check their actual
    // spacing inside the bar area, excluding label and activity-marker gutters.
    return (
      TICK_COUNT_STEPS.findLast(count => {
        const { ticks } = waterfallTimeAxis(bounds.duration, count)
        return (
          ticks.length < 2 ||
          ((ticks[1].offsetPercent - ticks[0].offsetPercent) * timelinePlotPx) /
            100 >=
            TICK_LABEL_SLOT_PX
        )
      }) ?? TICK_COUNT_STEPS[0]
    )
  })

  const timeAxis = $derived(waterfallTimeAxis(bounds.duration, targetTickCount))

  let matchedIDs = $derived(
    new Set(spans.filter(n => n.matched).map(n => n.spanData.spanID))
  )

  // Structural, not parentSpanID-based: collapse, visibility and reveal all
  // operate on the tree as rendered, so orphans and salvaged cycle entries at
  // depth 0 behave as roots instead of being hidden by their own "children".
  let structuralMaps = $derived(buildStructuralMaps(spans))
  let parentBySpanID = $derived(structuralMaps.parentBySpanID)

  function computeAncestorsOfMatched(
    matched: Set<string>,
    parentOf: Map<string, string | null>
  ): Set<string> {
    const ancestors = new Set<string>()
    for (const id of matched) {
      let pid = parentOf.get(id) ?? null
      while (pid !== null && !ancestors.has(pid)) {
        ancestors.add(pid)
        pid = parentOf.get(pid) ?? null
      }
    }
    return ancestors
  }

  let ancestorsOfMatched = $derived(
    computeAncestorsOfMatched(matchedIDs, parentBySpanID)
  )

  /**
   * Span IDs collapsed by row toggles, keyboard arrows, or collapse-all.
   * Refetches, searches, selections, and resizes must not write this state.
   */
  let userCollapsed = $derived(collapsedForTrace(collapseKey))

  // Search uses a response-scoped overlay so clearing it restores user state.
  let searchCollapsedParents = $derived.by((): Set<string> | null =>
    searchActive
      ? computeSearchCollapsedParents(
          spans,
          matchedIDs,
          ancestorsOfMatched,
          childrenBySpanID
        )
      : null
  )
  let searchOverrides = $state(new Map<string, boolean>())
  $effect(() => {
    // A new response has a new match set and therefore a new overlay.
    void spans
    searchOverrides = new Map()
  })

  let effectiveCollapsed = $derived.by((): ReadonlySet<string> => {
    if (!searchCollapsedParents) return userCollapsed
    const out = new Set(searchCollapsedParents)
    for (const [id, collapsed] of searchOverrides) {
      if (collapsed) out.add(id)
      else out.delete(id)
    }
    return out
  })

  let rowVisibilityBySpanID = $derived(
    rowVisibilityMap(spans, parentBySpanID, effectiveCollapsed)
  )

  async function toggleCollapse(spanID: string) {
    cancelExpansionScroll()
    const expanding = effectiveCollapsed.has(spanID)
    const toggledTraceID = traceID
    if (searchCollapsedParents) {
      const next = new Map(searchOverrides)
      next.set(spanID, !effectiveCollapsed.has(spanID))
      searchOverrides = next
    } else {
      const next = new Set(userCollapsed)
      if (!next.delete(spanID)) next.add(spanID)
      setCollapsedForTrace(collapseKey, next)
    }
    await clampScroll()
    if (
      expanding &&
      traceID === toggledTraceID &&
      !effectiveCollapsed.has(spanID)
    ) {
      activeScroll = centerExpandedBranch(spanID)
      await activeScroll
    }
  }

  /** Every parent that has children, for collapse-all. */
  let collapsibleSpanIDs = $derived.by(() => {
    const out: string[] = []
    for (const node of spans) {
      const id = node.spanData.spanID
      if ((childrenBySpanID.get(id)?.length ?? 0) > 0) out.push(id)
    }
    return out
  })

  // Separate idempotent verbs support mixed collapse states.
  function setAll(collapsed: boolean) {
    cancelExpansionScroll()
    if (searchCollapsedParents) {
      const next = new Map<string, boolean>()
      for (const id of collapsibleSpanIDs) next.set(id, collapsed)
      searchOverrides = next
    } else {
      setCollapsedForTrace(
        collapseKey,
        collapsed ? new Set(collapsibleSpanIDs) : new Set()
      )
    }
    void clampScroll()
  }

  let childrenBySpanID = $derived(structuralMaps.childrenBySpanID)

  let visibleRows = $derived.by(() =>
    rows.filter(
      row => rowVisibilityBySpanID.get(row.spanNode.spanData.spanID) ?? true
    )
  )

  let rowBySpanID = $derived.by(
    () => new Map(rows.map(row => [row.spanNode.spanData.spanID, row]))
  )

  const stickyTree = $derived(
    buildStickyTree(visibleRows.map(row => row.spanNode))
  )
  let treeScrollTop = $state(0)
  let stickyRowHeight = $state(WATERFALL_ROW_HEIGHT_PX)
  let stickyViewportWidth = $state(0)
  let stickyMaxHeight = $state(WATERFALL_ROW_HEIGHT_PX)
  const pinnedAncestors = $derived(
    stickyAncestors(stickyTree, treeScrollTop, stickyRowHeight, stickyMaxHeight)
  )
  let stickyFrame = 0

  function queueStickyUpdate() {
    if (stickyFrame) return
    stickyFrame = requestAnimationFrame(() => {
      stickyFrame = 0
      const viewport = gridHostEl?.querySelector<HTMLElement>(
        '.waterfall-vlist-viewport'
      )
      if (!viewport) return
      const rowBounds = viewport
        .querySelector('tr[data-span-id]')
        ?.getBoundingClientRect()
      const height = rowBounds?.height || WATERFALL_ROW_HEIGHT_PX
      const maxHeight = Math.max(height, viewport.clientHeight - height)
      const active =
        document.activeElement?.closest<HTMLElement>('[data-sticky-span]')
      if (
        active &&
        gridHostEl?.contains(active) &&
        !stickyAncestors(
          stickyTree,
          viewport.scrollTop,
          height,
          maxHeight
        ).some(
          ancestor =>
            visibleRows[ancestor.index]?.spanNode.spanData.spanID ===
            active.dataset.stickySpan
        )
      ) {
        const row = viewport.querySelector<HTMLElement>(
          `tr[data-span-id="${escapeForSelector(active.dataset.stickySpan!)}"]`
        )
        const bounds = row?.getBoundingClientRect()
        const view = viewport.getBoundingClientRect()
        if (
          row &&
          bounds &&
          bounds.top >= view.top &&
          bounds.bottom <= view.bottom
        )
          row.focus({ preventScroll: true })
        else viewport.focus({ preventScroll: true })
      }
      stickyRowHeight = height
      stickyViewportWidth =
        rowBounds?.width || viewport.clientWidth || gridWidth
      stickyMaxHeight = maxHeight
      treeScrollTop = viewport.scrollTop
    })
  }

  $effect(() => {
    if (gridHostEl && visibleRows.length) queueStickyUpdate()
  })

  async function goToAncestor(id: string) {
    const key = collapseKey
    lastScrolledSelection = id
    gridHostEl
      ?.querySelector<HTMLElement>('.waterfall-vlist-viewport')
      ?.focus({ preventScroll: true })
    onSelectSpan(id)
    await revealAndScrollToSpan(id, false)
    if (key === collapseKey) await focusRowTr(id)
  }

  let keyboardAnchorID = $derived(
    keyboardAnchorSpanID(
      selectedSpanID,
      visibleRows.map(row => row.spanNode.spanData.spanID),
      parentBySpanID
    )
  )

  type VirtualListRef = {
    scroll: (options: {
      index: number
      smoothScroll?: boolean
      shouldThrowOnBounds?: boolean
      align?: 'auto' | 'top' | 'bottom' | 'nearest' | 'center'
    }) => Promise<void>
  }

  let vlistRef = $state<VirtualListRef | null>(null)
  let lastScrolledSelection: string | null = null
  let activeScroll: Promise<void> = Promise.resolve()
  let cancelExpansionScroll = () => {}
  onDestroy(() => {
    cancelExpansionScroll()
    if (stickyFrame) cancelAnimationFrame(stickyFrame)
  })
  let treeScrolled = $state(false)

  function centerExpandedBranch(spanID: string): Promise<void> {
    const viewport = scrollContainerEl?.querySelector<HTMLElement>(
      '.waterfall-vlist-viewport'
    )
    const row = viewport?.querySelector<HTMLTableRowElement>(
      `tr[data-span-id="${escapeForSelector(spanID)}"]`
    )
    if (!viewport || !row) return Promise.resolve()

    const rowBounds = row.getBoundingClientRect()
    const viewportBounds = viewport.getBoundingClientRect()
    const start = viewport.scrollTop
    const target = Math.max(
      0,
      Math.min(
        viewport.scrollHeight - viewport.clientHeight,
        start +
          rowBounds.top +
          rowBounds.height / 2 -
          viewportBounds.top -
          viewport.clientHeight / 2
      )
    )
    if (window.matchMedia('(prefers-reduced-motion: reduce)').matches) {
      viewport.scrollTop = target
      return Promise.resolve()
    }

    // Scroll the measured row directly: the virtual-list index API can snap
    // to its estimated destination while a native smooth scroll is in flight.
    return new Promise(resolve => {
      const listeners = new AbortController()
      let frame = 0
      const finish = () => {
        cancelAnimationFrame(frame)
        listeners.abort()
        cancelExpansionScroll = () => {}
        resolve()
      }
      cancelExpansionScroll = finish
      for (const event of ['wheel', 'touchstart', 'pointerdown', 'keydown']) {
        viewport.addEventListener(event, finish, {
          passive: true,
          signal: listeners.signal,
        })
      }
      const started = performance.now()
      const step = (now: number) => {
        const progress = Math.min(1, (now - started) / 180)
        const eased = progress * progress * (3 - 2 * progress)
        viewport.scrollTop = start + (target - start) * eased
        if (progress < 1) frame = requestAnimationFrame(step)
        else finish()
      }
      frame = requestAnimationFrame(step)
    })
  }

  function handleTreeScroll(event: Event) {
    const viewport = event.target
    if (
      viewport instanceof HTMLElement &&
      viewport.classList.contains('waterfall-vlist-viewport')
    ) {
      treeScrolled = viewport.scrollTop > 1
      queueStickyUpdate()
    }
  }

  function scrollTreeToTop() {
    cancelExpansionScroll()
    void vlistRef?.scroll({
      index: 0,
      align: 'top',
      smoothScroll: !window.matchMedia('(prefers-reduced-motion: reduce)')
        .matches,
      shouldThrowOnBounds: false,
    })
  }

  function visibleRowIndex(spanID: string): number {
    return visibleRows.findIndex(row => row.spanNode.spanData.spanID === spanID)
  }

  // Preserve collapsed branches; scroll hidden selections to a visible ancestor.
  async function revealAndScrollToSpan(
    spanID: string,
    smoothScroll = true
  ): Promise<void> {
    cancelExpansionScroll()
    let idx = visibleRowIndex(spanID)
    if (idx < 0) {
      for (const aid of ancestorIdsOf(spanID, parentBySpanID)) {
        idx = visibleRowIndex(aid)
        if (idx >= 0) break
      }
    }
    if (idx < 0 || !vlistRef) return
    activeScroll = vlistRef.scroll({
      index: idx,
      align: 'nearest',
      smoothScroll,
      shouldThrowOnBounds: false,
    })
    await activeScroll
    await tick()
    const viewport = gridHostEl?.querySelector<HTMLElement>(
      '.waterfall-vlist-viewport'
    )
    const row = viewport?.querySelector<HTMLElement>(
      `tr[data-span-id="${escapeForSelector(spanID)}"]`
    )
    if (viewport && row) {
      const padding = Math.min(
        ancestorIdsOf(spanID, parentBySpanID).length * stickyRowHeight,
        stickyMaxHeight
      )
      const top =
        row.getBoundingClientRect().top - viewport.getBoundingClientRect().top
      if (top < padding)
        viewport.scrollTop = Math.max(0, viewport.scrollTop + top - padding)
      queueStickyUpdate()
    }
  }

  $effect(() => {
    const id = selectedSpanID
    if (!vlistRef || !id) return
    if (id === lastScrolledSelection) return
    lastScrolledSelection = id
    void revealAndScrollToSpan(id)
  })

  $effect(() => {
    if (!selectedSpanID) lastScrolledSelection = null
  })

  let gridHostEl = $state<HTMLDivElement | null>(null)

  $effect(() => {
    const grid = gridHostEl
    const rowCount = rows.length
    const columnCount = 2 + optionalColumns.length
    if (!grid) return

    // The package owns the scroll viewport, so apply the treegrid semantics to
    // that element after mount. Generic wrappers beneath it stay transparent
    // to the accessibility tree and the virtual rows have a valid parent.
    void tick().then(() => {
      if (gridHostEl !== grid) return
      const viewport = grid.querySelector<HTMLElement>(
        '.waterfall-vlist-viewport'
      )
      if (!viewport) return
      treeScrolled = viewport.scrollTop > 1
      queueStickyUpdate()
      viewport.role = 'treegrid'
      viewport.setAttribute('aria-label', 'Span waterfall')
      viewport.setAttribute('aria-rowcount', String(rowCount))
      viewport.setAttribute('aria-colcount', String(columnCount))
      // Rows own keyboard entry; the viewport would otherwise add a redundant
      // Tab stop immediately before the roving row.
      viewport.tabIndex = -1
    })
  })

  async function focusRowTr(spanID: string) {
    await tick()
    await activeScroll
    await tick()
    const safe = escapeForSelector(spanID)
    scrollContainerEl
      ?.querySelector<HTMLTableRowElement>(`tr[data-span-id="${safe}"]`)
      ?.focus()
  }

  function shouldHandleGridKey(el: Element | null): boolean {
    if (!el || !gridHostEl?.contains(el)) return false
    if (el.closest('input, textarea, select, [contenteditable="true"]'))
      return false
    if (el.closest('button')) return false
    return true
  }

  function navigateVisibleRow(nextIdx: number) {
    const row = visibleRows[nextIdx]
    if (!row) return
    const id = row.spanNode.spanData.spanID
    onSelectSpan(id)
    void focusRowTr(id)
  }

  function handleTreeKeys(e: KeyboardEvent, currentID: string | null): boolean {
    if (!currentID) return false

    const row = rowBySpanID.get(currentID)
    const hasChildren = (row?.tree.childrenCount ?? 0) > 0

    if (e.key === 'ArrowRight' || e.key === 'l') {
      if (hasChildren && effectiveCollapsed.has(currentID)) {
        toggleCollapse(currentID)
        e.preventDefault()
      }
      return true
    }

    if (e.key === 'ArrowLeft' || e.key === 'h') {
      if (hasChildren && !effectiveCollapsed.has(currentID)) {
        toggleCollapse(currentID)
      } else {
        const parentID = parentBySpanID.get(currentID) ?? null
        if (parentID) {
          onSelectSpan(parentID)
          void focusRowTr(parentID)
        }
      }
      e.preventDefault()
      return true
    }

    return false
  }

  function handleGridKeydown(e: KeyboardEvent) {
    const target = e.target
    if (!(target instanceof Element) || !shouldHandleGridKey(target)) return
    if (visibleRows.length === 0) return
    if (
      e.altKey ||
      e.metaKey ||
      (e.ctrlKey && e.key !== 'Home' && e.key !== 'End')
    )
      return

    const focused = document.activeElement
    const focusedID =
      focused
        ?.closest<HTMLTableRowElement>('tr[data-span-id]')
        ?.getAttribute('data-span-id') ?? null

    if (handleTreeKeys(e, focusedID ?? selectedSpanID)) return

    if (e.key === 'Enter' || e.key === ' ') {
      const id = focusedID ?? selectedSpanID
      if (id) {
        e.preventDefault()
        onSelectSpan(id)
        void focusRowTr(id)
      }
      return
    }

    // Vim-unimpaired's bracket idiom: [ and ] page through status
    // errors, the keyboard twin of the chevrons on the error badge.
    // Same anchor as the badge (the selection), same nearest-directional
    // behaviour, same refusal to wrap.
    if (e.key === '[' || e.key === ']') {
      const id = e.key === ']' ? nextErrorSpanID : previousErrorSpanID
      if (id) {
        e.preventDefault()
        onSelectSpan(id)
        void focusRowTr(id)
      }
      return
    }

    const delta = keyDeltaFor(e.key, GRID_PAGE_STEP)
    if (!delta) return

    e.preventDefault()

    const currentIdx =
      focusedID != null
        ? visibleRowIndex(focusedID)
        : selectedSpanID
          ? visibleRowIndex(selectedSpanID)
          : -1

    if (currentIdx < 0) {
      navigateVisibleRow(0)
      return
    }

    const nextIdx = resolveNextPos(delta, currentIdx, visibleRows.length - 1)
    if (nextIdx === currentIdx) return
    navigateVisibleRow(nextIdx)
  }

  async function clampScroll() {
    await tick()
    const viewport = scrollContainerEl?.querySelector<HTMLElement>(
      '.waterfall-vlist-viewport'
    )
    if (!viewport) return
    const max = viewport.scrollHeight - viewport.clientHeight
    if (viewport.scrollTop > max) viewport.scrollTop = max
  }
</script>

<div
  class="waterfall-view"
  data-unmatched-log-count={logPartition.unmatched.length}
  style:--tree-count-width={`max(var(--waterfall-gutter-col), calc(${countDigits + 2}ch + 0.5rem))`}
>
  <PaneHeader
    pill
    mode="title"
    title={headerName}
    subtitleService={headerService || undefined}
    ariaLabel="Trace waterfall"
  >
    {#snippet badge()}
      <SignalBadges signal="trace" spanCount={spans.length} errorCount={0} />
    {/snippet}
    {#snippet metaCenter()}
      {#if errorSpans.length > 0}
        <ErrorNavigationBadge
          count={headerErrorCount}
          index={selectedErrorIndex >= 0 ? selectedErrorIndex + 1 : null}
          previous={previousErrorSpanID !== null}
          next={nextErrorSpanID !== null}
          onPrevious={selectPreviousError}
          onNext={selectNextError}
        />
      {/if}
    {/snippet}
    {#snippet metaRight()}
      <div
        class="header-icon-controls"
        role="group"
        aria-label="Span disclosure"
      >
        <button
          type="button"
          class="btn btn-xs chart-header-control tooltip tooltip-left"
          onclick={() => setAll(true)}
          disabled={collapsibleSpanIDs.length === 0}
          aria-label="Collapse all spans"
          data-tip="Collapse all spans"
        >
          <HugeiconsIcon
            icon={UnfoldLessIcon}
            size="1em"
            strokeWidth={1.5}
            class="h-4 w-4"
            aria-hidden="true"
          />
        </button>
        <button
          type="button"
          class="btn btn-xs chart-header-control tooltip tooltip-left"
          onclick={() => setAll(false)}
          disabled={collapsibleSpanIDs.length === 0}
          aria-label="Expand all spans"
          data-tip="Expand all spans"
        >
          <HugeiconsIcon
            icon={UnfoldMoreIcon}
            size="1em"
            strokeWidth={1.5}
            class="h-4 w-4"
            aria-hidden="true"
          />
        </button>
      </div>
      <WaterfallColumnsPicker
        options={availableColumns}
        selected={selectedColumns}
        onchange={setColumns}
      />
      {#key traceID}
        <ExportButton signal="trace" id={traceID} />
      {/key}
    {/snippet}
  </PaneHeader>
  {#if subtree}
    <nav
      class="flex shrink-0 flex-wrap items-center gap-1 px-3 py-1 text-sm"
      aria-label="Focused span ancestry"
    >
      <button
        type="button"
        class="btn btn-ghost btn-xs"
        onclick={() => focusSubtree(null)}
      >
        Full trace
      </button>
      {#each subtree.ancestors as node (node.spanData.spanID)}
        <span aria-hidden="true">›</span>
        <button
          type="button"
          class="btn btn-ghost btn-xs"
          onclick={() => focusSubtree(node.spanData.spanID)}
        >
          {node.spanData.name}
        </button>
      {/each}
      <span aria-hidden="true">›</span>
      <span aria-current="location">{subtree.root.spanData.name}</span>
    </nav>
  {/if}
  <div class="waterfall-view__tree">
    <div
      class="waterfall-view__scroll"
      bind:this={scrollContainerEl}
      onscrollcapture={handleTreeScroll}
    >
      <div
        class="col-resize-context waterfall-view__grid-host"
        style:width="{gridWidth}px"
      >
        {#each barPositions as bar (bar.id)}
          <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
          <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
          <div
            class="col-resize-bar col-resize-bar--column"
            use:resizeRowHover
            class:col-resize-bar--active={activeResizeCol === bar.id}
            style:left="{bar.left}px"
            role="separator"
            aria-orientation="vertical"
            aria-label="Resize {resizeColumnLabel(bar.id)} column"
            aria-valuenow={resizeBarPercent(bar.left)}
            aria-valuemin={resizeBarPercent(bar.min)}
            aria-valuemax={resizeBarPercent(bar.max)}
            aria-valuetext="{resizeBarPercent(bar.left)} percent from left"
            tabindex="0"
            onpointerdown={e => handleStartResize(bar.id, e)}
            onkeydown={e => handleResizeKeydown(bar.id, e)}
          >
            <div class="col-resize-bar__line"></div>
          </div>
        {/each}
        <table
          class="split-table waterfall-view__header-table table table-sm w-full min-w-[36rem] border-collapse"
        >
          <thead class="waterfall-view__thead">
            <WaterfallTimeAxisHeader
              traceDurationNs={bounds.duration}
              {targetTickCount}
              tickLabelWidth={TICK_LABEL_SLOT_PX}
              {spanColWidth}
              {optionalColumns}
            />
          </thead>
        </table>
        <!-- Keyboard events bubble from the runtime treegrid; this host stays semantic. -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          bind:this={gridHostEl}
          class="waterfall-view__vlist-host"
          onkeydown={handleGridKeydown}
        >
          {#if pinnedAncestors.length}
            <nav
              class="waterfall-view__sticky"
              aria-label="Span ancestors"
              style:width="{stickyViewportWidth}px"
              style:max-height="{stickyMaxHeight}px"
              style:--sticky-row-height="{stickyRowHeight}px"
            >
              <ol>
                {#each pinnedAncestors as ancestor (visibleRows[ancestor.index].spanNode.spanData.spanID)}
                  {@const row = visibleRows[ancestor.index]}
                  {@const sid = row.spanNode.spanData.spanID}
                  <li>
                    <StickyAncestorRow
                      {row}
                      selected={sid === selectedSpanID}
                      {spanColWidth}
                      {optionalColumns}
                      {timeAxis}
                      offset={ancestor.offset}
                      onSelect={() => void goToAncestor(sid)}
                    />
                  </li>
                {/each}
              </ol>
            </nav>
          {/if}
          <VirtualList
            bind:this={vlistRef}
            items={visibleRows}
            defaultEstimatedItemHeight={WATERFALL_ROW_HEIGHT_PX}
            bufferSize={12}
            itemKey={row => row.spanNode.spanData.spanID}
            containerClass="waterfall-vlist"
            viewportClass="waterfall-vlist-viewport"
            viewportLabel="Span waterfall rows"
            itemsClass="waterfall-vlist-items"
            onRangeChange={queueStickyUpdate}
          >
            {#snippet renderItem(row, index)}
              {@const sid = row.spanNode.spanData.spanID}
              <WaterfallRow
                {row}
                selected={sid === selectedSpanID}
                tabbable={sid === keyboardAnchorID}
                visible={true}
                subtreeCollapsed={effectiveCollapsed.has(sid)}
                rowIndex={rowIndexBySpanID.get(sid) ?? index + 1}
                matched={searchActive && matchedIDs.has(sid)}
                matchBefore={searchActive &&
                  !!visibleRows[index - 1] &&
                  matchedIDs.has(
                    visibleRows[index - 1]!.spanNode.spanData.spanID
                  )}
                matchAfter={searchActive &&
                  !!visibleRows[index + 1] &&
                  matchedIDs.has(
                    visibleRows[index + 1]!.spanNode.spanData.spanID
                  )}
                {spanColWidth}
                {optionalColumns}
                timelineWidth={timelinePlotPx}
                {timeAxis}
                traceStart={bounds.start}
                traceEnd={bounds.end}
                onRowClick={() => {
                  onSelectSpan(sid)
                  void focusRowTr(sid)
                }}
                onFocusSubtree={row.tree.childrenCount > 0 &&
                sid !== subtree?.root.spanData.spanID
                  ? () => focusSubtree(sid)
                  : undefined}
                onToggleExpand={() => toggleCollapse(sid)}
                onInspectColumn={column => {
                  if (onInspectColumn) onInspectColumn(sid, column)
                  else onSelectSpan(sid)
                }}
                onSelectTimelineRecord={record =>
                  onSelectTimelineRecord(sid, record)}
              />
            {/snippet}
          </VirtualList>
        </div>
      </div>
    </div>
    <ScrollToTopButton
      visible={treeScrolled}
      label="Scroll span tree to top"
      onclick={scrollTreeToTop}
    />
  </div>
  {#if footer}
    {@render footer()}
  {/if}
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .waterfall-view {
    --table-cell-inline-padding: 1rem;
    @apply flex h-full min-h-0 min-w-0 flex-col overflow-hidden bg-base-100;
  }

  .waterfall-view__scroll {
    @apply flex min-h-0 flex-1 flex-col overflow-x-auto;
  }

  .waterfall-view__tree {
    @apply relative flex min-h-0 flex-1 flex-col;
  }

  /* Column labels remain on the data canvas and can grow with provenance. */
  .waterfall-view__thead {
    box-sizing: border-box;
    min-height: var(--table-header-h);
    height: auto;
    background-color: transparent;
    border-radius: 0;
  }

  .waterfall-view__thead :global(tr),
  .waterfall-view__thead :global(th) {
    border-radius: 0;
  }

  .waterfall-view__scroll > :global(.col-resize-context) {
    @apply flex min-h-0 flex-1 flex-col;
  }

  .waterfall-view__grid-host {
    @apply flex min-h-0 flex-1 flex-col;
  }

  .waterfall-view__header-table {
    @apply shrink-0;
  }

  .waterfall-view__vlist-host {
    @apply relative min-h-0 flex-1 overflow-hidden outline-none;
  }

  .waterfall-view__sticky {
    @apply absolute top-0 left-0 z-20 overflow-x-hidden overflow-y-auto;
    pointer-events: none;
  }
  .waterfall-view__sticky ol {
    @apply m-0 list-none p-0;
  }
  .waterfall-view__sticky li {
    @apply overflow-hidden;
    height: var(--sticky-row-height);
  }
  .waterfall-view__vlist-host :global(.waterfall-vlist-viewport:focus-visible) {
    outline: 2px solid var(--color-primary);
    outline-offset: -2px;
  }

  .waterfall-view__vlist-host :global(.waterfall-vlist) {
    @apply relative h-full w-full overflow-hidden;
  }

  .waterfall-view__vlist-host :global(.waterfall-vlist-viewport) {
    @apply absolute inset-0 overflow-y-scroll;
    -webkit-overflow-scrolling: touch;
    scrollbar-width: thin;
  }

  .waterfall-view__vlist-host :global(.waterfall-vlist-items) {
    @apply absolute left-0 top-0 w-full;
  }

  /* Match `.split-table > tbody > tr`: each virtual row is its own
     fixed-layout table so columns line up with the header row. */
  .waterfall-view__vlist-host :global(.waterfall-vlist-items > div) {
    @apply w-full;
  }

  .waterfall-view__vlist-host :global(tr.waterfall-row) {
    display: table;
    width: 100%;
    table-layout: fixed;
  }
</style>
