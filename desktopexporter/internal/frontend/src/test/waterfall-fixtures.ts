import type { SpanNode } from '@/types/api-types'

export function spanNode(
  id: string,
  parentID: string | null,
  depth: number
): SpanNode {
  return {
    depth,
    matched: true,
    spanData: {
      spanID: id,
      flags: 0,
      parentSpanID: parentID,
      traceID: 'trace-1',
      name: id,
      kindCode: 1,
      startTime: BigInt(depth) * 1_000_000n,
      endTime: BigInt(depth + 1) * 1_000_000n,
      statusCode: 'Ok',
      statusCodeValue: 1,
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
