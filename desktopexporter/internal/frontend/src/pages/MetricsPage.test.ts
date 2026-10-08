// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach, afterEach } from 'vitest'
import { fireEvent, screen, waitFor, within } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import { tick } from 'svelte'
import MetricsPage from './MetricsPage.svelte'
import type {
  DataPoint,
  MetricSummary,
  MetricViewData,
  MetricType,
  Stats,
  SumDataPoint,
} from '@/types/api-types'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import { navigateCurrentRoute, readRoute, withQueryPatch } from '@/route'
import { downloadOTLP } from '@/services/export-service'

vi.mock('@/services/export-service', () => ({ downloadOTLP: vi.fn() }))

// Only histograms request the whole-window bucket merge.

const {
  searchMetricSummaries,
  getStats,
  getMetricView,
  getMetricAggregateView,
  getMetricAttributeDefinitions,
} = vi.hoisted(() => ({
  searchMetricSummaries: vi.fn(),
  getStats: vi.fn(),
  getMetricView: vi.fn(),
  getMetricAggregateView: vi.fn(),
  getMetricAttributeDefinitions: vi.fn(),
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
      getMetricView,
      getMetricAggregateView,
      getMetricAttributeDefinitions,
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
    metricRef: 'metric-1',
    name: 'demo.metric',
    description: '',
    unit: 'ms',
    metricType,
    aggregationTemporality: metricType === 'Gauge' ? null : 'Cumulative',
    aggregationTemporalityCode: metricType === 'Gauge' ? null : 2,
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
): MetricViewData {
  return {
    lastSeenNs: 1_700_000_000_000_000_000n,
    metricRef: 'metric-1',
    name: 'demo.metric',
    description: '',
    metadata: [],
    unit: 'ms',
    metricType,
    aggregationTemporality: metricType === 'Gauge' ? null : 'Cumulative',
    aggregationTemporalityCode: metricType === 'Gauge' ? null : 2,
    isMonotonic: metricType === 'Sum' ? true : null,
    resourceDroppedAttributesCount: 0,
    resourceSchemaUrl: '',
    resource: EMPTY_RESOURCE,
    scopeName: '',
    scopeVersion: '',
    scopeSchemaUrl: '',
    scopeDroppedAttributesCount: 0,
    scope: EMPTY_SCOPE,
    timeseries: [
      {
        seriesRef: 'route=/checkout',
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
    window: {
      requested: { startNs: null, endNs: null },
      effective: {
        startNs: 1_700_000_000_000_000_000n,
        endNs: 1_700_000_001_000_000_000n,
      },
    },
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
    aggregationTemporalityCode: 2,
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
  vi.mocked(downloadOTLP).mockReset()
  vi.mocked(downloadOTLP).mockResolvedValue()
  searchMetricSummaries.mockReset()
  getStats.mockReset()
  getMetricView.mockReset()
  getMetricAggregateView.mockReset()
  getMetricAttributeDefinitions.mockReset()
  getMetricAttributeDefinitions.mockResolvedValue([])
})

afterEach(() => {
  vi.useRealTimers()
  localStorage.removeItem('time-selection')
  localStorage.removeItem('time-tz')
})

async function renderSelected(metricType: MetricType) {
  searchMetricSummaries.mockResolvedValue([makeSummary(metricType)])
  getStats.mockResolvedValue(makeStats())
  getMetricView.mockResolvedValue(makeMetric(metricType))
  getMetricAggregateView.mockResolvedValue({
    aggregate: null,
    scalarAggregate: null,
  })
  setTestUrl('/metrics/metric-1')
  renderWithContexts(MetricsPage)
  // The aggregate fetch is debounced until after detail loading.
  await waitFor(() => expect(getMetricView).toHaveBeenCalled())
  await waitFor(() => expect(getMetricAggregateView).toHaveBeenCalled(), {
    timeout: 3000,
  })
}

describe('Metric main-header export', () => {
  it.each(['Gauge', 'Sum', 'Histogram', 'ExponentialHistogram'] as const)(
    'exports the whole %s Metric from its chart title header',
    async metricType => {
      searchMetricSummaries.mockResolvedValue([makeSummary(metricType)])
      getStats.mockResolvedValue(makeStats())
      getMetricView.mockResolvedValue(makeMetric(metricType))
      getMetricAggregateView.mockResolvedValue({
        aggregate: null,
        scalarAggregate: null,
      })
      setTestUrl('/metrics/metric-1')
      renderWithContexts(MetricsPage)
      await screen.findByRole('tablist', { name: 'Metric detail tabs' })

      const header = screen.getByRole('region', { name: 'Metric chart' })
      const button = within(header).getByRole('button', {
        name: 'Export metric',
      })
      const title = within(header).getByText('demo.metric', { exact: true })
      expect(
        button.compareDocumentPosition(title) & Node.DOCUMENT_POSITION_FOLLOWING
      ).not.toBe(0)
      const chartTabs = within(header).queryByRole('tablist')
      if (chartTabs) {
        expect(
          button.compareDocumentPosition(chartTabs) &
            Node.DOCUMENT_POSITION_FOLLOWING
        ).not.toBe(0)
      }
      expect(
        screen.getAllByRole('button', { name: 'Export metric' })
      ).toHaveLength(1)
      await userEvent.click(button)
      await userEvent.click(screen.getByRole('menuitem', { name: 'OTLP JSON' }))
      expect(downloadOTLP).toHaveBeenCalledWith(
        'metric',
        'metric-1',
        'json',
        expect.any(AbortSignal)
      )
    }
  )
})

/** targetBuckets is the 4th positional argument; the whole-window call is the
 *  one that asks for exactly 1 bucket. */
function wholeWindowCalls() {
  return getMetricAggregateView.mock.calls.filter(args => args[3] === 1)
}

function rawSeriesCalls() {
  return getMetricView.mock.calls.filter(args => args[3] === 0)
}

describe('MetricsPage aggregate fetching', () => {
  it('uses an unbounded detail request and its effective window for aggregates', async () => {
    await renderSelected('Histogram')
    const detail = getMetricView.mock.calls.find(args => args[3] !== 0)
    expect(detail?.slice(1, 3)).toEqual([null, null])
    expect(getMetricAggregateView.mock.calls[0]?.slice(1, 3)).toEqual([
      1_700_000_000_000_000_000n,
      1_700_000_001_000_000_000n,
    ])
  })

  it('does not issue aggregate queries for an empty unbounded effective window', async () => {
    searchMetricSummaries.mockResolvedValue([makeSummary('Histogram')])
    getStats.mockResolvedValue(makeStats())
    const metric = makeMetric('Histogram')
    metric.window.effective = { startNs: null, endNs: null }
    getMetricView.mockResolvedValue(metric)
    getMetricAggregateView.mockResolvedValue({
      aggregate: null,
      scalarAggregate: null,
    })
    setTestUrl('/metrics/metric-1')
    renderWithContexts(MetricsPage)

    await waitFor(() => expect(getMetricView).toHaveBeenCalled())
    await new Promise(resolve => setTimeout(resolve, 300))
    expect(getMetricAggregateView).not.toHaveBeenCalled()
  })

  it('passes a named timezone through for calendar bucket alignment', async () => {
    localStorage.setItem('time-tz', 'America/New_York')
    await renderSelected('Gauge')
    const detailCall = getMetricView.mock.calls[0]
    expect(detailCall[6]).toEqual(expect.any(Number))
    expect(detailCall[10]).toBe('America/New_York')
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
    // Scalar aggregates use the bucketed call.
    expect(getMetricAggregateView.mock.calls.length).toBeGreaterThan(0)
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
      seriesRef: `series-${index}`,
    }))

    searchMetricSummaries.mockResolvedValue([
      { ...makeSummary('Sum'), seriesCount: metric.timeseries.length },
    ])
    getStats.mockResolvedValue(makeStats())
    getMetricView.mockResolvedValue(metric)
    getMetricAggregateView.mockResolvedValue({
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
    getMetricAggregateView.mockResolvedValue({
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
        type: 'preset',
        durationMs: duration,
      })
    )
    prepareRawSeriesTest()
    getMetricView.mockImplementation((...args: unknown[]) =>
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
      BigInt(requestNow - duration) * 1_000_000n,
      BigInt(requestNow) * 1_000_000n,
    ])
  })

  it('starts a new-window request while the old-window request is in flight', async () => {
    prepareRawSeriesTest()
    const stale = deferred<MetricViewData | null>()
    const current = deferred<MetricViewData | null>()
    let rawRequest = 0
    getMetricView.mockImplementation((...args: unknown[]) => {
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
    expect(rawSeriesCalls()[0]?.slice(0, 3)).toEqual([
      'metric-1',
      100_000_000n,
      200_000_000n,
    ])

    navigateCurrentRoute(
      withQueryPatch(readRoute().query, { start: '300', end: '400' }),
      'replace'
    )

    await waitFor(() => expect(rawSeriesCalls()).toHaveLength(2))
    expect(rawSeriesCalls()[1]?.slice(0, 3)).toEqual([
      'metric-1',
      300_000_000n,
      400_000_000n,
    ])

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

  const terminalResponses: Array<[string, () => MetricViewData | null]> = [
    ['a null result', () => null],
    ['an omitted series', () => ({ ...makeMetric('Sum'), timeseries: [] })],
    ['an empty requested series', () => makeMetric('Sum')],
  ]

  it.each(terminalResponses)(
    'publishes terminal empty datapoints for %s',
    async (_name, response) => {
      prepareRawSeriesTest()
      const raw = deferred<MetricViewData | null>()
      getMetricView.mockImplementation((...args: unknown[]) =>
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
