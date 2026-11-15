import { afterEach, describe, expect, it, vi } from 'vitest'
import { telemetryAPI } from './telemetry-service'
import { getOperatorsForFieldType, OPERATORS } from '@/search/model'
import type { TraceFixture } from '@/test/telemetry-fixture-types'
import { arrowResponse, traceSummaryTable } from '@/test/arrow-fixtures'
import { Table, Bool, vectorFromArray } from 'apache-arrow'
import { telemetryDataset } from '@/test/native-telemetry-fixtures'
import {
  stubArrowResult,
  stubRpcResult,
} from '@/test/telemetry-service-fixtures'

function stubTraceResult(result: TraceFixture) {
  stubArrowResult(telemetryDataset('getTraceView', result))
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('attribute discovery', () => {
  it('keeps received array kinds searchable without inventing an element type', async () => {
    stubRpcResult([
      {
        name: 'items',
        attributeScope: 'span',
        type: 'array',
      },
    ])

    const [field] = await telemetryAPI.getTraceAttributeDefinitions()

    expect(field).toMatchObject({
      name: 'items',
      type: 'array',
      owner: 'span',
      path: ['attributes', 'items'],
    })
    if (!field) {
      throw new Error('Expected an attribute field')
    }
    expect(
      getOperatorsForFieldType(field.type).map(operator => operator.symbol)
    ).toEqual([OPERATORS.CONTAINS.symbol, OPERATORS.NOT_CONTAINS.symbol])
  })
})

describe('telemetryAPI.searchTraceSummaries', () => {
  it('reads native timestamps and computes exact durations', async () => {
    const summaries = [
      {
        traceID: 'trace-1',
        hasRootSpan: true,
        rootSpan: { serviceName: 'checkout', name: 'GET /checkout' },
        startTime: 1700000000000000000n,
        endTime: 1700000000000012345n,
        spanCount: 2,
        errorCount: 0,
      },
    ]
    stubArrowResult(traceSummaryTable(summaries))

    await expect(
      telemetryAPI.searchTraceSummaries(0n, 1n)
    ).resolves.toMatchObject([
      {
        traceID: 'trace-1',
        startTime: 1700000000000000000n,
        durationNs: 12345n,
      },
    ])
  })

  it('preserves optional matching span identities', async () => {
    const matchedSpans = [
      {
        traceID: '00000000000000000000000000000001',
        spanID: '0000000000000002',
      },
    ]
    stubArrowResult(
      traceSummaryTable([
        {
          traceID: '00000000000000000000000000000001',
          hasRootSpan: true,
          rootSpan: { serviceName: 'service-a', name: 'root' },
          startTime: 1n,
          endTime: 11n,
          spanCount: 2,
          errorCount: 1,
          matchedSpans,
        },
        {
          traceID: '00000000000000000000000000000002',
          hasRootSpan: false,
          rootSpan: null,
          startTime: 2n,
          endTime: null,
          spanCount: 1,
          errorCount: 0,
        },
      ])
    )

    const summaries = await telemetryAPI.searchTraceSummaries(0n, 10n)

    expect(summaries[0]?.matchedSpans).toEqual(matchedSpans)
    expect(summaries[1]?.matchedSpans).toBeUndefined()
  })

  it('preserves unsigned nanoseconds and nullable durations', async () => {
    stubArrowResult(
      traceSummaryTable([
        {
          traceID: 'trace-1',
          hasRootSpan: false,
          rootSpan: null,
          startTime: 18446744073709551615n,
          endTime: null,
          spanCount: 0,
          errorCount: 0,
        },
        {
          traceID: 'trace-2',
          hasRootSpan: false,
          rootSpan: null,
          startTime: 16n,
          endTime: 28n,
          spanCount: 0,
          errorCount: 0,
        },
      ])
    )

    await expect(
      telemetryAPI.searchTraceSummaries(0n, 1n)
    ).resolves.toMatchObject([
      { startTime: 18_446_744_073_709_551_615n, durationNs: null },
      { startTime: 16n, durationNs: 12n },
    ])
  })

  it('rejects a wrongly typed scalar when read instead of coercing it', async () => {
    stubArrowResult(
      new Table({ startTime: vectorFromArray([true], new Bool()) })
    )
    const summaries = await telemetryAPI.searchTraceSummaries(0n, 1n)
    expect(() => summaries[0].startTime).toThrow('Expected an Arrow integer')
  })
})

describe('telemetryAPI.getSpanTimings', () => {
  const reference = {
    traceID: '00000000000000000000000000000011',
    spanID: 'ffffffffffffffff',
  }

  it('reads exact span pairs without whole traces, preserves nanoseconds and deduplicates requests', async () => {
    stubRpcResult({
      rows: [
        [
          reference.traceID,
          reference.spanID,
          'question',
          'agent',
          '1700000000000000001',
          '1700000000000000023',
          2,
        ],
      ],
      truncated: false,
    })
    const controller = new AbortController()
    const result = await telemetryAPI.getSpanTimings(
      [reference, reference],
      controller.signal
    )
    expect(result[0]).toMatchObject({
      ...reference,
      startTime: 1700000000000000001n,
      endTime: 1700000000000000023n,
      statusCodeValue: 2,
    })
    const call = vi.mocked(fetch).mock.calls[0]!
    const request = JSON.parse(String(call[1]!.body))
    expect(request.method).toBe('query')
    expect(request.params.limit).toBe(1)
    expect(request.params.sql).toContain("'0xffffffffffffffff'::ubigint")
    expect(request.params.sql).toContain(
      's.trace_id = r.trace_id and s.span_id = r.span_id'
    )
    expect(call[1]!.signal).toBe(controller.signal)
  })

  it('bounds each projection to 64 references, keeps identical span IDs in different traces and reports absent spans as absent', async () => {
    stubRpcResult({ rows: [], truncated: false })
    const references = Array.from({ length: 65 }, (_, index) => ({
      ...reference,
      traceID: (index + 1).toString(16).padStart(32, '0'),
    }))
    expect(await telemetryAPI.getSpanTimings(references)).toEqual([])
    expect(
      vi
        .mocked(fetch)
        .mock.calls.map(
          ([, init]) => JSON.parse(String(init!.body)).params.limit
        )
    ).toEqual([64, 1])
  })

  it('rejects non-hex references before querying and skips empty lookups', async () => {
    stubRpcResult({ rows: [], truncated: false })
    await expect(
      telemetryAPI.getSpanTimings([
        { ...reference, spanID: "'); DROP TABLE spans;--" },
      ])
    ).rejects.toThrow('32-hex trace ID and a 16-hex span ID')
    expect(await telemetryAPI.getSpanTimings([])).toEqual([])
    expect(fetch).not.toHaveBeenCalled()
  })
})

describe('telemetryAPI.getReferencedTraceViews', () => {
  it('discovers a bounded complete selection for trace-only references and coalesces exact references', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation(async (_url, init: RequestInit) => {
        const request = JSON.parse(String(init.body))
        if (!request.method)
          return arrowResponse(
            telemetryDataset('getTraceView', {
              traceID: request.traceID,
              traceStart: '0',
              resources: {},
              scopes: {},
              spans: [],
              unplacedSpanCount: 0,
            })
          )
        return {
          ok: true,
          json: async () => ({
            jsonrpc: '2.0',
            id: request.id,
            result:
              request.method === 'query'
                ? {
                    rows: [['1'.repeat(16)], ['2'.repeat(16)]],
                    truncated: false,
                  }
                : {
                    traceID: request.params.traceID,
                    traceStart: '0',
                    resources: {},
                    scopes: {},
                    spans: [],
                    unplacedSpanCount: 0,
                  },
          }),
        }
      })
    )
    const traceID = 'a'.repeat(32)
    const result = await telemetryAPI.getReferencedTraceViews([
      { traceID },
      { traceID, spanID: '1'.repeat(16) },
      { traceID },
    ])
    expect(result.unavailableTraceIDs).toEqual([])
    const calls = vi
      .mocked(fetch)
      .mock.calls.map(([, init]) => JSON.parse(String(init!.body)))
    expect(calls).toHaveLength(2)
    expect(calls[0]).toMatchObject({ method: 'query', params: { limit: 257 } })
    expect(calls[1]).toMatchObject({
      traceID,
      spanIDs: ['1'.repeat(16), '2'.repeat(16)],
    })
  })

  it('does not present oversized trace-only selections as complete trees', async () => {
    stubRpcResult({
      rows: Array.from({ length: 257 }, (_, i) => [
        i.toString(16).padStart(16, '0'),
      ]),
      truncated: false,
    })
    const traceID = 'a'.repeat(32)
    expect(await telemetryAPI.getReferencedTraceViews([{ traceID }])).toEqual({
      traces: [],
      unavailableTraceIDs: [traceID],
    })
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it('deduplicates exact selections, keeps traces separate and retains successful groups when another fails', async () => {
    vi.stubGlobal(
      'fetch',
      vi.fn().mockImplementation(async (_url, init: RequestInit) => {
        const request = JSON.parse(String(init.body))
        if (request.traceID === 'b'.repeat(32))
          return Response.json(
            { code: -1, message: 'missing' },
            { status: 404 }
          )
        return arrowResponse(
          telemetryDataset('getTraceView', {
            traceID: request.traceID,
            traceStart: '0',
            resources: {},
            scopes: {},
            spans: [],
            unplacedSpanCount: 0,
          })
        )
      })
    )
    const reference = { traceID: 'a'.repeat(32), spanID: 'f'.repeat(16) }
    const controller = new AbortController()
    const result = await telemetryAPI.getReferencedTraceViews(
      [reference, reference, { ...reference, traceID: 'b'.repeat(32) }],
      controller.signal
    )
    expect(result.traces.map(trace => trace.traceID)).toEqual([
      reference.traceID,
    ])
    expect(result.unavailableTraceIDs).toEqual(['b'.repeat(32)])
    expect(
      vi
        .mocked(fetch)
        .mock.calls.map(([, init]) => JSON.parse(String(init!.body)))
    ).toEqual([
      expect.objectContaining({
        traceID: reference.traceID,
        spanIDs: [reference.spanID],
      }),
      expect.objectContaining({
        traceID: 'b'.repeat(32),
        spanIDs: [reference.spanID],
      }),
    ])
    expect(
      vi
        .mocked(fetch)
        .mock.calls.every(([, init]) => init!.signal === controller.signal)
    ).toBe(true)
  })

  it('does not silently split or fetch an unbounded trace when a selection exceeds the limit', async () => {
    stubRpcResult({})
    const result = await telemetryAPI.getReferencedTraceViews(
      Array.from({ length: 257 }, (_, id) => ({
        traceID: 'a'.repeat(32),
        spanID: id.toString(16).padStart(16, '0'),
      }))
    )
    expect(result).toEqual({
      traces: [],
      unavailableTraceIDs: ['a'.repeat(32)],
    })
    expect(await telemetryAPI.getReferencedTraceViews([])).toEqual({
      traces: [],
      unavailableTraceIDs: [],
    })
    await expect(
      telemetryAPI.getReferencedTraceViews([{ traceID: 'bad', spanID: '1' }])
    ).rejects.toThrow('32-hex trace ID')
    expect(fetch).not.toHaveBeenCalled()
  })
})

describe('telemetryAPI.getTraceSummary', () => {
  it('looks up one exact trace independently of the Logs time range and preserves exact times', async () => {
    stubArrowResult(
      traceSummaryTable([
        {
          traceID: '00000000000000000000000000000011',
          hasRootSpan: true,
          rootSpan: { name: 'checkout', serviceName: 'api' },
          startTime: 1700000000000000001n,
          endTime: 1709007199254740994n,
          spanCount: 5,
          errorCount: 2,
        },
      ])
    )
    const result = await telemetryAPI.getTraceSummary(
      '00000000000000000000000000000011'
    )
    expect(result).toMatchObject({
      startTime: 1700000000000000001n,
      durationNs: 9007199254740993n,
    })
    const request = JSON.parse(String(vi.mocked(fetch).mock.calls[0]![1]!.body))
    expect(vi.mocked(fetch).mock.calls[0][0]).toBe(
      '/api/data/searchTraceSummaries'
    )
    expect(request).toMatchObject({
      startTime: null,
      endTime: null,
      limit: 1,
      query: {
        type: 'condition',
        query: {
          field: { owner: 'span', path: ['traceID'] },
          fieldOperator: '=',
          value: '00000000000000000000000000000011',
        },
      },
    })
  })

  it('reports no stored trace without fabricating a summary', async () => {
    stubArrowResult(traceSummaryTable([]))
    expect(
      await telemetryAPI.getTraceSummary('00000000000000000000000000000011')
    ).toBeNull()
  })
})

describe('telemetryAPI.getTraceView rehydration', () => {
  const wire = {
    traceID: 'abc123',
    traceStart: '1700000000000000000',
    unplacedSpanCount: 0,
    resources: {
      '7': {
        attributes: [
          {
            id: 'resource-checkout',
            key: 'service.name',
            value: { kind: 'string', value: 'checkout' },
          },
        ],
        droppedAttributesCount: 0,
      },
      '9': {
        attributes: [
          {
            id: 'resource-payments',
            key: 'service.name',
            value: { kind: 'string', value: 'payments' },
          },
        ],
        droppedAttributesCount: 2,
      },
    },
    scopes: {
      '3': {
        name: 'otelhttp',
        version: '1.2.0',
        attributes: [],
        droppedAttributesCount: 0,
      },
    },
    spans: [
      {
        spanData: {
          traceState: '',
          spanID: 'aaaa',
          parentSpanID: null,
          flags: 0,
          name: 'root',
          kindCode: 2,
          kind: 'Server',
          start: '0',
          dur: '5000000',
          attributes: [],
          events: [
            {
              name: 'e',
              timestamp: '1700000000000000123',
              droppedAttributesCount: 0,
              attributes: [],
            },
          ],
          links: [
            {
              traceID: 'linked-trace',
              spanID: 'linked-span',
              traceState: '',
              flags: 0,
              droppedAttributesCount: 0,
              attributes: [],
              summary: {
                target: { name: 'consume', serviceName: 'worker' },
              },
            },
          ],
          r: 7,
          s: 3,
          droppedAttributesCount: 0,
          droppedEventsCount: 0,
          droppedLinksCount: 0,
          statusCodeValue: 1,
          statusCode: 'Ok',
          statusMessage: '',
        },
        depth: 0,
        matched: true,
      },
      {
        spanData: {
          traceState: '',
          spanID: 'bbbb',
          parentSpanID: 'aaaa',
          flags: 0,
          name: 'child',
          kindCode: -1,
          kind: 'Unknown (-1)',
          start: '1200000000',
          dur: '3000000',
          attributes: [],
          events: [],
          links: [],
          r: 9,
          s: 3,
          droppedAttributesCount: 0,
          droppedEventsCount: 0,
          droppedLinksCount: 0,
          statusCodeValue: 99,
          statusCode: 'Unknown (99)',
          statusMessage: '',
        },
        depth: 1,
        matched: true,
      },
    ],
  } satisfies TraceFixture

  async function fetchTrace() {
    stubTraceResult(wire)
    return telemetryAPI.getTraceView('abc123')
  }

  it('exposes a lightweight tree and independent catalogue, not empty full Details payloads', async () => {
    const trace = await fetchTrace()
    const span = trace.spans[0]!.spanData
    expect(span).not.toHaveProperty('attributes')
    expect(span).not.toHaveProperty('links')
    expect(span.resource).not.toHaveProperty('attributes')
    expect(span.scope).not.toHaveProperty('attributes')
    expect(span.events[0]).not.toHaveProperty('attributes')
    expect(span.events[0]).toHaveProperty('summaryAttributes')
    expect(trace.availableColumns).toContainEqual({
      source: 'resource',
      key: 'service.name',
      count: 2,
    })
  })

  it('rehydrates referenced spans with exact timing, ancestry and full shared Details payloads', async () => {
    const traceID = 'a'.repeat(32)
    const spanID = 'b'.repeat(16)
    stubTraceResult({
      ...wire,
      traceID,
      spans: [
        { ...wire.spans[0], spanData: { ...wire.spans[0].spanData, spanID } },
      ],
    })
    const result = await telemetryAPI.getReferencedTraceViews([
      { traceID, spanID },
    ])
    expect(result.traces[0]!.spans[0]!.spanData).toMatchObject({
      traceID,
      spanID,
      startTime: 1700000000000000000n,
      endTime: 1700000000005000000n,
      resource: {
        attributes: [
          { key: 'service.name', value: { kind: 'string', value: 'checkout' } },
        ],
      },
      events: [{ timestamp: 1700000000000000123n }],
    })
  })

  it('retains derived link names separately from received link attributes', async () => {
    stubArrowResult(telemetryDataset('getSpan', wire))
    const span = await telemetryAPI.getSpan('abc123', 'aaaa')
    expect(span?.links[0]).toMatchObject({
      traceID: 'linked-trace',
      spanID: 'linked-span',
      attributes: [],
      summary: {
        target: { name: 'consume', serviceName: 'worker' },
      },
    })
  })

  it('resolves each span against its own resource, not the first one', async () => {
    const trace = await fetchTrace()
    expect(trace.spans[0].spanData.resource).toMatchObject({
      serviceName: 'checkout',
    })
    expect(trace.spans[1].spanData.resource).toMatchObject({
      serviceName: 'payments',
    })
    expect(trace.spans[1].spanData.resource.droppedAttributesCount).toBe(2)
  })

  it('resolves scopes by reference', async () => {
    const trace = await fetchTrace()
    expect(trace.spans[0].spanData.scope.name).toBe('otelhttp')
    expect(trace.spans[1].spanData.scope.name).toBe('otelhttp')
  })

  it('reconstructs absolute times from the baseline, offset and duration', async () => {
    const trace = await fetchTrace()
    const base = 1700000000000000000n

    expect(trace.spans[0].spanData.startTime).toBe(base)
    expect(trace.spans[0].spanData.endTime).toBe(base + 5_000_000n)

    // End time is relative to the span start, not the trace baseline.
    expect(trace.spans[1].spanData.startTime).toBe(base + 1_200_000_000n)
    expect(trace.spans[1].spanData.endTime).toBe(base + 1_203_000_000n)
  })

  it('reattaches the traceID the wire format drops per span', async () => {
    const trace = await fetchTrace()
    expect(trace.traceID).toBe('abc123')
    for (const span of trace.spans) {
      expect(span.spanData.traceID).toBe('abc123')
    }
  })

  it('leaves no wire-only fields on the decoded span', async () => {
    const trace = await fetchTrace()
    const keys = Object.keys(trace.spans[0].spanData)
    for (const wireOnly of ['r', 's', 'start', 'dur']) {
      expect(keys).not.toContain(wireOnly)
    }
  })

  it('shares resolved resources rather than copying them per span', async () => {
    stubTraceResult({
      ...wire,
      spans: [wire.spans[0], { ...wire.spans[0], depth: 1 }],
    })
    const trace = await telemetryAPI.getTraceView('abc123')
    expect(trace.spans[0].spanData.resource).toBe(
      trace.spans[1].spanData.resource
    )
  })

  it('still promotes event timestamps to bigint', async () => {
    const trace = await fetchTrace()
    expect(trace.spans[0].spanData.events[0].timestamp).toBe(
      1700000000000000123n
    )
  })

  it('preserves unplacedSpanCount when the wire reports zero', async () => {
    const trace = await fetchTrace()
    expect(trace.unplacedSpanCount).toBe(0)
  })

  it('preserves unplacedSpanCount when the wire reports spans stranded on a cycle', async () => {
    stubTraceResult({ ...wire, unplacedSpanCount: 3 })
    const trace = await telemetryAPI.getTraceView('abc123')
    expect(trace.unplacedSpanCount).toBe(3)
  })

  it('does not invent salvaged or cyclePoint on spans the wire never flagged', async () => {
    const trace = await fetchTrace()
    for (const span of trace.spans) {
      expect('salvaged' in span).toBe(false)
      expect('cyclePoint' in span).toBe(false)
    }
  })

  it('preserves salvaged and cyclePoint on a span recovered from a cycle', async () => {
    stubTraceResult({
      ...wire,
      unplacedSpanCount: 0,
      spans: [
        ...wire.spans,
        {
          spanData: {
            ...wire.spans[1].spanData,
            spanID: 'cccc',
            parentSpanID: 'dddd',
          },
          depth: 0,
          matched: true,
          salvaged: true,
          cyclePoint: false,
        },
        {
          spanData: {
            ...wire.spans[1].spanData,
            spanID: 'dddd',
            parentSpanID: 'cccc',
          },
          depth: 1,
          matched: true,
          salvaged: true,
          cyclePoint: true,
        },
      ],
    })
    const trace = await telemetryAPI.getTraceView('abc123')

    const recovered = trace.spans.find(s => s.spanData.spanID === 'cccc')!
    expect(recovered.salvaged).toBe(true)
    expect(recovered.cyclePoint).toBe(false)

    const cyclePoint = trace.spans.find(s => s.spanData.spanID === 'dddd')!
    expect(cyclePoint.salvaged).toBe(true)
    expect(cyclePoint.cyclePoint).toBe(true)

    expect('salvaged' in trace.spans[0]).toBe(false)
    expect('salvaged' in trace.spans[1]).toBe(false)
  })
})
