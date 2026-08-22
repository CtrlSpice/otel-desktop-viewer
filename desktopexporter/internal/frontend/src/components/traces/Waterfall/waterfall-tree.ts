// Span-tree helpers for the waterfall, and the search-driven collapse shape.
//
// A depth/subtree auto-collapse heuristic used to live here (collapse any
// parent at depth 4 or with 12 descendants). It was written when the waterfall
// rendered every row; the list is virtualised now, so the cost it avoided is
// gone, and what remained was a guess about which parts of a trace the reader
// cared about -- reported twice, both times as the view collapsing on its own.
// Traces open fully expanded, and collapsing is the reader's to do.

import type { SpanData, SpanNode } from '@/types/api-types'

export function isErrorSpan(span: SpanData): boolean {
  return (
    span.statusCode === 'Error' || span.events.some(e => e.name === 'exception')
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
): {
  parentBySpanID: Map<string, string | null>
  childrenBySpanID: Map<string, string[]>
} {
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
  // Unlike ancestorIdsOf, this guard cannot fire through the current call
  // site, and a mutation test confirms it: single-parent graphs only form
  // disjoint simple loops, the walk starts only from matched spans, and a
  // matched span is always in `relevant` -- so any lap of a cycle is stopped
  // at the entry one step before revisiting. The guard exists so termination
  // is a property of this function rather than an invariant every future
  // caller must know about.
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
