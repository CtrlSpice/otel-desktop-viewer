import { afterEach, describe, expect, it, vi } from 'vitest'
import { telemetryAPI, JsonRpcError } from './telemetry-service'
import type { QueryNode } from '@/components/shared/Search/queryTree'

// The backend signals not-found with JSON-RPC errors (one convention across
// all signals; see internal/server/errors.go). getMetric's callers expect
// MetricData | null, so the service translates exactly one code -- -32003,
// metric not found -- back to null. These tests pin that translation.

function stubRpcResponse(body: unknown) {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({
      ok: true,
      json: async () => body,
    })
  )
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('telemetryAPI.getMetric', () => {
  it('returns null when the backend reports metric not found (-32003)', async () => {
    stubRpcResponse({
      jsonrpc: '2.0',
      id: 1,
      error: { code: -32003, message: 'Metric not found' },
    })
    await expect(
      telemetryAPI.getMetric('some-stream', 0, 1)
    ).resolves.toBeNull()
  })

  it('rethrows JSON-RPC errors other than metric not found', async () => {
    stubRpcResponse({
      jsonrpc: '2.0',
      id: 1,
      error: { code: -32009, message: 'Invalid metric stream ID' },
    })
    const call = telemetryAPI.getMetric('not-a-stream', 0, 1)
    await expect(call).rejects.toBeInstanceOf(JsonRpcError)
    await expect(call).rejects.toMatchObject({ code: -32009 })
  })

  it('parses a successful result into MetricData', async () => {
    stubRpcResponse({
      jsonrpc: '2.0',
      id: 1,
      result: {
        name: 'test.gauge',
        unit: 'bytes',
        metricType: 'Gauge',
        timeseries: [],
        window: {
          requested: { startNs: null, endNs: null },
          effective: { startNs: '10', endNs: '20' },
        },
      },
    })
    const metric = await telemetryAPI.getMetric('some-stream', 0, 1)
    expect(metric).not.toBeNull()
    expect(metric!.name).toBe('test.gauge')
    expect(metric!.timeseries).toEqual([])
    expect(metric!.window).toEqual({
      requested: { startNs: null, endNs: null },
      effective: { startNs: 10n, endNs: 20n },
    })
  })

  it('revives typed exemplar values without losing int64 precision', async () => {
    stubRpcResponse({
      jsonrpc: '2.0',
      id: 1,
      result: {
        name: 'test.gauge',
        unit: '1',
        metricType: 'Gauge',
        timeseries: [
          {
            attributesKey: 'series-1',
            attributes: [],
            resource: { attributes: [], droppedAttributesCount: 0 },
            datapoints: [
              {
                id: 'dp-1',
                timestamp: '100',
                timestampMs: 0,
                startTime: '0',
                flags: 0,
                metricType: 'Gauge',
                doubleValue: 1,
                intValue: null,
                valueType: 'Double',
                exemplars: [
                  {
                    timestamp: '101',
                    valueType: 'Int',
                    doubleValue: null,
                    intValue: '9223372036854775807',
                    traceID: null,
                    spanID: null,
                    filteredAttributes: [],
                  },
                  {
                    timestamp: '102',
                    valueType: 'Double',
                    doubleValue: 1.25,
                    intValue: null,
                    traceID: null,
                    spanID: null,
                    filteredAttributes: [],
                  },
                  {
                    timestamp: '103',
                    valueType: 'Empty',
                    doubleValue: null,
                    intValue: null,
                    traceID: null,
                    spanID: null,
                    filteredAttributes: [],
                  },
                  {
                    timestamp: '104',
                    valueType: 'Double',
                    doubleValue: 'NaN',
                    intValue: null,
                    traceID: null,
                    spanID: null,
                    filteredAttributes: [],
                  },
                  {
                    timestamp: '105',
                    valueType: 'Double',
                    doubleValue: 'Infinity',
                    intValue: null,
                    traceID: null,
                    spanID: null,
                    filteredAttributes: [],
                  },
                  {
                    timestamp: '106',
                    valueType: 'Double',
                    doubleValue: '-Infinity',
                    intValue: null,
                    traceID: null,
                    spanID: null,
                    filteredAttributes: [],
                  },
                ],
              },
            ],
            stats: null,
            datapointCount: 1,
            lastSeenNs: '100',
            views: null,
            rateStats: null,
            sparkline: null,
          },
        ],
        window: {
          requested: { startNs: null, endNs: null },
          effective: { startNs: '100', endNs: '100' },
        },
      },
    })

    const metric = await telemetryAPI.getMetric('some-stream', 0, 1)
    const exemplars = metric!.timeseries[0]!.datapoints[0]!.exemplars
    expect(exemplars).toEqual([
      expect.objectContaining({
        timestamp: 101n,
        valueType: 'Int',
        doubleValue: null,
        intValue: 9_223_372_036_854_775_807n,
      }),
      expect.objectContaining({
        timestamp: 102n,
        valueType: 'Double',
        doubleValue: 1.25,
        intValue: null,
      }),
      expect.objectContaining({
        timestamp: 103n,
        valueType: 'Empty',
        doubleValue: null,
        intValue: null,
      }),
      expect.objectContaining({
        timestamp: 104n,
        valueType: 'Double',
        doubleValue: Number.NaN,
        intValue: null,
      }),
      expect.objectContaining({
        timestamp: 105n,
        valueType: 'Double',
        doubleValue: Number.POSITIVE_INFINITY,
        intValue: null,
      }),
      expect.objectContaining({
        timestamp: 106n,
        valueType: 'Double',
        doubleValue: Number.NEGATIVE_INFINITY,
        intValue: null,
      }),
    ])
  })
})

// searchSpans ships a compressed wire shape -- resource and scope as
// references into top-level maps, times as an offset plus a duration, no
// per-span traceID -- and this service is the single place it is decoded.
//
// That makes these the only tests standing between a decoding bug and every
// view silently rendering wrong data: the waterfall, the detail panel and the
// search results all read the rehydrated SpanData and none of them can tell
// that a resource was mismatched or a timestamp reconstructed wrongly.
describe('telemetryAPI.searchSpans rehydration', () => {
  const wire = {
    traceID: 'abc123',
    traceStart: '1700000000000000000',
    unplacedSpanCount: 0,
    resources: {
      '7': {
        attributes: [
          { key: 'service.name', value: 'checkout', type: 'string' },
        ],
        droppedAttributesCount: 0,
      },
      '9': {
        attributes: [
          { key: 'service.name', value: 'payments', type: 'string' },
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
          name: 'root',
          kind: 'Server',
          start: 0,
          dur: 5_000_000,
          attributes: [],
          events: [
            {
              name: 'e',
              timestamp: '1700000000000000123',
              droppedAttributesCount: 0,
              attributes: [],
            },
          ],
          links: [],
          r: 7,
          s: 3,
          droppedAttributesCount: 0,
          droppedEventsCount: 0,
          droppedLinksCount: 0,
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
          name: 'child',
          kind: 'Internal',
          start: 1_200_000_000,
          dur: 3_000_000,
          attributes: [],
          events: [],
          links: [],
          r: 9,
          s: 3,
          droppedAttributesCount: 0,
          droppedEventsCount: 0,
          droppedLinksCount: 0,
          statusCode: 'Ok',
          statusMessage: '',
        },
        depth: 1,
        matched: true,
      },
    ],
  }

  async function fetchTrace() {
    stubRpcResponse({ jsonrpc: '2.0', id: 1, result: wire })
    return telemetryAPI.searchSpans('abc123')
  }

  it('resolves each span against its own resource, not the first one', async () => {
    const trace = await fetchTrace()
    // Two spans, two different resources -- a decoder that ignored `r` would
    // still look plausible if every span shared one.
    expect(trace.spans[0].spanData.resource.attributes[0].value).toBe(
      'checkout'
    )
    expect(trace.spans[1].spanData.resource.attributes[0].value).toBe(
      'payments'
    )
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

    // The child starts 1.2s in and lasts 3ms: end is start + dur, not
    // baseline + dur, which is the mistake the shape invites.
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
    stubRpcResponse({
      jsonrpc: '2.0',
      id: 1,
      result: {
        ...wire,
        spans: [wire.spans[0], { ...wire.spans[0], depth: 1 }],
      },
    })
    const trace = await telemetryAPI.searchSpans('abc123')
    // Copying would rebuild client-side the duplication the wire format
    // exists to remove.
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

  // unplacedSpanCount and the per-span salvaged/cyclePoint flags are what the
  // UI reads to warn about a trace with a broken parent chain. A decoder that
  // dropped or defaulted these wrongly would make a malformed trace look
  // healthy, or an ordinary trace look broken.
  it('preserves unplacedSpanCount when the wire reports zero', async () => {
    const trace = await fetchTrace()
    expect(trace.unplacedSpanCount).toBe(0)
  })

  it('preserves unplacedSpanCount when the wire reports spans stranded on a cycle', async () => {
    stubRpcResponse({
      jsonrpc: '2.0',
      id: 1,
      result: { ...wire, unplacedSpanCount: 3 },
    })
    const trace = await telemetryAPI.searchSpans('abc123')
    expect(trace.unplacedSpanCount).toBe(3)
  })

  it('defaults unplacedSpanCount to 0 when an older store omits the field', async () => {
    const { unplacedSpanCount: _drop, ...wireWithoutField } = wire
    stubRpcResponse({
      jsonrpc: '2.0',
      id: 1,
      result: wireWithoutField,
    })
    const trace = await telemetryAPI.searchSpans('abc123')
    expect(trace.unplacedSpanCount).toBe(0)
  })

  it('does not invent salvaged or cyclePoint on spans the wire never flagged', async () => {
    const trace = await fetchTrace()
    for (const span of trace.spans) {
      expect('salvaged' in span).toBe(false)
      expect('cyclePoint' in span).toBe(false)
    }
  })

  it('preserves salvaged and cyclePoint on a span recovered from a cycle', async () => {
    stubRpcResponse({
      jsonrpc: '2.0',
      id: 1,
      result: {
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
      },
    })
    const trace = await telemetryAPI.searchSpans('abc123')

    const recovered = trace.spans.find(s => s.spanData.spanID === 'cccc')!
    expect(recovered.salvaged).toBe(true)
    expect(recovered.cyclePoint).toBe(false)

    const offender = trace.spans.find(s => s.spanData.spanID === 'dddd')!
    expect(offender.salvaged).toBe(true)
    expect(offender.cyclePoint).toBe(true)

    // The two healthy spans from the base fixture are untouched.
    expect('salvaged' in trace.spans[0]).toBe(false)
    expect('salvaged' in trace.spans[1]).toBe(false)
  })
})

// What the client puts on the wire, which nothing else here looks at.
//
// Every test above stubs fetch and reads the response, so all of them pass
// whether the request carried named parameters, positional ones, or nothing
// recognisable at all. That blindness has cost twice already: deleteMetricStream
// was registered under a plural name it does not take, and `params: {}` was
// rejected for every method with nothing to name. Both reached a running server
// before anyone noticed, because a green suite said nothing about the request.
//
// So these pin the request instead of the response. They are deliberately exact
// -- a full deep-equal on params rather than a check that some key is present --
// because the failures worth catching are a renamed key, an extra key, and a
// silent return to positional arrays, and a loose assertion sees none of them.
function captureRequest() {
  const fetchMock = vi.fn().mockResolvedValue({
    ok: true,
    json: async () => ({ jsonrpc: '2.0', id: 1, result: [] }),
  })
  vi.stubGlobal('fetch', fetchMock)
  return () => JSON.parse(fetchMock.mock.calls[0][1].body)
}

describe('request parameters', () => {
  // Named methods, with the exact object each one is expected to send.
  // toNanoseconds renders milliseconds as a decimal string, hence '2000000'.
  const named: [string, () => Promise<unknown>, Record<string, unknown>][] = [
    [
      'searchAttributes',
      () => telemetryAPI.searchAttributes('http'),
      { term: 'http' },
    ],
    [
      'getAttributesByTraceID',
      () => telemetryAPI.getAttributesByTraceID('abc'),
      { traceID: 'abc' },
    ],
    [
      'searchTraces',
      () => telemetryAPI.searchTraces(2, 5),
      { startTime: '2000000', endTime: '5000000' },
    ],
    [
      'searchLogs',
      () => telemetryAPI.searchLogs(2, 5),
      { startTime: '2000000', endTime: '5000000' },
    ],
    [
      'searchMetricSummaries',
      () => telemetryAPI.searchMetricSummaries(2, 5),
      { startTime: '2000000', endTime: '5000000' },
    ],
    [
      'getTraceSpanCount',
      () => telemetryAPI.getTraceSpanCount('abc'),
      { traceID: 'abc' },
    ],
    [
      'deleteMetricStream',
      () => telemetryAPI.deleteMetricStream('s1'),
      { streamID: 's1' },
    ],
  ]

  it.each(named)(
    '%s sends exactly its named parameters',
    async (method, invoke, expected) => {
      const sent = captureRequest()
      await invoke().catch(() => {})
      const request = sent()
      expect(request.method).toBe(method)
      expect(Array.isArray(request.params)).toBe(false)
      expect(request.params).toEqual(expected)
    }
  )

  // searchSpans is separate: it returns an object rather than an array, so the
  // shared stub's `result: []` would fail to rehydrate before the assertion runs.
  it('searchSpans sends its traceID by name', async () => {
    const sent = captureRequest()
    await telemetryAPI.searchSpans('abc123').catch(() => {})
    expect(sent().params).toEqual({ traceID: 'abc123' })
  })

  // The reason the ternaries went. An omitted query must be an absent key --
  // not null, and not a third array slot -- because the store reads a present
  // `query` as a filter to apply and would return a narrowed result for a
  // search the user never typed.
  //
  // Note what this does *not* pin: `named` dropping undefined keys is not
  // observable here, because JSON.stringify omits undefined-valued keys anyway.
  // Deleting that filter leaves the wire bytes identical, so no test at this
  // level can fail on it. The filter earns its place by making the intent
  // explicit and the return type honest, not by changing the request. The
  // distinction that does survive serialisation is null and [] -- both real
  // values, both sent -- which is what the getMetric tests cover.
  it('omits query entirely when no query tree is supplied', async () => {
    const sent = captureRequest()
    await telemetryAPI.searchTraces(2, 5).catch(() => {})
    expect('query' in sent().params).toBe(false)
  })

  it('includes query when a query tree is supplied', async () => {
    const tree = {
      id: 'q1',
      type: 'condition',
      query: {
        field: { name: 'name', type: 'string', searchScope: 'global' },
        operator: { symbol: 'contains' },
        value: 'checkout',
      },
    } as unknown as QueryNode

    const sent = captureRequest()
    await telemetryAPI.searchTraces(2, 5, tree).catch(() => {})
    const params = sent().params
    expect(Object.keys(params).sort()).toEqual([
      'endTime',
      'query',
      'startTime',
    ])
    expect(params.query).toMatchObject({ id: 'q1', type: 'condition' })
  })

  it('includes a trace result limit without requiring a query tree', async () => {
    const sent = captureRequest()
    await telemetryAPI.searchTraces(2, 5, undefined, 250).catch(() => {})
    expect(sent().params).toEqual({
      startTime: '2000000',
      endTime: '5000000',
      limit: 250,
    })
  })

  it.each([
    ['logs', () => telemetryAPI.searchLogs(2, 5, undefined, 250), 'searchLogs'],
    [
      'metrics',
      () => telemetryAPI.searchMetricSummaries(2, 5, undefined, 250),
      'searchMetricSummaries',
    ],
  ])(
    'includes a %s result limit without requiring a query tree',
    async (_signal, invoke, method) => {
      const sent = captureRequest()
      await invoke().catch(() => {})
      expect(sent()).toMatchObject({
        method,
        params: {
          startTime: '2000000',
          endTime: '5000000',
          limit: 250,
        },
      })
      expect('query' in sent().params).toBe(false)
    }
  )

  it.each([
    [
      'traces',
      () =>
        telemetryAPI.searchTraces(2, 5, undefined, 25, {
          field: 'duration',
          direction: 'desc',
        }),
      'searchTraces',
      'duration',
    ],
    [
      'logs',
      () =>
        telemetryAPI.searchLogs(2, 5, undefined, 25, {
          field: 'severity',
          direction: 'asc',
        }),
      'searchLogs',
      'severity',
    ],
    [
      'metrics',
      () =>
        telemetryAPI.searchMetricSummaries(2, 5, undefined, 25, {
          field: 'dataPointCount',
          direction: 'desc',
        }),
      'searchMetricSummaries',
      'dataPointCount',
    ],
  ])(
    'includes the selected %s sort with a result limit',
    async (_signal, invoke, method, field) => {
      const sent = captureRequest()
      await invoke().catch(() => {})
      expect(sent()).toMatchObject({
        method,
        params: {
          limit: 25,
          sort: { field, direction: field === 'severity' ? 'asc' : 'desc' },
        },
      })
    }
  )

  // The deliberate exceptions. parseIDParams reads the whole params array as
  // the id list, so wrapping it in an object would nest the array a level
  // deeper and delete nothing.
  it.each([
    [
      'deleteSpansByTraceID',
      () => telemetryAPI.deleteTraces(['a', 'b']),
      ['a', 'b'],
    ],
    ['deleteLogByID', () => telemetryAPI.deleteLogByID('log-1'), ['log-1']],
  ])(
    '%s stays positional, because its params are the ids',
    async (method, invoke, expected) => {
      const sent = captureRequest()
      await invoke().catch(() => {})
      const request = sent()
      expect(request.method).toBe(method)
      expect(request.params).toEqual(expected)
    }
  )

  // Methods with nothing to name send no params at all, rather than an empty
  // object or an empty array.
  it.each([
    ['getTraceAttributes', () => telemetryAPI.getTraceAttributes()],
    ['getLogAttributes', () => telemetryAPI.getLogAttributes()],
    ['getMetricAttributes', () => telemetryAPI.getMetricAttributes()],
    ['clearTraces', () => telemetryAPI.clearTraces()],
    ['clearLogs', () => telemetryAPI.clearLogs()],
    ['clearMetrics', () => telemetryAPI.clearMetrics()],
  ])('%s sends no params', async (method, invoke) => {
    const sent = captureRequest()
    await invoke().catch(() => {})
    const request = sent()
    expect(request.method).toBe(method)
    expect('params' in request).toBe(false)
  })

  it.each([
    ['searchTraces', () => telemetryAPI.searchTraces(null, null)],
    ['searchLogs', () => telemetryAPI.searchLogs(null, null)],
    [
      'searchMetricSummaries',
      () => telemetryAPI.searchMetricSummaries(null, null),
    ],
    ['getMetric', () => telemetryAPI.getMetric('stream-1', null, null)],
    [
      'getMetricAggregate',
      () =>
        telemetryAPI.getMetricAggregate(
          'stream-1',
          null,
          null,
          10,
          null,
          [],
          0
        ),
    ],
  ])('%s serializes unbounded bounds as null', async (method, invoke) => {
    const sent = captureRequest()
    await invoke().catch(() => {})
    expect(sent()).toMatchObject({
      method,
      params: { startTime: null, endTime: null },
    })
  })

  it('getMetric sends the final named parameter contract exactly', async () => {
    const sent = captureRequest()
    await telemetryAPI
      .getMetric(
        'stream-1',
        2,
        5,
        10,
        ['series-1'],
        [0.5],
        7,
        8,
        9,
        ['selected-1'],
        'America/New_York',
        ['datapoints-1'],
        3
      )
      .catch(() => {})
    expect(sent().params).toEqual({
      streamID: 'stream-1',
      startTime: '2000000',
      endTime: '5000000',
      targetBuckets: 10,
      seriesIDs: ['series-1'],
      quantiles: [0.5],
      tzOffsetNs: 7,
      viewBuckets: 8,
      sparklineBuckets: 9,
      selectedSeriesIDs: ['selected-1'],
      tzName: 'America/New_York',
      datapointSeriesIDs: ['datapoints-1'],
      datapointSeriesLimit: 3,
    })
  })

  it('getMetricAggregate sends the final named parameter contract exactly', async () => {
    const sent = captureRequest()
    await telemetryAPI
      .getMetricAggregate(
        'stream-1',
        2n,
        5n,
        10,
        ['series-1'],
        [0.95],
        7,
        8,
        ['selected-1'],
        'UTC'
      )
      .catch(() => {})
    expect(sent().params).toEqual({
      streamID: 'stream-1',
      startTime: '2',
      endTime: '5',
      targetBuckets: 10,
      seriesIDs: ['series-1'],
      quantiles: [0.95],
      tzOffsetNs: 7,
      viewBuckets: 8,
      selectedSeriesIDs: ['selected-1'],
      tzName: 'UTC',
    })
  })
})
