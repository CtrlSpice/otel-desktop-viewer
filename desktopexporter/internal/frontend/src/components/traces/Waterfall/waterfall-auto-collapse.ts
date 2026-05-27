import type { SpanData, SpanNode } from '@/types/api-types'

/** Collapse parents at or below this depth when they have children. */
export const WATERFALL_AUTO_COLLAPSE_DEPTH = 4

/** Collapse parents whose descendant count (excluding self) reaches this size. */
export const WATERFALL_AUTO_COLLAPSE_SUBTREE_SIZE = 12

export function isErrorSpan(span: SpanData): boolean {
  return (
    span.statusCode === 'Error' ||
    span.events.some(e => e.name === 'exception')
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

export function computeAutoCollapsedParents(
  spans: readonly SpanNode[],
  childrenBySpanId: ReadonlyMap<string, readonly string[]>,
  options?: {
    depthThreshold?: number
    subtreeSizeThreshold?: number
  }
): Set<string> {
  const depthThreshold =
    options?.depthThreshold ?? WATERFALL_AUTO_COLLAPSE_DEPTH
  const sizeThreshold =
    options?.subtreeSizeThreshold ?? WATERFALL_AUTO_COLLAPSE_SUBTREE_SIZE

  const spanById = new Map(spans.map(n => [n.spanData.spanID, n]))
  const subtreeSizeCache = new Map<string, number>()
  const subtreeErrorCache = new Map<string, boolean>()

  function subtreeSize(id: string): number {
    const cached = subtreeSizeCache.get(id)
    if (cached !== undefined) return cached
    const kids = childrenBySpanId.get(id) ?? []
    let size = 0
    for (const kid of kids) {
      size += 1 + subtreeSize(kid)
    }
    subtreeSizeCache.set(id, size)
    return size
  }

  function subtreeHasError(id: string): boolean {
    const cached = subtreeErrorCache.get(id)
    if (cached !== undefined) return cached
    const node = spanById.get(id)
    if (!node) {
      subtreeErrorCache.set(id, false)
      return false
    }
    if (isErrorSpan(node.spanData)) {
      subtreeErrorCache.set(id, true)
      return true
    }
    for (const kid of childrenBySpanId.get(id) ?? []) {
      if (subtreeHasError(kid)) {
        subtreeErrorCache.set(id, true)
        return true
      }
    }
    subtreeErrorCache.set(id, false)
    return false
  }

  const collapsed = new Set<string>()
  for (const node of spans) {
    const id = node.spanData.spanID
    const kids = childrenBySpanId.get(id)
    if (!kids || kids.length === 0) continue
    if (isErrorSpan(node.spanData) || subtreeHasError(id)) continue
    if (
      node.depth >= depthThreshold ||
      subtreeSize(id) >= sizeThreshold
    ) {
      collapsed.add(id)
    }
  }
  return collapsed
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
