import { afterEach, describe, expect, it, vi } from 'vitest'
import { runSearch, type SearchContext } from './search-dispatch'
import { telemetryAPI } from '@/services/telemetry-service'
import type { LogSummary, MetricSummary, TraceSummary } from '@/types/api-types'
import type { QueryNode } from '@/search/model'

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
    logRef: 'log-1',
    timestamp: 4n,
    severityText: 'INFO',
    severityNumber: 9,
    serviceName: 'checkout',
    bodyPreview: 'paid',
  },
] satisfies LogSummary[]

const metricResults = [
  {
    metricRef: 'metric-1',
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
    const searchTraceSummaries = vi
      .spyOn(telemetryAPI, 'searchTraceSummaries')
      .mockResolvedValue(traceResults)
    const searchLogSummaries = vi.spyOn(telemetryAPI, 'searchLogSummaries')
    const searchMetrics = vi.spyOn(telemetryAPI, 'searchMetricSummaries')

    await expect(
      runSearch(traceContext, 11, queryTree, 25, sort)
    ).resolves.toEqual({
      signal: 'traces',
      results: traceResults,
      queryTree,
      updateSeq: 11,
    })
    expect(searchTraceSummaries).toHaveBeenCalledWith(10n, 20n, queryTree, 25, sort)
    expect(searchLogSummaries).not.toHaveBeenCalled()
    expect(searchMetrics).not.toHaveBeenCalled()
  })

  it('runs log search and retains its result identity', async () => {
    const searchTraceSummaries = vi.spyOn(telemetryAPI, 'searchTraceSummaries')
    const searchLogSummaries = vi
      .spyOn(telemetryAPI, 'searchLogSummaries')
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
    expect(searchLogSummaries).toHaveBeenCalledWith(10n, 20n, queryTree, 50, sort)
    expect(searchTraceSummaries).not.toHaveBeenCalled()
    expect(searchMetrics).not.toHaveBeenCalled()
  })

  it('runs metric search and retains its result identity', async () => {
    const searchTraceSummaries = vi.spyOn(telemetryAPI, 'searchTraceSummaries')
    const searchLogSummaries = vi.spyOn(telemetryAPI, 'searchLogSummaries')
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
    expect(searchTraceSummaries).not.toHaveBeenCalled()
    expect(searchLogSummaries).not.toHaveBeenCalled()
  })

  it('propagates the selected search error', async () => {
    const error = new Error('search unavailable')
    const searchTraceSummaries = vi
      .spyOn(telemetryAPI, 'searchTraceSummaries')
      .mockRejectedValue(error)
    const searchLogSummaries = vi.spyOn(telemetryAPI, 'searchLogSummaries')
    const searchMetrics = vi.spyOn(telemetryAPI, 'searchMetricSummaries')

    await expect(runSearch(traceContext, 14)).rejects.toBe(error)
    expect(searchTraceSummaries).toHaveBeenCalledWith(
      10n,
      20n,
      undefined,
      undefined,
      undefined
    )
    expect(searchLogSummaries).not.toHaveBeenCalled()
    expect(searchMetrics).not.toHaveBeenCalled()
  })
})
