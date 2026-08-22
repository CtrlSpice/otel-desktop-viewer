import { describe, expect, it } from 'vitest'
import type { SpanNode } from '@/types/api-types'
import {
  buildChildrenBySpanId,
  buildStructuralMaps,
  computeSearchCollapsedParents,
  isErrorSpan,
} from './waterfall-tree'

function spanNode(
  id: string,
  parentId: string | null,
  depth: number,
  statusCode: 'Ok' | 'Error' = 'Ok'
): SpanNode {
  return {
    depth,
    matched: true,
    spanData: {
      spanID: id,
      parentSpanID: parentId,
      traceID: 'trace-1',
      name: id,
      startTime: 0n,
      endTime: 1n,
      statusCode,
      events: [],
      links: [],
      attributes: [],
      resource: { attributes: [], droppedAttributesCount: 0 },
      scope: {
        name: '',
        version: '',
        attributes: [],
        droppedAttributesCount: 0,
      },
      traceState: '',
      kind: '',
      droppedAttributesCount: 0,
      droppedEventsCount: 0,
      droppedLinksCount: 0,
      statusMessage: '',
    },
  }
}

describe('isErrorSpan', () => {
  it('treats exception events as errors', () => {
    const node = spanNode('x', null, 0)
    node.spanData.events = [
      {
        name: 'exception',
        timestamp: 0n,
        attributes: [],
        droppedAttributesCount: 0,
      },
    ]
    expect(isErrorSpan(node.spanData)).toBe(true)
  })
})

describe('computeSearchCollapsedParents with parent cycles', () => {
  // A salvaged trace can make two spans each other's child in
  // childrenBySpanId. The relevance walk must terminate rather than
  // overflow the stack -- parentSpanID is reported data, not verified
  // structure.
  it('survives a two-span cycle while search is active', () => {
    const spans = [
      spanNode('root', null, 0),
      spanNode('old-biff', 'young-biff', 0),
      spanNode('young-biff', 'old-biff', 1),
    ]
    const children = buildChildrenBySpanId(spans)
    // Only the root matches; both cycle members are irrelevant, have kids
    // (each other), and must be collapsed rather than looped over.
    const collapsed = computeSearchCollapsedParents(
      spans,
      new Set(['root']),
      new Set(),
      children
    )
    expect(collapsed).toEqual(new Set(['old-biff', 'young-biff']))
  })

  it('still finds a relevant descendant hiding below a cycle member', () => {
    const spans = [
      spanNode('loop-a', 'loop-b', 0),
      spanNode('loop-b', 'loop-a', 1),
      spanNode('treasure', 'loop-b', 2),
    ]
    const children = buildChildrenBySpanId(spans)
    // loop-a matches and treasure (its transitive descendant through the
    // cycle) is relevant, so loop-a must NOT be collapsed: the walk has to
    // reach through the cycle exactly once, not refuse to enter it.
    const collapsed = computeSearchCollapsedParents(
      spans,
      new Set(['loop-a']),
      new Set(),
      children
    )
    expect(collapsed.has('loop-a')).toBe(false)
  })
})

describe('buildStructuralMaps', () => {
  it('matches parentSpanID maps on a healthy tree', () => {
    const spans = [
      spanNode('root', null, 0),
      spanNode('child', 'root', 1),
      spanNode('grandchild', 'child', 2),
      spanNode('child2', 'root', 1),
    ]
    const { parentBySpanId, childrenBySpanId } = buildStructuralMaps(spans)
    expect(parentBySpanId.get('root')).toBeNull()
    expect(parentBySpanId.get('child')).toBe('root')
    expect(parentBySpanId.get('grandchild')).toBe('child')
    expect(parentBySpanId.get('child2')).toBe('root')
    expect(childrenBySpanId.get('root')).toEqual(['child', 'child2'])
  })

  it('treats a promoted orphan at depth 0 as a root, not a child of a ghost', () => {
    const spans = [
      spanNode('root', null, 0),
      spanNode('orphan', 'not-in-response', 0),
      spanNode('orphan-kid', 'orphan', 1),
    ]
    const { parentBySpanId, childrenBySpanId } = buildStructuralMaps(spans)
    expect(parentBySpanId.get('orphan')).toBeNull()
    expect(parentBySpanId.get('orphan-kid')).toBe('orphan')
    expect(childrenBySpanId.has('not-in-response')).toBe(false)
  })

  it('a salvaged cycle entry is a root and never its own descendant', () => {
    // parentSpanID says old-biff and young-biff are each other's parent.
    // The rendered tree says old-biff is the entry at depth 0. Structure wins:
    // collapse-all must leave old-biff visible, and young-biff must have no
    // collapsible children at all.
    const spans = [
      spanNode('old-biff', 'young-biff', 0),
      spanNode('young-biff', 'old-biff', 1),
    ]
    const { parentBySpanId, childrenBySpanId } = buildStructuralMaps(spans)
    expect(parentBySpanId.get('old-biff')).toBeNull()
    expect(parentBySpanId.get('young-biff')).toBe('old-biff')
    expect(childrenBySpanId.get('old-biff')).toEqual(['young-biff'])
    expect(childrenBySpanId.has('young-biff')).toBe(false)
  })

  it('a second tree after a deep one does not inherit stale stack entries', () => {
    const spans = [
      spanNode('root-a', null, 0),
      spanNode('deep', 'root-a', 3),
      spanNode('root-b', null, 0),
      spanNode('b-kid', 'root-b', 1),
    ]
    const { parentBySpanId } = buildStructuralMaps(spans)
    expect(parentBySpanId.get('root-b')).toBeNull()
    expect(parentBySpanId.get('b-kid')).toBe('root-b')
  })
})
