<script module lang="ts">
  import type { SpanNode, SpanData } from '@/types/api-types'
  import type { TreeConnectorMeta } from './WaterfallTreeGutter.svelte'
  import { parseBigInt } from '@/utils/bigint'
  import { getServiceName } from '@/utils/resource'
  import { categoricalPalette } from '@/utils/chart-palette'
  import { themeSignal } from '@/state/theme.svelte'

  // --- Shared types ---

  export type TraceBounds = {
    start: bigint
    end: bigint
    duration: bigint
  }

  // --- Categorical coloring ---
  //
  // Categorical key per span: span.name in single-service traces, service
  // name in multi-service traces. Error spans are coloured by a separate
  // semantic token (`--color-error`) and never participate in the rotation.
  //
  // Colours come from `categoricalPalette()` -- same HCL-interpolated arc
  // the metric charts use. We anchor the start stem to `iris` so the first
  // span (root, in a well-formed trace) lands on `--color-primary`, then
  // walks iris→pine→foam→gold→rose for subsequent keys. Palette size is
  // `max(uniqueKeys, 5)` so traces with many services get distinct
  // midpoints; small traces still hit the five named stems exactly.

  /** Minimum palette size: the five named stems, so single-service traces
   *  with only 1-2 keys still land on iris/pine/etc. exactly rather than
   *  a degenerate interpolation. */
  const MIN_TRACE_PALETTE = 5

  export type EventMarker = {
    percent: number
    name: string
    eventIndex: number
  }

  export type WaterfallRowData = {
    spanNode: SpanNode
    /** CSS-ready colour string for the bar / gutter / event dot.
     *  Error spans pass `--color-error` via CSS var to preserve semantic
     *  theming; non-error spans get a concrete HCL colour from the palette. */
    color: string
    /** Whether this row is an error span. Consumers branch on this for the
     *  matched/error tinting (which uses semantic vars, not the palette). */
    isError: boolean
    offsetPercent: number
    widthPercent: number
    tree: TreeConnectorMeta
    eventMarkers: EventMarker[]
  }

  export function getTraceBounds(spans: SpanNode[]): TraceBounds {
    if (spans.length === 0) {
      return { start: 0n, end: 0n, duration: 0n }
    }
    const seed = {
      start: parseBigInt(spans[0].spanData.startTime),
      end: parseBigInt(spans[0].spanData.endTime),
    }
    const { start, end } = spans.reduce((acc, node) => {
      const st = parseBigInt(node.spanData.startTime)
      const en = parseBigInt(node.spanData.endTime)
      return {
        start: st < acc.start ? st : acc.start,
        end: en > acc.end ? en : acc.end,
      }
    }, seed)
    return { start, end, duration: end - start }
  }

  // --- Bar layout (ns → %) ---

  function getOffsetPercent(
    traceStart: bigint,
    traceDuration: bigint,
    point: bigint
  ): number {
    if (traceDuration <= 0n) return 0
    return Number(((point - traceStart) * 10000n) / traceDuration) / 100
  }

  function getWidthPercent(
    traceDuration: bigint,
    spanDuration: bigint
  ): number {
    if (traceDuration <= 0n) return 0
    return Math.max(0.3, Number((spanDuration * 10000n) / traceDuration) / 100)
  }

  // --- Span-derived fields ---

  function isErrorSpan(span: SpanData): boolean {
    return (
      span.statusCode === 'Error' ||
      span.events.some(e => e.name === 'exception')
    )
  }

  // --- Tree gutter connectors (helpers composed in computeTreeMeta) ---

  type DepthList = readonly { depth: number }[]

  /** Fold a depth-ordered list into per-index direct-child counts via a stack accumulator. */
  function childrenCounts(spans: DepthList): number[] {
    const { counts } = spans.reduce<{ stack: number[]; counts: number[] }>(
      ({ stack, counts }, { depth }, i) => {
        const trimmed = stack.slice(0, depth)
        if (depth > 0 && trimmed.length > 0) {
          counts[trimmed[trimmed.length - 1]]++
        }
        return { stack: [...trimmed, i], counts }
      },
      { stack: [], counts: Array<number>(spans.length).fill(0) }
    )
    return counts
  }

  /** True when no sibling at the same depth follows before the parent's subtree ends. */
  function isLastChild(spans: DepthList, i: number): boolean {
    const depth = spans[i].depth
    const tail = spans.slice(i + 1)
    const nextAtOrAbove = tail.find(s => s.depth <= depth)
    return nextAtOrAbove === undefined || nextAtOrAbove.depth < depth
  }

  /** For each ancestor depth 0..depth-1, is there another child of that ancestor after row i? */
  function ancestorContinuationFlags(spans: DepthList, i: number): boolean[] {
    const depth = spans[i].depth
    const tail = spans.slice(i + 1)
    return Array.from({ length: depth }, (_, d) => {
      // Find where this ancestor's subtree ends: the next span at depth ≤ d.
      // Only spans within that window could be siblings at depth d+1.
      const endIdx = tail.findIndex(s => s.depth <= d)
      const segment = endIdx === -1 ? tail : tail.slice(0, endIdx)
      return segment.some(s => s.depth === d + 1)
    })
  }

  function computeTreeMeta(spans: SpanNode[]): TreeConnectorMeta[] {
    const counts = childrenCounts(spans)
    return spans.map((_, i) => ({
      childrenCount: counts[i],
      isLastChild: spans[i].depth === 0 ? false : isLastChild(spans, i),
      ancestorHasNextSibling:
        spans[i].depth === 0 ? [] : ancestorContinuationFlags(spans, i),
    }))
  }

  // --- Row model for the grid ---

  function categoricalKeyFor(
    span: SpanData,
    multiService: boolean
  ): string | null {
    if (isErrorSpan(span)) return null
    return multiService ? (getServiceName(span.resource) ?? '') : span.name
  }

  function isMultiService(spans: SpanNode[]): boolean {
    const services = spans.reduce((acc, n) => {
      const s = getServiceName(n.spanData.resource)
      return s !== undefined ? acc.add(s) : acc
    }, new Set<string>())
    return services.size > 1
  }

  /** Build a Map<key, color> by folding spans in order. The palette is
   *  sized to the unique-key count (min 5), so every categorical key gets
   *  its own colour up to whatever services/span-names the trace contains.
   *  Iris is the start stem -- first key seen → iris → --color-primary. */
  function buildColorMap(
    spans: SpanNode[],
    keyFn: (s: SpanData) => string | null,
    theme: string
  ): Map<string, string> {
    const orderedKeys = spans.reduce<string[]>((acc, node) => {
      const k = keyFn(node.spanData)
      if (k !== null && !acc.includes(k)) acc.push(k)
      return acc
    }, [])
    const palette = categoricalPalette(
      Math.max(orderedKeys.length, MIN_TRACE_PALETTE),
      'iris',
      theme
    )
    return new Map(orderedKeys.map((k, i) => [k, palette[i]!]))
  }

  /** Palette is assigned in first-seen order of categorical keys; error
   *  spans short-circuit to `--color-error` so the semantic colour wins
   *  over the rotation. */
  export function buildWaterfallRows(
    spans: SpanNode[],
    bounds: TraceBounds,
    theme: string
  ): WaterfallRowData[] {
    const multi = isMultiService(spans)
    const keyFn = (s: SpanData) => categoricalKeyFor(s, multi)
    const colorMap = buildColorMap(spans, keyFn, theme)
    const treeMeta = computeTreeMeta(spans)

    return spans.map((node, i) => {
      const key = keyFn(node.spanData)
      const isError = key === null
      const color = isError
        ? 'var(--color-error)'
        : (colorMap.get(key) ?? 'var(--color-primary)')
      return {
        spanNode: node,
        color,
        isError,
        offsetPercent: getOffsetPercent(
          bounds.start,
          bounds.duration,
          parseBigInt(node.spanData.startTime)
        ),
        widthPercent: getWidthPercent(
          bounds.duration,
          parseBigInt(node.spanData.endTime) -
            parseBigInt(node.spanData.startTime)
        ),
        tree: treeMeta[i]!,
        eventMarkers: node.spanData.events.map((e, eventIndex) => ({
          percent: getOffsetPercent(
            bounds.start,
            bounds.duration,
            parseBigInt(e.timestamp)
          ),
          name: e.name,
          eventIndex,
        })),
      }
    })
  }
</script>

<script lang="ts">
  import { onDestroy, tick, untrack } from 'svelte'
  import type { Snippet } from 'svelte'
  import VirtualList from '@humanspeak/svelte-virtual-list'
  import PaneHeader from '@/components/shared/PaneHeader.svelte'
  import SignalBadges from '@/components/shared/SignalBadges.svelte'
  import { ArrowLeftIcon, ArrowRightIcon } from '@/icons'
  import WaterfallTimeAxisHeader, {
    waterfallTimeAxis,
  } from './WaterfallTimeAxisHeader.svelte'
  import WaterfallRow from './WaterfallRow.svelte'
  import {
    escapeForSelector,
    resolveNextPos,
    type KeyDelta,
  } from '@/components/shared/utils/table-keyboard-nav'
  import {
    computeSearchCollapsedParents,
    buildStructuralMaps,
  } from './waterfall-tree'
  import { ancestorIdsOf, keyboardAnchorSpanID } from './waterfall-reveal'
  import {
    collapsedForTrace,
    setCollapsedForTrace,
  } from './waterfall-collapse-store'

  const WATERFALL_ROW_HEIGHT_PX = 28
  const GRID_PAGE_STEP = 8

  const KEY_DELTAS: Record<string, KeyDelta> = {
    ArrowDown: { kind: 'relative', offset: 1 },
    j: { kind: 'relative', offset: 1 },
    ArrowUp: { kind: 'relative', offset: -1 },
    k: { kind: 'relative', offset: -1 },
    PageDown: { kind: 'relative', offset: GRID_PAGE_STEP },
    PageUp: { kind: 'relative', offset: -GRID_PAGE_STEP },
    Home: { kind: 'absolute', position: 'first' },
    End: { kind: 'absolute', position: 'last' },
  }

  // --- Visibility from collapse state (pure) ---

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

  // --- Props & derived data ---

  type Props = {
    spans: SpanNode[]
    selectedSpanID: string | null
    onSelectSpan: (spanID: string) => void
    onSelectEvent?: (spanID: string, eventIndex: number) => void
    loading?: boolean
    footer?: Snippet
  }

  let {
    spans,
    selectedSpanID,
    onSelectSpan,
    onSelectEvent,
    loading = false,
    footer,
  }: Props = $props()

  let bounds = $derived(getTraceBounds(spans))
  let rows = $derived(buildWaterfallRows(spans, bounds, themeSignal.value))

  let traceTimeRange = $derived.by(
    (): { startMs: number; endMs: number } | undefined => {
      if (spans.length === 0) return undefined
      return {
        startMs: Number(bounds.start / 1_000_000n),
        endMs: Number(bounds.end / 1_000_000n),
      }
    }
  )

  let headerName = $derived.by(() => {
    if (spans.length === 0) return 'Trace'
    const root = spans.find(n => n.depth === 0) ?? spans[0]
    return root?.spanData.name?.trim() || 'Trace'
  })

  let headerService = $derived.by(() => {
    if (spans.length === 0) return ''
    const root = spans.find(n => n.depth === 0) ?? spans[0]
    return root ? (getServiceName(root.spanData.resource)?.trim() ?? '') : ''
  })

  let errorSpans = $derived(
    spans.filter(node => node.spanData.statusCode === 'Error')
  )
  let headerErrorCount = $derived(errorSpans.length)

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
        .findLast(node => node.spanData.statusCode === 'Error')?.spanData
        .spanID ?? null
    )
  })

  let nextErrorSpanID = $derived.by(() => {
    const startIndex = selectedSpanIndex < 0 ? 0 : selectedSpanIndex + 1
    return (
      spans.slice(startIndex).find(node => node.spanData.statusCode === 'Error')
        ?.spanData.spanID ?? null
    )
  })

  function selectPreviousError() {
    if (previousErrorSpanID) onSelectSpan(previousErrorSpanID)
  }

  function selectNextError() {
    if (nextErrorSpanID) onSelectSpan(nextErrorSpanID)
  }

  // --- Column widths (resizable) ---
  import {
    flex,
    initialWidths,
    fitWidths,
    reconcileWidths,
    resizeBar,
    barPositions as computeBarPositions,
    startColumnResize,
    type ColumnWidths,
  } from '@/components/shared/utils/column-resize'
  import type { DragHandle } from '@/components/shared/utils/drag'

  const wfCols = [
    flex('span', 140, 2),
    flex('service', 100, 1),
    flex('timeline', 240, 4),
  ]

  /* Widths key by column id, never position: the column set is about to
   * become user-configurable, and a stored or derived width must land on
   * the column it belongs to no matter what was added or removed around
   * it. Stored widths from a different column set reconcile on load. */
  const COLUMN_WIDTHS_KEY = 'waterfall-column-widths'
  const COLUMN_RESIZE_STEP_PX = 16
  const COLUMN_RESIZE_LARGE_STEP_PX = 64

  function loadStoredWidths(): ColumnWidths {
    if (typeof localStorage === 'undefined') return {}
    try {
      const parsed: unknown = JSON.parse(
        localStorage.getItem(COLUMN_WIDTHS_KEY) ?? 'null'
      )
      if (parsed === null || typeof parsed !== 'object') return {}
      const out: ColumnWidths = {}
      for (const [id, w] of Object.entries(parsed)) {
        if (typeof w === 'number' && Number.isFinite(w)) out[id] = w
      }
      return out
    } catch {
      return {}
    }
  }

  function saveColumnWidths() {
    if (typeof localStorage === 'undefined') return
    try {
      localStorage.setItem(COLUMN_WIDTHS_KEY, JSON.stringify(colWidths))
    } catch {
      // A full or blocked store costs the preference, not the drag.
    }
  }

  let activeResizeCol = $state<string | null>(null)
  let colWidths = $state<ColumnWidths>(initialWidths(wfCols, 800))
  let colDrag: DragHandle | null = null

  let spanColWidth = $derived(colWidths['span'] ?? 0)
  let serviceColWidth = $derived(colWidths['service'] ?? 0)

  let barPositions = $derived(computeBarPositions(wfCols, colWidths))

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
      colWidths = reconcileWidths(wfCols, {}, containerPx)
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
    if (!scrollContainerEl) return
    untrack(() => {
      colWidths = reconcileWidths(
        wfCols,
        loadStoredWidths(),
        scrollContainerEl!.clientWidth
      )
    })
    const ro = new ResizeObserver(entries => {
      const w = entries[0]?.contentRect.width ?? 800
      scrollContainerW = w
      if (activeResizeCol === null) {
        colWidths = fitWidths(wfCols, colWidths, w)
      }
    })
    ro.observe(scrollContainerEl)
    return () => ro.disconnect()
  })

  const TICK_LABEL_SLOT_PX = 80
  const RULER_PADDING_PX = 48
  const TICK_COUNT_STEPS = [2, 4, 6]

  let timelineColPx = $derived(
    scrollContainerW - spanColWidth - serviceColWidth
  )
  let targetTickCount = $derived.by(() => {
    const fits = Math.floor(
      (timelineColPx - RULER_PADDING_PX) / TICK_LABEL_SLOT_PX
    )
    return TICK_COUNT_STEPS.findLast(n => n <= fits) ?? TICK_COUNT_STEPS[0]
  })

  let barGridPercents = $derived(
    waterfallTimeAxis(bounds.duration, targetTickCount).ticks.map(
      t => t.offsetPercent
    )
  )

  // --- Search match annotation ---

  let matchedIDs = $derived(
    new Set(spans.filter(n => n.matched).map(n => n.spanData.spanID))
  )

  let hasActiveSearch = $derived(
    spans.length > 0 && matchedIDs.size > 0 && spans.some(n => !n.matched)
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

  // --- Expand/collapse ---

  /** Span IDs whose descendant rows are hidden (`visibility: collapse` on child `<tr>`s). */
  // Collapse state has exactly three writers, all of them reader gestures on
  // this tree: the row toggle, the keyboard arrows, and the collapse-all
  // button. Nothing else writes it -- not a resize, not a refetch, not a
  // search, not selecting a span. Every prior variant of "the waterfall
  // collapsed itself" (#348, #230) came from something other than the reader
  // holding a pen.
  let traceID = $derived(spans[0]?.spanData.traceID ?? '')
  let userCollapsed = $derived(collapsedForTrace(traceID))

  // Search is a lens over the reader's state, not a mutation of it. While a
  // search is active the tree takes the search's shape -- branches with no
  // match fold away -- and toggles made during the search live in an overlay
  // scoped to this response. Clearing the search puts the reader's own
  // arrangement back exactly, because it was never touched.
  let searchShape = $derived.by((): Set<string> | null =>
    hasActiveSearch
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
    // A new response is a new match set, so the overlay resets with it. This
    // writes only the overlay -- the reader's own set is not in reach.
    void spans
    searchOverrides = new Map()
  })

  let effectiveCollapsed = $derived.by((): ReadonlySet<string> => {
    if (!searchShape) return userCollapsed
    const out = new Set(searchShape)
    for (const [id, collapsed] of searchOverrides) {
      if (collapsed) out.add(id)
      else out.delete(id)
    }
    return out
  })

  let rowVisibilityBySpanID = $derived(
    rowVisibilityMap(spans, parentBySpanID, effectiveCollapsed)
  )

  function toggleCollapse(spanID: string) {
    if (searchShape) {
      const next = new Map(searchOverrides)
      next.set(spanID, !effectiveCollapsed.has(spanID))
      searchOverrides = next
    } else {
      const next = new Set(userCollapsed)
      if (!next.delete(spanID)) next.add(spanID)
      setCollapsedForTrace(traceID, next)
    }
    void clampScroll()
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

  // Two verbs, not a toggle. A toggle labelled by the current state only
  // offers expand-all once every last parent is collapsed, so from any mixed
  // arrangement there was no way to open everything. Each of these is
  // idempotent -- invoking it in a state it already produced writes the same
  // state again, which is a no-op worth exactly nothing to prevent.
  function setAll(collapsed: boolean) {
    if (searchShape) {
      const next = new Map<string, boolean>()
      for (const id of collapsibleSpanIDs) next.set(id, collapsed)
      searchOverrides = next
    } else {
      setCollapsedForTrace(
        traceID,
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

  function visibleRowIndex(spanID: string): number {
    return visibleRows.findIndex(row => row.spanNode.spanData.spanID === spanID)
  }

  // A scroll, not an edit. This used to expand every collapsed ancestor of
  // the selected span, which meant collapsing a branch that contained the
  // selection quietly undid itself. If the reader closed the branch, the
  // selected row being inside it is not a reason to open it again -- so a
  // hidden selection scrolls to the nearest visible ancestor instead, and the
  // detail panel shows the span either way.
  async function revealAndScrollToSpan(
    spanID: string,
    smoothScroll = true
  ): Promise<void> {
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
      align: 'center',
      smoothScroll,
      shouldThrowOnBounds: false,
    })
    await activeScroll
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

  // --- Focus & keyboard on the grid ---

  let gridHostEl = $state<HTMLDivElement | null>(null)

  $effect(() => {
    const grid = gridHostEl
    if (!grid) return

    // Rows own keyboard entry; the package's focusable scroll viewport would
    // otherwise add a dead Tab stop immediately before the roving row.
    void tick().then(() => {
      if (gridHostEl !== grid) return
      const viewport = grid.querySelector<HTMLElement>(
        '.waterfall-vlist-viewport'
      )
      if (viewport) viewport.tabIndex = -1
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

  function shouldHandleGridKey(el: HTMLElement | null): boolean {
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
    if (!shouldHandleGridKey(e.target as HTMLElement | null)) return
    if (visibleRows.length === 0) return

    const focused = document.activeElement as HTMLElement | null
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

    const delta = KEY_DELTAS[e.key]
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

<div class="waterfall-view {loading ? 'opacity-70' : 'opacity-100'}">
  <PaneHeader
    mode="title"
    title={headerName}
    subtitle={headerService || undefined}
    timeRange={traceTimeRange}
    ariaLabel="Trace waterfall"
  >
    {#snippet badge()}
      <!-- errorCount={0} suppresses the plain err badge: in this header
           the badge IS the navigation (below), while drawer cards keep
           the plain one. -->
      <SignalBadges signal="trace" spanCount={spans.length} errorCount={0} />
      {#if errorSpans.length > 0}
        <span
          class="badge badge-xs badge-soft badge-error waterfall-view__error-nav shrink-0 tabular-nums"
          role="group"
          aria-label="Error navigation"
        >
          <button
            type="button"
            class="waterfall-view__error-nav-btn"
            onclick={selectPreviousError}
            disabled={previousErrorSpanID === null}
            aria-label="Previous error"
            title="Previous error"
          >
            <ArrowLeftIcon class="h-3 w-3" aria-hidden="true" />
          </button>
          <span>{headerErrorCount} err</span>
          <button
            type="button"
            class="waterfall-view__error-nav-btn"
            onclick={selectNextError}
            disabled={nextErrorSpanID === null}
            aria-label="Next error"
            title="Next error"
          >
            <ArrowRightIcon class="h-3 w-3" aria-hidden="true" />
          </button>
        </span>
      {/if}
    {/snippet}
    {#snippet right()}
      {#if collapsibleSpanIDs.length > 0}
        <button
          type="button"
          class="btn btn-ghost btn-xs"
          onclick={() => setAll(false)}
          aria-label="Expand all spans"
        >
          Expand all
        </button>
        <button
          type="button"
          class="btn btn-ghost btn-xs"
          onclick={() => setAll(true)}
          aria-label="Collapse all spans"
        >
          Collapse all
        </button>
      {/if}
    {/snippet}
  </PaneHeader>
  <div class="waterfall-view__scroll" bind:this={scrollContainerEl}>
    <div class="col-resize-context waterfall-view__grid-host">
      {#each barPositions as bar (bar.id)}
        <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
        <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
        <div
          class="col-resize-bar col-resize-bar--guide"
          class:col-resize-bar--active={activeResizeCol === bar.id}
          style:left="{bar.left}px"
          role="separator"
          aria-orientation="vertical"
          aria-label="Resize {bar.id} column"
          aria-valuenow={Math.round(bar.left)}
          aria-valuemin={Math.round(bar.min)}
          aria-valuemax={Math.round(bar.max)}
          aria-valuetext="{Math.round(bar.left)} pixels from left"
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
        <thead class="header-surface waterfall-view__thead">
          <WaterfallTimeAxisHeader
            traceDurationNs={bounds.duration}
            {targetTickCount}
            tickLabelWidth={TICK_LABEL_SLOT_PX}
            {spanColWidth}
            {serviceColWidth}
            onStartResize={handleStartResize}
          />
        </thead>
      </table>
      <div
        bind:this={gridHostEl}
        class="waterfall-view__vlist-host"
        role="grid"
        aria-label="Span waterfall"
        aria-rowcount={visibleRows.length}
        aria-colcount={3}
        tabindex="-1"
        onkeydown={handleGridKeydown}
      >
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
        >
          {#snippet renderItem(row)}
            {@const sid = row.spanNode.spanData.spanID}
            <WaterfallRow
              {row}
              {barGridPercents}
              selected={sid === selectedSpanID}
              tabbable={sid === keyboardAnchorID}
              visible={true}
              subtreeCollapsed={effectiveCollapsed.has(sid)}
              matched={hasActiveSearch && matchedIDs.has(sid)}
              {spanColWidth}
              {serviceColWidth}
              onRowClick={() => {
                onSelectSpan(sid)
                void focusRowTr(sid)
              }}
              onSelectEvent={eventIndex => onSelectEvent?.(sid, eventIndex)}
              onToggleExpand={() => toggleCollapse(sid)}
            />
          {/snippet}
        </VirtualList>
      </div>
    </div>
  </div>
  {#if footer}
    {@render footer()}
  {/if}
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .waterfall-view {
    @apply flex h-full min-h-0 min-w-0 flex-col overflow-hidden bg-base-200 transition-opacity duration-200;
  }

  /* The error badge is the navigation: chevrons flank the count inside
     one pill. Tight side padding so the chevrons read as part of the
     badge rather than buttons that happen to be near it. */
  .waterfall-view__error-nav {
    @apply gap-0.5 pr-0.5 pl-0.5;
  }

  /* The visible target is small, so the hit target is quietly larger:
     vertical padding cancelled by negative margin extends the clickable
     area past the badge without changing its look.

     Hover is a colour change, not a background: the chevrons rest a
     step quieter than the count and come up to the badge's full colour
     under the pointer. Derived from currentColor, so it follows the
     badge through both themes. */
  .waterfall-view__error-nav-btn {
    @apply -my-1 inline-flex cursor-pointer items-center justify-center px-0.5 py-1 transition-colors duration-100;
    color: color-mix(in oklab, currentColor 60%, transparent);
  }

  .waterfall-view__error-nav-btn:hover:not(:disabled),
  .waterfall-view__error-nav-btn:focus-visible {
    color: inherit;
  }

  .waterfall-view__error-nav-btn:disabled {
    @apply cursor-default;
    color: color-mix(in oklab, currentColor 30%, transparent);
  }

  .waterfall-view__scroll {
    @apply flex min-h-0 flex-1 flex-col;
  }

  /* Local override on top of `.header-surface`: drop the warm
     primary-tinted fill so the thead inherits the panel's
     bg-base-200, and remove the inset top highlight + primary-mix
     bottom border that were tuned for a brighter body. The thead
     keeps its `.header-surface` height + sizing rules but visually
     merges into the panel surface. */
  .waterfall-view__thead {
    background-color: transparent;
    box-shadow: none;
    border-bottom-color: transparent;
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
