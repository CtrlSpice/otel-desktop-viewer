import { afterEach, expect, it, vi } from 'vitest'
import { telemetryAPI } from '@/services/telemetry-service'
import type { ServiceSummary } from '@/services/discovery-types'
import type { TraceSummary } from '@/types/api-types'
import { loadServiceTelemetry } from './service-telemetry'
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

it('combines typed service identity with the submitted search and preserves whole trace summaries and nanosecond bounds', async () => {
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
  const signal = new AbortController().signal
  const cards = await loadServiceTelemetry(
    'traces',
    service,
    {
      startTime: '9007199254740993',
      endTime: '9007199254741003',
      queries: { traces: query, logs: false, metrics: false },
    },
    signal
  )
  expect(cards).toEqual([
    {
      signal: 'traces',
      trace,
    },
  ])
  expect(search.mock.calls[0].slice(0, 2)).toEqual([
    9007199254740993n,
    9007199254741003n,
  ])
  expect(JSON.stringify(search.mock.calls[0][2])).toContain('needle')
  expect(JSON.stringify(search.mock.calls[0][2])).toContain(
    JSON.stringify(service.serviceIdentity).slice(1, -1)
  )
  expect(search.mock.calls[0][3]).toBeUndefined()
  expect(search.mock.calls[0][4]).toBeUndefined()
  expect(search.mock.calls[0][5]).toBe(signal)
})

it('does not fetch an excluded signal or continue after selection cancellation', async () => {
  const read = vi
    .spyOn(telemetryAPI, 'searchLogSummaries')
    .mockResolvedValue([])
  const pending = Promise.withResolvers<TraceSummary[]>()
  const search = vi
    .spyOn(telemetryAPI, 'searchTraceSummaries')
    .mockReturnValue(pending.promise)
  const controller = new AbortController()
  expect(
    await loadServiceTelemetry(
      'logs',
      service,
      { queries: { traces: null, logs: false, metrics: null } },
      controller.signal
    )
  ).toEqual([])
  expect(read).not.toHaveBeenCalled()
  const request = loadServiceTelemetry('traces', service, {}, controller.signal)
  expect(search.mock.calls[0].slice(0, 2)).toEqual([null, null])
  expect(search.mock.calls[0][5]).toBe(controller.signal)
  controller.abort()
  pending.resolve([])
  await expect(request).rejects.toThrow()
})
