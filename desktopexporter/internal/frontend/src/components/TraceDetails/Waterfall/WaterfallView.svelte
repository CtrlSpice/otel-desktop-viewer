<script module lang="ts">
  import type { SpanNode, SpanData } from '@/types/api-types'
  import type { TreeConnectorMeta } from './WaterfallTreeGutter.svelte'
  import { getServiceName } from '@/utils/resource'

  // --- Shared types ---

  export type TraceBounds = {
    start: bigint
    end: bigint
    duration: bigint
  }

  // --- Categorical coloring ---

  const CATEGORICAL_TOKENS = ['iris', 'pine', 'gold', 'rose', 'foam'] as const
  export type CategoricalToken = (typeof CATEGORICAL_TOKENS)[number]

  export type EventMarker = { percent: number; name: string }

  export type WaterfallRowData = {
    spanNode: SpanNode
    colorToken: CategoricalToken | 'error'
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
      start: spans[0].spanData.startTime,
      end: spans[0].spanData.endTime,
    }
    const { start, end } = spans.reduce(
      (acc, node) => ({
        start: ((a, b) => (a < b ? a : b))(acc.start, node.spanData.startTime),
        end: ((a, b) => (a > b ? a : b))(acc.end, node.spanData.endTime),
      }),
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

  /** Build a Map<key, token> by folding spans in order, assigning the next token on first encounter. */
  function buildColorMap(
    spans: SpanNode[],
    keyFn: (s: SpanData) => string | null
  ): Map<string, CategoricalToken> {
    return spans.reduce<{ map: Map<string, CategoricalToken>; next: number }>(
      ({ map, next }, node) => {
        const k = keyFn(node.spanData)
        if (k === null || map.has(k)) return { map, next }
        return {
          map: new Map([
            ...map,
            [k, CATEGORICAL_TOKENS[next % CATEGORICAL_TOKENS.length]],
          ]),
          next: next + 1,
        }
      },
      { map: new Map(), next: 0 }
    ).map
  }

  function colorTokenFor(
    key: string | null,
    colorMap: Map<string, CategoricalToken>
  ): CategoricalToken | 'error' {
    return key === null ? 'error' : (colorMap.get(key) ?? CATEGORICAL_TOKENS[0])
  }

  /** Palette tokens are assigned in first-seen order of categorical keys. */
  export function buildWaterfallRows(
    spans: SpanNode[],
    bounds: TraceBounds
  ): WaterfallRowData[] {
    const multi = isMultiService(spans)
    const keyFn = (s: SpanData) => categoricalKeyFor(s, multi)
    const colorMap = buildColorMap(spans, keyFn)
    const treeMeta = computeTreeMeta(spans)

    return spans.map((node, i) => ({
      spanNode: node,
      colorToken: colorTokenFor(keyFn(node.spanData), colorMap),
      offsetPercent: getOffsetPercent(
        bounds.start,
        bounds.duration,
        node.spanData.startTime
      ),
      widthPercent: getWidthPercent(
        bounds.duration,
        node.spanData.endTime - node.spanData.startTime
      ),
      tree: treeMeta[i]!,
      eventMarkers: node.spanData.events.map(e => ({
        percent: getOffsetPercent(bounds.start, bounds.duration, e.timestamp),
        name: e.name,
      })),
    }))
  }
</script>

<script lang="ts">
  import { tick, untrack } from 'svelte'
  import type { Snippet } from 'svelte'
  import WaterfallTimeAxisHeader, {
    waterfallTimeAxis,
  } from './WaterfallTimeAxisHeader.svelte'
  import WaterfallRow from './WaterfallRow.svelte'
  import { tableNav, escapeForSelector } from '@/utils/table-keyboard-nav'

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
  let rows = $derived(buildWaterfallRows(spans, bounds))

  // --- Column widths (resizable) ---
  import {
    fixed,
    flex,
    computeInitialWidths,
    redistributeWidths,
    computeBarPositions,
    applyColumnResize,
    startColumnResize,
  } from '@/utils/column-resize'

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

  let childrenBySpanId = $derived(() => {
    const map = new Map<string, string[]>()
    for (const n of spans) {
      const pid = n.spanData.parentSpanID
      if (pid) {
        const list = map.get(pid)
        if (list) list.push(n.spanData.spanID)
        else map.set(pid, [n.spanData.spanID])
      }
    }
    return map
  })

  function hasRelevantDescendant(
    sid: string,
    children: Map<string, string[]>,
    relevant: Set<string>
  ): boolean {
    const kids = children.get(sid)
    if (!kids) return false
    for (const kid of kids) {
      if (relevant.has(kid)) return true
      if (hasRelevantDescendant(kid, children, relevant)) return true
    }
    return false
  }

  // Auto-collapse irrelevant branches when search results arrive; reset when cleared.
  $effect(() => {
    if (hasActiveSearch) {
      const relevant = new Set([...matchedIDs, ...ancestorsOfMatched])
      const children = childrenBySpanId()
      const toCollapse = new Set<string>()
      for (const node of spans) {
        const sid = node.spanData.spanID
        const hasKids = (children.get(sid)?.length ?? 0) > 0
        if (!hasKids) continue
        if (!relevant.has(sid)) {
          toCollapse.add(sid)
        } else if (
          matchedIDs.has(sid) &&
          !hasRelevantDescendant(sid, children, relevant)
        ) {
          toCollapse.add(sid)
        }
      }
      collapsedParents = toCollapse
    } else {
      collapsedParents = new Set()
    }
  })

  // --- Focus & keyboard on the grid ---

  let scrollEl = $state<HTMLTableSectionElement | null>(null)
  let gridTableEl = $state<HTMLTableElement | null>(null)

  async function clampScroll() {
    await tick()
    if (!scrollEl) return
    const max = scrollEl.scrollHeight - scrollEl.clientHeight
    if (scrollEl.scrollTop > max) scrollEl.scrollTop = max
  }

  async function focusRowTr(spanId: string) {
    await tick()
    const safe = escapeForSelector(spanId)
    gridTableEl
      ?.querySelector<HTMLTableRowElement>(`tr[data-span-id="${safe}"]`)
      ?.focus()
  }

  function handleTreeKeys(e: KeyboardEvent, currentId: string | null): boolean {
    if (!currentId) return false

    const idx = spans.findIndex(n => n.spanData.spanID === currentId)
    if (idx < 0) return false

    const hasChildren = (rows[idx]?.tree.childrenCount ?? 0) > 0

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
</script>

<div class="waterfall-panel">
  <div class="waterfall-view {loading ? 'opacity-70' : 'opacity-100'}">
    <div class="waterfall-view__scroll" bind:this={scrollContainerEl}>
      <div class="col-resize-context">
        <!-- svelte-ignore a11y_no_noninteractive_element_to_interactive_role -->
        <table
          bind:this={gridTableEl}
          class="split-table waterfall-grid table table-sm w-full min-w-[36rem] border-collapse"
          role="grid"
          aria-label="Span waterfall"
          aria-colcount={3}
          tabindex="-1"
          use:tableNav={{
            rowIdAttr: 'span-id',
            onSelect: id => {
              onSelectSpan(id)
              void focusRowTr(id)
            },
            pageStep: 8,
            skipHidden: true,
            onKey: handleTreeKeys,
          }}
        >
          <thead class="table-header-surface">
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
          <tbody class="table-body-surface" bind:this={scrollEl}>
            {#each rows as row (row.spanNode.spanData.spanID)}
              {@const sid = row.spanNode.spanData.spanID}
              <WaterfallRow
                {row}
                {barGridPercents}
                selected={sid === selectedSpanID}
                visible={rowVisibilityBySpanId.get(sid) ?? true}
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
</div>

<style lang="postcss">
  @reference "../../../app.css";
  .waterfall-panel {
    @apply h-full;
  }

  .waterfall-view {
    @apply flex h-full min-h-0 flex-col overflow-hidden rounded-xl border border-base-300/70 bg-base-100/80 shadow-surface-sm backdrop-blur-sm transition-opacity duration-200;
  }

  .waterfall-grid {
    @apply min-h-0 flex-1;
  }

  .waterfall-grid:focus-within {
    @apply rounded-xl ring-1 ring-primary/30;
  }

  .waterfall-view__scroll {
    @apply flex min-h-0 flex-1 flex-col;
  }

  .waterfall-view__scroll > :global(.col-resize-context) {
    @apply flex min-h-0 flex-1 flex-col;
  }
</style>
