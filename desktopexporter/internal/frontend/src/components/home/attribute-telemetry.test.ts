import { afterEach, expect, it, vi } from 'vitest'
import { telemetryAPI } from '@/services/telemetry-service'
import type { ServiceSummary } from '@/services/discovery-types'
import type { TraceSummary } from '@/types/api-types'
import { loadAttributeTelemetry } from './attribute-telemetry'
import { parseQuery } from '@/components/shared/Search/queryParser'
import { getStaticFieldsForSearch } from '@/constants/fields'

afterEach(() => vi.restoreAllMocks())
const service: ServiceSummary = {
  serviceName: 'api',
  serviceNamespace: '7',
  serviceIdentity:
    '[{"kind":"string","value":"api"},{"kind":"int64","value":"7"}]',
  traceCount: 1,
  spanCount: 3,
  errorSpanCount: 0,
  logCount: 0,
  errorLogCount: 0,
  logSeverityCounts: {
    trace: 0,
    debug: 0,
    info: 0,
    warn: 0,
    error: 0,
    fatal: 0,
    other: 0,
  },
  metricCount: 0,
  dataPointCount: 0,
  exemplarCount: 0,
  lastSeen: null,
}

it('intersects exact attribute owners with the submitted per-span search and preserves whole trace summaries', async () => {
  const read = vi
    .spyOn(telemetryAPI, 'getAttributeRecordMatches')
    .mockResolvedValue([
      { itemRef: 'trace', spanIDs: ['matches', 'wrong-service'], count: 2 },
      { itemRef: 'excluded-trace', spanIDs: ['other'], count: 1 },
    ])
  const trace: TraceSummary = {
    traceID: 'trace',
    hasRootSpan: true,
    rootSpan: { name: 'root', serviceName: 'upstream' },
    startTime: 1n,
    durationNs: 9n,
    spanCount: 10,
    errorCount: 2,
    matchedSpans: ['matches', 'wrong-value'].map(spanID => ({
      traceID: 'trace',
      spanID,
    })),
  }
  const search = vi
    .spyOn(telemetryAPI, 'searchTraceSummaries')
    .mockResolvedValue([trace])
  const query = parseQuery(
    'name = "needle"',
    getStaticFieldsForSearch('traces')
  )!
  const identity =
    '{"kind":"array","value":[{"kind":"int64","value":"9007199254740993"}]}'
  const cards = await loadAttributeTelemetry(
    'traces',
    { key: "quote'key", kind: 'array', ownerType: 'event', foundOn: [] },
    identity,
    service,
    {
      startTime: '1',
      endTime: '10',
      queries: { traces: query, logs: false, metrics: false },
    },
    new AbortController().signal
  )
  expect(cards).toEqual([
    {
      signal: 'traces',
      trace: {
        ...trace,
        matchedSpans: [{ traceID: 'trace', spanID: 'matches' }],
      },
      count: 1,
    },
  ])
  expect(read.mock.calls[0][0]).toMatchObject({
    key: "quote'key",
    kind: 'array',
    valueIdentity: identity,
    startTime: '1',
    endTime: '10',
  })
  expect(search.mock.calls[0].slice(0, 2)).toEqual([1n, 10n])
  expect(JSON.stringify(search.mock.calls[0][2])).toContain('needle')
  expect(JSON.stringify(search.mock.calls[0][2])).toContain(
    JSON.stringify(service.serviceIdentity).slice(1, -1)
  )
  expect(search.mock.calls[0][3]).toBeUndefined()
})

it('does not fetch an excluded signal or continue after selection cancellation', async () => {
  const read = vi
    .spyOn(telemetryAPI, 'getAttributeRecordMatches')
    .mockResolvedValue([])
  const attribute = {
    key: 'env',
    kind: 'string',
    ownerType: 'resource' as const,
    foundOn: [],
  }
  const controller = new AbortController()
  expect(
    await loadAttributeTelemetry(
      'logs',
      attribute,
      undefined,
      service,
      { queries: { traces: null, logs: false, metrics: null } },
      controller.signal
    )
  ).toEqual([])
  expect(read).not.toHaveBeenCalled()
  controller.abort()
  await expect(
    loadAttributeTelemetry(
      'traces',
      attribute,
      undefined,
      service,
      {},
      controller.signal
    )
  ).rejects.toThrow()
})
