import { afterEach, describe, expect, it, vi } from 'vitest'
import { runSearch, type SearchContext } from './search-dispatch'
import { telemetryAPI } from '@/services/telemetry-service'
import type { LogSummary, MetricSummary, TraceSummary } from '@/types/api-types'
import type { QueryNode } from './queryTree'

const traceResults = [
  {
    traceID: 'trace-1',
    hasRootSpan: true,
    rootSpan: { serviceName: 'checkout', name: 'GET /checkout' },
    startTime: 1n,
    durationNs: 2n,
    spanCount: 3,
    errorCount: 0,
  },
] satisfies TraceSummary[]

const logResults = [
  {
    id: 'log-1',
    timestamp: 4n,
    severityText: 'INFO',
    severityNumber: 9,
    serviceName: 'checkout',
    bodyPreview: 'paid',
  },
] satisfies LogSummary[]

const metricResults = [
  {
    id: 'metric-1',
    name: 'orders',
    description: 'Orders placed',
    unit: '{order}',
    metricType: 'Sum',
    aggregationTemporality: 'Cumulative',
    aggregationTemporalityCode: 2,
    isMonotonic: true,
    serviceName: 'checkout',
    seriesCount: 1,
    seriesCardinality: 1,
    dataPointCount: 2,
    lastValue: 2,
    lastSeen: 5n,
  },
] satisfies MetricSummary[]

const traceContext = {
  signal: 'traces',
  startTime: 10n,
  endTime: 20n,
} satisfies SearchContext
const logContext = {
  signal: 'logs',
  startTime: 10n,
  endTime: 20n,
} satisfies SearchContext
const metricContext = {
  signal: 'metrics',
  startTime: 10n,
  endTime: 20n,
} satisfies SearchContext

const queryTree = {
  id: 'query-1',
  type: 'group',
  group: { operator: 'AND', children: [] },
} satisfies QueryNode
const sort = { field: 'startTime', direction: 'desc' } as const

afterEach(() => {
  vi.restoreAllMocks()
})

describe('search dispatch', () => {
  it('runs trace search and retains its result identity', async () => {
    const searchTraces = vi
      .spyOn(telemetryAPI, 'searchTraces')
      .mockResolvedValue(traceResults)
    const searchLogs = vi.spyOn(telemetryAPI, 'searchLogs')
    const searchMetrics = vi.spyOn(telemetryAPI, 'searchMetricSummaries')

    await expect(
      runSearch(traceContext, 11, queryTree, 25, sort)
    ).resolves.toEqual({
      signal: 'traces',
      results: traceResults,
      queryTree,
      updateSeq: 11,
    })
    expect(searchTraces).toHaveBeenCalledWith(10n, 20n, queryTree, 25, sort)
    expect(searchLogs).not.toHaveBeenCalled()
    expect(searchMetrics).not.toHaveBeenCalled()
  })

  it('runs log search and retains its result identity', async () => {
    const searchTraces = vi.spyOn(telemetryAPI, 'searchTraces')
    const searchLogs = vi
      .spyOn(telemetryAPI, 'searchLogs')
      .mockResolvedValue(logResults)
    const searchMetrics = vi.spyOn(telemetryAPI, 'searchMetricSummaries')

    await expect(
      runSearch(logContext, 12, queryTree, 50, sort)
    ).resolves.toEqual({
      signal: 'logs',
      results: logResults,
      queryTree,
      updateSeq: 12,
    })
    expect(searchLogs).toHaveBeenCalledWith(10n, 20n, queryTree, 50, sort)
    expect(searchTraces).not.toHaveBeenCalled()
    expect(searchMetrics).not.toHaveBeenCalled()
  })

  it('runs metric search and retains its result identity', async () => {
    const searchTraces = vi.spyOn(telemetryAPI, 'searchTraces')
    const searchLogs = vi.spyOn(telemetryAPI, 'searchLogs')
    const searchMetrics = vi
      .spyOn(telemetryAPI, 'searchMetricSummaries')
      .mockResolvedValue(metricResults)

    await expect(
      runSearch(metricContext, 13, queryTree, 75, sort)
    ).resolves.toEqual({
      signal: 'metrics',
      results: metricResults,
      queryTree,
      updateSeq: 13,
    })
    expect(searchMetrics).toHaveBeenCalledWith(10n, 20n, queryTree, 75, sort)
    expect(searchTraces).not.toHaveBeenCalled()
    expect(searchLogs).not.toHaveBeenCalled()
  })

  it('propagates the selected search error', async () => {
    const error = new Error('search unavailable')
    const searchTraces = vi
      .spyOn(telemetryAPI, 'searchTraces')
      .mockRejectedValue(error)
    const searchLogs = vi.spyOn(telemetryAPI, 'searchLogs')
    const searchMetrics = vi.spyOn(telemetryAPI, 'searchMetricSummaries')

    await expect(runSearch(traceContext, 14)).rejects.toBe(error)
    expect(searchTraces).toHaveBeenCalledWith(
      10n,
      20n,
      undefined,
      undefined,
      undefined
    )
    expect(searchLogs).not.toHaveBeenCalled()
    expect(searchMetrics).not.toHaveBeenCalled()
  })
})
