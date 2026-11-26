import type {
  WaterfallSpan as SpanData,
  WaterfallSpanNode as SpanNode,
} from '@/types/api-types'
import type { TreeConnectorMeta } from '@/components/shared/TreeGutter.svelte'

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

export function computeTreeMeta(
  spans: readonly {
    depth: number
    spanData: Pick<SpanData, 'spanID' | 'parentSpanID'>
  }[],
  focusedSpanID?: string
): TreeConnectorMeta[] {
  const counts = childrenCounts(spans)
  const parents: string[] = []
  return spans.map((span, i) => {
    const parent = span.depth > 0 ? parents[span.depth - 1] : null
    parents[span.depth] = span.spanData.spanID
    parents.length = span.depth + 1
    return {
      childrenCount: counts[i],
      brokenParent:
        span.spanData.spanID !== focusedSpanID &&
        (span.spanData.parentSpanID ?? null) !== (parent ?? null),
      isLastChild: spans[i].depth === 0 ? false : isLastChild(spans, i),
      ancestorHasNextSibling:
        spans[i].depth === 0 ? [] : ancestorContinuationFlags(spans, i),
    }
  })
}

export interface StructuralMaps {
  parentBySpanID: Map<string, string | null>
  childrenBySpanID: Map<string, string[]>
}

export function isErrorSpan(span: SpanData): boolean {
  return (
    span.statusCodeValue === 2 || span.events.some(e => e.name === 'exception')
  )
}

/**
 * Parent/child maps of the *rendered* tree, reconstructed from row order and
 * depth -- the structure the server actually built -- rather than from
 * parentSpanID, which is reported data.
 *
 * The two agree on healthy traces and diverge exactly where it matters: a
 * promoted orphan's parentSpanID names a span that is not in the response,
 * and a salvaged cycle's members name each other, so a parentSpanID-based
 * map makes them mutually collapsible -- collapse-all hid both and the whole
 * salvaged tree vanished. Structurally, every depth-0 row is a root: real
 * root, orphan, or cycle entry alike. Collapse-all collapses down to them,
 * never past them.
 */
export function buildStructuralMaps(
  spans: readonly { spanData: { spanID: string }; depth: number }[]
): StructuralMaps {
  const parentBySpanID = new Map<string, string | null>()
  const childrenBySpanID = new Map<string, string[]>()
  // stack[d] holds the most recent row seen at depth d; a row's structural
  // parent is the nearest preceding row one level up.
  const stack: string[] = []
  for (const n of spans) {
    const id = n.spanData.spanID
    const depth = Math.max(0, n.depth)
    const parent = depth === 0 ? null : (stack[depth - 1] ?? null)
    parentBySpanID.set(id, parent)
    if (parent !== null) {
      const list = childrenBySpanID.get(parent)
      if (list) list.push(id)
      else childrenBySpanID.set(parent, [id])
    }
    stack[depth] = id
    stack.length = depth + 1
  }
  return { parentBySpanID, childrenBySpanID }
}

export function buildChildrenBySpanID(
  spans: readonly SpanNode[]
): Map<string, string[]> {
  const map = new Map<string, string[]>()
  for (const n of spans) {
    const pid = n.spanData.parentSpanID
    if (!pid) continue
    const list = map.get(pid)
    if (list) list.push(n.spanData.spanID)
    else map.set(pid, [n.spanData.spanID])
  }
  return map
}

function hasRelevantDescendant(
  sid: string,
  children: ReadonlyMap<string, readonly string[]>,
  relevant: ReadonlySet<string>,
  // Keep termination local when reported parent relationships contain cycles.
  seen: Set<string> = new Set()
): boolean {
  if (seen.has(sid)) return false
  seen.add(sid)
  const kids = children.get(sid)
  if (!kids) return false
  for (const kid of kids) {
    if (relevant.has(kid)) return true
    if (hasRelevantDescendant(kid, children, relevant, seen)) return true
  }
  return false
}

export function computeSearchCollapsedParents(
  spans: readonly SpanNode[],
  matchedIDs: ReadonlySet<string>,
  ancestorsOfMatched: ReadonlySet<string>,
  childrenBySpanID: ReadonlyMap<string, readonly string[]>
): Set<string> {
  const relevant = new Set([...matchedIDs, ...ancestorsOfMatched])
  const toCollapse = new Set<string>()
  for (const node of spans) {
    const sid = node.spanData.spanID
    const hasKids = (childrenBySpanID.get(sid)?.length ?? 0) > 0
    if (!hasKids) continue
    if (!relevant.has(sid)) {
      toCollapse.add(sid)
    } else if (
      matchedIDs.has(sid) &&
      !hasRelevantDescendant(sid, childrenBySpanID, relevant)
    ) {
      toCollapse.add(sid)
    }
  }
  return toCollapse
}

export function spanSubtree(nodes: SpanNode[], id: string) {
  const index = nodes.findIndex(node => node.spanData.spanID === id)
  if (index < 0) return null
  const root = nodes[index]
  const ancestors: SpanNode[] = []
  let depth = root.depth
  for (let i = index - 1; depth > 0 && i >= 0; i--) {
    if (nodes[i].depth < depth) {
      ancestors.push(nodes[i])
      depth = nodes[i].depth
    }
  }
  let end = index + 1
  while (end < nodes.length && nodes[end].depth > root.depth) end++
  return {
    root,
    ancestors: ancestors.reverse(),
    spans: nodes
      .slice(index, end)
      .map(node => ({ ...node, depth: node.depth - root.depth })),
  }
}
