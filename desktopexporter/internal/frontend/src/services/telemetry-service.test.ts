import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  telemetryAPI,
  isAbortError,
  JsonRpcError,
  RequestAbortedError,
} from './telemetry-service'
import { SPAN_FIELDS } from '@/constants/fields'
import {
  getOperatorsForFieldType,
  OPERATORS,
  type FieldDefinition,
  type QueryNode,
} from '@/search/model'
import type {
  JsonMetricViewData,
  JsonLogData,
  JsonTraceLogSummary,
  JsonTraceData,
  JsonTraceSummary,
} from '@/types/wire-types'
import { parseDuration } from '@/utils/time'
import { parseQuery } from '@/components/shared/Search/queryParser'
import type { ReceivedHistogramDataPoint } from '@/types/api-types'

type StubRpcResponse<T> = {
  jsonrpc?: unknown
  id?: unknown
  result?: T
  error?: { code: number; message: string }
}

function stubRpcResponse<T>(body: StubRpcResponse<T>) {
  vi.stubGlobal(
    'fetch',
    vi.fn().mockImplementation(async (_url, init: RequestInit) => {
      const request = JSON.parse(String(init.body)) as { id: number }
      const responseBody = 'id' in body ? body : { ...body, id: request.id }
      return {
        ok: true,
        json: async () => responseBody,
      }
    })
  )
}

function stubRpcResult<T>(result: T) {
  stubRpcResponse({ jsonrpc: '2.0', result })
}

function metricResult(
  overrides: Partial<JsonMetricViewData> = {}
): JsonMetricViewData {
  return {
    lastSeenNs: null,
    metricRef: 'some-metric',
    name: 'test.gauge',
    description: '',
    metadata: [],
    unit: '1',
    metricType: 'Gauge',
    aggregationTemporalityCode: null,
    aggregationTemporality: null,
    isMonotonic: null,
    resourceDroppedAttributesCount: 0,
    resourceSchemaUrl: '',
    resource: { attributes: [], droppedAttributesCount: 0 },
    scopeName: '',
    scopeSchemaUrl: '',
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
  vi.restoreAllMocks()
})

describe('JSON-RPC response identity', () => {
  it('accepts a successful response with the numeric request ID', async () => {
    stubRpcResult('cleared')

    await expect(telemetryAPI.clearTraces()).resolves.toBe('cleared')
  })

  it('preserves an RPC error with the numeric request ID', async () => {
    stubRpcResponse({
      jsonrpc: '2.0',
      error: { code: -32009, message: 'Invalid request' },
    })

    await expect(telemetryAPI.clearTraces()).rejects.toMatchObject({
      name: 'JsonRpcError',
      code: -32009,
      message: 'Invalid request',
    })
  })

  it.each([
    ['missing', undefined],
    ['wrong', '1.0'],
    ['null', null],
  ])('rejects a %s JSON-RPC version', async (_case, jsonrpc) => {
    stubRpcResponse({ jsonrpc, result: 'cleared' })

    await expect(telemetryAPI.clearTraces()).rejects.toThrow(
      `Invalid JSON-RPC version: ${String(jsonrpc)}`
    )
  })

  it.each([
    ['missing', undefined],
    ['wrong numeric', -1],
    ['null', null],
    ['matching digits as a string', '0'],
  ])('rejects a %s response ID', async (_case, id) => {
    vi.spyOn(Math, 'random').mockReturnValue(0)
    stubRpcResponse({ jsonrpc: '2.0', id, result: 'cleared' })

    await expect(telemetryAPI.clearTraces()).rejects.toThrow(
      'JSON-RPC response ID does not match request ID'
    )
  })

  it('rejects an invalid ID before handling an RPC error', async () => {
    stubRpcResponse({
      jsonrpc: '2.0',
      id: null,
      error: { code: -32009, message: 'Invalid request' },
    })

    await expect(telemetryAPI.clearTraces()).rejects.toMatchObject({
      name: 'Error',
      message: 'JSON-RPC response ID does not match request ID',
    })
  })
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
      telemetryAPI.getTraceView('trace-1', undefined, controller.signal)
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
      logRef: 'log-1',
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

describe('telemetryAPI.getTraceLogSummaries', () => {
  it('revives exact timestamps and preserves nullable span IDs', async () => {
    const result: JsonTraceLogSummary[] = [
      {
        logRef: 'log-1',
        timestamp: '9223372036854775807',
        spanID: null,
        severityText: 'INFO',
        severityNumber: 9,
        serviceName: 'checkout',
        eventName: 'order.received',
        bodyPreview: 'received',
      },
    ]
    stubRpcResult(result)

    await expect(telemetryAPI.getTraceLogSummaries('trace-1')).resolves.toEqual([
      { ...result[0], timestamp: 9_223_372_036_854_775_807n },
    ])
  })
})

describe('telemetryAPI.getMetricView', () => {
  it('returns null when the backend reports metric not found (-32003)', async () => {
    stubRpcResponse({
      jsonrpc: '2.0',
      error: { code: -32003, message: 'Metric not found' },
    })
    await expect(
      telemetryAPI.getMetricView('some-metric', 0n, 1n)
    ).resolves.toBeNull()
  })

  it('rethrows JSON-RPC errors other than metric not found', async () => {
    stubRpcResponse({
      jsonrpc: '2.0',
      error: { code: -32009, message: 'Invalid Metric reference' },
    })
    const call = telemetryAPI.getMetricView('not-a-metric', 0n, 1n)
    await expect(call).rejects.toBeInstanceOf(JsonRpcError)
    await expect(call).rejects.toMatchObject({ code: -32009 })
  })

  it('parses a successful result into MetricViewData', async () => {
    stubRpcResult(
      metricResult({
        unit: 'bytes',
        resourceSchemaUrl: 'https://example.test/resource/1.0',
        scopeSchemaUrl: 'https://example.test/scope/1.0',
        window: {
          requested: { startNs: null, endNs: null },
          effective: { startNs: '10', endNs: '20' },
        },
      })
    )
    const metric = await telemetryAPI.getMetricView('some-metric', 0n, 1n)
    expect(metric).not.toBeNull()
    expect(metric!.name).toBe('test.gauge')
    expect(metric!.resourceSchemaUrl).toBe('https://example.test/resource/1.0')
    expect(metric!.scopeSchemaUrl).toBe('https://example.test/scope/1.0')
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
            seriesRef: 'series-1',
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
                doubleValue: '0x8000000000000000',
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
                    doubleValue: '0x7ff8000000000001',
                    intValue: null,
                    traceID: null,
                    spanID: null,
                    filteredAttributes: [],
                  },
                  {
                    timestamp: '105',
                    valueType: 'Double',
                    doubleValue: '0x7ff0000000000000',
                    intValue: null,
                    traceID: null,
                    spanID: null,
                    filteredAttributes: [],
                  },
                  {
                    timestamp: '106',
                    valueType: 'Double',
                    doubleValue: '0xfff0000000000000',
                    intValue: null,
                    traceID: null,
                    spanID: null,
                    filteredAttributes: [],
                  },
                  {
                    timestamp: '107',
                    valueType: 'Double',
                    doubleValue: '0xfff8000000000002',
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

    const metric = await telemetryAPI.getMetricView('some-metric', 0n, 1n)
    expect(metric!.timeseries[0]!.datapoints[0]).toMatchObject({
      intValue: null,
    })
    const datapoint = metric!.timeseries[0]!.datapoints[0]!
    expect(datapoint.metricType).toBe('Gauge')
    if (datapoint.metricType !== 'Gauge') throw new Error('expected Gauge')
    expect(Object.is(datapoint.doubleValue, -0)).toBe(true)
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
      // JavaScript exposes both payloads as NaN; stored/wire bit identity is
      // asserted by the store tests rather than inferred from Number.NaN.
      expect.objectContaining({
        timestamp: 107n,
        valueType: 'Double',
        doubleValue: Number.NaN,
        intValue: null,
      }),
    ])
  })

  it('revives received metric numeric fields without losing precision', async () => {
    const wire = metricResult({
      timeseries: [
        {
          seriesRef: 'series-1',
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
              aggregationTemporalityCode: 2,
              aggregationTemporality: 'Cumulative',
              delta: '18446744073709551615',
              isReset: false,
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
              sum: '0x7ff8000000000001',
              min: '0xfff0000000000000',
              max: '0x7ff0000000000000',
              bucketCounts: ['0', '9007199254740993', '18446744073709551615'],
              explicitBounds: ['0x8000000000000000', 1],
              quantiles: { '0.5': '0x7ff0000000000000' },
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
              zeroThreshold: '0x8000000000000000',
              positiveBucketOffset: 0,
              positiveBucketCounts: ['18446744073709551615'],
              negativeBucketOffset: 0,
              negativeBucketCounts: [],
              quantiles: null,
              aggregationTemporalityCode: 1,
              aggregationTemporality: 'Delta',
            },
            {
              id: 'sum-overflow',
              timestamp: '104',
              timestampMs: 0,
              startTime: '0',
              flags: 0,
              exemplars: [],
              metricType: 'Sum',
              doubleValue: 1,
              intValue: null,
              valueType: 'Double',
              isMonotonic: true,
              aggregationTemporalityCode: 2,
              aggregationTemporality: 'Cumulative',
              delta: '0x7ff0000000000000',
              isReset: false,
            },
          ],
          stats: null,
          datapointCount: 5,
          lastSeenNs: '104',
          views: null,
          rateStats: null,
          sparkline: null,
        },
      ],
    })
    stubRpcResult(wire)

    const metric = await telemetryAPI.getMetricView('some-metric', 0n, 1n)
    const datapoints = metric!.timeseries[0]!.datapoints

    expect(datapoints[0]).toMatchObject({
      intValue: -9_223_372_036_854_775_808n,
    })
    expect(datapoints[1]).toMatchObject({
      intValue: 9_223_372_036_854_775_807n,
      delta: 18_446_744_073_709_551_615n,
    })
    expect(datapoints[2]).toMatchObject({
      count: 18_446_744_073_709_551_615n,
      bucketCounts: [0n, 9_007_199_254_740_993n, 18_446_744_073_709_551_615n],
      aggregationTemporalityCode: -1,
      aggregationTemporality: 'Unknown (-1)',
    })
    const histogram = datapoints[2]!
    if (histogram.metricType !== 'Histogram') {
      throw new Error('expected Histogram')
    }
    expect(Number.isNaN(histogram.sum)).toBe(true)
    expect(histogram.min).toBe(Number.NEGATIVE_INFINITY)
    expect(histogram.max).toBe(Number.POSITIVE_INFINITY)
    expect(Object.is(histogram.explicitBounds[0], -0)).toBe(true)
    expect(histogram.quantiles).toEqual({
      '0.5': Number.POSITIVE_INFINITY,
    })
    expect(datapoints[3]).toMatchObject({
      count: 18_446_744_073_709_551_615n,
      zeroCount: 9_007_199_254_740_993n,
      positiveBucketCounts: [18_446_744_073_709_551_615n],
      negativeBucketCounts: [],
    })
    const exponential = datapoints[3]!
    if (exponential.metricType !== 'ExponentialHistogram') {
      throw new Error('expected ExponentialHistogram')
    }
    expect(Object.is(exponential.zeroThreshold, -0)).toBe(true)
    expect(datapoints[4]).toMatchObject({
      delta: Number.POSITIVE_INFINITY,
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
            seriesRef: 'series-1',
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

    const metric = await telemetryAPI.getMetricView('some-metric', 0n, 2n)
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

    const value = (await telemetryAPI.getMetricView('some-metric', 0n, 1n))!
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

    const metadata = (await telemetryAPI.getMetricView('some-metric', 0n, 1n))!
      .metadata
    expect(metadata[0]!.hasConflict).toBe(true)
    expect(metadata[1]!.hasConflict).toBe(true)
    expect(metadata[2]!.value).toMatchObject({
      kind: 'map',
      conflictingKeys: ['payload'],
    })
  })
})

describe('exact received metric API', () => {
  it('decodes exact identity and computed series summaries', async () => {
    stubRpcResult({
      metricRef: 'metric-1',
      name: 'requests',
      description: '',
      unit: '1',
      metadata: [],
      metricType: 'Sum',
      aggregationTemporalityCode: 2,
      isMonotonic: true,
      resource: {
        droppedAttributesCount: 0,
        schemaUrl: '',
        attributes: [
          {
            key: 'nested',
            value: {
              kind: 'map',
              value: [
                {
                  key: 'limit',
                  value: { kind: 'int64', value: '9007199254740993' },
                },
              ],
            },
          },
        ],
      },
      scope: {
        name: 'sdk',
        version: '1',
        attributes: [],
        droppedAttributesCount: 0,
        schemaUrl: 'scope-schema',
      },
      series: [
        {
          seriesRef: 'series-1',
          attributes: [],
          datapointCount: '18446744073709551615',
          firstDatapointTimestamp: '1',
          lastDatapointTimestamp: null,
        },
      ],
    })

    const metric = await telemetryAPI.getMetric('metric-1')
    expect(metric).toMatchObject({
      metricRef: 'metric-1',
      aggregationTemporalityCode: 2,
      isMonotonic: true,
    })
    expect(metric!.series[0]).toMatchObject({
      datapointCount: 18446744073709551615n,
      firstDatapointTimestamp: 1n,
      lastDatapointTimestamp: null,
    })
    expect(metric!.resource.attributes[0]!.value).toMatchObject({
      kind: 'map',
      value: [
        { key: 'limit', value: { kind: 'int64', value: 9007199254740993n } },
      ],
    })
  })

  it('decodes exact histogram values, all exemplars, and optional presence', async () => {
    stubRpcResult({
      metricRef: 'metric-1',
      name: 'latency',
      description: 'received report',
      unit: 'ms',
      metadata: [],
      metricType: 'Histogram',
      aggregationTemporalityCode: 1,
      resource: {
        attributes: [],
        droppedAttributesCount: 2,
        schemaUrl: 'resource-schema',
      },
      scope: {
        name: 'sdk',
        version: '1',
        attributes: [],
        droppedAttributesCount: 3,
        schemaUrl: 'scope-schema',
      },
      seriesRef: 'series-1',
      attributes: [],
      datapoints: [
        {
          datapointID: 'point-1',
          timestamp: '18446744073709551615',
          startTime: '0',
          flags: 1,
          count: '18446744073709551615',
          sum: 0,
          max: '0x8000000000000000',
          bucketCounts: ['18446744073709551615'],
          explicitBounds: ['0x7ff0000000000000'],
          exemplars: [
            {
              timestamp: '9',
              valueType: 'Int',
              intValue: '-9223372036854775808',
              doubleValue: null,
              traceID: null,
              spanID: null,
              filteredAttributes: [],
            },
            {
              timestamp: '10',
              valueType: 'Double',
              intValue: null,
              doubleValue: '0x8000000000000000',
              traceID: '00000000000000000000000000000001',
              spanID: '0000000000000001',
              filteredAttributes: [],
            },
          ],
        },
      ],
    })

    const selected = await telemetryAPI.getMetricSeries(
      'metric-1',
      'series-1',
      null,
      18446744073709551615n
    )
    const point = selected!.datapoints[0]! as ReceivedHistogramDataPoint
    expect(point).toMatchObject({
      timestamp: 18446744073709551615n,
      count: 18446744073709551615n,
      sum: 0,
      bucketCounts: [18446744073709551615n],
    })
    expect('min' in point).toBe(false)
    expect(Object.is(point.max, -0)).toBe(true)
    expect(point.exemplars).toHaveLength(2)
    expect(point.exemplars[0]).toMatchObject({
      intValue: -9223372036854775808n,
    })
    expect(Object.is(point.exemplars[1]!.doubleValue, -0)).toBe(true)
  })

  it('decodes every received number union arm', async () => {
    stubRpcResult({
      metricRef: 'metric-1',
      name: 'counter',
      description: '',
      unit: '1',
      metadata: [],
      metricType: 'Sum',
      aggregationTemporalityCode: 2,
      isMonotonic: false,
      resource: { attributes: [], droppedAttributesCount: 0, schemaUrl: '' },
      scope: {
        name: '',
        version: '',
        attributes: [],
        droppedAttributesCount: 0,
        schemaUrl: '',
      },
      seriesRef: 'series-1',
      attributes: [],
      datapoints: [
        {
          datapointID: 'i',
          timestamp: '1',
          startTime: '0',
          flags: 0,
          exemplars: [],
          valueType: 'Int',
          intValue: '9223372036854775807',
          doubleValue: null,
        },
        {
          datapointID: 'd',
          timestamp: '2',
          startTime: '0',
          flags: 0,
          exemplars: [],
          valueType: 'Double',
          intValue: null,
          doubleValue: '0x7ff8000000000001',
        },
        {
          datapointID: 'e',
          timestamp: '3',
          startTime: '0',
          flags: 0,
          exemplars: [],
          valueType: 'Empty',
          intValue: null,
          doubleValue: null,
        },
      ],
    })

    const selected = await telemetryAPI.getMetricSeries(
      'metric-1',
      'series-1',
      null,
      null
    )
    const points = selected!.datapoints
    expect(points[0]).toMatchObject({
      valueType: 'Int',
      intValue: 9223372036854775807n,
    })
    expect(
      Number.isNaN((points[1] as { doubleValue: number }).doubleValue)
    ).toBe(true)
    expect(points[2]).toMatchObject({
      valueType: 'Empty',
      intValue: null,
      doubleValue: null,
    })
  })

  it('decodes received exponential histogram buckets without chart fields', async () => {
    stubRpcResult({
      metricRef: 'metric-1',
      name: 'distribution',
      description: '',
      unit: '1',
      metadata: [],
      metricType: 'ExponentialHistogram',
      aggregationTemporalityCode: 1,
      resource: { attributes: [], droppedAttributesCount: 0, schemaUrl: '' },
      scope: {
        name: '',
        version: '',
        attributes: [],
        droppedAttributesCount: 0,
        schemaUrl: '',
      },
      seriesRef: 'series-1',
      attributes: [],
      datapoints: [
        {
          datapointID: 'point-1',
          timestamp: '1',
          startTime: '0',
          flags: 0,
          exemplars: [],
          count: '18446744073709551615',
          scale: -10,
          zeroCount: '9007199254740993',
          zeroThreshold: '0x8000000000000000',
          positive: {
            offset: -2,
            bucketCounts: ['1', '18446744073709551615'],
          },
          negative: { offset: 3, bucketCounts: [] },
        },
      ],
    })

    const selected = await telemetryAPI.getMetricSeries(
      'metric-1',
      'series-1',
      0n,
      1n
    )
    expect(selected!.datapoints[0]).toMatchObject({
      count: 18446744073709551615n,
      scale: -10,
      zeroCount: 9007199254740993n,
      positive: { offset: -2, bucketCounts: [1n, 18446744073709551615n] },
      negative: { offset: 3, bucketCounts: [] },
    })
  })
})

describe('telemetryAPI.getMetricAggregateView', () => {
  it('decodes aggregate doubles while preserving omitted extents', async () => {
    stubRpcResult({
      aggregate: [
        {
          timestamp: '100',
          startTime: '0',
          count: 0,
          sum: '0x8000000000000000',
          bucketCounts: [0, 0],
          explicitBounds: ['0x7ff0000000000000'],
          quantiles: { '0.5': '0x7ff8000000000001' },
        },
      ],
      scalarAggregate: null,
    })

    const result = await telemetryAPI.getMetricAggregateView(
      'some-metric',
      0n,
      1n,
      1,
      null,
      [0.5],
      0
    )

    const aggregate = result!.aggregate![0]!
    expect(Object.is(aggregate.sum, -0)).toBe(true)
    expect(aggregate.explicitBounds).toEqual([Number.POSITIVE_INFINITY])
    expect(Number.isNaN(aggregate.quantiles!['0.5'])).toBe(true)
    expect(aggregate).not.toHaveProperty('min')
    expect(aggregate).not.toHaveProperty('max')
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

    const [field] = await telemetryAPI.getTraceAttributeDefinitions()

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

describe('telemetryAPI.searchTraceSummaries', () => {
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

    await expect(telemetryAPI.searchTraceSummaries(0n, 1n)).resolves.toMatchObject([
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
    stubRpcResult([
      {
        traceID: '00000000000000000000000000000001',
        hasRootSpan: true,
        rootSpan: { serviceName: 'service-a', name: 'root' },
        startTime: '1',
        durationNs: '10',
        spanCount: 2,
        errorCount: 1,
        matchedSpans,
      },
      {
        traceID: '00000000000000000000000000000002',
        hasRootSpan: false,
        rootSpan: null,
        startTime: '2',
        durationNs: null,
        spanCount: 1,
        errorCount: 0,
      },
    ] satisfies JsonTraceSummary[])

    const summaries = await telemetryAPI.searchTraceSummaries(0n, 10n)

    expect(summaries[0]?.matchedSpans).toEqual(matchedSpans)
    expect(summaries[1]).not.toHaveProperty('matchedSpans')
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

    await expect(telemetryAPI.searchTraceSummaries(0n, 1n)).resolves.toMatchObject([
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

    await expect(telemetryAPI.searchTraceSummaries(0n, 1n)).rejects.toThrow(
      'Invalid bigint wire value: expected string, got boolean'
    )
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
    return telemetryAPI.getTraceView('abc123')
  }

  it('resolves each span against its own resource, not the first one', async () => {
    const trace = await fetchTrace()
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
    stubRpcResult<JsonTraceData>({
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
    stubRpcResult<JsonTraceData>({ ...wire, unplacedSpanCount: 3 })
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

describe('telemetryAPI metric bigint boundary', () => {
  it('keeps only the established lastSeenNs missing-field normalizations', async () => {
    // SAFETY: This fixture intentionally omits typed lastSeenNs fields to exercise their parser normalization.
    const result = metricResult({
      timeseries: [
        {
          seriesRef: 'series-1',
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

    const metric = await telemetryAPI.getMetricView('some-metric', 0n, 1n)
    expect(metric!.lastSeenNs).toBeNull()
    expect(metric!.timeseries[0]!.lastSeenNs).toBeNull()
    expect(metric!.timeseries[0]!.views![0]!.bucketStart).toBe(
      9_223_372_036_854_775_807n
    )
    expect(metric!.timeseries[0]!.sparkline![0]!.timestamp).toBe(-1n)
  })
})

function captureRequest() {
  const fetchMock = vi
    .fn()
    .mockImplementation(async (_url, init: RequestInit) => {
      const request = JSON.parse(String(init.body)) as { id: number }
      return {
        ok: true,
        json: async () => ({ jsonrpc: '2.0', id: request.id, result: [] }),
      }
    })
  vi.stubGlobal('fetch', fetchMock)
  return () => JSON.parse(fetchMock.mock.calls[0][1].body)
}

describe('request parameters', () => {
  const named = [
    [
      'searchAttributeMatches',
      () => telemetryAPI.searchAttributeMatches('http'),
      { term: 'http' },
    ],
    [
      'getTraceAttributeDefinitionsByTraceID',
      () => telemetryAPI.getTraceAttributeDefinitionsByTraceID('abc'),
      { traceID: 'abc' },
    ],
    [
      'searchTraceSummaries',
      () => telemetryAPI.searchTraceSummaries(2n, 5n),
      { startTime: '2', endTime: '5' },
    ],
    [
      'searchLogSummaries',
      () => telemetryAPI.searchLogSummaries(2n, 5n),
      { startTime: '2', endTime: '5' },
    ],
    [
      'searchMetricSummaries',
      () => telemetryAPI.searchMetricSummaries(2n, 5n),
      { startTime: '2', endTime: '5' },
    ],
    [
      'getTraceSpanCount',
      () => telemetryAPI.getTraceSpanCount('abc'),
      { traceID: 'abc' },
    ],
    [
      'getTraceLogSummaries',
      () => telemetryAPI.getTraceLogSummaries('abc'),
      { traceID: 'abc' },
    ],
    [
      'deleteMetric',
      () => telemetryAPI.deleteMetric('s1'),
      { metricRef: 's1' },
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

  it('getTraceView sends its traceID by name', async () => {
    const sent = captureRequest()
    await telemetryAPI.getTraceView('abc123').catch(() => {})
    expect(sent().params).toEqual({ traceID: 'abc123' })
  })

  it('omits query entirely when no query tree is supplied', async () => {
    const sent = captureRequest()
    await telemetryAPI.searchTraceSummaries(2n, 5n).catch(() => {})
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
    await telemetryAPI.searchTraceSummaries(2n, 5n, tree).catch(() => {})
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
    await telemetryAPI.searchTraceSummaries(2n, 5n, tree).catch(() => {})
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

  it.each(['env', 'Env', 'ENV'])(
    'serializes the exact parsed attribute key %s',
    async name => {
      const fields: FieldDefinition[] = ['env', 'Env', 'ENV'].map(
        (fieldName, index) => ({
          name: fieldName,
          type: index === 0 ? 'string' : index === 1 ? 'boolean' : 'int64',
          searchScope: 'attribute',
          attributeScope:
            index === 0 ? 'resource' : index === 1 ? 'span' : 'event',
          operators: [OPERATORS.EQUALS],
        })
      )
      const tree = parseQuery(`${name} = value`, fields)
      if (!tree) throw new Error('Expected an attribute query')
      const sent = captureRequest()

      await telemetryAPI.searchTraceSummaries(2n, 5n, tree).catch(() => {})

      expect(sent().params.query.query.field.name).toBe(name)
    }
  )

  it.each([
    ['attr(log, "same.key", int64) = 42', 'int64', '42'],
    ['attr(log, "same.key", string) = "42"', 'string', '42'],
  ])(
    'serializes the explicit stored kind from %s',
    async (text, type, value) => {
      const tree = parseQuery(text, [], 'logs')
      if (!tree) throw new Error('Expected an explicit attribute query')
      const sent = captureRequest()

      await telemetryAPI.searchLogSummaries(2n, 5n, tree).catch(() => {})

      expect(sent().params.query.query).toMatchObject({
        field: {
          name: 'same.key',
          type,
          searchScope: 'attribute',
          attributeScope: 'log',
        },
        fieldOperator: '=',
        value,
      })
    }
  )

  it('includes a trace result limit without requiring a query tree', async () => {
    const sent = captureRequest()
    await telemetryAPI.searchTraceSummaries(2n, 5n, undefined, 250).catch(() => {})
    expect(sent().params).toEqual({
      startTime: '2',
      endTime: '5',
      limit: 250,
    })
  })

  it.each([
    [
      'logs',
      () => telemetryAPI.searchLogSummaries(2n, 5n, undefined, 250),
      'searchLogSummaries',
    ],
    [
      'metrics',
      () => telemetryAPI.searchMetricSummaries(2n, 5n, undefined, 250),
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
          startTime: '2',
          endTime: '5',
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
        telemetryAPI.searchTraceSummaries(2n, 5n, undefined, 25, {
          field: 'duration',
          direction: 'desc',
        }),
      'searchTraceSummaries',
      'duration',
    ],
    [
      'logs',
      () =>
        telemetryAPI.searchLogSummaries(2n, 5n, undefined, 25, {
          field: 'severity',
          direction: 'asc',
        }),
      'searchLogSummaries',
      'severity',
    ],
    [
      'metrics',
      () =>
        telemetryAPI.searchMetricSummaries(2n, 5n, undefined, 25, {
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

  // These methods accept the ID list as positional params.
  it.each([
    [
      'deleteSpansByTraceID',
      () => telemetryAPI.deleteTraces(['a', 'b']),
      ['a', 'b'],
    ],
    ['deleteLogsByRefs', () => telemetryAPI.deleteLogsByRefs('log-1'), ['log-1']],
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

  it.each([
    ['getTraceAttributeDefinitions', () => telemetryAPI.getTraceAttributeDefinitions()],
    ['getLogAttributeDefinitions', () => telemetryAPI.getLogAttributeDefinitions()],
    ['getMetricAttributeDefinitions', () => telemetryAPI.getMetricAttributeDefinitions()],
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
    ['searchTraceSummaries', () => telemetryAPI.searchTraceSummaries(null, null)],
    ['searchLogSummaries', () => telemetryAPI.searchLogSummaries(null, null)],
    [
      'searchMetricSummaries',
      () => telemetryAPI.searchMetricSummaries(null, null),
    ],
    ['getMetricView', () => telemetryAPI.getMetricView('metric-1', null, null)],
    [
      'getMetricAggregateView',
      () =>
        telemetryAPI.getMetricAggregateView(
          'metric-1',
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

  it.each([
    ['searchTraceSummaries', () => telemetryAPI.searchTraceSummaries(null, 5n)],
    ['searchLogSummaries', () => telemetryAPI.searchLogSummaries(null, 5n)],
    [
      'searchMetricSummaries',
      () => telemetryAPI.searchMetricSummaries(null, 5n),
    ],
    ['getMetricView', () => telemetryAPI.getMetricView('metric-1', null, 5n)],
    [
      'getMetricAggregateView',
      () =>
        telemetryAPI.getMetricAggregateView(
          'metric-1',
          null,
          5n,
          10,
          null,
          [],
          0
        ),
    ],
  ])(
    '%s preserves an independently unbounded start',
    async (method, invoke) => {
      const sent = captureRequest()
      await invoke().catch(() => {})
      expect(sent()).toMatchObject({
        method,
        params: { startTime: null, endTime: '5' },
      })
    }
  )

  it.each([
    ['searchTraceSummaries', () => telemetryAPI.searchTraceSummaries(2n, null)],
    ['searchLogSummaries', () => telemetryAPI.searchLogSummaries(2n, null)],
    [
      'searchMetricSummaries',
      () => telemetryAPI.searchMetricSummaries(2n, null),
    ],
    ['getMetricView', () => telemetryAPI.getMetricView('metric-1', 2n, null)],
    [
      'getMetricAggregateView',
      () =>
        telemetryAPI.getMetricAggregateView(
          'metric-1',
          2n,
          null,
          10,
          null,
          [],
          0
        ),
    ],
  ])('%s preserves an independently unbounded end', async (method, invoke) => {
    const sent = captureRequest()
    await invoke().catch(() => {})
    expect(sent()).toMatchObject({
      method,
      params: { startTime: '2', endTime: null },
    })
  })

  it('getMetricView sends the final named parameter contract exactly', async () => {
    const sent = captureRequest()
    await telemetryAPI
      .getMetricView(
        'metric-1',
        2n,
        5n,
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
      metricRef: 'metric-1',
      startTime: '2',
      endTime: '5',
      targetBuckets: 10,
      seriesRefs: ['series-1'],
      quantiles: [0.5],
      tzOffsetNs: 7,
      viewBuckets: 8,
      sparklineBuckets: 9,
      selectedSeriesRefs: ['selected-1'],
      tzName: 'America/New_York',
      datapointSeriesRefs: ['datapoints-1'],
      datapointSeriesLimit: 3,
    })
  })

  it('getMetric sends only the Metric reference', async () => {
    const sent = captureRequest()
    await telemetryAPI.getMetric('metric-1').catch(() => {})
    expect(sent()).toMatchObject({
      method: 'getMetric',
      params: { metricRef: 'metric-1' },
    })
  })

  it('getMetricSeries sends references and nullable unsigned nanosecond bounds', async () => {
    const sent = captureRequest()
    await telemetryAPI
      .getMetricSeries('metric-1', 'series-1', null, 18446744073709551615n)
      .catch(() => {})
    expect(sent()).toMatchObject({
      method: 'getMetricSeries',
      params: {
        metricRef: 'metric-1',
        seriesRef: 'series-1',
        startTime: null,
        endTime: '18446744073709551615',
      },
    })
  })

  it('preserves omitted, empty, and null series selections', async () => {
    const omitted = captureRequest()
    await telemetryAPI.getMetricView('metric-1', 2n, 5n).catch(() => {})
    expect('seriesRefs' in omitted().params).toBe(false)

    const empty = captureRequest()
    await telemetryAPI
      .getMetricView('metric-1', 2n, 5n, undefined, [])
      .catch(() => {})
    expect(empty().params.seriesRefs).toEqual([])

    const unfiltered = captureRequest()
    await telemetryAPI
      .getMetricAggregateView('metric-1', 2n, 5n, 10, null, [], 0)
      .catch(() => {})
    expect(unfiltered().params.seriesRefs).toBeNull()
  })

  it('getMetricAggregateView sends the final named parameter contract exactly', async () => {
    const sent = captureRequest()
    await telemetryAPI
      .getMetricAggregateView(
        'metric-1',
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
      metricRef: 'metric-1',
      startTime: '2',
      endTime: '5',
      targetBuckets: 10,
      seriesRefs: ['series-1'],
      quantiles: [0.95],
      tzOffsetNs: 7,
      viewBuckets: 8,
      selectedSeriesRefs: ['selected-1'],
      tzName: 'UTC',
    })
  })

  it('serializes exact bigint bounds without converting them through number', async () => {
    const sent = captureRequest()
    await telemetryAPI
      .searchTraceSummaries(18_446_744_073_709_551_615n, 0n)
      .catch(() => {})

    expect(sent().params).toEqual({
      startTime: '18446744073709551615',
      endTime: '0',
    })
  })
})
