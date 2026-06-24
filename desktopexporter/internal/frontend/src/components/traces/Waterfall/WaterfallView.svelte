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

  export type EventMarker = { percent: number; name: string }

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
    const { start, end } = spans.reduce(
      (acc, node) => {
        const st = parseBigInt(node.spanData.startTime)
        const en = parseBigInt(node.spanData.endTime)
        return {
          start: st < acc.start ? st : acc.start,
          end: en > acc.end ? en : acc.end,
        }
      },
      seed
    )
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
        eventMarkers: node.spanData.events.map(e => ({
          percent: getOffsetPercent(
            bounds.start,
            bounds.duration,
            parseBigInt(e.timestamp)
          ),
          name: e.name,
        })),
      }
    })
  }
</script>

<script lang="ts">
  import { tick, untrack } from 'svelte'
  import type { Snippet } from 'svelte'
  import VirtualList from '@humanspeak/svelte-virtual-list'
  import PaneHeader from '@/components/shared/PaneHeader.svelte'
  import SignalBadges from '@/components/shared/SignalBadges.svelte'
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
    buildChildrenBySpanId,
    computeAutoCollapsedParents,
    computeSearchCollapsedParents,
  } from './waterfall-auto-collapse'

  const WATERFALL_ROW_HEIGHT_PX = 28
  const GRID_PAGE_STEP = 8
  const VISIBLE_MARGIN_PX = 24

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
  function hasCollapsedAncestor(
    id: string,
    parentOf: Map<string, string | null>,
    collapsed: Set<string>
  ): boolean {
    const pid = parentOf.get(id) ?? null
    if (pid === null) return false
    if (collapsed.has(pid)) return true
    return hasCollapsedAncestor(pid, parentOf, collapsed)
  }

  function rowVisibilityMap(
    spans: readonly { spanData: { spanID: string } }[],
    parentBySpanId: Map<string, string | null>,
    collapsedParents: Set<string>
  ): Map<string, boolean> {
    return new Map(
      spans.map(n => [
        n.spanData.spanID,
        !hasCollapsedAncestor(
          n.spanData.spanID,
          parentBySpanId,
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
    loading?: boolean
    footer?: Snippet
  }

  let {
    spans,
    selectedSpanID,
    onSelectSpan,
    loading = false,
    footer,
  }: Props = $props()

  let bounds = $derived(getTraceBounds(spans))
  let rows = $derived(buildWaterfallRows(spans, bounds, themeSignal.value))

  let traceTimeRange = $derived.by(():
    | { startMs: number; endMs: number }
    | undefined => {
    if (spans.length === 0) return undefined
    return {
      startMs: Number(bounds.start / 1_000_000n),
      endMs: Number(bounds.end / 1_000_000n),
    }
  })

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

  let headerErrorCount = $derived(
    spans.filter(n => n.spanData.statusCode === 'Error').length
  )

  // --- Column widths (resizable) ---
  import {
    fixed,
    flex,
    computeInitialWidths,
    redistributeWidths,
    computeBarPositions,
    applyColumnResize,
    startColumnResize,
  } from '@/components/shared/utils/column-resize'

  const wfCols = [
    flex('span', 140, 2),
    flex('service', 100, 1),
    flex('timeline', 240, 4),
  ]

  let activeResizeCol = $state<number | null>(null)
  let colWidths = $state(wfCols.map(d => d.min))

  let spanColWidth = $derived(colWidths[0])
  let serviceColWidth = $derived(colWidths[1])

  let barPositions = $derived(computeBarPositions(wfCols, colWidths))

  function handleStartResize(colIndex: number, e: PointerEvent) {
    activeResizeCol = colIndex
    startColumnResize(
      wfCols,
      () => colWidths,
      colIndex,
      e,
      next => {
        colWidths = next
      },
      () => {
        activeResizeCol = null
      }
    )
  }

  let scrollContainerEl = $state<HTMLDivElement | null>(null)
  let scrollContainerW = $state(800)

  $effect(() => {
    if (!scrollContainerEl) return
    untrack(() => {
      colWidths = computeInitialWidths(wfCols, scrollContainerEl!.clientWidth)
    })
    const ro = new ResizeObserver(entries => {
      const w = entries[0]?.contentRect.width ?? 800
      scrollContainerW = w
      if (activeResizeCol === null) {
        colWidths = redistributeWidths(wfCols, colWidths, w)
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

  let parentBySpanId = $derived(
    new Map(
      spans.map(n => [n.spanData.spanID, n.spanData.parentSpanID] as const)
    )
  )

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
    computeAncestorsOfMatched(matchedIDs, parentBySpanId)
  )

  // --- Expand/collapse ---

  /** Span IDs whose descendant rows are hidden (`visibility: collapse` on child `<tr>`s). */
  let collapsedParents = $state<Set<string>>(new Set())

  let rowVisibilityBySpanId = $derived(
    rowVisibilityMap(spans, parentBySpanId, collapsedParents)
  )

  function toggleCollapse(spanID: string) {
    const next = new Set(collapsedParents)
    if (next.has(spanID)) next.delete(spanID)
    else next.add(spanID)
    collapsedParents = next
    void clampScroll()
  }

  let childrenBySpanId = $derived.by(() => buildChildrenBySpanId(spans))

  let visibleRows = $derived.by(() =>
    rows.filter(
      row =>
        rowVisibilityBySpanId.get(row.spanNode.spanData.spanID) ?? true
    )
  )

  let rowBySpanId = $derived.by(
    () => new Map(rows.map(row => [row.spanNode.spanData.spanID, row]))
  )

  // Auto-collapse (depth/subtree hybrid) or search-driven collapse.
  $effect(() => {
    if (hasActiveSearch) {
      collapsedParents = computeSearchCollapsedParents(
        spans,
        matchedIDs,
        ancestorsOfMatched,
        childrenBySpanId
      )
      return
    }
    if (spans.length === 0) {
      collapsedParents = new Set()
      return
    }
    collapsedParents = computeAutoCollapsedParents(spans, childrenBySpanId)
  })

  type VirtualListRef = {
    scroll: (options: {
      index: number
      smoothScroll?: boolean
      shouldThrowOnBounds?: boolean
      align?: 'auto' | 'top' | 'bottom' | 'nearest'
    }) => Promise<void>
  }

  let vlistRef = $state<VirtualListRef | null>(null)
  let lastScrolledSelection: string | null = null

  function visibleRowIndex(spanId: string): number {
    return visibleRows.findIndex(
      row => row.spanNode.spanData.spanID === spanId
    )
  }

  function isComfortablyVisible(idx: number): boolean {
    const viewport = scrollContainerEl?.querySelector<HTMLElement>(
      '.waterfall-vlist-viewport'
    )
    const row = viewport?.querySelector<HTMLElement>(
      `[data-original-index="${idx}"]`
    )
    if (!viewport || !row) return false
    const vRect = viewport.getBoundingClientRect()
    const rRect = row.getBoundingClientRect()
    return (
      rRect.top >= vRect.top + VISIBLE_MARGIN_PX &&
      rRect.bottom <= vRect.bottom - VISIBLE_MARGIN_PX
    )
  }

  $effect(() => {
    const id = selectedSpanID
    if (!vlistRef || !id) return
    if (id === lastScrolledSelection) return
    const idx = visibleRowIndex(id)
    if (idx < 0) return
    lastScrolledSelection = id
    if (isComfortablyVisible(idx)) return
    void vlistRef.scroll({
      index: idx,
      align: 'auto',
      smoothScroll: true,
      shouldThrowOnBounds: false,
    })
  })

  $effect(() => {
    if (!selectedSpanID) lastScrolledSelection = null
  })

  // --- Focus & keyboard on the grid ---

  let gridHostEl = $state<HTMLDivElement | null>(null)

  async function scrollRowIntoView(spanId: string) {
    const idx = visibleRowIndex(spanId)
    if (idx >= 0 && vlistRef) {
      await vlistRef.scroll({
        index: idx,
        align: 'nearest',
        smoothScroll: false,
        shouldThrowOnBounds: false,
      })
    }
  }

  async function focusRowTr(spanId: string) {
    await scrollRowIntoView(spanId)
    await tick()
    const safe = escapeForSelector(spanId)
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

  function handleTreeKeys(e: KeyboardEvent, currentId: string | null): boolean {
    if (!currentId) return false

    const row = rowBySpanId.get(currentId)
    const hasChildren = (row?.tree.childrenCount ?? 0) > 0

    if (e.key === 'ArrowRight' || e.key === 'l') {
      if (hasChildren && collapsedParents.has(currentId)) {
        toggleCollapse(currentId)
        e.preventDefault()
      }
      return true
    }

    if (e.key === 'ArrowLeft' || e.key === 'h') {
      if (hasChildren && !collapsedParents.has(currentId)) {
        toggleCollapse(currentId)
      } else {
        const parentId = parentBySpanId.get(currentId) ?? null
        if (parentId) {
          onSelectSpan(parentId)
          void focusRowTr(parentId)
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
    const focusedId =
      focused?.dataset.spanId ??
      (selectedSpanID && focused?.closest(`tr[data-span-id]`)
        ? selectedSpanID
        : null)

    if (handleTreeKeys(e, focusedId ?? selectedSpanID)) return

    if (e.key === 'Enter' || e.key === ' ') {
      const id = focusedId ?? selectedSpanID
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
      focusedId != null ? visibleRowIndex(focusedId) : selectedSpanID
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
        <SignalBadges
          signal="trace"
          spanCount={spans.length}
          errorCount={headerErrorCount}
        />
      {/snippet}
    </PaneHeader>
    <div class="waterfall-view__scroll" bind:this={scrollContainerEl}>
      <div class="col-resize-context waterfall-view__grid-host">
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
              onResizeSpanCol={w => {
                const next = applyColumnResize(wfCols, colWidths, 0, w)
                if (next !== colWidths) colWidths = next
              }}
              onResizeServiceCol={w => {
                const next = applyColumnResize(wfCols, colWidths, 1, w)
                if (next !== colWidths) colWidths = next
              }}
            />
          </thead>
        </table>
        <div
          bind:this={gridHostEl}
          class="waterfall-view__vlist-host"
          role="grid"
          aria-label="Span waterfall"
          aria-colcount={3}
          tabindex="-1"
          onkeydown={handleGridKeydown}
        >
          <VirtualList
            bind:this={vlistRef}
            items={visibleRows}
            defaultEstimatedItemHeight={WATERFALL_ROW_HEIGHT_PX}
            bufferSize={12}
            containerClass="waterfall-vlist"
            viewportClass="waterfall-vlist-viewport"
            itemsClass="waterfall-vlist-items"
          >
            {#snippet renderItem(row)}
              {@const sid = row.spanNode.spanData.spanID}
              <WaterfallRow
                {row}
                {barGridPercents}
                selected={sid === selectedSpanID}
                visible={true}
                subtreeCollapsed={collapsedParents.has(sid)}
                matched={hasActiveSearch && matchedIDs.has(sid)}
                {spanColWidth}
                {serviceColWidth}
                onRowClick={() => {
                  onSelectSpan(sid)
                  void focusRowTr(sid)
                }}
                onToggleExpand={() => toggleCollapse(sid)}
              />
            {/snippet}
          </VirtualList>
        </div>
        {#each barPositions as bar}
          <div
            class="col-resize-bar col-resize-bar--guide"
            class:col-resize-bar--active={activeResizeCol === bar.index}
            style:left="{bar.left}px"
            role="separator"
            aria-orientation="vertical"
            aria-label="Resize {wfCols[bar.index].id} column"
            onpointerdown={e => handleStartResize(bar.index, e)}
          >
            <div class="col-resize-bar__line"></div>
          </div>
        {/each}
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
