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

export function buildChildrenBySpanId(
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
  relevant: ReadonlySet<string>
): boolean {
  const kids = children.get(sid)
  if (!kids) return false
  for (const kid of kids) {
    if (relevant.has(kid)) return true
    if (hasRelevantDescendant(kid, children, relevant)) return true
  }
  return false
}

export function computeSearchCollapsedParents(
  spans: readonly SpanNode[],
  matchedIDs: ReadonlySet<string>,
  ancestorsOfMatched: ReadonlySet<string>,
  childrenBySpanId: ReadonlyMap<string, readonly string[]>
): Set<string> {
  const relevant = new Set([...matchedIDs, ...ancestorsOfMatched])
  const toCollapse = new Set<string>()
  for (const node of spans) {
    const sid = node.spanData.spanID
    const hasKids = (childrenBySpanId.get(sid)?.length ?? 0) > 0
    if (!hasKids) continue
    if (!relevant.has(sid)) {
      toCollapse.add(sid)
    } else if (
      matchedIDs.has(sid) &&
      !hasRelevantDescendant(sid, childrenBySpanId, relevant)
    ) {
      toCollapse.add(sid)
    }
  }
  return toCollapse
}
