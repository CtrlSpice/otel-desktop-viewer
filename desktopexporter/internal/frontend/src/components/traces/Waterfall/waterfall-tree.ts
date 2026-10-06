import type { SpanData, SpanNode } from '@/types/api-types'

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
