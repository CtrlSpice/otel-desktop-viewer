// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import type { AttributeValue, SpanData } from '@/types/api-types'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import { SPAN_FIELDS } from '@/constants/fields'
import TraceDetailView from './TraceDetailView.svelte'
import { spanCopyJSON } from './span-copy'

function makeSpan(): SpanData {
  return {
    traceID: '12345678901234567890123456789012',
    spanID: '1234567890123456',
    parentSpanID: null,
    traceState: 'vendor=value',
    flags: 1,
    name: 'selected span',
    kind: 'Server',
    kindCode: 2,
    startTime: 1_700_000_000_123_456_789n,
    endTime: 1_700_000_001_987_654_321n,
    statusCode: 'Error',
    statusCodeValue: 2,
    statusMessage: 'failed',
    attributes: [
      {
        key: 'sequence',
        value: { kind: 'int64', value: 9_223_372_036_854_775_807n },
      },
    ],
    resource: {
      attributes: [
        { key: 'service.name', value: { kind: 'string', value: 'checkout' } },
      ],
      droppedAttributesCount: 1,
    },
    scope: {
      name: 'instrumentation',
      version: '1.2',
      attributes: [{ key: 'ready', value: { kind: 'bool', value: false } }],
      droppedAttributesCount: 2,
    },
    events: [
      {
        name: 'event',
        timestamp: 1_700_000_000_999_999_999n,
        attributes: [{ key: 'value', value: { kind: 'double', value: -0 } }],
        droppedAttributesCount: 3,
      },
    ],
    links: [
      {
        traceID: 'linked-trace',
        spanID: 'linked-span',
        traceState: 'linked=value',
        flags: 257,
        attributes: [{ key: 'bytes', value: { kind: 'bytes', value: 'AQI=' } }],
        droppedAttributesCount: 4,
      },
    ],
    droppedAttributesCount: 5,
    droppedEventsCount: 6,
    droppedLinksCount: 7,
  }
}

describe('selected span JSON', () => {
  it('copies one span with its resource, scope, received enums, flags, events and links', () => {
    const source = makeSpan()
    const json = spanCopyJSON(source)
    const resourceSpans = JSON.parse(json).resourceSpans
    expect(resourceSpans).toHaveLength(1)
    expect(resourceSpans[0].resource).toEqual({
      attributes: [{ key: 'service.name', value: { stringValue: 'checkout' } }],
      droppedAttributesCount: 1,
    })
    expect(resourceSpans[0].scopeSpans).toHaveLength(1)
    const scopeSpans = resourceSpans[0].scopeSpans[0]
    expect(scopeSpans.scope).toEqual({
      name: 'instrumentation',
      version: '1.2',
      attributes: [{ key: 'ready', value: { boolValue: false } }],
      droppedAttributesCount: 2,
    })
    expect(scopeSpans.spans).toHaveLength(1)
    const span = scopeSpans.spans[0]
    expect(span).toMatchObject({
      traceId: source.traceID,
      spanId: source.spanID,
      traceState: source.traceState,
      name: source.name,
      kind: 2,
      flags: 1,
      startTimeUnixNano: '1700000000123456789',
      endTimeUnixNano: '1700000001987654321',
      status: { code: 2, message: 'failed' },
      droppedAttributesCount: 5,
      droppedEventsCount: 6,
      droppedLinksCount: 7,
    })
    expect(span).not.toHaveProperty('parentSpanId')
    expect(span.attributes).toEqual([
      { key: 'sequence', value: { intValue: '9223372036854775807' } },
    ])
    expect(span.events[0]).toMatchObject({
      name: 'event',
      timeUnixNano: '1700000000999999999',
      droppedAttributesCount: 3,
    })
    expect(Object.is(span.events[0].attributes[0].value.doubleValue, -0)).toBe(
      true
    )
    expect(span.links[0]).toMatchObject({
      traceId: 'linked-trace',
      spanId: 'linked-span',
      traceState: 'linked=value',
      flags: 257,
      droppedAttributesCount: 4,
      attributes: [{ key: 'bytes', value: { bytesValue: 'AQI=' } }],
    })
    expect(json).toContain('\n  "resourceSpans": [')
  })

  it('retains typed duplicate keys and every AnyValue kind without JSON number coercion', () => {
    const source = makeSpan()
    const values: AttributeValue[] = [
      { kind: 'empty', value: null },
      { kind: 'string', value: 'line\n"quoted"' },
      { kind: 'bytes', value: 'AQI=' },
      { kind: 'bool', value: true },
      { kind: 'int64', value: -9_223_372_036_854_775_808n },
      { kind: 'double', value: Infinity },
      { kind: 'array', value: [{ kind: 'double', value: NaN }] },
      {
        kind: 'map',
        conflictingKeys: ['same'],
        value: [
          { key: 'same', value: { kind: 'int64', value: 1n } },
          { key: 'same', value: { kind: 'string', value: '1' } },
        ],
      },
    ]
    source.attributes = values.map(value => ({ key: 'same', value }))
    source.parentSpanID = 'parent'
    const span = JSON.parse(spanCopyJSON(source)).resourceSpans[0].scopeSpans[0]
      .spans[0]
    expect(span.parentSpanId).toBe('parent')
    expect(
      span.attributes.map((entry: { value: unknown }) => entry.value)
    ).toEqual([
      {},
      { stringValue: 'line\n"quoted"' },
      { bytesValue: 'AQI=' },
      { boolValue: true },
      { intValue: '-9223372036854775808' },
      { doubleValue: 'Infinity' },
      { arrayValue: { values: [{ doubleValue: 'NaN' }] } },
      {
        kvlistValue: {
          values: [
            { key: 'same', value: { intValue: '1' } },
            { key: 'same', value: { stringValue: '1' } },
          ],
        },
      },
    ])
    expect(source.attributes).toHaveLength(8)
  })
})

describe('Copy span action', () => {
  it('copies all selected-span data even when displayed fields are filtered, without toggling the disclosure', async () => {
    setTestUrl('/traces/trace?span=selected')
    const user = userEvent.setup()
    const write = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
    const source = makeSpan()
    const { container } = renderWithContexts(TraceDetailView, {
      span: source,
      columnFilter: [
        SPAN_FIELDS.find(field => 'name' in field && field.name === 'name')!,
      ],
    })
    const button = screen.getByRole('button', {
      name: 'Copy span',
    })
    const disclosure = container.querySelector('details')!
    expect(button.closest('summary')).toBeNull()
    await user.click(button)
    expect(write).toHaveBeenCalledWith(spanCopyJSON(source))
    expect(disclosure.open).toBe(true)
    const copied = JSON.parse(write.mock.calls[0]![0])
    expect(
      copied.resourceSpans[0].scopeSpans[0].spans[0].attributes
    ).toHaveLength(1)
    await user.click(disclosure.querySelector('summary')!)
    expect(disclosure.open).toBe(false)
    expect(button.isConnected).toBe(true)
  })

  it('offers no span-copy action when no span is selected', () => {
    renderWithContexts(TraceDetailView, { span: undefined })
    expect(screen.queryByRole('button', { name: 'Copy span' })).toBeNull()
  })
})
