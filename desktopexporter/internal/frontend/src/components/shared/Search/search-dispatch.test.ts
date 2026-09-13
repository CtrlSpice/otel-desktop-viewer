import { beforeEach, describe, expect, expectTypeOf, it } from 'vitest'
import {
  buildSearchEventFactory,
  createSearchDispatch,
  type SearchContext,
} from './search-dispatch'
import type { LogSummary, MetricSummary, TraceSummary } from '@/types/api-types'

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
    isMonotonic: true,
    serviceName: 'checkout',
    seriesCount: 1,
    seriesCardinality: 1,
    dataPointCount: 2,
    lastValue: 2,
    lastSeen: 5n,
  },
] satisfies MetricSummary[]

const calls: string[] = []
const api = {
  searchTraces: async () => {
    calls.push('traces')
    return traceResults
  },
  searchLogs: async () => {
    calls.push('logs')
    return logResults
  },
  searchMetricSummaries: async () => {
    calls.push('metrics')
    return metricResults
  },
} satisfies Parameters<typeof createSearchDispatch>[0]

const dispatch = createSearchDispatch(api)

const traceContext = {
  signal: 'traces',
  startTime: 10,
  endTime: 20,
} satisfies SearchContext<'traces'>
const logContext = {
  signal: 'logs',
  startTime: 10,
  endTime: 20,
} satisfies SearchContext<'logs'>
const metricContext = {
  signal: 'metrics',
  startTime: 10,
  endTime: 20,
} satisfies SearchContext<'metrics'>

beforeEach(() => {
  calls.length = 0
})

describe('search dispatch', () => {
  it('correlates the trace factory with trace results and events', async () => {
    const results = await dispatch.traces(traceContext)()
    expectTypeOf(results).toEqualTypeOf<TraceSummary[]>()
    expect(results).toBe(traceResults)
    expect(calls).toEqual(['traces'])

    const eventFactory = buildSearchEventFactory(dispatch, traceContext)
    if (!eventFactory) throw new Error('missing trace event factory')
    await expect(eventFactory(11)).resolves.toEqual({
      signal: 'traces',
      results: traceResults,
      queryTree: undefined,
      updateSeq: 11,
    })
  })

  it('correlates the log factory with log results and events', async () => {
    const results = await dispatch.logs(logContext)()
    expectTypeOf(results).toEqualTypeOf<LogSummary[]>()
    expect(results).toBe(logResults)
    expect(calls).toEqual(['logs'])

    const eventFactory = buildSearchEventFactory(dispatch, logContext)
    if (!eventFactory) throw new Error('missing log event factory')
    await expect(eventFactory(12)).resolves.toEqual({
      signal: 'logs',
      results: logResults,
      queryTree: undefined,
      updateSeq: 12,
    })
  })

  it('correlates the metric factory with metric results and events', async () => {
    const results = await dispatch.metrics(metricContext)()
    expectTypeOf(results).toEqualTypeOf<MetricSummary[]>()
    expect(results).toBe(metricResults)
    expect(calls).toEqual(['metrics'])

    const eventFactory = buildSearchEventFactory(dispatch, metricContext)
    if (!eventFactory) throw new Error('missing metric event factory')
    await expect(eventFactory(13)).resolves.toEqual({
      signal: 'metrics',
      results: metricResults,
      queryTree: undefined,
      updateSeq: 13,
    })
  })
})
