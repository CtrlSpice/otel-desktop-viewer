import type {
  AttributeMapEntry,
  Attributes,
  AttributeValue,
  SpanData,
} from '@/types/api-types'
import { attributeValueTokens } from '@/components/shared/detail-value'

const text = (value: string): AttributeValue => ({ kind: 'string', value })
const number = (value: number): AttributeValue => ({ kind: 'double', value })
const array = (value: AttributeValue[]): AttributeValue => ({
  kind: 'array',
  value,
})
const object = (
  fields: Record<string, AttributeValue>
): Extract<AttributeValue, { kind: 'map' }> => ({
  kind: 'map',
  value: Object.entries(fields).map(([key, value]) => ({ key, value })),
})

function attributes(entries: Attributes | AttributeMapEntry[]): AttributeValue {
  return array(
    entries.map(entry =>
      object({ key: text(entry.key), value: anyValue(entry.value) })
    )
  )
}

function anyValue(value: AttributeValue): AttributeValue {
  switch (value.kind) {
    case 'empty':
      return object({})
    case 'string':
      return object({ stringValue: text(value.value) })
    case 'bytes':
      return object({ bytesValue: text(value.value) })
    case 'bool':
      return object({ boolValue: value })
    case 'int64':
      return object({ intValue: text(value.value.toString()) })
    case 'double':
      return object({ doubleValue: value })
    case 'array':
      return object({
        arrayValue: object({ values: array(value.value.map(anyValue)) }),
      })
    case 'map':
      return object({
        kvlistValue: object({ values: attributes(value.value) }),
      })
  }
}

/** One selected span in an OTLP JSON envelope, including its resource/scope.
 * Use the received enum codes, decimal strings for 64-bit fields, and the
 * existing lossless formatter so signed zero and duplicate typed keys survive. */
export function spanCopyJSON(span: SpanData): string {
  const selected = object({
    traceId: text(span.traceID),
    spanId: text(span.spanID),
    traceState: text(span.traceState),
    flags: number(span.flags),
    name: text(span.name),
    kind: number(span.kindCode),
    startTimeUnixNano: text(span.startTime.toString()),
    endTimeUnixNano: text(span.endTime.toString()),
    attributes: attributes(span.attributes),
    droppedAttributesCount: number(span.droppedAttributesCount),
    events: array(
      span.events.map(event =>
        object({
          name: text(event.name),
          timeUnixNano: text(event.timestamp.toString()),
          attributes: attributes(event.attributes),
          droppedAttributesCount: number(event.droppedAttributesCount),
        })
      )
    ),
    droppedEventsCount: number(span.droppedEventsCount),
    links: array(
      span.links.map(link => {
        const copiedLink = object({
          traceState: text(link.traceState),
          flags: number(link.flags),
          attributes: attributes(link.attributes),
          droppedAttributesCount: number(link.droppedAttributesCount),
        })
        if (link.traceID !== null)
          copiedLink.value.push({ key: 'traceId', value: text(link.traceID) })
        if (link.spanID !== null)
          copiedLink.value.push({ key: 'spanId', value: text(link.spanID) })
        return copiedLink
      })
    ),
    droppedLinksCount: number(span.droppedLinksCount),
    status: object({
      code: number(span.statusCodeValue),
      message: text(span.statusMessage),
    }),
  })
  if (span.parentSpanID !== null)
    selected.value.push({ key: 'parentSpanId', value: text(span.parentSpanID) })
  const envelope = object({
    resourceSpans: array([
      object({
        resource: object({
          attributes: attributes(span.resource.attributes),
          droppedAttributesCount: number(span.resource.droppedAttributesCount),
        }),
        scopeSpans: array([
          object({
            scope: object({
              name: text(span.scope.name),
              version: text(span.scope.version),
              attributes: attributes(span.scope.attributes),
              droppedAttributesCount: number(span.scope.droppedAttributesCount),
            }),
            spans: array([selected]),
          }),
        ]),
      }),
    ]),
  })
  return attributeValueTokens(envelope)
    .map(token => token.text)
    .join('')
}
