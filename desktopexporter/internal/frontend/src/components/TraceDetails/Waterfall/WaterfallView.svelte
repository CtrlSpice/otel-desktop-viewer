<script module lang="ts">
  import type { SpanNode, SpanData } from '@/types/api-types'
  import type { TreeConnectorMeta } from './WaterfallTreeGutter.svelte'

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

  function serviceName(span: SpanData): string | undefined {
    return span.resource.attributes.find(a => a.key === 'service.name')?.value
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
    return multiService ? (serviceName(span) ?? '') : span.name
  }

  function isMultiService(spans: SpanNode[]): boolean {
    const services = spans.reduce((acc, n) => {
      const s = serviceName(n.spanData)
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
  import { tick } from 'svelte'
  import WaterfallTimeAxisHeader, {
    waterfallTimeAxis,
  } from './WaterfallTimeAxisHeader.svelte'
  import WaterfallRow from './WaterfallRow.svelte'

  // --- Keyboard paging ---

  const PAGE_STEP = 8

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
  }

  let { spans, selectedSpanID, onSelectSpan, loading = false }: Props = $props()

  let tableHeight = $state(0)
  let panelHeight = $state(0)

  let bounds = $derived(getTraceBounds(spans))
  const TARGET_TICK_COUNT = 6
  let barGridPercents = $derived(
    waterfallTimeAxis(bounds.duration, TARGET_TICK_COUNT).ticks.map(
      t => t.offsetPercent
    )
  )
  let rows = $derived(buildWaterfallRows(spans, bounds))

  // --- Column widths (resizable) ---

  const MIN_SPAN_COL = 120
  const MIN_SERVICE_COL = 80
  let spanColWidth = $state(192)
  let serviceColWidth = $state(120)

  // --- Expand/collapse ---

  /** Span IDs whose descendant rows are hidden (`visibility: collapse` on child `<tr>`s). */
  let collapsedParents = $state<Set<string>>(new Set())

  let parentBySpanId = $derived(
    new Map(
      spans.map(n => [n.spanData.spanID, n.spanData.parentSpanID] as const)
    )
  )

  let rowVisibilityBySpanId = $derived(
    rowVisibilityMap(spans, parentBySpanId, collapsedParents)
  )

  let visibleRowIndices = $derived(
    spans
      .map((n, i) => (rowVisibilityBySpanId.get(n.spanData.spanID) ? i : -1))
      .filter((i): i is number => i >= 0)
  )

  function toggleCollapse(spanID: string) {
    const next = new Set(collapsedParents)
    if (next.has(spanID)) next.delete(spanID)
    else next.add(spanID)
    collapsedParents = next
  }

  // --- Focus & keyboard on the grid ---

  let gridTableEl = $state<HTMLTableElement | null>(null)

  function escapeSpanIdForSelector(spanId: string): string {
    return typeof CSS !== 'undefined' && typeof CSS.escape === 'function'
      ? CSS.escape(spanId)
      : spanId.replace(/\\/g, '\\\\').replace(/"/g, '\\"')
  }

  async function focusRowTr(spanId: string) {
    await tick()
    const safe = escapeSpanIdForSelector(spanId)
    gridTableEl
      ?.querySelector<HTMLTableRowElement>(`tr[data-span-id="${safe}"]`)
      ?.focus()
  }

  type KeyDelta =
    | { kind: 'relative'; offset: number }
    | { kind: 'absolute'; position: 'first' | 'last' }

  const KEY_DELTAS: Record<string, KeyDelta> = {
    ArrowDown: { kind: 'relative', offset: 1 },
    j: { kind: 'relative', offset: 1 },
    ArrowUp: { kind: 'relative', offset: -1 },
    k: { kind: 'relative', offset: -1 },
    PageDown: { kind: 'relative', offset: PAGE_STEP },
    PageUp: { kind: 'relative', offset: -PAGE_STEP },
    Home: { kind: 'absolute', position: 'first' },
    End: { kind: 'absolute', position: 'last' },
  }

  function resolveNextPos(
    delta: KeyDelta,
    currentPos: number,
    lastPos: number
  ): number {
    const raw =
      delta.kind === 'absolute'
        ? delta.position === 'first'
          ? 0
          : lastPos
        : currentPos + delta.offset
    return Math.max(0, Math.min(raw, lastPos))
  }

  function isGridNavTarget(
    el: HTMLElement | null,
    table: HTMLElement | null
  ): boolean {
    if (!el || !table?.contains(el)) return false
    if (el.closest('input, textarea, select, [contenteditable="true"]'))
      return false
    if (el.closest('button')) return false
    return true
  }

  function handleKeydown(e: KeyboardEvent) {
    if (!isGridNavTarget(e.target as HTMLElement | null, gridTableEl)) return
    if (!spans.length) return

    const vis = visibleRowIndices
    if (vis.length === 0) return

    const currentIdx =
      (selectedSpanID ?? '') !== ''
        ? spans.findIndex(n => n.spanData.spanID === selectedSpanID)
        : -1
    const pos = currentIdx >= 0 ? vis.indexOf(currentIdx) : -1

    if (pos < 0) {
      const firstId = spans[vis[0]].spanData.spanID
      onSelectSpan(firstId)
      void focusRowTr(firstId)
      e.preventDefault()
      return
    }

    const currentSpan = spans[currentIdx]
    const currentId = currentSpan.spanData.spanID
    const hasChildren = (rows[currentIdx]?.tree.childrenCount ?? 0) > 0

    if (e.key === 'ArrowRight' || e.key === 'l') {
      if (hasChildren && collapsedParents.has(currentId)) {
        toggleCollapse(currentId)
        e.preventDefault()
      }
      return
    }

    if (e.key === 'ArrowLeft' || e.key === 'h') {
      if (hasChildren && !collapsedParents.has(currentId)) {
        toggleCollapse(currentId)
      } else {
        const parentId = parentBySpanId.get(currentId) ?? null
        if (parentId) {
          onSelectSpan(parentId)
          void focusRowTr(parentId)
          scrollRowIntoView(parentId)
        }
      }
      e.preventDefault()
      return
    }

    const delta = KEY_DELTAS[e.key]
    if (!delta) return

    e.preventDefault()
    const nextPos = resolveNextPos(delta, pos, vis.length - 1)
    if (nextPos === pos) return

    const nextId = spans[vis[nextPos]!].spanData.spanID
    onSelectSpan(nextId)
    void focusRowTr(nextId)
    scrollRowIntoView(nextId)
  }

  function scrollRowIntoView(spanID: string) {
    if (!gridTableEl) return
    const safe = escapeSpanIdForSelector(spanID)
    gridTableEl
      .querySelector(`[data-span-id="${safe}"]`)
      ?.scrollIntoView({ block: 'nearest' })
  }
</script>

<div class="waterfall-panel" bind:clientHeight={panelHeight}>
  <div
    class="waterfall-view {loading ? 'opacity-70' : 'opacity-100'}"
    style:height={tableHeight > 0 && panelHeight > 0
      ? `${Math.min(tableHeight, panelHeight)}px`
      : undefined}
  >
    <div class="waterfall-view__scroll">
    <!-- svelte-ignore a11y_no_noninteractive_element_to_interactive_role -->
    <table
      bind:this={gridTableEl}
      bind:clientHeight={tableHeight}
      class="waterfall-grid table table-sm table-zebra table-fixed w-full min-w-[36rem] border-collapse"
      role="grid"
      aria-label="Span waterfall"
      aria-colcount={3}
      tabindex="-1"
      onkeydown={handleKeydown}
    >
      <colgroup>
        <col style:width="{spanColWidth}px" />
        <col style:width="{serviceColWidth}px" />
        <col />
      </colgroup>
      <thead>
        <WaterfallTimeAxisHeader
          traceDurationNs={bounds.duration}
          targetTickCount={TARGET_TICK_COUNT}
          {spanColWidth}
          {serviceColWidth}
          onResizeSpanCol={(w: number) => {
            spanColWidth = Math.max(MIN_SPAN_COL, w)
          }}
          onResizeServiceCol={(w: number) => {
            serviceColWidth = Math.max(MIN_SERVICE_COL, w)
          }}
        />
      </thead>
      <tbody>
        {#each rows as row (row.spanNode.spanData.spanID)}
          {@const sid = row.spanNode.spanData.spanID}
          <WaterfallRow
            {row}
            {barGridPercents}
            selected={sid === selectedSpanID}
            visible={rowVisibilityBySpanId.get(sid) ?? true}
            subtreeCollapsed={collapsedParents.has(sid)}
            onRowClick={() => {
              onSelectSpan(sid)
              void focusRowTr(sid)
            }}
            onToggleExpand={() => toggleCollapse(sid)}
          />
        {/each}
      </tbody>
    </table>
    </div>
  </div>
</div>

<style lang="postcss">
  .waterfall-panel {
    @apply h-full;
  }

  .waterfall-view {
    @apply flex flex-col overflow-hidden rounded-xl border border-base-300/70 bg-base-100/80 shadow-surface-sm backdrop-blur-sm transition-opacity duration-200;
  }

  .waterfall-grid :global(thead) {
    @apply sticky top-0 z-10 bg-base-100;
  }

  .waterfall-grid :global(thead tr) {
    border-bottom-width: 0;
  }

  .waterfall-grid:focus-within {
    @apply ring-1 ring-primary/30 rounded;
  }

  .waterfall-view__scroll {
    @apply min-h-0 flex-1 overflow-x-auto overflow-y-auto;
    scrollbar-width: thin;
  }
</style>
