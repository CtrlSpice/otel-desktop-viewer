// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { waitFor } from '@testing-library/svelte'
import MetricsPage from './MetricsPage.svelte'
import type {
  MetricSummary,
  MetricData,
  MetricType,
  Stats,
} from '@/types/api-types'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

// A legend toggle fetches the aggregate at two grids: a bucketed one for the
// heatmap, and a whole-window collapse for the summary distribution. Only a
// histogram has the second. A Gauge or Sum has no bucket vectors to merge, so
// the store prunes that chain outright (aggregateShapeFor) and the projection
// answers a literal null -- asking for it spends a round trip and a full query
// plan to be told so.
//
// The store side of that contract is pinned in Go by TestGetMetricAggregate.
// This is the client side: that it asks for the second grid only when the
// metric can answer.

const { searchMetricSummaries, getStats, getMetric, getMetricAggregate } =
  vi.hoisted(() => ({
    searchMetricSummaries: vi.fn(),
    getStats: vi.fn(),
    getMetric: vi.fn(),
    getMetricAggregate: vi.fn(),
  }))

vi.mock('@/services/telemetry-service', async importOriginal => {
  const actual =
    await importOriginal<typeof import('@/services/telemetry-service')>()
  return {
    ...actual,
    telemetryAPI: {
      ...actual.telemetryAPI,
      searchMetricSummaries,
      getStats,
      getMetric,
      getMetricAggregate,
    },
  }
})

const EMPTY_RESOURCE = { attributes: [], droppedAttributesCount: 0 }
const EMPTY_SCOPE = {
  name: '',
  version: '',
  attributes: [],
  droppedAttributesCount: 0,
}

function makeSummary(metricType: MetricType): MetricSummary {
  return {
    id: 'metric-1',
    name: 'demo.metric',
    description: '',
    unit: 'ms',
    metricType,
    aggregationTemporality: 'Cumulative',
    isMonotonic: metricType === 'Sum' ? true : null,
    serviceName: 'checkout-api',
    seriesCount: 1,
    seriesCardinality: 1,
    dataPointCount: 4,
    lastValue: metricType === 'Gauge' ? 1 : null,
    lastSeen: 1_700_000_000_000_000_000n,
  }
}

function makeMetric(metricType: MetricType): MetricData {
  return {
    lastSeenNs: 1_700_000_000_000_000_000n,
    id: 'metric-1',
    name: 'demo.metric',
    description: '',
    metadata: [],
    unit: 'ms',
    metricType,
    aggregationTemporality: 'Cumulative',
    isMonotonic: metricType === 'Sum' ? true : null,
    resourceDroppedAttributesCount: 0,
    resource: EMPTY_RESOURCE,
    scopeName: '',
    scopeVersion: '',
    scopeDroppedAttributesCount: 0,
    scope: EMPTY_SCOPE,
    timeseries: [
      {
        attributesKey: 'route=/checkout',
        attributes: [],
        resource: EMPTY_RESOURCE,
        datapoints: [],
        stats: null,
        views: null,
        rateStats: null,
        sparkline: null,
        datapointCount: 4,
        lastSeenNs: 1_700_000_000_000_000_000n,
      },
    ],
    datapointCount: 4,
    boundsMismatch: null,
    window: { fittedToData: false, startNs: null, endNs: null },
  }
}

function makeStats(): Stats {
  return {
    traces: {
      traceCount: 0,
      spanCount: 0,
      serviceCount: 0,
      errorCount: 0,
      lastReceived: null,
    },
    logs: { logCount: 0, errorCount: 0, lastReceived: null },
    metrics: { metricCount: 1, dataPointCount: 4, lastReceived: null },
  }
}

beforeEach(() => {
  if (typeof Element.prototype.scrollTo !== 'function') {
    Element.prototype.scrollTo = () => {}
  }
  searchMetricSummaries.mockReset()
  getStats.mockReset()
  getMetric.mockReset()
  getMetricAggregate.mockReset()
})

async function renderSelected(metricType: MetricType) {
  searchMetricSummaries.mockResolvedValue([makeSummary(metricType)])
  getStats.mockResolvedValue(makeStats())
  getMetric.mockResolvedValue(makeMetric(metricType))
  getMetricAggregate.mockResolvedValue({
    aggregate: null,
    scalarAggregate: null,
  })
  setTestUrl('/metrics/metric-1')
  renderWithContexts(MetricsPage)
  // The aggregate fetch is debounced behind the detail landing; wait for the
  // detail first so the wait below is for the debounce, not the round trip.
  await waitFor(() => expect(getMetric).toHaveBeenCalled())
  await waitFor(() => expect(getMetricAggregate).toHaveBeenCalled(), {
    timeout: 3000,
  })
}

/** targetBuckets is the 4th positional argument; the whole-window call is the
 *  one that asks for exactly 1 bucket. */
function wholeWindowCalls() {
  return getMetricAggregate.mock.calls.filter(args => args[3] === 1)
}

describe('MetricsPage aggregate fetching', () => {
  it('asks for the whole-window merge for a histogram', async () => {
    await renderSelected('Histogram')
    await waitFor(() => expect(wholeWindowCalls().length).toBe(1))
  })

  it('does not ask a Gauge for a merge it cannot produce', async () => {
    await renderSelected('Gauge')
    // Let any debounced follow-up land before asserting absence.
    await new Promise(r => setTimeout(r, 400))
    expect(wholeWindowCalls()).toHaveLength(0)
    // The bucketed call still happens -- the scalar pools ride on it.
    expect(getMetricAggregate.mock.calls.length).toBeGreaterThan(0)
  })

  it('does not ask a Sum either', async () => {
    await renderSelected('Sum')
    await new Promise(r => setTimeout(r, 400))
    expect(wholeWindowCalls()).toHaveLength(0)
  })
})
