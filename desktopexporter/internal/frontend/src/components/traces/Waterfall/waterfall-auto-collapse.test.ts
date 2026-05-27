import { describe, expect, it } from 'vitest'
import type { SpanNode } from '@/types/api-types'
import {
  buildChildrenBySpanId,
  computeAutoCollapsedParents,
  isErrorSpan,
} from './waterfall-auto-collapse'

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

describe('computeAutoCollapsedParents', () => {
  it('collapses deep parents but not shallow ones', () => {
    const spans = [
      spanNode('a', null, 0),
      spanNode('b', 'a', 1),
      spanNode('c', 'b', 2),
      spanNode('d', 'c', 3),
      spanNode('e', 'd', 4),
      spanNode('f', 'e', 5),
    ]
    const children = buildChildrenBySpanId(spans)
    const collapsed = computeAutoCollapsedParents(spans, children, {
      depthThreshold: 4,
      subtreeSizeThreshold: 100,
    })
    expect(collapsed.has('e')).toBe(true)
    expect(collapsed.has('d')).toBe(false)
    expect(collapsed.has('c')).toBe(false)
  })

  it('collapses wide subtrees before depth threshold', () => {
    const spans = [spanNode('root', null, 0)]
    for (let i = 0; i < 14; i++) {
      spans.push(spanNode(`leaf-${i}`, 'root', 1))
    }
    const children = buildChildrenBySpanId(spans)
    const collapsed = computeAutoCollapsedParents(spans, children, {
      depthThreshold: 100,
      subtreeSizeThreshold: 12,
    })
    expect(collapsed.has('root')).toBe(true)
  })

  it('never auto-collapses error spans or branches containing errors', () => {
    const spans = [
      spanNode('root', null, 0),
      spanNode('err', 'root', 1, 'Error'),
      ...Array.from({ length: 14 }, (_, i) =>
        spanNode(`leaf-${i}`, 'err', 2)
      ),
    ]
    const children = buildChildrenBySpanId(spans)
    const collapsed = computeAutoCollapsedParents(spans, children, {
      depthThreshold: 1,
      subtreeSizeThreshold: 1,
    })
    expect(collapsed.has('root')).toBe(false)
    expect(collapsed.has('err')).toBe(false)
  })
})

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
