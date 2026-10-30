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
import {
  navigateCurrentRoute,
  navigateToItem,
  readRoute,
  withQueryPatch,
} from '@/route'
import { downloadOTLP } from '@/services/export-service'
import { savePersistedTimeseriesVisible } from '@/components/metrics/utils/metric-timeseries-visible'

vi.mock('@/services/export-service', () => ({ downloadOTLP: vi.fn() }))

// Only histograms request the whole-window bucket merge.

const {
  searchMetricSummaries,
  getStats,
  getMetricView,
  getMetric,
  getMetricSeries,
  getMetricSeriesOrder,
  getMetricAggregateView,
  getMetricAttributeDefinitions,
  clearMetrics,
} = vi.hoisted(() => ({
  searchMetricSummaries: vi.fn(),
  getStats: vi.fn(),
  getMetricView: vi.fn(),
  getMetric: vi.fn(),
  getMetricSeries: vi.fn(),
  getMetricSeriesOrder: vi.fn(),
  getMetricAggregateView: vi.fn(),
  getMetricAttributeDefinitions: vi.fn(),
  clearMetrics: vi.fn(),
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
      getMetric,
      getMetricSeries,
      getMetricSeriesOrder,
      getMetricAggregateView,
      getMetricAttributeDefinitions,
      clearMetrics,
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
  let reject!: (reason: Error) => void
  const promise = new Promise<T>((next, fail) => {
    resolve = next
    reject = fail
  })
  return { promise, resolve, reject }
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
  clearMetrics.mockReset()
  clearMetrics.mockResolvedValue(undefined)
  vi.mocked(downloadOTLP).mockReset()
  vi.mocked(downloadOTLP).mockResolvedValue()
  searchMetricSummaries.mockReset()
  getStats.mockReset()
  getMetricView.mockReset()
  getMetric.mockReset()
  getMetricSeries.mockReset()
  getMetricSeriesOrder.mockReset()
  getMetric.mockResolvedValue(null)
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
  if (metricType === 'Histogram' || metricType === 'ExponentialHistogram')
    await waitFor(() => expect(getMetricAggregateView).toHaveBeenCalled(), {
      timeout: 3000,
    })
}

describe('Metric list and freshness statistics', () => {
  it.each(['search', 'refresh'] as const)(
    'retains the selected chart and details through a failed %s and recovery',
    async action => {
      await renderSelected('Gauge')
      const chart = document.querySelector('.metrics-page__chart')!
      const chartContent = chart.firstElementChild
      const fields = screen.getByRole('table', { name: 'Metric fields' })
      const header = screen.getByRole('region', { name: 'Metric chart' })
      searchMetricSummaries.mockRejectedValueOnce(
        new Error('summaries unavailable')
      )
      if (action === 'search') {
        navigateCurrentRoute(
          withQueryPatch(readRoute().query, { q: 'name = "current"' }),
          'replace'
        )
      } else
        await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
      await screen.findByText('Error: summaries unavailable')
      expect(chart.isConnected).toBe(true)
      expect(chart.firstElementChild).toBe(chartContent)
      expect(fields.isConnected).toBe(true)
      expect(header.isConnected).toBe(true)
      expect(window.location.pathname).toBe('/metrics/metric-1')
      await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
      await waitFor(() =>
        expect(screen.queryByText('Error: summaries unavailable')).toBeNull()
      )
      expect(document.querySelector('.metrics-page__chart')).toBe(chart)
      expect(fields.isConnected).toBe(true)
    }
  )

  it.each(['pending', 'failed'] as const)(
    'publishes the list and loads its selection while statistics are %s',
    async outcome => {
      const stats = deferred<Stats>()
      const summaries = deferred<MetricSummary[]>()
      searchMetricSummaries.mockReturnValue(summaries.promise)
      getStats.mockReturnValue(stats.promise)
      getMetricView.mockResolvedValue(makeMetric('Gauge'))
      setTestUrl('/metrics?time=all')
      renderWithContexts(MetricsPage)
      await waitFor(() =>
        expect(searchMetricSummaries).toHaveBeenCalledTimes(1)
      )
      expect(getStats).toHaveBeenCalledTimes(1)
      if (outcome === 'failed')
        stats.reject(new Error('Statistics unavailable'))
      summaries.resolve([makeSummary('Gauge')])
      await waitFor(() => expect(getMetricView).toHaveBeenCalled())
      expect(
        await screen.findByRole('table', { name: 'Metric fields' })
      ).toHaveTextContent('demo.metric')
      expect(window.location.pathname).toBe('/metrics/metric-1')
      expect(screen.queryByText(/Statistics unavailable/)).toBeNull()
      expect(screen.queryByText('Loading metrics…')).toBeNull()
    }
  )

  it('publishes refreshed summaries without waiting for their statistics', async () => {
    await renderSelected('Gauge')
    const fields = screen.getByRole('table', { name: 'Metric fields' })
    const stats = deferred<Stats>()
    getStats.mockReturnValue(stats.promise)
    searchMetricSummaries.mockResolvedValue([
      { ...makeSummary('Gauge'), name: 'fresh.metric' },
    ])
    await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() => expect(searchMetricSummaries).toHaveBeenCalledTimes(2))
    expect(
      await screen.findByRole('button', { name: /fresh.metric/ })
    ).toBeInTheDocument()
    expect(getStats).toHaveBeenCalledTimes(2)
    expect(fields.isConnected).toBe(true)
  })

  it.each(['success', 'failure'] as const)(
    'rejects stale statistics %s after refresh and keeps polling owned by the displayed list',
    async outcome => {
      vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
      const stale = deferred<Stats>()
      getStats.mockReturnValueOnce(stale.promise)
      searchMetricSummaries.mockResolvedValue([makeSummary('Gauge')])
      getMetricView.mockResolvedValue(makeMetric('Gauge'))
      setTestUrl('/metrics/metric-1?time=all')
      const view = renderWithContexts(MetricsPage)
      await screen.findByRole('table', { name: 'Metric fields' })
      const staleSignal = getStats.mock.calls[0]![0] as AbortSignal
      await vi.advanceTimersByTimeAsync(6000)
      expect(getStats).toHaveBeenCalledTimes(1)

      const refreshed = makeStats()
      refreshed.metrics = {
        ...refreshed.metrics,
        metricCount: 10,
        dataPointCount: 40,
      }
      getStats.mockResolvedValueOnce(refreshed)
      await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
      await waitFor(() => expect(getStats).toHaveBeenCalledTimes(2))
      expect(staleSignal.aborted).toBe(true)
      if (outcome === 'success') stale.resolve(makeStats())
      else stale.reject(new Error('obsolete statistics failed'))
      await tick()

      const polled = makeStats()
      polled.metrics = {
        ...polled.metrics,
        metricCount: 11,
        dataPointCount: 44,
      }
      getStats.mockResolvedValueOnce(polled)
      await vi.advanceTimersByTimeAsync(3000)
      expect(
        screen.getByRole('button', { name: 'Refresh: +1 metric, +4 dps' })
      ).toBeInTheDocument()

      const pendingPoll = deferred<Stats>()
      getStats.mockReturnValueOnce(pendingPoll.promise)
      await vi.advanceTimersByTimeAsync(3000)
      const pollSignal = getStats.mock.calls.at(-1)![0] as AbortSignal
      view.unmount()
      expect(pollSignal.aborted).toBe(true)
      pendingPoll.resolve(makeStats())
      await vi.advanceTimersByTimeAsync(6000)
      expect(getStats).toHaveBeenCalledTimes(4)
    }
  )

  it('recovers freshness polling after a failed baseline without discarding the loaded list', async () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
    getStats.mockRejectedValueOnce(new Error('stats unavailable'))
    searchMetricSummaries.mockResolvedValue([makeSummary('Gauge')])
    getMetricView.mockResolvedValue(makeMetric('Gauge'))
    setTestUrl('/metrics/metric-1?time=all')
    renderWithContexts(MetricsPage)
    const fields = await screen.findByRole('table', { name: 'Metric fields' })
    getStats.mockResolvedValueOnce(makeStats())
    await vi.advanceTimersByTimeAsync(3000)
    expect(screen.getByRole('button', { name: 'Refresh' })).toBeInTheDocument()
    const polled = makeStats()
    polled.metrics = { ...polled.metrics, metricCount: 2, dataPointCount: 8 }
    getStats.mockResolvedValueOnce(polled)
    await vi.advanceTimersByTimeAsync(3000)
    expect(
      screen.getByRole('button', { name: 'Refresh: +1 metric, +4 dps' })
    ).toBeInTheDocument()
    expect(fields.isConnected).toBe(true)
    expect(searchMetricSummaries).toHaveBeenCalledTimes(1)
    expect(screen.queryByText(/stats unavailable/)).toBeNull()
  })

  it('preserves the displayed freshness baseline after a failed list refresh', async () => {
    vi.useFakeTimers({ toFake: ['setInterval', 'clearInterval'] })
    await renderSelected('Gauge')
    const polled = makeStats()
    polled.metrics = { ...polled.metrics, metricCount: 2, dataPointCount: 8 }
    getStats.mockResolvedValueOnce(polled)
    await vi.advanceTimersByTimeAsync(3000)
    const refresh = screen.getByRole('button', {
      name: 'Refresh: +1 metric, +4 dps',
    })
    const stats = deferred<Stats>()
    getStats.mockReturnValueOnce(stats.promise)
    searchMetricSummaries.mockRejectedValueOnce(new Error('list unavailable'))
    await fireEvent.click(refresh)
    await screen.findByText('Error: list unavailable')
    expect(refresh).toHaveAccessibleName('Refresh: +1 metric, +4 dps')
    expect((getStats.mock.calls.at(-1)![0] as AbortSignal).aborted).toBe(true)
    stats.resolve(makeStats())
    await tick()
    expect(refresh).toHaveAccessibleName('Refresh: +1 metric, +4 dps')
  })

  it('cancels both outstanding list and baseline requests on unmount', async () => {
    const summaries = deferred<MetricSummary[]>()
    const stats = deferred<Stats>()
    searchMetricSummaries.mockReturnValue(summaries.promise)
    getStats.mockReturnValue(stats.promise)
    setTestUrl('/metrics?time=all')
    const view = renderWithContexts(MetricsPage)
    await waitFor(() => expect(searchMetricSummaries).toHaveBeenCalledTimes(1))
    const listSignal = searchMetricSummaries.mock.calls[0]!.at(
      -1
    ) as AbortSignal
    const statsSignal = getStats.mock.calls[0]![0] as AbortSignal
    view.unmount()
    expect(listSignal.aborted).toBe(true)
    expect(statsSignal.aborted).toBe(true)
    summaries.resolve([makeSummary('Gauge')])
    stats.resolve(makeStats())
    await tick()
    expect(getMetricView).not.toHaveBeenCalled()
  })
})

describe('Metric main-header range', () => {
  it.each(['failure', 'missing'] as const)(
    'reports an out-of-list metric lookup %s and retries without changing the URL',
    async outcome => {
      searchMetricSummaries.mockResolvedValue([])
      getStats.mockResolvedValue(makeStats())
      if (outcome === 'failure')
        getMetric.mockRejectedValueOnce(new Error('lookup failed'))
      else getMetric.mockResolvedValueOnce(null)
      getMetric.mockResolvedValueOnce(makeMetric('Gauge'))
      getMetricView.mockResolvedValue(makeMetric('Gauge'))
      const consoleError = vi
        .spyOn(console, 'error')
        .mockImplementation(() => {})
      setTestUrl('/metrics/metric-1?q=name%20%3D%20%22excluded%22')
      renderWithContexts(MetricsPage)
      const message =
        outcome === 'failure'
          ? 'Could not load the selected metric.'
          : 'The selected metric was not found.'
      expect(await screen.findByRole('alert')).toHaveTextContent(message)
      expect(screen.queryByText('No metrics in this time range')).toBeNull()
      expect(getMetricView).not.toHaveBeenCalled()
      await fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
      expect(
        await screen.findByRole('table', { name: 'Metric fields' })
      ).toHaveTextContent('demo.metric')
      expect(getMetric).toHaveBeenCalledTimes(2)
      expect(screen.queryByRole('alert')).toBeNull()
      expect(window.location.pathname).toBe('/metrics/metric-1')
      expect(new URLSearchParams(window.location.search).get('q')).toBe(
        'name = "excluded"'
      )
      consoleError.mockRestore()
    }
  )

  it('retains the displayed metric while an out-of-list identity loads and ignores its late response after navigation', async () => {
    await renderSelected('Gauge')
    const fields = await screen.findByRole('table', { name: 'Metric fields' })
    const header = screen.getByRole('region', { name: 'Metric chart' })
    const pending = deferred<ReturnType<typeof makeMetric> | null>()
    getMetric.mockReturnValueOnce(pending.promise)
    navigateToItem('metrics', 'outside-list')
    await waitFor(() =>
      expect(getMetric).toHaveBeenCalledWith(
        'outside-list',
        expect.any(AbortSignal)
      )
    )
    expect(fields.isConnected).toBe(true)
    expect(header).toHaveTextContent('demo.metric')
    expect(await screen.findByText('Loading selected metric…')).toBeVisible()
    await fireEvent.click(
      within(header).getByRole('button', { name: 'Download metric' })
    )
    expect(downloadOTLP).toHaveBeenCalledWith(
      'metric',
      'metric-1',
      expect.any(AbortSignal)
    )
    navigateToItem('metrics', 'metric-1')
    await tick()
    pending.resolve({
      ...makeMetric('Gauge'),
      metricRef: 'outside-list',
      name: 'late.metric',
    })
    await tick()
    await waitFor(() =>
      expect(screen.queryByText('Loading selected metric…')).toBeNull()
    )
    expect(header).toHaveTextContent('demo.metric')
    expect(fields.isConnected).toBe(true)
    expect(screen.queryByText('late.metric')).toBeNull()
    expect(getMetricView.mock.calls.every(call => call[0] === 'metric-1')).toBe(
      true
    )
  })

  it('opens an exact route-selected metric outside the filtered list on a fresh mount', async () => {
    searchMetricSummaries.mockResolvedValue([])
    getStats.mockResolvedValue(makeStats())
    getMetric.mockResolvedValue({
      metricRef: 'metric-1',
      name: 'demo.metric',
      description: '',
      unit: 'ms',
      metricType: 'Gauge',
      metadata: [],
      series: [],
      resource: {
        ...EMPTY_RESOURCE,
        schemaUrl: '',
        attributes: [
          {
            key: 'service.name',
            value: { kind: 'string', value: 'checkout-api' },
          },
        ],
      },
      scope: { ...EMPTY_SCOPE, schemaUrl: '' },
    })
    getMetricView.mockResolvedValue(makeMetric('Gauge'))
    getMetricAggregateView.mockResolvedValue({
      aggregate: null,
      scalarAggregate: null,
    })
    setTestUrl('/metrics/metric-1?q=name%20%3D%20%22excluded%22')
    renderWithContexts(MetricsPage)
    await waitFor(() =>
      expect(getMetric).toHaveBeenCalledWith(
        'metric-1',
        expect.any(AbortSignal)
      )
    )
    await waitFor(() => expect(getMetricView).toHaveBeenCalled())
    expect(
      await screen.findByRole('region', { name: 'Metric chart' })
    ).toHaveTextContent('demo.metric')
    expect(window.location.pathname).toBe('/metrics/metric-1')
    expect(screen.queryByText('No metrics in this time range')).toBeNull()
    expect(document.querySelector('.metrics-page__chart')).not.toBeNull()
    expect(new URLSearchParams(window.location.search).get('q')).toBe(
      'name = "excluded"'
    )
  })
  it('requires confirmation before deleting all stored metrics', async () => {
    await renderSelected('Gauge')
    const confirm = vi.spyOn(window, 'confirm').mockReturnValue(false)
    const button = screen.getByRole('button', { name: 'Delete all metrics' })
    await fireEvent.click(button)
    expect(confirm).toHaveBeenCalledWith(
      expect.stringContaining('including those outside the current filters')
    )
    expect(clearMetrics).not.toHaveBeenCalled()
    expect(window.location.pathname).toBe('/metrics/metric-1')
    confirm.mockReturnValue(true)
    await fireEvent.click(button)
    await waitFor(() => expect(clearMetrics).toHaveBeenCalledTimes(1))
    confirm.mockRestore()
  })
  it.each(['Gauge', 'Sum'] as const)(
    'omits the duplicate %s time range from the compact header and plot',
    async metricType => {
      await renderSelected(metricType)
      const header = screen.getByRole('region', {
        name: 'Metric chart',
      })
      expect(header.querySelector('[aria-label="Chart time range"]')).toBeNull()
      expect(
        document.querySelectorAll('[aria-label="Chart time range"]')
      ).toHaveLength(0)
    }
  )

  it.each(['Histogram', 'ExponentialHistogram'] as const)(
    'removes the redundant %s time range from the count-legend header',
    async metricType => {
      await renderSelected(metricType)
      const header = await screen.findByRole('region', { name: 'Metric chart' })
      expect(header.querySelector('[aria-label="Chart time range"]')).toBeNull()
      expect(
        document.querySelector('.metric-chart-view .heatmap-legend')
      ).toBeNull()
    }
  )
})

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
      await screen.findByRole('region', { name: 'Metric details' })

      const header = await screen.findByRole('region', { name: 'Metric chart' })
      const button = within(header).getByRole('button', {
        name: 'Download metric',
      })
      const title = within(header).getByText('demo.metric', { exact: true })
      expect(
        title.compareDocumentPosition(button) & Node.DOCUMENT_POSITION_FOLLOWING
      ).not.toBe(0)
      expect(
        within(button.closest<HTMLElement>('.header-controls')!)
          .getAllByRole('button')
          .at(-1)
      ).toBe(button)
      const chartTabs = within(header).queryByRole('tablist')
      if (chartTabs) {
        expect(
          chartTabs.compareDocumentPosition(button) &
            Node.DOCUMENT_POSITION_FOLLOWING
        ).not.toBe(0)
      }
      expect(
        screen.getAllByRole('button', { name: 'Download metric' })
      ).toHaveLength(1)
      await userEvent.click(button)
      expect(downloadOTLP).toHaveBeenCalledWith(
        'metric',
        'metric-1',
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
    expect(getMetricAggregateView).not.toHaveBeenCalled()
  })

  it('does not ask a Sum either', async () => {
    await renderSelected('Sum')
    await new Promise(r => setTimeout(r, 400))
    expect(wholeWindowCalls()).toHaveLength(0)
  })
})

describe('MetricsPage chart control keyboard navigation', () => {
  it('keeps Rate separate and restores the combined line after switching back to Values', async () => {
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
    setTestUrl('/metrics/metric-1?agg=sum')
    renderWithContexts(MetricsPage)

    const sum = await screen.findByRole('checkbox', {
      name: 'Show combined sum',
    })
    const average = screen.getByRole('checkbox', {
      name: 'Show combined average',
    })
    expect(sum).toBeChecked()
    expect(average).not.toBeChecked()
    average.focus()
    expect(average.closest('.pane-header')).not.toBeNull()
    await user.keyboard(' ')
    expect(average).toBeChecked()
    expect(average).toHaveFocus()
    expect(sum).toBeChecked()
    expect(window.location.search).toContain('agg=sum-avg')
    const rate = screen.getByRole('tab', { name: 'Rate' })
    await user.click(rate)
    expect(rate).toHaveAttribute('aria-selected', 'true')
    await user.click(screen.getByRole('tab', { name: 'Values' }))
    expect(
      screen.getByRole('checkbox', { name: 'Show combined sum' })
    ).toBeChecked()
    expect(
      screen.getByRole('checkbox', { name: 'Show combined average' })
    ).toBeChecked()
    await user.click(
      screen.getByRole('checkbox', { name: 'Show combined sum' })
    )
    expect(
      screen.getByRole('checkbox', { name: 'Show combined average' })
    ).toBeChecked()
    expect(window.location.search).toContain('agg=avg')
    await user.click(
      screen.getByRole('checkbox', { name: 'Show combined average' })
    )
    expect(window.location.search).toContain('agg=raw')
    expect(screen.queryByRole('tab', { name: 'Raw' })).toBeNull()
    expect(screen.queryByRole('tab', { name: 'Sum' })).toBeNull()
    expect(screen.queryByRole('tab', { name: 'Average' })).toBeNull()
  })
})

describe('Metric Details request lifecycle', () => {
  function prepare() {
    searchMetricSummaries.mockResolvedValue([
      makeSummary('Gauge'),
      { ...makeSummary('Gauge'), metricRef: 'next', name: 'next.metric' },
    ])
    getStats.mockResolvedValue(makeStats())
    getMetricView.mockImplementation((id: string) =>
      Promise.resolve({
        ...makeMetric('Gauge'),
        metricRef: id,
        timeseries: [
          {
            ...makeMetric('Gauge').timeseries[0]!,
            attributesComplete: false,
            labelPreviews: [],
          },
        ],
      })
    )
    setTestUrl('/metrics/metric-1?time=all')
  }

  async function expand() {
    await fireEvent.click(
      await screen.findByRole('button', {
        name: 'Expand attributes for default series',
      })
    )
  }

  it('aborts replaced views and ignores an old A response after returning from B', async () => {
    prepare()
    const old = deferred<MetricViewData | null>()
    getMetricView.mockImplementationOnce(() => old.promise)
    const view = renderWithContexts(MetricsPage)
    await waitFor(() => expect(getMetricView).toHaveBeenCalledTimes(1))
    const signal = getMetricView.mock.calls[0]!.at(-1) as AbortSignal
    navigateToItem('metrics', 'next')
    await waitFor(() => expect(getMetricView).toHaveBeenCalledTimes(2))
    expect(signal.aborted).toBe(true)
    navigateToItem('metrics', 'metric-1')
    await waitFor(() => expect(getMetricView).toHaveBeenCalledTimes(3))
    old.resolve({ ...makeMetric('Gauge'), description: 'stale description' })
    await tick()
    expect(screen.queryByText('stale description')).toBeNull()
    const latestSignal = getMetricView.mock.calls.at(-1)!.at(-1) as AbortSignal
    view.unmount()
    expect(latestSignal.aborted).toBe(true)
  })

  it('rejects old attributes after A→B→A and cancels disclosure/unmount work', async () => {
    prepare()
    const old = deferred<unknown>()
    const current = deferred<unknown>()
    getMetricSeries
      .mockReturnValueOnce(old.promise)
      .mockReturnValueOnce(current.promise)
    const view = renderWithContexts(MetricsPage)
    await expand()
    await waitFor(() => expect(getMetricSeries).toHaveBeenCalledTimes(1))
    const oldSignal = getMetricSeries.mock.calls[0]!.at(-1) as AbortSignal
    navigateToItem('metrics', 'next')
    await screen.findByRole('button', {
      name: 'Expand attributes for default series',
    })
    expect(oldSignal.aborted).toBe(true)
    navigateToItem('metrics', 'metric-1')
    await waitFor(() => expect(getMetricView).toHaveBeenCalledTimes(3))
    await expand()
    await waitFor(() => expect(getMetricSeries).toHaveBeenCalledTimes(2))
    old.resolve({
      attributes: [
        { key: 'context', value: { kind: 'string', value: 'stale context' } },
      ],
    })
    await tick()
    expect(screen.queryByText('stale context')).toBeNull()
    expect(
      screen.queryByText('Loading series attributes…')
    ).not.toBeInTheDocument()
    expect(await screen.findByText('Loading series attributes…')).toBeVisible()
    const currentSignal = getMetricSeries.mock.calls[1]!.at(-1) as AbortSignal
    await fireEvent.click(
      screen.getByRole('button', {
        name: 'Collapse attributes for default series',
      })
    )
    expect(currentSignal.aborted).toBe(true)
    getMetricSeries.mockReturnValue(new Promise(() => {}))
    await expand()
    await waitFor(() => expect(getMetricSeries).toHaveBeenCalledTimes(3))
    const lastSignal = getMetricSeries.mock.calls[2]!.at(-1) as AbortSignal
    view.unmount()
    expect(lastSignal.aborted).toBe(true)
  })

  it('refreshes same-ID expanded attributes while preserving disclosure', async () => {
    prepare()
    getMetricSeries
      .mockResolvedValueOnce({
        attributes: [
          { key: 'context', value: { kind: 'string', value: 'old context' } },
        ],
      })
      .mockResolvedValueOnce({
        attributes: [
          { key: 'context', value: { kind: 'string', value: 'fresh context' } },
        ],
      })
    renderWithContexts(MetricsPage)
    await expand()
    await screen.findByText('old context')
    await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await screen.findByText('fresh context')
    expect(screen.queryByText('old context')).toBeNull()
    expect(getMetricSeries).toHaveBeenCalledTimes(2)
    expect(getMetricView).toHaveBeenCalledTimes(2)
    expect(
      screen.getByRole('button', {
        name: 'Collapse attributes for default series',
      })
    ).toHaveAttribute('aria-expanded', 'true')
  })
})

describe('Database metric series sorting', () => {
  function prepare() {
    const metric = makeMetric('Gauge')
    metric.window.requested = {
      startNs: 9007199254740993n,
      endNs: 9007199254741993n,
    }
    metric.timeseries = ['a', 'b', 'c'].map(seriesRef => ({
      ...metric.timeseries[0]!,
      seriesRef,
      attributes: [],
      attributesComplete: false,
      labelPreviews: [
        { key: 'n', text: 'bounded preview' },
        { key: 'other', text: 'other preview' },
      ],
    }))
    searchMetricSummaries.mockResolvedValue([makeSummary('Gauge')])
    getStats.mockResolvedValue(makeStats())
    getMetricView.mockImplementation(() => Promise.resolve({ ...metric }))
    setTestUrl('/metrics/metric-1?time=all')
    return { ...renderWithContexts(MetricsPage), metric }
  }
  const values = ['b', 'a', 'c']
  const order = () =>
    [...document.querySelectorAll('tr[data-series-key]')].map(row =>
      row.getAttribute('data-series-key')
    )

  it('sends the displayed window and cohort, caches each direction and reloads on refresh', async () => {
    getMetricSeriesOrder.mockImplementation(
      (_metric, _start, _end, _refs, _key, direction) =>
        Promise.resolve(direction === 'asc' ? values : ['c', 'a', 'b'])
    )
    prepare()
    const sort = await screen.findByRole('button', {
      name: 'Sort by n',
    })
    expect(getMetricSeriesOrder).not.toHaveBeenCalled()
    await fireEvent.click(sort)
    await waitFor(() => expect(getMetricSeriesOrder).toHaveBeenCalledTimes(1))
    expect(getMetricSeriesOrder.mock.calls[0]!.slice(0, 6)).toEqual([
      'metric-1',
      9007199254740993n,
      9007199254741993n,
      ['a', 'b', 'c'],
      'n',
      'asc',
    ])
    await waitFor(() => expect(order()).toEqual(values))
    await fireEvent.click(sort)
    await waitFor(() => expect(order()).toEqual(['c', 'a', 'b']))
    expect(getMetricSeriesOrder).toHaveBeenCalledTimes(2)
    await fireEvent.click(sort)
    expect(order()).toEqual(['a', 'b', 'c'])
    await fireEvent.click(sort)
    await waitFor(() => expect(order()).toEqual(values))
    expect(getMetricSeriesOrder).toHaveBeenCalledTimes(2)
    await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() => expect(getMetricSeriesOrder).toHaveBeenCalledTimes(3))
    await waitFor(() => expect(order()).toEqual(values))
  })

  it('rejects a pending comparison from before a same-ID refresh', async () => {
    const stale = deferred<typeof values>()
    getMetricSeriesOrder
      .mockReturnValueOnce(stale.promise)
      .mockResolvedValue(values)
    prepare()
    await fireEvent.click(
      await screen.findByRole('button', { name: 'Sort by n' })
    )
    await waitFor(() => expect(getMetricSeriesOrder).toHaveBeenCalledTimes(1))
    const signal = getMetricSeriesOrder.mock.calls[0]!.at(-1) as AbortSignal
    await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() => expect(getMetricSeriesOrder).toHaveBeenCalledTimes(2))
    expect(signal.aborted).toBe(true)
    await waitFor(() => expect(order()).toEqual(['b', 'a', 'c']))
    stale.resolve(['a', 'b', 'c'])
    await tick()
    expect(order()).toEqual(['b', 'a', 'c'])
  })

  it('does not retain an invisible sort when refresh removes its discriminator', async () => {
    getMetricSeriesOrder.mockResolvedValue(values)
    const { metric } = prepare()
    await fireEvent.click(
      await screen.findByRole('button', { name: 'Sort by n' })
    )
    await waitFor(() => expect(order()).toEqual(['b', 'a', 'c']))
    getMetricView.mockResolvedValue({
      ...metric,
      timeseries: metric.timeseries.map(series => ({
        ...series,
        labelPreviews: [{ key: 'other', text: 'other preview' }],
      })),
    })
    await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() =>
      expect(screen.queryByRole('button', { name: 'Sort by n' })).toBeNull()
    )
    expect(order()).toEqual(['a', 'b', 'c'])
    expect(getMetricSeriesOrder).toHaveBeenCalledTimes(1)
  })

  it('rejects incomplete cohorts and cancels a superseded column without accepting its late result', async () => {
    const stale = deferred<typeof values>()
    getMetricSeriesOrder
      .mockReturnValueOnce(stale.promise)
      .mockResolvedValueOnce([])
      .mockResolvedValue(values)
    const view = prepare()
    await fireEvent.click(
      await screen.findByRole('button', { name: 'Sort by n' })
    )
    await waitFor(() => expect(getMetricSeriesOrder).toHaveBeenCalledTimes(1))
    const signal = getMetricSeriesOrder.mock.calls[0]!.at(-1) as AbortSignal
    await fireEvent.click(screen.getByRole('button', { name: 'Sort by other' }))
    await screen.findByText('Series changed. Refresh the metric.')
    expect(signal.aborted).toBe(true)
    expect(order()).toEqual(['a', 'b', 'c'])
    stale.resolve(values)
    await tick()
    expect(order()).toEqual(['a', 'b', 'c'])
    await fireEvent.click(screen.getByRole('button', { name: 'Retry sort' }))
    await waitFor(() => expect(order()).toEqual(['b', 'a', 'c']))
    getMetricSeriesOrder.mockReturnValue(new Promise(() => {}))
    await fireEvent.click(screen.getByRole('button', { name: 'Sort by n' }))
    await waitFor(() => expect(getMetricSeriesOrder).toHaveBeenCalledTimes(4))
    const lastSignal = getMetricSeriesOrder.mock.calls[3]!.at(-1) as AbortSignal
    view.unmount()
    expect(lastSignal.aborted).toBe(true)
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

  it('publishes datapoint counts and units from the received response when it is newer than the chart', async () => {
    prepareRawSeriesTest()
    const points = Array.from({ length: 5 }, (_, index) =>
      makeSumDatapoint(`point-${index}`, 1_700_000_000_000 + index, index)
    )
    const chart = makeMetric('Sum', points.slice(0, 4))
    const received = {
      ...makeMetric('Sum', points),
      unit: 's',
      datapointCount: 5,
      timeseries: [
        { ...chart.timeseries[0]!, datapoints: points, datapointCount: 5 },
      ],
    }
    getMetricView.mockImplementation((...args: unknown[]) =>
      Promise.resolve(args[3] === 0 ? received : chart)
    )
    setTestUrl('/metrics/metric-1?time=all&series=route%3D%2Fcheckout')
    renderWithContexts(MetricsPage)
    await waitFor(() =>
      expect(document.querySelectorAll('[data-dp-id]')).toHaveLength(5)
    )
    expect(
      document.querySelector('[data-detail-section="datapoints"] summary')
    ).toHaveTextContent('[5]')
    expect(
      screen.getByRole('columnheader', { name: /^Value\s*\(s\)$/ })
    ).toBeInTheDocument()
    expect(
      within(screen.getByRole('table', { name: 'Metric fields' })).getByText(
        'ms',
        { exact: true }
      )
    ).toBeInTheDocument()
  })

  it('keeps cached raw rows interactive when chart enrichment replaces only the metric response', async () => {
    prepareRawSeriesTest()
    const point = makeSumDatapoint('kept-point', 1_700_000_000_000, 7)
    const base = makeMetric('Sum', [point])
    const first = { ...base.timeseries[0]!, seriesRef: 'series-a' }
    const second = { ...first, seriesRef: 'series-b', datapoints: [] }
    const enrichment = deferred<MetricViewData | null>()
    let viewCalls = 0
    getMetricView.mockImplementation((...args: unknown[]) => {
      if (args[3] === 0)
        return Promise.resolve({ ...base, timeseries: [first] })
      return ++viewCalls === 1
        ? Promise.resolve({ ...base, timeseries: [first, second] })
        : enrichment.promise
    })
    savePersistedTimeseriesVisible('metric-1', ['series-a'])
    setTestUrl('/metrics/metric-1?time=all&series=series-a')
    renderWithContexts(MetricsPage)
    await waitFor(() =>
      expect(document.querySelector('[data-dp-id="kept-point"]')).not.toBeNull()
    )
    const row = document.querySelector('[data-dp-id="kept-point"]')!
    const unchecked = screen
      .getAllByRole('checkbox', { name: /^Show .* in chart$/ })
      .find(element => !(element as HTMLInputElement).checked)!
    expect(unchecked).toBeDefined()
    await fireEvent.click(unchecked)
    await waitFor(() => expect(viewCalls).toBe(2))
    await fireEvent.click(row)
    expect(row).toHaveAttribute('aria-expanded', 'true')
    expect(readRoute().query.dp).toBe('kept-point')
    const expansion = document.querySelector('.dp-list__expansion')
    enrichment.resolve({
      ...base,
      timeseries: [
        first,
        {
          ...second,
          datapoints: [makeSumDatapoint('b-point', 1_700_000_000_000, 9)],
        },
      ],
    })
    await enrichment.promise
    await tick()
    expect(rawSeriesCalls()).toHaveLength(1)
    expect(document.querySelector('[data-dp-id="kept-point"]')).toBe(row)
    expect(document.querySelector('.dp-list__expansion')).toBe(expansion)
    expect(row).not.toHaveAttribute('aria-disabled', 'true')
    await fireEvent.keyDown(row, { key: 'Enter' })
    expect(row).toHaveAttribute('aria-expanded', 'false')
    expect(readRoute().query.dp).toBeUndefined()
    await fireEvent.keyDown(row, { key: ' ' })
    expect(row).toHaveAttribute('aria-expanded', 'true')
    expect(readRoute().query.dp).toBe('kept-point')
  })

  it('returns retained rows to live selection after A→B→A and rejects the late B response', async () => {
    prepareRawSeriesTest()
    const metric = makeMetric('Sum', [
      makeSumDatapoint('a-point', 1_700_000_000_000, 7),
    ])
    const first = { ...metric.timeseries[0]!, seriesRef: 'series-a' }
    const second = { ...first, seriesRef: 'series-b', datapoints: [] }
    const replacement = deferred<MetricViewData | null>()
    getMetricView.mockImplementation((...args: unknown[]) => {
      if (args[3] !== 0)
        return Promise.resolve({ ...metric, timeseries: [first, second] })
      return (args[4] as string[])[0] === 'series-a'
        ? Promise.resolve({ ...metric, timeseries: [first] })
        : replacement.promise
    })
    setTestUrl('/metrics/metric-1?time=all&series=series-a')
    renderWithContexts(MetricsPage)
    await waitFor(() =>
      expect(document.querySelector('[data-dp-id="a-point"]')).not.toBeNull()
    )
    const row = document.querySelector('[data-dp-id="a-point"]')!
    navigateCurrentRoute(
      withQueryPatch(readRoute().query, { series: 'series-b' }),
      'replace'
    )
    await tick()
    expect(rawSeriesCalls()).toHaveLength(2)
    await fireEvent.click(row)
    expect(row).toHaveAttribute('aria-expanded', 'true')
    expect(readRoute().query.series).toBe('series-b')
    expect(readRoute().query.dp).toBeUndefined()
    navigateCurrentRoute(
      withQueryPatch(readRoute().query, { series: 'series-a' }),
      'replace'
    )
    await tick()
    expect((rawSeriesCalls()[1]!.at(-1) as AbortSignal).aborted).toBe(true)
    await fireEvent.keyDown(row, { key: 'Enter' })
    expect(readRoute().query.dp).toBe('a-point')
    replacement.resolve({
      ...metric,
      timeseries: [
        {
          ...second,
          datapoints: [makeSumDatapoint('stale-b', 1_700_000_000_000, 99)],
        },
      ],
    })
    await replacement.promise
    await tick()
    expect(document.querySelector('[data-dp-id="a-point"]')).toBe(row)
    expect(document.querySelector('[data-dp-id="stale-b"]')).toBeNull()
    expect(readRoute().query.series).toBe('series-a')
    expect(readRoute().query.dp).toBe('a-point')
  })

  it.each(['success', 'failure'] as const)(
    'retains the displayed series count, identity, disclosure and action ownership through replacement %s',
    async outcome => {
      userEvent.setup()
      const copy = vi
        .spyOn(navigator.clipboard, 'writeText')
        .mockResolvedValue()
      prepareRawSeriesTest()
      const replacement = deferred<MetricViewData | null>()
      const a = {
        ...makeSumDatapoint('a-point', 1_700_000_000_000, 7),
        flags: 1,
      }
      const b = makeSumDatapoint('b-point', 1_700_000_000_000, 9)
      const metric = makeMetric('Sum', [a])
      const first = {
        ...metric.timeseries[0]!,
        seriesRef: 'series-a',
        attributes: [
          { key: 'route', value: { kind: 'string' as const, value: '/a' } },
        ],
      }
      const second = {
        ...first,
        seriesRef: 'series-b',
        datapointCount: 9,
        datapoints: [b],
        attributes: [
          { key: 'route', value: { kind: 'string' as const, value: '/b' } },
        ],
      }
      const full = { ...metric, timeseries: [first, second] }
      getMetricView.mockImplementation((...args: unknown[]) => {
        if (args[3] !== 0) return Promise.resolve(full)
        return (args[4] as string[])[0] === 'series-b'
          ? replacement.promise
          : Promise.resolve({ ...full, timeseries: [first] })
      })
      setTestUrl('/metrics/metric-1?time=all&series=series-a&dp=a-point')
      renderWithContexts(MetricsPage)
      await waitFor(() =>
        expect(document.querySelector('[data-dp-id="a-point"]')).not.toBeNull()
      )
      const row = document.querySelector<HTMLTableRowElement>(
        '[data-dp-id="a-point"]'
      )!
      await waitFor(() => expect(row).toHaveAttribute('aria-expanded', 'true'))
      const expansion = document.querySelector('.dp-list__expansion')
      const section = document.querySelector(
        '[data-detail-section="datapoints"]'
      )!
      const header = section.querySelector('summary')!
      const identity = screen.getByLabelText('Displayed datapoint series')
      expect(header).toHaveTextContent('[4]')
      expect(identity).toHaveTextContent('series-a')

      navigateCurrentRoute(
        withQueryPatch(readRoute().query, {
          series: 'series-b',
          dp: undefined,
        }),
        'replace'
      )
      await tick()
      // No debounce: B starts without advancing a timer or completing A's view.
      expect(rawSeriesCalls().at(-1)?.[4]).toEqual(['series-b'])
      const requestedURL = window.location.search
      expect(document.querySelector('[data-dp-id="a-point"]')).toBe(row)
      expect(document.querySelector('.dp-list__expansion')).toBe(expansion)
      expect(header).toHaveTextContent('[4]')
      expect(identity).toHaveTextContent('series-a')
      expect(screen.getByText('/b', { exact: true })).toBeInTheDocument()
      expect(screen.queryByText('Loading datapoints…')).toBeNull()
      await screen.findByText('Loading datapoints…')
      expect(row).not.toHaveAttribute('aria-disabled', 'true')
      await fireEvent.click(
        within(expansion as HTMLElement).getByRole('button', {
          name: 'Copy asDouble',
        })
      )
      await waitFor(() => expect(copy).toHaveBeenLastCalledWith('7'))
      await fireEvent.click(row)
      expect(row).toHaveAttribute('aria-expanded', 'false')
      expect(window.location.search).toBe(requestedURL)
      await fireEvent.keyDown(row, { key: 'Enter' })
      expect(window.location.search).toBe(requestedURL)
      expect(row).toHaveAttribute('aria-expanded', 'true')
      await fireEvent.click(
        within(
          document.querySelector<HTMLElement>('.dp-list__expansion')!
        ).getByRole('button', {
          name: 'Copy asDouble',
        })
      )
      await waitFor(() => expect(copy).toHaveBeenLastCalledWith('7'))
      if (outcome === 'failure') {
        const error = vi.spyOn(console, 'error').mockImplementation(() => {})
        replacement.reject(new Error('offline'))
        await screen.findByText('Could not load datapoints.')
        expect(document.querySelector('[data-dp-id="a-point"]')).toBe(row)
        expect(header).toHaveTextContent('[4]')
        expect(identity).toHaveTextContent('series-a')
        await fireEvent.keyDown(row, { key: ' ' })
        expect(row).toHaveAttribute('aria-expanded', 'false')
        await fireEvent.click(row)
        expect(row).toHaveAttribute('aria-expanded', 'true')
        expect(window.location.search).toBe(requestedURL)
        await fireEvent.click(
          within(
            document.querySelector<HTMLElement>('.dp-list__expansion')!
          ).getByRole('button', { name: 'Copy asDouble' })
        )
        await waitFor(() => expect(copy).toHaveBeenLastCalledWith('7'))
        error.mockRestore()
        return
      }

      replacement.resolve({ ...full, timeseries: [second] })
      await waitFor(() =>
        expect(document.querySelector('[data-dp-id="b-point"]')).not.toBeNull()
      )
      expect(header).toHaveTextContent('[9]')
      expect(identity).toHaveTextContent('series-b')
      expect(document.querySelector('[data-dp-id="a-point"]')).toBeNull()
      await fireEvent.click(document.querySelector('[data-dp-id="b-point"]')!)
      expect(readRoute().query.dp).toBe('b-point')
    }
  )

  it('retries a failed raw request without changing series or visibility', async () => {
    prepareRawSeriesTest()
    let rawRequest = 0
    getMetricView.mockImplementation((...args: unknown[]) => {
      if (args[3] !== 0) return Promise.resolve(makeMetric('Sum', [reduced]))
      return ++rawRequest === 1
        ? Promise.reject(new Error('temporary failure'))
        : Promise.resolve(makeMetric('Sum', [reduced]))
    })
    setTestUrl('/metrics/metric-1?start=100&end=200&series=route%3D%2Fcheckout')
    renderWithContexts(MetricsPage)
    await screen.findByText('Could not load datapoints.')
    const selection = window.location.search
    const checkbox = screen.getByRole('checkbox', {
      name: 'Show default series in chart',
    })
    expect(checkbox).toBeChecked()
    await fireEvent.click(
      screen.getByRole('button', { name: 'Retry datapoints' })
    )
    await waitFor(() =>
      expect(document.querySelector('[data-dp-id="dp-reduced"]')).not.toBeNull()
    )
    expect(rawSeriesCalls()).toHaveLength(2)
    expect(window.location.search).toBe(selection)
    expect(checkbox).toBeChecked()
    expect(screen.queryByText('Could not load datapoints.')).toBeNull()
  })

  it.each(['success', 'failure'] as const)(
    'keeps retained rows inspectable without changing a different requested metric through %s',
    async outcome => {
      userEvent.setup()
      const copy = vi
        .spyOn(navigator.clipboard, 'writeText')
        .mockResolvedValue()
      prepareRawSeriesTest()
      searchMetricSummaries.mockResolvedValue([
        makeSummary('Sum'),
        { ...makeSummary('Sum'), metricRef: 'next', name: 'next.metric' },
      ])
      const detail = deferred<MetricViewData | null>()
      const raw = deferred<MetricViewData | null>()
      getMetricView.mockImplementation((...args: unknown[]) =>
        args[0] === 'next'
          ? args[3] === 0
            ? raw.promise
            : detail.promise
          : Promise.resolve(makeMetric('Sum', [reduced]))
      )
      const error = vi.spyOn(console, 'error').mockImplementation(() => {})
      setTestUrl('/metrics/metric-1?time=all&series=route%3D%2Fcheckout')
      renderWithContexts(MetricsPage)
      await waitFor(() =>
        expect(
          document.querySelector('[data-dp-id="dp-reduced"]')
        ).not.toBeNull()
      )
      const row = document.querySelector('[data-dp-id="dp-reduced"]')!
      navigateToItem('metrics', 'next', 'push', { series: 'route=/checkout' })
      await waitFor(() =>
        expect(getMetricView.mock.calls.some(call => call[0] === 'next')).toBe(
          true
        )
      )
      const requestedURL = window.location.href
      await fireEvent.click(row)
      expect(row).toHaveAttribute('aria-expanded', 'true')
      expect(window.location.href).toBe(requestedURL)
      await fireEvent.click(
        screen.getByRole('button', { name: 'Copy asDouble' })
      )
      await waitFor(() => expect(copy).toHaveBeenLastCalledWith('1'))

      const next = {
        ...makeMetric('Sum', [reduced]),
        metricRef: 'next',
        name: 'next.metric',
      }
      if (outcome === 'failure') {
        detail.reject(new Error('replacement unavailable'))
        await screen.findByText('Could not load the selected metric.')
        expect(rawSeriesCalls()).toHaveLength(1)
      } else {
        detail.resolve(next)
        await waitFor(() => expect(rawSeriesCalls()).toHaveLength(2))
      }
      expect(document.querySelector('[data-dp-id="dp-reduced"]')).toBe(row)
      await fireEvent.keyDown(row, { key: 'Enter' })
      expect(row).toHaveAttribute('aria-expanded', 'false')
      await fireEvent.keyDown(row, { key: ' ' })
      expect(row).toHaveAttribute('aria-expanded', 'true')
      expect(window.location.href).toBe(requestedURL)
      await fireEvent.click(
        screen.getByRole('button', { name: 'Copy asDouble' })
      )
      await waitFor(() => expect(copy).toHaveBeenLastCalledWith('1'))

      if (outcome === 'success') {
        raw.resolve({
          ...next,
          timeseries: [
            {
              ...next.timeseries[0]!,
              datapoints: [
                makeSumDatapoint('next-point', 1_700_000_000_000, 9),
              ],
            },
          ],
        })
        await waitFor(() =>
          expect(
            document.querySelector('[data-dp-id="next-point"]')
          ).not.toBeNull()
        )
        await fireEvent.click(
          document.querySelector('[data-dp-id="next-point"]')!
        )
        expect(readRoute().query.dp).toBe('next-point')
      }
      error.mockRestore()
    }
  )

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
        screen.getByText('default series', { exact: true }).closest('tr')
      ).toBeInTheDocument()
    )
    expect(rawSeriesCalls()).toHaveLength(0)

    vi.setSystemTime(requestNow)
    const seriesRow = screen
      .getByText('default series', { exact: true })
      .closest('tr')!
    await fireEvent.click(seriesRow)
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
    expect((rawSeriesCalls()[0]!.at(-1) as AbortSignal).aborted).toBe(true)
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

  it('invalidates an in-flight raw response on a same-window refresh', async () => {
    prepareRawSeriesTest()
    const old = deferred<MetricViewData | null>()
    const current = deferred<MetricViewData | null>()
    let requests = 0
    getMetricView.mockImplementation((...args: unknown[]) =>
      args[3] === 0
        ? requests++ === 0
          ? old.promise
          : current.promise
        : Promise.resolve(makeMetric('Sum', [reduced]))
    )
    setTestUrl('/metrics/metric-1?time=all&series=route%3D%2Fcheckout')
    const view = renderWithContexts(MetricsPage)
    await waitFor(() => expect(rawSeriesCalls()).toHaveLength(1))
    await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() => expect(rawSeriesCalls()).toHaveLength(2))
    expect((rawSeriesCalls()[0]!.at(-1) as AbortSignal).aborted).toBe(true)
    current.resolve(
      makeMetric('Sum', [makeSumDatapoint('fresh', 1_700_000_000_000, 7)])
    )
    await waitFor(() =>
      expect(document.querySelector('[data-dp-id="fresh"]')).not.toBeNull()
    )
    old.resolve(
      makeMetric('Sum', [makeSumDatapoint('stale', 1_700_000_000_000, 2)])
    )
    await tick()
    expect(document.querySelector('[data-dp-id="stale"]')).toBeNull()
    view.unmount()
    expect((rawSeriesCalls()[1]!.at(-1) as AbortSignal).aborted).toBe(true)
  })

  it('keeps displayed datapoint rows mounted until a refreshed raw series is ready', async () => {
    prepareRawSeriesTest()
    const replacement = deferred<MetricViewData | null>()
    let rawRequests = 0
    let viewRequests = 0
    getMetricView.mockImplementation((...args: unknown[]) =>
      args[3] === 0
        ? rawRequests++ === 0
          ? Promise.resolve(
              makeMetric('Sum', [
                makeSumDatapoint('displayed', 1_700_000_000_000, 7),
              ])
            )
          : replacement.promise
        : Promise.resolve({
            ...makeMetric('Sum', [reduced]),
            unit: viewRequests++ === 0 ? 'ms' : 's',
          })
    )
    setTestUrl('/metrics/metric-1?time=all&series=route%3D%2Fcheckout')
    renderWithContexts(MetricsPage)
    await waitFor(() =>
      expect(document.querySelector('[data-dp-id="displayed"]')).not.toBeNull()
    )
    const row = document.querySelector('[data-dp-id="displayed"]')
    expect(
      screen.getByRole('columnheader', { name: /^Value\s*\(ms\)$/ })
    ).toBeInTheDocument()
    await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() => expect(rawSeriesCalls()).toHaveLength(2))
    expect(document.querySelector('[data-dp-id="displayed"]')).toBe(row)
    // The metric's independent fields are already fresh, while retained raw
    // values still use the unit belonging to their displayed response.
    expect(
      within(screen.getByRole('table', { name: 'Metric fields' })).getByText(
        's',
        { exact: true }
      )
    ).toBeInTheDocument()
    expect(
      screen.getByRole('columnheader', { name: /^Value\s*\(ms\)$/ })
    ).toBeInTheDocument()
    replacement.resolve({
      ...makeMetric('Sum', [
        makeSumDatapoint('replacement', 1_700_000_000_000, 9),
      ]),
      unit: 's',
    })
    await waitFor(() =>
      expect(
        document.querySelector('[data-dp-id="replacement"]')
      ).not.toBeNull()
    )
    expect(document.querySelector('[data-dp-id="displayed"]')).toBeNull()
    expect(
      screen.getByRole('columnheader', { name: /^Value\s*\(s\)$/ })
    ).toBeInTheDocument()
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
      expect(document.querySelector('tr[data-dp-id="dp-reduced"]')).toBeNull()
      expect(screen.queryByText('Loading datapoints…')).not.toBeInTheDocument()
      expect(await screen.findByText('Loading datapoints…')).toBeVisible()

      raw.resolve(response())
      await raw.promise
      await waitFor(() =>
        expect(document.querySelector('tr[data-dp-id="dp-reduced"]')).toBeNull()
      )

      const user = userEvent.setup()
      const seriesRow = screen
        .getByRole('button', { name: 'Select default series' })
        .closest('tr')!
      await user.click(seriesRow)
      await user.click(seriesRow)
      await tick()
      expect(rawSeriesCalls()).toHaveLength(1)
    }
  )
})
