import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  telemetryAPI,
  isAbortError,
  JsonRpcError,
  RequestAbortedError,
} from './telemetry-service'
import type { QueryNode } from '@/components/shared/Search/queryTree'
import { SPAN_FIELDS, type FieldDefinition } from '@/constants/fields'
import { OPERATORS } from '@/constants/operators'
import { getOperatorsForFieldType } from '@/constants/operators'
import type {
  JsonMetricData,
  JsonLogData,
  JsonTraceData,
  JsonTraceSummary,
} from '@/types/wire-types'
import { parseDuration } from '@/utils/time'

// The backend signals not-found with JSON-RPC errors (one convention across
// all signals; see internal/server/errors.go). getMetric's callers expect
// MetricData | null, so the service translates exactly one code -- -32003,
// metric not found -- back to null. These tests pin that translation.

type StubRpcResponse<T> = {
  jsonrpc: '2.0'
  id: number
  result?: T
  error?: { code: number; message: string }
}

function stubRpcResponse<T>(body: StubRpcResponse<T>) {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockResolvedValue({
      ok: true,
      json: async () => body,
    })
  )
}

function stubRpcResult<T>(result: T) {
  stubRpcResponse({ jsonrpc: '2.0', id: 1, result })
}

function metricResult(overrides: Partial<JsonMetricData> = {}): JsonMetricData {
  return {
    lastSeenNs: null,
    id: 'some-stream',
    name: 'test.gauge',
    description: '',
    metadata: [],
    unit: '1',
    metricType: 'Gauge',
    aggregationTemporalityCode: null,
    aggregationTemporality: null,
    isMonotonic: false,
    resourceDroppedAttributesCount: 0,
    resource: { attributes: [], droppedAttributesCount: 0 },
    scopeName: '',
    scopeVersion: '',
    scopeDroppedAttributesCount: 0,
    scope: { name: '', version: '', attributes: [], droppedAttributesCount: 0 },
    timeseries: [],
    aggregate: null,
    scalarAggregate: null,
    datapointCount: 0,
    boundsMismatch: null,
    window: {
      requested: { startNs: null, endNs: null },
      effective: { startNs: null, endNs: null },
    },
    ...overrides,
  }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('request cancellation', () => {
  it('recognizes only service and platform abort errors', () => {
    expect(isAbortError(new RequestAbortedError())).toBe(true)
    expect(isAbortError(new DOMException('aborted', 'AbortError'))).toBe(true)
    expect(isAbortError(new DOMException('failed', 'NetworkError'))).toBe(false)
    expect(isAbortError(new Error('aborted'))).toBe(false)
  })

  it('keeps non-abort platform failures usable after classification', () => {
    const failure = new DOMException('connection lost', 'NetworkError')

    if (isAbortError(failure)) throw new Error('expected a non-abort failure')

    expect(failure.message).toBe('connection lost')
  })

  it('normalizes a platform abort without losing the request signal', async () => {
    const fetchMock = vi
      .fn()
      .mockRejectedValue(new DOMException('aborted', 'AbortError'))
    vi.stubGlobal('fetch', fetchMock)
    const controller = new AbortController()

    await expect(
      telemetryAPI.searchSpans('trace-1', undefined, controller.signal)
    ).rejects.toBeInstanceOf(RequestAbortedError)
    expect(fetchMock).toHaveBeenCalledWith(
      '/rpc',
      expect.objectContaining({ signal: controller.signal })
    )
  })
})

describe('telemetryAPI.getLog', () => {
  it('revives an int64 body value as bigint', async () => {
    const result: JsonLogData = {
      id: 'log-1',
      timestamp: '100',
      observedTimestamp: '101',
      traceID: null,
      spanID: null,
      severityText: 'INFO',
      severityNumber: 9,
      body: { kind: 'int64', value: '9223372036854775807' },
      resource: { attributes: [], droppedAttributesCount: 0 },
      scope: {
        name: '',
        version: '',
        attributes: [],
        droppedAttributesCount: 0,
      },
      droppedAttributesCount: 0,
      flags: 0,
      eventName: '',
      attributes: [],
    }
    stubRpcResult(result)

    const log = await telemetryAPI.getLog('log-1')

    expect(log.body).toEqual({
      kind: 'int64',
      value: 9_223_372_036_854_775_807n,
    })
  })
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
    stubRpcResult(
      metricResult({
        unit: 'bytes',
        window: {
          requested: { startNs: null, endNs: null },
          effective: { startNs: '10', endNs: '20' },
        },
      })
    )
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
    stubRpcResult(
      metricResult({
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
      })
    )

    const metric = await telemetryAPI.getMetric('some-stream', 0, 1)
    expect(metric!.timeseries[0]!.datapoints[0]).toMatchObject({
      doubleValue: 1,
      intValue: null,
    })
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

  it('revives every received metric integer field without losing precision', async () => {
    const wire = metricResult({
      timeseries: [
        {
          attributesKey: 'series-1',
          attributes: [],
          resource: { attributes: [], droppedAttributesCount: 0 },
          datapoints: [
            {
              id: 'gauge-min',
              timestamp: '100',
              timestampMs: 0,
              startTime: '0',
              flags: 0,
              exemplars: [],
              metricType: 'Gauge',
              doubleValue: null,
              intValue: '-9223372036854775808',
              valueType: 'Int',
            },
            {
              id: 'sum-max',
              timestamp: '101',
              timestampMs: 0,
              startTime: '0',
              flags: 0,
              exemplars: [],
              metricType: 'Sum',
              doubleValue: null,
              intValue: '9223372036854775807',
              valueType: 'Int',
              isMonotonic: true,
              aggregationTemporalityCode: 1,
              aggregationTemporality: 'Delta',
            },
            {
              id: 'histogram-max',
              timestamp: '102',
              timestampMs: 0,
              startTime: '0',
              flags: 0,
              exemplars: [],
              metricType: 'Histogram',
              count: '18446744073709551615',
              sum: 1,
              min: 1,
              max: 1,
              bucketCounts: ['0', '9007199254740993', '18446744073709551615'],
              explicitBounds: [0, 1],
              quantiles: null,
              aggregationTemporalityCode: -1,
              aggregationTemporality: 'Unknown (-1)',
            },
            {
              id: 'exponential-max',
              timestamp: '103',
              timestampMs: 0,
              startTime: '0',
              flags: 0,
              exemplars: [],
              metricType: 'ExponentialHistogram',
              count: '18446744073709551615',
              sum: 1,
              min: 1,
              max: 1,
              scale: 0,
              zeroCount: '9007199254740993',
              zeroThreshold: 0,
              positiveBucketOffset: 0,
              positiveBucketCounts: ['18446744073709551615'],
              negativeBucketOffset: 0,
              negativeBucketCounts: [],
              quantiles: null,
              aggregationTemporalityCode: 1,
              aggregationTemporality: 'Delta',
            },
          ],
          stats: null,
          datapointCount: 4,
          lastSeenNs: '103',
          views: null,
          rateStats: null,
          sparkline: null,
        },
      ],
    })
    stubRpcResult(wire)

    const metric = await telemetryAPI.getMetric('some-stream', 0, 1)
    const datapoints = metric!.timeseries[0]!.datapoints

    expect(datapoints[0]).toMatchObject({
      intValue: -9_223_372_036_854_775_808n,
    })
    expect(datapoints[1]).toMatchObject({
      intValue: 9_223_372_036_854_775_807n,
    })
    expect(datapoints[2]).toMatchObject({
      count: 18_446_744_073_709_551_615n,
      bucketCounts: [0n, 9_007_199_254_740_993n, 18_446_744_073_709_551_615n],
      aggregationTemporalityCode: -1,
      aggregationTemporality: 'Unknown (-1)',
    })
    expect(datapoints[3]).toMatchObject({
      count: 18_446_744_073_709_551_615n,
      zeroCount: 9_007_199_254_740_993n,
      positiveBucketCounts: [18_446_744_073_709_551_615n],
      negativeBucketCounts: [],
    })
    expect(wire.timeseries[0]!.datapoints[2]).toMatchObject({
      count: '18446744073709551615',
      bucketCounts: ['0', '9007199254740993', '18446744073709551615'],
    })
  })

  it('keeps absent histogram statistics distinct from present zero', async () => {
    const base = {
      timestampMs: 0,
      startTime: '0',
      flags: 0,
      metricType: 'Histogram' as const,
      count: '0',
      bucketCounts: ['0'],
      explicitBounds: [],
      quantiles: null,
      aggregationTemporalityCode: 1,
      aggregationTemporality: 'Delta',
      exemplars: [],
    }
    stubRpcResult(
      metricResult({
        metricType: 'Histogram',
        timeseries: [
          {
            attributesKey: 'series-1',
            attributes: [],
            resource: { attributes: [], droppedAttributesCount: 0 },
            datapoints: [
              {
                ...base,
                id: 'absent',
                timestamp: '1',
                sum: null,
                min: null,
                max: null,
              },
              {
                ...base,
                id: 'zero',
                timestamp: '2',
                sum: 0,
                min: 0,
                max: 0,
              },
            ],
            stats: null,
            datapointCount: 2,
            lastSeenNs: '2',
            views: null,
            rateStats: null,
            sparkline: null,
          },
        ],
      })
    )

    const metric = await telemetryAPI.getMetric('some-stream', 0, 2)
    const [absent, zero] = metric!.timeseries[0]!.datapoints
    expect(absent).toMatchObject({ sum: null, min: null, max: null })
    expect(zero).toMatchObject({ sum: 0, min: 0, max: 0 })
  })

  it('decodes recursive attribute int64 values and exceptional double bits once', async () => {
    stubRpcResult(
      metricResult({
        metadata: [
          {
            id: 'metadata-1',
            key: 'payload',
            value: {
              kind: 'map',
              value: [
                {
                  key: 'count',
                  value: { kind: 'int64', value: '9223372036854775807' },
                },
                {
                  key: 'negativeZero',
                  value: { kind: 'double', value: '0x8000000000000000' },
                },
                {
                  key: 'list',
                  value: {
                    kind: 'array',
                    value: [{ kind: 'double', value: '0x7ff0000000000000' }],
                  },
                },
              ],
            },
          },
        ],
      })
    )

    const value = (await telemetryAPI.getMetric('some-stream', 0, 1))!
      .metadata[0]!.value
    expect(value).toMatchObject({ kind: 'map' })
    if (value.kind !== 'map') throw new Error('Expected a map value')
    expect(value.value[0]!.value).toEqual({
      kind: 'int64',
      value: 9_223_372_036_854_775_807n,
    })
    expect(
      Object.is((value.value[1]!.value as { value: number }).value, -0)
    ).toBe(true)
    expect(
      (value.value[2]!.value as { value: { value: number }[] }).value[0]!.value
    ).toBe(Number.POSITIVE_INFINITY)
  })

  it('detects distinct exceptional-double conflicts before decoding', async () => {
    stubRpcResult(
      metricResult({
        metadata: [
          {
            id: 'metadata-1',
            key: 'payload',
            value: { kind: 'double', value: '0x7ff8000000000001' },
          },
          {
            id: 'metadata-2',
            key: 'payload',
            value: { kind: 'double', value: '0x7ff8000000000002' },
          },
          {
            id: 'metadata-3',
            key: 'nested',
            value: {
              kind: 'map',
              value: [
                {
                  key: 'payload',
                  value: { kind: 'double', value: '0x7ff8000000000001' },
                },
                {
                  key: 'payload',
                  value: { kind: 'double', value: '0x7ff8000000000002' },
                },
              ],
            },
          },
        ],
      })
    )

    const metadata = (await telemetryAPI.getMetric('some-stream', 0, 1))!
      .metadata
    expect(metadata[0]!.hasConflict).toBe(true)
    expect(metadata[1]!.hasConflict).toBe(true)
    expect(metadata[2]!.value).toMatchObject({
      kind: 'map',
      conflictingKeys: ['payload'],
    })
  })
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

    const [field] = await telemetryAPI.getTraceAttributes()

    expect(field).toMatchObject({
      name: 'items',
      type: 'array',
      attributeScope: 'span',
    })
    if (field?.searchScope !== 'attribute') {
      throw new Error('Expected an attribute field')
    }
    expect(
      getOperatorsForFieldType(field.type).map(operator => operator.symbol)
    ).toEqual([OPERATORS.CONTAINS.symbol, OPERATORS.NOT_CONTAINS.symbol])
  })
})

describe('telemetryAPI.searchTraces', () => {
  it('promotes wire timestamps and durations to bigint values', async () => {
    const summaries: JsonTraceSummary[] = [
      {
        traceID: 'trace-1',
        hasRootSpan: true,
        rootSpan: { serviceName: 'checkout', name: 'GET /checkout' },
        startTime: '1700000000000000000',
        durationNs: '12345',
        spanCount: 2,
        errorCount: 0,
      },
    ]
    stubRpcResult(summaries)

    await expect(telemetryAPI.searchTraces(0, 1)).resolves.toMatchObject([
      {
        traceID: 'trace-1',
        startTime: 1700000000000000000n,
        durationNs: 12345n,
      },
    ])
  })

  it('preserves native bigint string parsing and nullable durations', async () => {
    stubRpcResult([
      {
        traceID: 'trace-1',
        hasRootSpan: false,
        rootSpan: null,
        startTime: '-9223372036854775808',
        durationNs: null,
        spanCount: 0,
        errorCount: 0,
      },
      {
        traceID: 'trace-2',
        hasRootSpan: false,
        rootSpan: null,
        startTime: '0x10',
        durationNs: '+12',
        spanCount: 0,
        errorCount: 0,
      },
    ] satisfies JsonTraceSummary[])

    await expect(telemetryAPI.searchTraces(0, 1)).resolves.toMatchObject([
      { startTime: -9_223_372_036_854_775_808n, durationNs: null },
      { startTime: 16n, durationNs: 12n },
    ])
  })

  it('rejects non-string bigint wire values before native coercion', async () => {
    stubRpcResult([
      {
        traceID: 'trace-1',
        hasRootSpan: false,
        rootSpan: null,
        startTime: true,
        durationNs: null,
        spanCount: 0,
        errorCount: 0,
      },
    ])

    await expect(telemetryAPI.searchTraces(0, 1)).rejects.toThrow(
      'Invalid bigint wire value: expected string, got boolean'
    )
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
          links: [],
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
  } satisfies JsonTraceData

  async function fetchTrace() {
    stubRpcResult<JsonTraceData>(wire)
    return telemetryAPI.searchSpans('abc123')
  }

  it('resolves each span against its own resource, not the first one', async () => {
    const trace = await fetchTrace()
    // Two spans, two different resources -- a decoder that ignored `r` would
    // still look plausible if every span shared one.
    expect(trace.spans[0].spanData.resource.attributes[0].value).toEqual({
      kind: 'string',
      value: 'checkout',
    })
    expect(trace.spans[1].spanData.resource.attributes[0].value).toEqual({
      kind: 'string',
      value: 'payments',
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
    stubRpcResult<JsonTraceData>({
      ...wire,
      spans: [wire.spans[0], { ...wire.spans[0], depth: 1 }],
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
    stubRpcResult<JsonTraceData>({ ...wire, unplacedSpanCount: 3 })
    const trace = await telemetryAPI.searchSpans('abc123')
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
    stubRpcResult<JsonTraceData>({
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
    const trace = await telemetryAPI.searchSpans('abc123')

    const recovered = trace.spans.find(s => s.spanData.spanID === 'cccc')!
    expect(recovered.salvaged).toBe(true)
    expect(recovered.cyclePoint).toBe(false)

    const cyclePoint = trace.spans.find(s => s.spanData.spanID === 'dddd')!
    expect(cyclePoint.salvaged).toBe(true)
    expect(cyclePoint.cyclePoint).toBe(true)

    // The two healthy spans from the base fixture are untouched.
    expect('salvaged' in trace.spans[0]).toBe(false)
    expect('salvaged' in trace.spans[1]).toBe(false)
  })
})

describe('telemetryAPI metric bigint boundary', () => {
  it('keeps only the established lastSeenNs missing-field normalizations', async () => {
    // SAFETY: This fixture intentionally omits typed lastSeenNs fields to exercise their parser normalization.
    const result = metricResult({
      timeseries: [
        {
          attributesKey: 'series-1',
          attributes: [],
          resource: { attributes: [], droppedAttributesCount: 0 },
          datapoints: [],
          stats: null,
          datapointCount: 0,
          lastSeenNs: undefined as never,
          views: [
            {
              bucketStart: '9223372036854775807',
              sampleCount: 1,
              sum: 1,
              avg: 1,
              rate: 1,
              slope: null,
              hasReset: false,
            },
          ],
          rateStats: null,
          sparkline: [{ timestamp: '-1', value: 1 }],
        },
      ],
    }) as { lastSeenNs?: unknown; timeseries: { lastSeenNs?: unknown }[] }
    delete result.lastSeenNs
    delete result.timeseries[0]!.lastSeenNs
    stubRpcResult(result)

    const metric = await telemetryAPI.getMetric('some-stream', 0, 1)
    expect(metric!.lastSeenNs).toBeNull()
    expect(metric!.timeseries[0]!.lastSeenNs).toBeNull()
    expect(metric!.timeseries[0]!.views![0]!.bucketStart).toBe(
      9_223_372_036_854_775_807n
    )
    expect(metric!.timeseries[0]!.sparkline![0]!.timestamp).toBe(-1n)
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
  const named = [
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
  ] as const

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
        field: { searchScope: 'global' },
        operator: OPERATORS.CONTAINS,
        value: 'checkout',
      },
    } satisfies QueryNode

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

  it('serializes an exact duration boundary query', async () => {
    const duration = parseDuration('9007199254740993ns')
    if (duration === null) throw new Error('Expected valid duration')
    const field = SPAN_FIELDS.find(
      (field): field is Exclude<FieldDefinition, { searchScope: 'global' }> =>
        'name' in field && field.name === 'duration'
    )
    if (!field) throw new Error('Expected duration field')
    const tree = {
      id: 'duration-boundary',
      type: 'condition',
      query: {
        field,
        operator: OPERATORS.EQUALS,
        value: duration.toString(),
      },
    } satisfies QueryNode

    const sent = captureRequest()
    await telemetryAPI.searchTraces(2, 5, tree).catch(() => {})
    expect(sent().params.query).toEqual({
      id: 'duration-boundary',
      type: 'condition',
      query: {
        field: { name: 'duration', type: 'int64', searchScope: 'field' },
        fieldOperator: '=',
        value: '9007199254740993',
      },
    })
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

  it('preserves omitted, empty, and null series selections', async () => {
    const omitted = captureRequest()
    await telemetryAPI.getMetric('stream-1', 2, 5).catch(() => {})
    expect('seriesIDs' in omitted().params).toBe(false)

    const empty = captureRequest()
    await telemetryAPI
      .getMetric('stream-1', 2, 5, undefined, [])
      .catch(() => {})
    expect(empty().params.seriesIDs).toEqual([])

    const unfiltered = captureRequest()
    await telemetryAPI
      .getMetricAggregate('stream-1', 2, 5, 10, null, [], 0)
      .catch(() => {})
    expect(unfiltered().params.seriesIDs).toBeNull()
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

  it('serializes exact bigint bounds without converting them through number', async () => {
    const sent = captureRequest()
    await telemetryAPI
      .searchTraces(9_223_372_036_854_775_807n, -9_223_372_036_854_775_808n)
      .catch(() => {})

    expect(sent().params).toEqual({
      startTime: '9223372036854775807',
      endTime: '-9223372036854775808',
    })
  })
})
