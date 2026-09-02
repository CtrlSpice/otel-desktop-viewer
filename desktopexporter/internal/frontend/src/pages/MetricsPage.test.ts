// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import { tick } from 'svelte'
import MetricsPage from './MetricsPage.svelte'
import type {
  DataPoint,
  MetricSummary,
  MetricData,
  MetricType,
  Stats,
  SumDataPoint,
} from '@/types/api-types'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import { navigateCurrentRoute, readRoute, withQueryPatch } from '@/route'

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

function makeMetric(
  metricType: MetricType,
  datapoints: DataPoint[] = []
): MetricData {
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
        datapoints,
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

function makeSumDatapoint(
  id: string,
  timestampMs: number,
  value: number
): SumDataPoint {
  const timestamp = BigInt(timestampMs) * 1_000_000n
  return {
    id,
    timestamp,
    timestampMs,
    startTime: timestamp,
    flags: 0,
    exemplars: [],
    metricType: 'Sum',
    doubleValue: value,
    intValue: null,
    valueType: 'double',
    isMonotonic: true,
    aggregationTemporality: 'Cumulative',
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  const promise = new Promise<T>(next => {
    resolve = next
  })
  return { promise, resolve }
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
    rejections: [],
  }
}

beforeEach(() => {
  if (typeof Element.prototype.scrollTo !== 'function') {
    Element.prototype.scrollTo = () => {}
  }
  if (typeof Element.prototype.scrollIntoView !== 'function') {
    Element.prototype.scrollIntoView = () => {}
  }
  searchMetricSummaries.mockReset()
  getStats.mockReset()
  getMetric.mockReset()
  getMetricAggregate.mockReset()
})

afterEach(() => {
  vi.useRealTimers()
  localStorage.removeItem('time-selection')
  localStorage.removeItem('time-tz')
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

function rawSeriesCalls() {
  return getMetric.mock.calls.filter(args => args[3] === 0)
}

describe('MetricsPage aggregate fetching', () => {
  it('passes a named timezone through for calendar bucket alignment', async () => {
    localStorage.setItem('time-tz', 'America/New_York')
    await renderSelected('Gauge')
    const detailCall = getMetric.mock.calls[0]
    expect(detailCall[6]).toEqual(expect.any(Number))
    expect(detailCall[12]).toBe('America/New_York')
  })

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

describe('MetricsPage chart control keyboard navigation', () => {
  it('toggles rate and overlays with Space while retaining focus', async () => {
    const user = userEvent.setup()
    const datapoints = [
      makeSumDatapoint('dp-1', 1_700_000_000_000, 10),
      makeSumDatapoint('dp-2', 1_700_000_001_000, 20),
    ]
    const metric = makeMetric('Sum', datapoints)
    const template = metric.timeseries[0]!
    metric.timeseries = Array.from({ length: 11 }, (_, index) => ({
      ...template,
      attributesKey: `series-${index}`,
    }))

    searchMetricSummaries.mockResolvedValue([
      { ...makeSummary('Sum'), seriesCount: metric.timeseries.length },
    ])
    getStats.mockResolvedValue(makeStats())
    getMetric.mockResolvedValue(metric)
    getMetricAggregate.mockResolvedValue({
      aggregate: null,
      scalarAggregate: null,
    })
    setTestUrl('/metrics/metric-1')
    renderWithContexts(MetricsPage)

    const rate = await screen.findByRole('checkbox', {
      name: 'Show rate across all series',
    })
    const overlays = screen.getByRole('checkbox', {
      name: 'Show chart stat overlays',
    })

    rate.focus()
    expect(rate).toHaveFocus()
    expect(rate).not.toBeChecked()
    await user.keyboard(' ')
    expect(rate).toBeChecked()
    expect(rate).toHaveFocus()

    await user.tab()
    expect(overlays).toHaveFocus()
    expect(overlays).toBeChecked()
    await user.keyboard(' ')
    expect(overlays).not.toBeChecked()
    expect(overlays).toHaveFocus()
  })
})

describe('MetricsPage raw series fetching', () => {
  const reduced = makeSumDatapoint('dp-reduced', 1_700_000_000_000, 1)

  function prepareRawSeriesTest() {
    searchMetricSummaries.mockResolvedValue([makeSummary('Sum')])
    getStats.mockResolvedValue(makeStats())
    getMetricAggregate.mockResolvedValue({
      aggregate: null,
      scalarAggregate: null,
    })
  }

  it('anchors a preset window when the series request is triggered', async () => {
    const initialNow = 1_700_000_000_000
    const requestNow = initialNow + 5 * 60_000
    const duration = 15 * 60_000
    vi.useFakeTimers()
    vi.setSystemTime(initialNow)
    localStorage.setItem(
      'time-selection',
      JSON.stringify({
        start: initialNow - duration,
        end: initialNow,
        type: 'preset',
        presetIndex: 1,
      })
    )
    prepareRawSeriesTest()
    getMetric.mockImplementation((...args: unknown[]) =>
      Promise.resolve(
        args[3] === 0 ? makeMetric('Sum') : makeMetric('Sum', [reduced])
      )
    )
    setTestUrl('/metrics/metric-1')

    renderWithContexts(MetricsPage)
    await vi.waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Expand default series' })
      ).toBeInTheDocument()
    )
    expect(rawSeriesCalls()).toHaveLength(0)

    vi.setSystemTime(requestNow)
    await fireEvent.click(
      screen.getByRole('button', { name: 'Expand default series' })
    )
    await tick()

    expect(rawSeriesCalls()).toHaveLength(1)
    expect(rawSeriesCalls()[0]?.slice(1, 3)).toEqual([
      requestNow - duration,
      requestNow,
    ])
  })

  it('starts a new-window request while the old-window request is in flight', async () => {
    prepareRawSeriesTest()
    const stale = deferred<MetricData | null>()
    const current = deferred<MetricData | null>()
    let rawRequest = 0
    getMetric.mockImplementation((...args: unknown[]) => {
      if (args[3] !== 0) {
        return Promise.resolve(makeMetric('Sum', [reduced]))
      }
      return rawRequest++ === 0 ? stale.promise : current.promise
    })
    setTestUrl(
      '/metrics/metric-1?start=100&end=200&series=route%3D%2Fcheckout&dp=dp-current'
    )

    renderWithContexts(MetricsPage)
    await waitFor(() => expect(rawSeriesCalls()).toHaveLength(1))
    expect(rawSeriesCalls()[0]?.slice(0, 3)).toEqual(['metric-1', 100, 200])

    navigateCurrentRoute(
      withQueryPatch(readRoute().query, { start: '300', end: '400' }),
      'replace'
    )

    await waitFor(() => expect(rawSeriesCalls()).toHaveLength(2))
    expect(rawSeriesCalls()[1]?.slice(0, 3)).toEqual(['metric-1', 300, 400])

    current.resolve(
      makeMetric('Sum', [makeSumDatapoint('dp-current', 1_700_000_000_300, 3)])
    )
    await waitFor(() =>
      expect(
        document.querySelector('tr[data-dp-id="dp-current"]')
      ).not.toBeNull()
    )

    stale.resolve(
      makeMetric('Sum', [makeSumDatapoint('dp-stale', 1_700_000_000_100, 2)])
    )
    await stale.promise
    await tick()

    expect(document.querySelector('tr[data-dp-id="dp-current"]')).not.toBeNull()
    expect(document.querySelector('tr[data-dp-id="dp-stale"]')).toBeNull()
  })

  const terminalResponses: Array<[string, () => MetricData | null]> = [
    ['a null result', () => null],
    ['an omitted series', () => ({ ...makeMetric('Sum'), timeseries: [] })],
    ['an empty requested series', () => makeMetric('Sum')],
  ]

  it.each(terminalResponses)(
    'publishes terminal empty datapoints for %s',
    async (_name, response) => {
      prepareRawSeriesTest()
      const raw = deferred<MetricData | null>()
      getMetric.mockImplementation((...args: unknown[]) =>
        args[3] === 0
          ? raw.promise
          : Promise.resolve(makeMetric('Sum', [reduced]))
      )
      setTestUrl(
        '/metrics/metric-1?start=100&end=200&series=route%3D%2Fcheckout&dp=dp-missing'
      )

      renderWithContexts(MetricsPage)
      await waitFor(() => expect(rawSeriesCalls()).toHaveLength(1))
      expect(
        document.querySelector('tr[data-dp-id="dp-reduced"]')
      ).not.toBeNull()

      raw.resolve(response())
      await raw.promise
      await waitFor(() =>
        expect(document.querySelector('tr[data-dp-id="dp-reduced"]')).toBeNull()
      )

      const user = userEvent.setup()
      await user.click(
        screen.getByRole('button', { name: 'Collapse default series' })
      )
      await user.click(
        screen.getByRole('button', { name: 'Expand default series' })
      )
      await tick()
      expect(rawSeriesCalls()).toHaveLength(1)
    }
  )
})
