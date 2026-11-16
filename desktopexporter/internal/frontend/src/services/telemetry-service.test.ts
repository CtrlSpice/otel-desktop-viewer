import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  telemetryAPI,
  isAbortError,
  RequestAbortedError,
} from './telemetry-service'
import { SPAN_FIELDS } from '@/constants/fields'
import { OPERATORS, type FieldDefinition, type QueryNode } from '@/search/model'
import { parseDuration } from '@/utils/time'
import { parseQuery } from '@/components/shared/Search/queryParser'
import {
  stubRpcResponse,
  stubRpcResult,
} from '@/test/telemetry-service-fixtures'

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
  return () => {
    const [url, init] = fetchMock.mock.calls[0]
    const body = JSON.parse(init.body)
    // The parameter contract is shared; Arrow addresses the method in the URL.
    return url.startsWith('/api/data/')
      ? { method: url.slice('/api/data/'.length), params: body }
      : body
  }
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('file-import endpoint discovery', () => {
  it('reads the configured OTLP port through RPC', async () => {
    stubRpcResult({ otlpHttpPort: 54318, otlpGrpcPort: 54317 })
    expect(await telemetryAPI.getImportConfig()).toEqual({
      otlpHttpPort: 54318,
      otlpGrpcPort: 54317,
    })
    expect(
      JSON.parse(vi.mocked(fetch).mock.calls[0][1]!.body as string).method
    ).toBe('getImportConfig')
  })

  it.each([
    null,
    {},
    { otlpHttpPort: 0 },
    { otlpHttpPort: '4318' },
    { otlpHttpPort: 65536 },
  ])('rejects unusable endpoint config %j', async config => {
    stubRpcResult(config)
    await expect(telemetryAPI.getImportConfig()).rejects.toThrow(
      'invalid OTLP HTTP port'
    )
  })

  it.each([0, '4317', 65536, null])(
    'rejects invalid gRPC port %j',
    async otlpGrpcPort => {
      stubRpcResult({ otlpHttpPort: 4318, otlpGrpcPort })
      await expect(telemetryAPI.getImportConfig()).rejects.toThrow(
        'invalid OTLP gRPC port'
      )
    }
  )
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
  it('reads ordered IDs and sends exact inclusive timestamp bounds', async () => {
    stubRpcResult(['b', 'a'])
    expect(
      await telemetryAPI.getMetricSeriesOrder(
        'metric',
        9007199254740993n,
        18446744073709551615n,
        ['a', 'b'],
        '',
        'desc'
      )
    ).toEqual(['b', 'a'])
    expect(fetch).toHaveBeenCalledWith(
      '/api/data/getMetricSeriesOrder',
      expect.objectContaining({
        body: JSON.stringify({
          metricRef: 'metric',
          startTime: '9007199254740993',
          endTime: '18446744073709551615',
          seriesRefs: ['a', 'b'],
          key: '',
          direction: 'desc',
        }),
      })
    )
  })

  it('cancels a database sort request', async () => {
    const fetch = vi
      .fn()
      .mockRejectedValue(new DOMException('aborted', 'AbortError'))
    vi.stubGlobal('fetch', fetch)
    const controller = new AbortController()
    await expect(
      telemetryAPI.getMetricSeriesOrder(
        'metric',
        null,
        null,
        ['series'],
        'key',
        'asc',
        controller.signal
      )
    ).rejects.toBeInstanceOf(RequestAbortedError)
    expect(fetch).toHaveBeenCalledWith(
      '/api/data/getMetricSeriesOrder',
      expect.objectContaining({ signal: controller.signal })
    )
  })
  it.each([
    [
      'getMetric',
      (signal: AbortSignal) => telemetryAPI.getMetric('metric', signal),
    ],
    [
      'getMetricSeries',
      (signal: AbortSignal) =>
        telemetryAPI.getMetricSeries(
          'metric',
          'series',
          null,
          null,
          false,
          signal
        ),
    ],
    [
      'getMetricView',
      (signal: AbortSignal) =>
        telemetryAPI.getMetricView(
          'metric',
          null,
          null,
          0,
          undefined,
          undefined,
          0,
          0,
          0,
          undefined,
          undefined,
          [],
          0,
          signal
        ),
    ],
    [
      'getMetricAggregateView',
      (signal: AbortSignal) =>
        telemetryAPI.getMetricAggregateView(
          'metric',
          null,
          null,
          1,
          [],
          [],
          0,
          0,
          undefined,
          undefined,
          undefined,
          signal
        ),
    ],
  ] as const)('forwards cancellation through %s', async (method, invoke) => {
    const fetch = vi
      .fn()
      .mockRejectedValue(new DOMException('aborted', 'AbortError'))
    vi.stubGlobal('fetch', fetch)
    const controller = new AbortController()
    await expect(invoke(controller.signal)).rejects.toBeInstanceOf(
      RequestAbortedError
    )
    expect(fetch).toHaveBeenCalledWith(
      `/api/data/${method}`,
      expect.objectContaining({ signal: controller.signal })
    )
  })

  it('cancels an outstanding freshness statistics request through the transport', async () => {
    const fetch = vi.fn(
      (_url: string, options: RequestInit) =>
        new Promise<Response>((_resolve, reject) => {
          options.signal?.addEventListener('abort', () =>
            reject(new DOMException('aborted', 'AbortError'))
          )
        })
    )
    vi.stubGlobal('fetch', fetch)
    const controller = new AbortController()
    const pending = telemetryAPI.getStats(controller.signal)
    expect(fetch).toHaveBeenCalledWith(
      '/rpc',
      expect.objectContaining({ signal: controller.signal })
    )
    controller.abort()
    await expect(pending).rejects.toBeInstanceOf(RequestAbortedError)
  })

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
      '/api/data/getTraceView',
      expect.objectContaining({ signal: controller.signal })
    )
  })
})

describe('request parameters', () => {
  const named = [
    [
      'searchAttributeMatches',
      () => telemetryAPI.searchAttributeMatches('http'),
      { term: 'http' },
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
      type: 'text',
      query: {
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
    expect(params.query).toMatchObject({ id: 'q1', type: 'text' })
  })

  it('serializes an exact duration boundary query', async () => {
    const duration = parseDuration('9007199254740993ns')
    if (duration === null) throw new Error('Expected valid duration')
    const field = SPAN_FIELDS.find(field => field.name === 'duration')
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
        field: { owner: 'span', path: ['duration'], type: 'int64' },
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
          path: ['attributes', fieldName],
          owner: index === 0 ? 'resource' : index === 1 ? 'span' : 'event',
          operators: [OPERATORS.EQUALS],
        })
      )
      const tree = parseQuery(`${name} = value`, fields)
      if (!tree) throw new Error('Expected an attribute query')
      const sent = captureRequest()

      await telemetryAPI.searchTraceSummaries(2n, 5n, tree).catch(() => {})

      expect(sent().params.query.query.field.path).toEqual(['attributes', name])
    }
  )

  it.each([
    ['log.attributes["same.key"]::BIGINT = 42', 'int64', '42'],
    ['log.attributes["same.key"]::VARCHAR = "42"', 'string', '42'],
  ])(
    'serializes the explicit stored kind from %s',
    async (text, type, value) => {
      const tree = parseQuery(text, [], 'logs')
      if (!tree) throw new Error('Expected an explicit attribute query')
      const sent = captureRequest()

      await telemetryAPI.searchLogSummaries(2n, 5n, tree).catch(() => {})

      expect(sent().params.query.query).toMatchObject({
        field: {
          type,
          owner: 'log',
          path: ['attributes', 'same.key'],
        },
        fieldOperator: '=',
        value,
      })
    }
  )

  it('includes a trace result limit without requiring a query tree', async () => {
    const sent = captureRequest()
    await telemetryAPI
      .searchTraceSummaries(2n, 5n, undefined, 250)
      .catch(() => {})
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
    [
      'deleteLogsByRefs',
      () => telemetryAPI.deleteLogsByRefs('log-1'),
      ['log-1'],
    ],
    [
      'deleteLogsByRefs',
      () => telemetryAPI.deleteLogsByRefs('log-1', 'log-2'),
      ['log-1', 'log-2'],
    ],
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
    [
      'getTraceAttributeDefinitions',
      () => telemetryAPI.getTraceAttributeDefinitions(),
    ],
    [
      'getLogAttributeDefinitions',
      () => telemetryAPI.getLogAttributeDefinitions(),
    ],
    [
      'getMetricAttributeDefinitions',
      () => telemetryAPI.getMetricAttributeDefinitions(),
    ],
    ['clearTraces', () => telemetryAPI.clearTraces()],
    ['clearLogs', () => telemetryAPI.clearLogs()],
    ['clearMetrics', () => telemetryAPI.clearMetrics()],
  ])('%s sends no arguments', async (method, invoke) => {
    const sent = captureRequest()
    await invoke().catch(() => {})
    const request = sent()
    expect(request.method).toBe(method)
    if (method.endsWith('AttributeDefinitions'))
      expect(request.params).toEqual({})
    else expect('params' in request).toBe(false)
  })

  it.each([
    [
      'searchTraceSummaries',
      () => telemetryAPI.searchTraceSummaries(null, null),
    ],
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
