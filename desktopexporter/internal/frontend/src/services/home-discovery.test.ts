import { afterEach, expect, it, vi } from 'vitest'
import { Int64, Table, vectorFromArray } from 'apache-arrow'
import { arrowResponse } from '@/test/arrow-fixtures'
import { telemetryAPI } from './telemetry-service'

afterEach(() => vi.unstubAllGlobals())

it('requests one cross-signal Home page with the full scope and cancellation signal', async () => {
  const result = {
    values: [],
    kinds: [],
    denominators: { traces: 0, metrics: 0, logs: 0 },
    denominator: 0,
    total: 0,
    truncated: false,
  }
  const fetch = vi.fn(async (_url: string, init: RequestInit) => {
    const request = JSON.parse(String(init.body))
    return Response.json({ jsonrpc: '2.0', id: request.id, result })
  })
  vi.stubGlobal('fetch', fetch)
  const signal = new AbortController().signal
  const options = {
    ownerType: 'resource' as const,
    key: 'deep',
    valueKind: 'map',
    offset: 50,
    limit: 25,
    serviceIdentity:
      '[{"kind":"string","value":"svc"},{"kind":"string","value":""}]',
    startTime: '100',
    endTime: '200',
    sortBy: 'name' as const,
    sortDirection: 'desc' as const,
    locale: 'en-US',
    term: 'outside preview',
    queries: { traces: null, metrics: false as const, logs: null },
  }
  expect(
    await telemetryAPI.discoverHomeAttributeValues(options, signal)
  ).toEqual(result)
  expect(fetch).toHaveBeenCalledTimes(1)
  const [, init] = fetch.mock.calls[0]!
  expect(init.signal).toBe(signal)
  expect(JSON.parse(String(init.body))).toMatchObject({
    method: 'discoverTelemetry',
    params: { options: { ...options, kind: 'home-values' } },
  })
})

it('loads received native integers only when explicitly resolving the representative', async () => {
  const fetch = vi.fn(async () =>
    arrowResponse(
      new Table({ value: vectorFromArray([9007199254740993n], new Int64()) })
    )
  )
  vi.stubGlobal('fetch', fetch)
  const signal = new AbortController().signal
  const value = await telemetryAPI.getHomeAttributeValue(
    'home:representative',
    signal
  )
  expect(value).toEqual({ kind: 'int64', value: 9007199254740993n })
  expect(fetch).toHaveBeenCalledWith(
    '/api/data/discoverTelemetry',
    expect.objectContaining({
      signal,
      body: JSON.stringify({
        options: { kind: 'home-value', valueRef: 'home:representative' },
      }),
    })
  )
})

it('resolves canonical membership on demand without using the preview or reference as SQL equality', async () => {
  const identity = '{"kind":"int64","value":"9007199254740993"}'
  const requests: {
    method: string
    params: { sql?: string; options?: object }
  }[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn(async (_url: string, init: RequestInit) => {
      const request = JSON.parse(String(init.body))
      requests.push(request)
      return Response.json({
        jsonrpc: '2.0',
        id: request.id,
        result:
          request.method === 'discoverTelemetry'
            ? { identity }
            : { rows: [['trace', ['span'], 1]], truncated: false },
      })
    })
  )
  const result = await telemetryAPI.getAttributeRecordMatches({
    signal: 'traces',
    ownerType: 'span',
    key: 'deep',
    kind: 'int64',
    valueIdentity: 'home:canonical-row',
  })
  expect(result).toEqual([{ itemRef: 'trace', spanIDs: ['span'], count: 1 }])
  expect(requests).toHaveLength(2)
  expect(requests[0]).toMatchObject({
    method: 'discoverTelemetry',
    params: { options: { kind: 'home-value', valueRef: 'home:canonical-row' } },
  })
  expect(requests[1].params.sql).toContain(identity)
  expect(requests[1].params.sql).not.toContain('home:canonical-row')
})

it('reports an expired exact reference instead of matching or copying another value', async () => {
  const fetch = vi.fn(async (_url: string, init: RequestInit) => {
    const request = JSON.parse(String(init.body))
    return Response.json({
      jsonrpc: '2.0',
      id: request.id,
      error: {
        code: -32011,
        message:
          'Home value is unavailable; refresh attribute values and select it again',
      },
    })
  })
  vi.stubGlobal('fetch', fetch)
  await expect(
    telemetryAPI.getAttributeRecordMatches({
      signal: 'logs',
      ownerType: 'log',
      key: 'deep',
      kind: 'map',
      valueIdentity: 'home:expired',
    })
  ).rejects.toThrow('refresh attribute values')
  expect(fetch).toHaveBeenCalledTimes(1)
})
