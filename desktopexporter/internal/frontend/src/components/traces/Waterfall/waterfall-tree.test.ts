import { describe, expect, it } from 'vitest'
import type { SpanNode } from '@/types/api-types'
import { buildChildrenBySpanId, isErrorSpan } from './waterfall-tree'

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
