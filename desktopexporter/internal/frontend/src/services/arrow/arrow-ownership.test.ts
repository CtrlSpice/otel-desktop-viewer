import { afterEach, describe, expect, it, vi } from 'vitest'
import { Field, List, Struct, Table, Utf8, vectorFromArray } from 'apache-arrow'
import { ArrowCell, ArrowFrame } from './arrow-data'
import { traceData, traceWaterfallData } from './arrow-traces'
import { logData, logTableRow, traceSummary } from './arrow-summaries'
import { attributeKey, serviceConnection } from './arrow-discovery'
import { attributeMatch } from './arrow-search'
import { telemetryDataset } from '@/test/native-telemetry-fixtures'
import { traceSummaryTable } from '@/test/arrow-fixtures'
import type { TraceFixture } from '@/test/telemetry-fixture-types'

afterEach(() => vi.restoreAllMocks())

function traceFrame(eventCount: number) {
  const attributes = [
    {
      key: 'exact',
      value: { kind: 'int64' as const, value: '9007199254740993' },
    },
  ]
  const trace: TraceFixture = {
    traceID: '1'.repeat(32),
    traceStart: '18446744073709551600',
    unplacedSpanCount: 0,
    resources: { 0: { attributes, droppedAttributesCount: 0 } },
    scopes: {
      0: { name: 'scope', version: '1', attributes, droppedAttributesCount: 0 },
    },
    spans: [
      {
        depth: 0,
        matched: true,
        spanData: {
          spanID: '2'.repeat(16),
          parentSpanID: null,
          name: 'span',
          r: 0,
          s: 0,
          start: 0,
          dur: 10,
          traceState: '',
          flags: 0,
          kind: 'Internal',
          kindCode: 1,
          attributes,
          events: Array.from({ length: eventCount }, (_, i) => ({
            name: `event-${i}`,
            timestamp: '18446744073709551601',
            attributes: [
              ...attributes,
              {
                key: 'gen_ai.tool.name',
                value: { kind: 'string', value: 'tool' },
              },
            ],
            droppedAttributesCount: 0,
          })),
          links: [
            {
              traceID: '3'.repeat(32),
              spanID: '4'.repeat(16),
              traceState: '',
              flags: 0,
              attributes,
              droppedAttributesCount: 0,
              summary: { target: { name: 'target', serviceName: 'service' } },
            },
          ],
          droppedAttributesCount: 0,
          droppedEventsCount: 0,
          droppedLinksCount: 0,
          statusCode: 'Ok',
          statusCodeValue: 1,
          statusMessage: '',
        },
      },
    ],
  }
  return new ArrowFrame(telemetryDataset('getSpan', trace))
}

describe('response-owned Arrow collections', () => {
  it('reuses large trace collections without decoding child fields on count reads', () => {
    const frame = traceFrame(10_000)
    const span = traceData(frame).spans[0].spanData
    const text = vi.spyOn(ArrowCell.prototype, 'text')
    const events = span.events
    const links = span.links
    expect(text).not.toHaveBeenCalled()
    const read = vi.spyOn(ArrowFrame.prototype, 'cell')
    for (let i = 0; i < 20; i++) {
      expect(span.events).toBe(events)
      expect(span.events.length).toBe(10_000)
      expect(span.links).toBe(links)
    }
    expect(read).not.toHaveBeenCalled()
    expect(Object.isFrozen(events)).toBe(true)
    expect(Object.isFrozen(links)).toBe(true)
    expect(events[9999].name).toBe('event-9999')
    expect(events[0].timestamp).toBe(18446744073709551601n)
    for (const owner of [
      span,
      span.resource,
      span.scope,
      events[0],
      links[0],
    ]) {
      const attributes = owner.attributes
      expect(owner.attributes).toBe(attributes)
      expect(attributes[0].value).toBe(attributes[0].value)
      expect(attributes[0].value.value).toBe(9007199254740993n)
    }
    expect(links[0].summary).toBe(links[0].summary)
    expect(links[0].summary?.target?.name).toBe('target')
    const fresh = traceData(frame).spans[0].spanData
    expect(fresh.events).not.toBe(events)
    expect(fresh.links[0]).not.toBe(links[0])
    expect(fresh.resource.attributes).not.toBe(span.resource.attributes)
  })

  it('reuses waterfall event attributes and summary collections', () => {
    const event = traceWaterfallData(traceFrame(1)).spans[0].spanData.events[0]
    if (!('summaryAttributes' in event))
      throw new Error('expected waterfall event')
    expect(event.summaryAttributes).toBe(event.summaryAttributes)
    const frame = new ArrowFrame(
      traceSummaryTable([
        {
          traceID: 'trace',
          startTime: 1n,
          endTime: 2n,
          rootSpan: { name: 'root', serviceName: 'service' },
          matchedSpans: [{ traceID: 'trace', spanID: 'span' }],
        },
      ])
    )
    const summary = traceSummary(frame, 0)
    expect(summary.rootSpan).toBe(summary.rootSpan)
    expect(summary.matchedSpans).toBe(summary.matchedSpans)
    expect(summary.serviceBreakdown).toBe(summary.serviceBreakdown)
    expect(Object.isFrozen(summary.serviceBreakdown)).toBe(true)
    expect(traceSummary(frame, 0).matchedSpans).not.toBe(summary.matchedSpans)
  })

  it('retains log bodies, attributes and contexts within one response only', () => {
    const attributes = [{ key: 'zero', value: { kind: 'double', value: -0 } }]
    const frame = new ArrowFrame(
      telemetryDataset('getLog', {
        logRef: 'log',
        timestamp: '18446744073709551615',
        body: {
          kind: 'array',
          value: [{ kind: 'int64', value: '9007199254740993' }],
        },
        attributes,
        resource: { attributes },
        scope: { attributes },
      })
    )
    const log = logData(frame, 0)
    const read = vi.spyOn(frame, 'cell')
    expect(read).not.toHaveBeenCalled()
    const body = log.body
    expect(log.body).toBe(body)
    if (body.kind !== 'array') throw new Error('expected array')
    expect(body.value).toBe(body.value)
    expect(body.value[0].value).toBe(9007199254740993n)
    expect(log.timestamp).toBe(18446744073709551615n)
    for (const owner of [log, log.resource, log.scope]) {
      expect(owner.attributes).toBe(owner.attributes)
      expect(Object.is(owner.attributes[0].value.value, -0)).toBe(true)
    }
    expect(logData(frame, 0).body).not.toBe(body)
  })

  it('reuses log column previews and cross-signal discovery collections', () => {
    const columns = [
      { source: 'log' as const, kind: 'field' as const, key: 'body' },
    ]
    const frame = new ArrowFrame(
      telemetryDataset('searchLogSummaries', [
        {
          logRef: 'log',
          timestamp: '1',
          columnValues: [
            {
              ...columns[0],
              present: true,
              valueType: 'string',
              preview: 'preview',
            },
          ],
        },
      ])
    )
    const row = logTableRow(frame, 0, columns)
    expect(row.columnValues).toBe(row.columnValues)
    expect(row.columnValues?.[0].preview).toBe('preview')
    const serviceType = new Struct([new Field('serviceName', new Utf8())])
    const connections = new ArrowFrame(
      new Table({
        source: vectorFromArray([{ serviceName: 'parent' }], serviceType),
        target: vectorFromArray([{ serviceName: 'child' }], serviceType),
      })
    )
    const connection = serviceConnection(connections, 0)
    expect(connection.source).toBe(connection.source)
    expect(connection.target).toBe(connection.target)
    expect(connection.source.serviceName).toBe('parent')
    const locations = new Struct([
      new Field('signal', new Utf8()),
      new Field('ownerType', new Utf8()),
    ])
    const keys = new ArrowFrame(
      new Table({
        foundOn: vectorFromArray(
          [[{ signal: 'logs', ownerType: 'resource' }]],
          new List(new Field('item', locations))
        ),
        sampleValues: vectorFromArray(
          [['雪']],
          new List(new Field('item', new Utf8()))
        ),
      })
    )
    const key = attributeKey(keys, 0)
    expect(key.foundOn).toBe(key.foundOn)
    const match = attributeMatch(keys, 0)
    expect(match.sampleValues).toBe(match.sampleValues)
    expect(match.sampleValues[0].value).toBe('雪')
  })
})
