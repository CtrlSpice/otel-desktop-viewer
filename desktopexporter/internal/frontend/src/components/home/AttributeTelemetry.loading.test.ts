// @vitest-environment jsdom
import { cleanup, fireEvent, waitFor } from '@testing-library/svelte'
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { tick } from 'svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import { telemetryAPI } from '@/services/telemetry-service'
import type { LogSummary, MetricSummary, TraceSummary } from '@/types/api-types'
import type { ServiceSummary } from '@/services/discovery-types'
import { parseQuery } from '@/components/shared/Search/queryParser'
import AttributeTelemetry from './AttributeTelemetry.svelte'
import { homeQueries } from './home-search'

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason: Error) => void
  const promise = new Promise<T>((yes, no) => {
    resolve = yes
    reject = no
  })
  return { promise, resolve, reject }
}

function service(name: string): ServiceSummary {
  return {
    serviceName: name,
    serviceNamespace: '',
    traceCount: 26,
    spanCount: 26,
    errorSpanCount: 0,
    logCount: 26,
    errorLogCount: 0,
    logSeverityCounts: {
      trace: 0,
      debug: 0,
      info: 26,
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
}

function traces(prefix: string): TraceSummary[] {
  return Array.from({ length: 26 }, (_, index) => ({
    traceID: `${prefix}-${index}`,
    hasRootSpan: true,
    rootSpan: { name: `${prefix} trace ${index}`, serviceName: prefix },
    startTime: 1n,
    durationNs: 1n,
    spanCount: 1,
    errorCount: 0,
  }))
}

function logs(prefix: string): LogSummary[] {
  return Array.from({ length: 26 }, (_, index) => ({
    logRef: `${prefix}-${index}`,
    timestamp: 1n,
    severityText: 'INFO',
    severityNumber: 9,
    serviceName: prefix,
    bodyPreview: `${prefix} log ${index}`,
  }))
}

const props = {
  attribute: null,
  service: service('A'),
  options: {},
  revision: 0,
}

function metrics(prefix: string): MetricSummary[] {
  return Array.from({ length: 26 }, (_, index) => ({
    metricRef: `${prefix}-${index}`,
    name: `${prefix} metric ${index}`,
    serviceName: prefix,
    unit: '',
    metricType: 'Gauge',
    aggregationTemporality: null,
    aggregationTemporalityCode: null,
    isMonotonic: null,
    seriesCount: 1,
    seriesCardinality: 1,
    dataPointCount: 1,
    lastValue: 1,
    lastSeen: 1n,
  }))
}

beforeEach(() => {
  setTestUrl('/?time=all')
  localStorage.removeItem('home-telemetry-page-size')
  vi.spyOn(telemetryAPI, 'searchMetricSummaries').mockImplementation(
    () => new Promise(() => {})
  )
  vi.spyOn(telemetryAPI, 'searchLogSummaries').mockImplementation(
    () => new Promise(() => {})
  )
})

afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.restoreAllMocks()
})

it('publishes and pages the active signal without waiting for unrelated signals, retaining displayed links during replacement', async () => {
  const first = deferred<TraceSummary[]>()
  const replacement = deferred<TraceSummary[]>()
  const traceSearch = vi
    .spyOn(telemetryAPI, 'searchTraceSummaries')
    .mockReturnValueOnce(first.promise)
    .mockReturnValueOnce(replacement.promise)
  const view = renderWithContexts(AttributeTelemetry, props)
  await waitFor(() => expect(traceSearch).toHaveBeenCalledTimes(1))
  expect(telemetryAPI.searchMetricSummaries).toHaveBeenCalledTimes(1)
  expect(telemetryAPI.searchLogSummaries).toHaveBeenCalledTimes(1)
  expect(view.queryByText('0 / 0 pages')).toBeNull()
  expect(view.getByRole('tab', { name: /^Traces/ })).not.toHaveAccessibleName(
    /\[0\]/
  )
  expect(view.getByRole('button', { name: 'Next page' })).toBeDisabled()

  first.resolve(traces('A'))
  const firstLink = await view.findByRole('link', { name: /A trace 0/ })
  const tabs = view.getByRole('tablist', { name: 'Telemetry signals' })
  const next = view.getByRole('button', { name: 'Next page' })
  expect(next).toBeEnabled()
  await waitFor(() =>
    expect(
      view.getByRole('region', { name: 'Matching traces' })
    ).toHaveAttribute('aria-busy', 'false')
  )
  expect(view.getByRole('tab', { name: /^Traces/ })).toHaveAccessibleName(
    /\[26\]/
  )

  await view.rerender({
    componentProps: { ...props, service: service('B'), revision: 1 },
  })
  expect(view.getByRole('link', { name: /A trace 0/ })).toBe(firstLink)
  expect(firstLink).toHaveAttribute('href', expect.stringContaining('A-0'))
  expect(view.getByRole('tablist', { name: 'Telemetry signals' })).toBe(tabs)
  expect(view.getByRole('button', { name: 'Next page' })).toBe(next)
  await fireEvent.click(next)
  expect(view.getByRole('link', { name: /A trace 25/ })).toBeInTheDocument()

  replacement.resolve(traces('B'))
  const newLink = await view.findByRole('link', { name: /B trace 0/ })
  expect(newLink).toHaveAttribute('href', expect.stringContaining('B-0'))
  expect(view.queryByRole('link', { name: /A trace/ })).toBeNull()
  expect(view.getByText('1 / 2 pages')).toBeInTheDocument()
  await fireEvent.click(next)
  expect(view.getByRole('link', { name: /B trace 25/ })).toBeInTheDocument()
})

it('retains independent signal pages and scroll when a different signal completes', async () => {
  const slowLogs = deferred<LogSummary[]>()
  vi.spyOn(telemetryAPI, 'searchTraceSummaries').mockResolvedValue(traces('A'))
  vi.mocked(telemetryAPI.searchLogSummaries).mockReturnValue(slowLogs.promise)
  const view = renderWithContexts(AttributeTelemetry, props)
  await view.findByRole('link', { name: /A trace 0/ })
  await fireEvent.click(view.getByRole('button', { name: 'Next page' }))
  const viewport = view.getByRole('region', { name: 'Matching traces' })
  viewport.scrollTop = 73
  await fireEvent.scroll(viewport)
  slowLogs.resolve(logs('A'))
  await waitFor(() =>
    expect(view.getByRole('tab', { name: /^Logs/ })).toHaveAccessibleName(
      /\[26\]/
    )
  )
  expect(view.getByRole('link', { name: /A trace 25/ })).toBeInTheDocument()
  expect(viewport.scrollTop).toBe(73)
  await fireEvent.click(view.getByRole('tab', { name: /^Logs/ }))
  expect(view.getByRole('link', { name: /A log 0/ })).toBeInTheDocument()
  await fireEvent.click(view.getByRole('button', { name: 'Next page' }))
  viewport.scrollTop = 41
  await fireEvent.scroll(viewport)
  await fireEvent.click(view.getByRole('tab', { name: /^Traces/ }))
  expect(view.getByRole('link', { name: /A trace 25/ })).toBeInTheDocument()
  expect(viewport.scrollTop).toBe(73)
  await fireEvent.click(view.getByRole('tab', { name: /^Logs/ }))
  expect(view.getByRole('link', { name: /A log 25/ })).toBeInTheDocument()
  expect(viewport.scrollTop).toBe(41)
})

it.each(['pending', 'failed'] as const)(
  'keeps each displayed signal range on newly paged links during a %s replacement',
  async outcome => {
    const nextTraces = deferred<TraceSummary[]>()
    const nextLogs = deferred<LogSummary[]>()
    const nextMetrics = deferred<MetricSummary[]>()
    vi.spyOn(telemetryAPI, 'searchTraceSummaries')
      .mockResolvedValueOnce(traces('A'))
      .mockReturnValueOnce(nextTraces.promise)
    vi.mocked(telemetryAPI.searchLogSummaries)
      .mockResolvedValueOnce(logs('A'))
      .mockReturnValueOnce(nextLogs.promise)
    vi.mocked(telemetryAPI.searchMetricSummaries)
      .mockResolvedValueOnce(metrics('A'))
      .mockReturnValueOnce(nextMetrics.promise)
    setTestUrl('/?start=1000&end=2000&q=A')
    const view = renderWithContexts(AttributeTelemetry, {
      ...props,
      options: {
        startTime: '1000000000',
        endTime: '2000000000',
        queries: homeQueries(parseQuery("search('A')", [])),
      },
    })
    await view.findByRole('link', { name: /A trace 0/ })
    await waitFor(() =>
      expect(view.getByRole('tab', { name: /^Logs/ })).toHaveAccessibleName(
        /\[26\]/
      )
    )
    setTestUrl('/?start=3000&end=4000&q=B')
    await view.rerender({
      componentProps: {
        ...props,
        options: {
          startTime: '3000000000',
          endTime: '4000000000',
          queries: homeQueries(parseQuery("search('B')", [])),
        },
        revision: 1,
      },
    })
    if (outcome === 'failed') {
      nextTraces.reject(new Error('Trace replacement failed'))
      nextLogs.reject(new Error('Log replacement failed'))
      nextMetrics.reject(new Error('Metric replacement failed'))
      await view.findByRole('alert')
    }
    for (const [signal, name] of [
      ['Traces', 'A trace 25'],
      ['Logs', 'A log 25'],
      ['Metrics', 'A metric 25'],
    ]) {
      await fireEvent.click(
        view.getByRole('tab', { name: new RegExp(`^${signal}`) })
      )
      await fireEvent.click(view.getByRole('button', { name: 'Next page' }))
      const link = view.getByRole('link', { name: new RegExp(name) })
      expect(link).toHaveAttribute(
        'href',
        expect.stringContaining('start=1000')
      )
      expect(link).toHaveAttribute('href', expect.stringContaining('end=2000'))
      expect(
        new URL(link.getAttribute('href')!, 'http://local').searchParams.has(
          'q'
        )
      ).toBe(false)
    }
    if (outcome === 'pending') {
      nextTraces.resolve(traces('B'))
      const link = await view.findByRole('tab', { name: /^Traces/ })
      await fireEvent.click(link)
      const card = await view.findByRole('link', { name: /B trace 0/ })
      expect(card).toHaveAttribute(
        'href',
        expect.stringContaining('start=3000')
      )
      expect(card).toHaveAttribute('href', expect.stringContaining('end=4000'))
      expect(
        new URL(card.getAttribute('href')!, 'http://local').searchParams.has(
          'q'
        )
      ).toBe(false)
    }
    view.unmount()
  }
)

it('rejects obsolete A → B → A responses and errors without completing the current request', async () => {
  const oldA = deferred<TraceSummary[]>()
  const oldB = deferred<TraceSummary[]>()
  const newA = deferred<TraceSummary[]>()
  const search = vi
    .spyOn(telemetryAPI, 'searchTraceSummaries')
    .mockReturnValueOnce(oldA.promise)
    .mockReturnValueOnce(oldB.promise)
    .mockReturnValueOnce(newA.promise)
  const view = renderWithContexts(AttributeTelemetry, props)
  await waitFor(() => expect(search).toHaveBeenCalledTimes(1))
  await view.rerender({ componentProps: { ...props, service: service('B') } })
  await view.rerender({ componentProps: { ...props, service: service('A') } })
  expect(search).toHaveBeenCalledTimes(3)
  expect(search.mock.calls[0]![5]?.aborted).toBe(true)
  expect(search.mock.calls[1]![5]?.aborted).toBe(true)
  oldA.resolve(traces('obsolete'))
  oldB.reject(new Error('obsolete failure'))
  await tick()
  await tick()
  expect(view.queryByRole('alert')).toBeNull()
  expect(view.queryByRole('link')).toBeNull()
  expect(view.getByRole('region', { name: 'Matching traces' })).toHaveAttribute(
    'aria-busy',
    'true'
  )
  newA.resolve(traces('current'))
  await view.findByRole('link', { name: /current trace 0/ })
  expect(view.queryByText('obsolete failure')).toBeNull()
})

it('publishes active errors immediately, retains its cards and paging, and has no redundant Retry', async () => {
  const failed = deferred<TraceSummary[]>()
  vi.spyOn(telemetryAPI, 'searchTraceSummaries')
    .mockResolvedValueOnce(traces('A'))
    .mockReturnValueOnce(failed.promise)
  const view = renderWithContexts(AttributeTelemetry, props)
  await view.findByRole('link', { name: /A trace 0/ })
  await view.rerender({ componentProps: { ...props, revision: 1 } })
  failed.reject(new Error('Trace search unavailable'))
  expect(await view.findByRole('alert')).toHaveTextContent(
    'Trace search unavailable'
  )
  expect(view.getByRole('link', { name: /A trace 0/ })).toBeInTheDocument()
  expect(view.queryByRole('button', { name: 'Retry' })).toBeNull()
  await fireEvent.click(view.getByRole('button', { name: 'Next page' }))
  expect(view.getByRole('link', { name: /A trace 25/ })).toBeInTheDocument()
})

it.each(['success', 'failure'] as const)(
  'aborts disconnected telemetry on unmount and ignores late %s',
  async outcome => {
    const pending = deferred<TraceSummary[]>()
    const search = vi
      .spyOn(telemetryAPI, 'searchTraceSummaries')
      .mockReturnValue(pending.promise)
    const view = renderWithContexts(AttributeTelemetry, props)
    await waitFor(() => expect(search).toHaveBeenCalledTimes(1))
    view.unmount()
    expect(search.mock.calls[0]![5]?.aborted).toBe(true)
    if (outcome === 'success') pending.resolve(traces('unmounted'))
    else pending.reject(new Error('Unmounted telemetry failure'))
    await tick()
    await tick()
    expect(view.container).toBeEmptyDOMElement()
  }
)

it.each(['success', 'failure', 'abort'] as const)(
  'retains an initial signal error during replacement until fresh %s',
  async outcome => {
    vi.useFakeTimers()
    const initial = deferred<TraceSummary[]>()
    const replacement = deferred<TraceSummary[]>()
    const current = deferred<TraceSummary[]>()
    const search = vi
      .spyOn(telemetryAPI, 'searchTraceSummaries')
      .mockReturnValueOnce(initial.promise)
      .mockReturnValueOnce(replacement.promise)
      .mockReturnValueOnce(current.promise)
    const view = renderWithContexts(AttributeTelemetry, props)
    initial.reject(new Error('Original trace error'))
    await vi.advanceTimersByTimeAsync(0)
    const alert = view.getByRole('alert')
    expect(alert).toHaveTextContent('Original trace error')

    await view.rerender({
      componentProps: { ...props, service: service('B'), revision: 1 },
    })
    expect(view.getByRole('alert')).toBe(alert)
    expect(alert).toHaveTextContent('Original trace error')
    expect(view.queryByText('Loading matching traces…')).toBeNull()
    await vi.advanceTimersByTimeAsync(200)
    expect(view.getByText('Loading matching traces…')).toBeInTheDocument()
    expect(view.getByRole('alert')).toBe(alert)

    await fireEvent.click(view.getByRole('tab', { name: /^Logs/ }))
    expect(view.queryByRole('alert')).toBeNull()
    await fireEvent.click(view.getByRole('tab', { name: /^Traces/ }))
    expect(view.getByRole('alert')).toHaveTextContent('Original trace error')

    if (outcome === 'failure') {
      replacement.reject(new Error('Fresh trace error'))
      await vi.advanceTimersByTimeAsync(0)
      expect(view.getByRole('alert')).toHaveTextContent('Fresh trace error')
      expect(view.queryByText('Original trace error')).toBeNull()
      expect(view.queryByText('Loading matching traces…')).toBeNull()
    } else {
      if (outcome === 'abort') {
        await view.rerender({
          componentProps: { ...props, service: service('A'), revision: 2 },
        })
        expect(search.mock.calls[1]![5]?.aborted).toBe(true)
        replacement.reject(new Error('Aborted trace error'))
        await vi.advanceTimersByTimeAsync(0)
        expect(view.getByRole('alert')).toHaveTextContent(
          'Original trace error'
        )
        expect(view.queryByText('Aborted trace error')).toBeNull()
        expect(
          view.getByRole('region', { name: 'Matching traces' })
        ).toHaveAttribute('aria-busy', 'true')
        current.resolve(traces('current'))
      } else replacement.resolve(traces('current'))
      await vi.advanceTimersByTimeAsync(0)
      expect(view.queryByRole('alert')).toBeNull()
      expect(
        view.getByRole('link', { name: /current trace 0/ })
      ).toBeInTheDocument()
      expect(view.queryByText('Loading matching traces…')).toBeNull()
    }
    expect(view.queryByRole('button', { name: 'Retry' })).toBeNull()
  }
)

it('retains failed attribute context while pending and changes it only with a fresh failure or success', async () => {
  const next =
    deferred<
      Awaited<ReturnType<typeof telemetryAPI.getAttributeRecordMatches>>
    >()
  const matches = vi
    .spyOn(telemetryAPI, 'getAttributeRecordMatches')
    .mockRejectedValueOnce(new Error('Original attribute error'))
    .mockReturnValueOnce(next.promise)
    .mockResolvedValueOnce([
      { itemRef: 'final-0', spanIDs: ['span'], count: 1 },
    ])
  const trace = traces('final')[0]!
  vi.spyOn(telemetryAPI, 'searchTraceSummaries').mockResolvedValue([
    { ...trace, matchedSpans: [{ traceID: trace.traceID, spanID: 'span' }] },
  ])
  const attribute = {
    key: 'old.key',
    kind: 'string',
    ownerType: 'span' as const,
    foundOn: [],
  }
  const view = renderWithContexts(AttributeTelemetry, { ...props, attribute })
  await view.findByRole('alert')
  await view.rerender({
    componentProps: { ...props, attribute: { ...attribute, key: 'new.key' } },
  })
  expect(view.getByText('old.key')).toBeInTheDocument()
  expect(view.queryByText('new.key')).toBeNull()
  expect(view.getByRole('alert')).toHaveTextContent('Original attribute error')
  next.reject(new Error('Fresh attribute error'))
  await waitFor(() =>
    expect(view.getByRole('alert')).toHaveTextContent('Fresh attribute error')
  )
  expect(view.getByText('new.key')).toBeInTheDocument()
  await view.rerender({
    componentProps: { ...props, attribute: { ...attribute, key: 'final.key' } },
  })
  await waitFor(() => expect(view.queryByRole('alert')).toBeNull())
  expect(matches).toHaveBeenCalledTimes(3)
  expect(view.getByText('final.key')).toBeInTheDocument()
})

it('publishes attribute context with each signal rather than relabeling another signal’s retained cards', async () => {
  const nextTraces = deferred<TraceSummary[]>()
  const nextLogs = deferred<LogSummary[]>()
  vi.spyOn(telemetryAPI, 'searchTraceSummaries')
    .mockResolvedValueOnce(traces('A'))
    .mockReturnValueOnce(nextTraces.promise)
  vi.mocked(telemetryAPI.searchLogSummaries)
    .mockResolvedValueOnce(logs('A'))
    .mockReturnValueOnce(nextLogs.promise)
  vi.spyOn(telemetryAPI, 'getAttributeRecordMatches').mockResolvedValue([
    { itemRef: 'B-0', spanIDs: ['span'], count: 1 },
  ])
  const view = renderWithContexts(AttributeTelemetry, props)
  await view.findByRole('link', { name: /A trace 0/ })
  await waitFor(() =>
    expect(view.getByRole('tab', { name: /^Logs/ })).toHaveAccessibleName(
      /\[26\]/
    )
  )
  await view.rerender({
    componentProps: {
      ...props,
      service: service('B'),
      attribute: {
        key: 'host.key',
        kind: 'string',
        ownerType: 'resource',
        foundOn: [],
      },
    },
  })
  expect(view.queryByText('host.key')).toBeNull()
  const trace = traces('B')[0]!
  nextTraces.resolve([
    { ...trace, matchedSpans: [{ traceID: trace.traceID, spanID: 'span' }] },
  ])
  await view.findByRole('link', { name: /B trace 0/ })
  expect(view.getByText('host.key')).toBeInTheDocument()
  await fireEvent.click(view.getByRole('tab', { name: /^Logs/ }))
  expect(view.queryByText('host.key')).toBeNull()
  const oldLink = view.getByRole('link', { name: /A log 0/ })
  expect(oldLink).toHaveAttribute('href', expect.stringContaining('A-0'))
  nextLogs.resolve(logs('B'))
  const newLink = await view.findByRole('link', { name: /B log 0/ })
  expect(newLink).toHaveAttribute('href', expect.stringContaining('B-0'))
  expect(view.getByText('host.key')).toBeInTheDocument()
  expect(view.queryByRole('link', { name: /A log/ })).toBeNull()
})

it('resets delayed feedback on request identity and ends it when the active signal completes', async () => {
  vi.useFakeTimers()
  const current = deferred<TraceSummary[]>()
  vi.spyOn(telemetryAPI, 'searchTraceSummaries')
    .mockImplementationOnce(() => new Promise(() => {}))
    .mockReturnValueOnce(current.promise)
  const view = renderWithContexts(AttributeTelemetry, props)
  await tick()
  await vi.advanceTimersByTimeAsync(200)
  expect(view.getByText('Loading matching traces…')).toBeInTheDocument()
  await view.rerender({ componentProps: { ...props, revision: 1 } })
  expect(view.queryByText('Loading matching traces…')).toBeNull()
  await vi.advanceTimersByTimeAsync(199)
  expect(view.queryByText('Loading matching traces…')).toBeNull()
  await vi.advanceTimersByTimeAsync(1)
  expect(view.getByText('Loading matching traces…')).toBeInTheDocument()
  current.resolve(traces('A'))
  await vi.advanceTimersByTimeAsync(0)
  expect(view.queryByText('Loading matching traces…')).toBeNull()
  expect(view.getByRole('link', { name: /A trace 0/ })).toBeInTheDocument()
  view.unmount()
  await vi.advanceTimersByTimeAsync(200)
  expect(view.queryByRole('status')).toBeNull()
})
