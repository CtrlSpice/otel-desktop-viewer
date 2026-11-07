// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import {
  act,
  fireEvent,
  screen,
  waitFor,
  within,
} from '@testing-library/svelte'
import { tick } from 'svelte'
import userEvent from '@testing-library/user-event'
import TracesPage from './TracesPage.svelte'
import type {
  TraceSummary,
  TraceData,
  Stats,
  SpanNode,
  TraceLogSummary,
  SpanData,
  TraceColumnPreview,
} from '@/types/api-types'
import type { QueryNode } from '@/search/model'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import { navigate } from '@/route'
import { saveColumns } from '@/components/traces/Waterfall/waterfall-columns'

const {
  searchTraceSummaries,
  getStats,
  getTraceView,
  getSpan,
  getTraceColumnPreviews,
  getTraceLogSummaries,
  getTraceAttributeDefinitions,
  clearTraces,
  deleteTraces,
} = vi.hoisted(() => ({
  searchTraceSummaries: vi.fn(),
  getStats: vi.fn(),
  getTraceView: vi.fn(),
  getSpan: vi.fn(),
  getTraceColumnPreviews: vi.fn(),
  getTraceLogSummaries: vi.fn(),
  getTraceAttributeDefinitions: vi.fn(),
  clearTraces: vi.fn(),
  deleteTraces: vi.fn(),
}))

vi.mock('@/services/telemetry-service', async importOriginal => {
  const actual =
    await importOriginal<typeof import('@/services/telemetry-service')>()
  return {
    ...actual,
    telemetryAPI: {
      ...actual.telemetryAPI,
      searchTraceSummaries,
      getStats,
      getTraceView,
      getSpan,
      getTraceColumnPreviews,
      getTraceLogSummaries,
      getTraceAttributeDefinitions,
      clearTraces,
      deleteTraces,
    },
  }
})

describe('TracesPage activity ownership and readiness', () => {
  it.each(['pending', 'failed'] as const)(
    'keeps unfinished displayed activity running through a %s replacement',
    async outcome => {
      const oldLogs = deferred<TraceLogSummary[]>()
      const nextTree = deferred<TraceData>()
      const nextSpan = deferred<SpanData>()
      const nextLogs = deferred<TraceLogSummary[]>()
      searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
      getStats.mockResolvedValue(makeStats())
      getTraceView
        .mockResolvedValueOnce(makeTraceData(0))
        .mockReturnValueOnce(nextTree.promise)
      getSpan
        .mockResolvedValueOnce(makeSpanNode('root').spanData)
        .mockReturnValueOnce(nextSpan.promise)
      getTraceLogSummaries
        .mockReturnValueOnce(oldLogs.promise)
        .mockReturnValueOnce(nextLogs.promise)
      const consoleError = vi
        .spyOn(console, 'error')
        .mockImplementation(() => {})
      try {
        setTestUrl('/traces/trace-1?span=root')
        renderWithContexts(TracesPage)
        const fields = await screen.findByRole('table', { name: 'Span fields' })
        await screen.findByRole('row', { name: /^root —/ })
        const oldSignal = getTraceLogSummaries.mock.calls[0]![1] as AbortSignal
        navigate('/traces/trace-2?span=next')
        await waitFor(() =>
          expect(getTraceLogSummaries).toHaveBeenCalledTimes(2)
        )
        expect(oldSignal.aborted).toBe(false)
        if (outcome === 'failed') {
          nextTree.reject(new Error('replacement tree failed'))
          nextSpan.reject(new Error('replacement span failed'))
          await screen.findByText('Could not load the selected trace.')
        }
        oldLogs.resolve([
          makeTraceLog({ bodyPreview: 'late retained activity' }),
        ])
        await within(
          screen.getByRole('region', { name: 'Span details' })
        ).findByText('late retained activity')
        expect(screen.getByRole('table', { name: 'Span fields' })).toBe(fields)
        expect(
          screen.getByRole('button', { name: 'Select 1 log' })
        ).toBeInTheDocument()
        expect(window.location.pathname).toBe('/traces/trace-2')
        expect(
          (getTraceLogSummaries.mock.calls[1]![1] as AbortSignal).aborted
        ).toBe(false)
      } finally {
        consoleError.mockRestore()
      }
    }
  )

  it('cancels unfinished activity once neither the displayed tree nor Details owns it', async () => {
    const oldLogs = deferred<TraceLogSummary[]>()
    const nextTree = deferred<TraceData>()
    const nextSpan = deferred<SpanData>()
    searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    getTraceView
      .mockResolvedValueOnce(makeTraceData(0))
      .mockReturnValueOnce(nextTree.promise)
    getSpan
      .mockResolvedValueOnce(makeSpanNode('root').spanData)
      .mockReturnValueOnce(nextSpan.promise)
    getTraceLogSummaries
      .mockReturnValueOnce(oldLogs.promise)
      .mockResolvedValueOnce([])
    setTestUrl('/traces/trace-1?span=root')
    renderWithContexts(TracesPage)
    const fields = await screen.findByRole('table', { name: 'Span fields' })
    await screen.findByRole('row', { name: /^root —/ })
    const oldSignal = getTraceLogSummaries.mock.calls[0]![1] as AbortSignal
    navigate('/traces/trace-2?span=next')
    await waitFor(() => expect(getTraceView).toHaveBeenCalledTimes(2))
    nextTree.resolve(makeTraceData(0, 'trace-2', 'next'))
    await screen.findByRole('row', { name: /^next —/ })
    expect(oldSignal.aborted).toBe(false)
    expect(fields.querySelector('[data-field="traceId"]')).toHaveTextContent(
      'trace-1'
    )
    nextSpan.resolve(makeSpanNode('next', 'trace-2').spanData)
    await waitFor(() => expect(oldSignal.aborted).toBe(true))
    oldLogs.resolve([makeTraceLog({ bodyPreview: 'obsolete activity' })])
    await tick()
    expect(screen.queryByText('obsolete activity')).toBeNull()
    expect(fields.querySelector('[data-field="traceId"]')).toHaveTextContent(
      'trace-2'
    )
  })

  it.each(['pending', 'failed'] as const)(
    'retries retained activity independently of a %s different trace',
    async outcome => {
      const tree = deferred<TraceData>()
      const exact = deferred<SpanData>()
      const logs = deferred<TraceLogSummary[]>()
      const retry = deferred<TraceLogSummary[]>()
      searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
      getStats.mockResolvedValue(makeStats())
      getTraceView
        .mockResolvedValueOnce(makeTraceData(0))
        .mockReturnValueOnce(tree.promise)
      getSpan
        .mockResolvedValueOnce(makeSpanNode('root').spanData)
        .mockReturnValueOnce(exact.promise)
      getTraceLogSummaries
        .mockRejectedValueOnce(new Error('old activity unavailable'))
        .mockReturnValueOnce(logs.promise)
        .mockReturnValueOnce(retry.promise)
      const consoleError = vi
        .spyOn(console, 'error')
        .mockImplementation(() => {})
      try {
        setTestUrl('/traces/trace-1?span=root')
        renderWithContexts(TracesPage)
        const detail = screen.getByRole('region', { name: 'Span details' })
        await within(detail).findByText('Could not load correlated logs.')
        const retryButton = within(detail).getByRole('button', {
          name: 'Retry correlated logs',
        })
        navigate('/traces/missing-trace?span=root&log=next-log')
        await waitFor(() =>
          expect(getTraceLogSummaries).toHaveBeenCalledTimes(2)
        )
        const nextSignal = getTraceLogSummaries.mock.calls[1]![1] as AbortSignal
        if (outcome === 'failed') {
          tree.reject(new Error('new tree unavailable'))
          await screen.findByText('Could not load the selected trace.')
        }
        expect(
          within(detail).getByRole('button', { name: 'Retry correlated logs' })
        ).toBe(retryButton)
        await fireEvent.click(retryButton)
        await waitFor(() =>
          expect(getTraceLogSummaries).toHaveBeenCalledTimes(3)
        )
        expect(getTraceLogSummaries.mock.calls[2]![0]).toBe('trace-1')
        expect(nextSignal.aborted).toBe(false)
        retry.resolve([
          makeTraceLog({ bodyPreview: 'retried retained activity' }),
        ])
        await within(detail).findByText('retried retained activity')
        expect(window.location.pathname).toBe('/traces/missing-trace')
        expect(window.location.search).toBe('?span=root&log=next-log')
        expect(getTraceView).toHaveBeenCalledTimes(2)
        expect(getSpan).toHaveBeenCalledTimes(2)
        logs.resolve([
          makeTraceLog({
            logRef: 'next-log',
            bodyPreview: 'new trace activity',
          }),
        ])
        await tick()
        expect(within(detail).queryByText('new trace activity')).toBeNull()
        exact.resolve(makeSpanNode('root', 'missing-trace').spanData)
        await within(detail).findByText('new trace activity')
        expect(
          within(detail).queryByText('retried retained activity')
        ).toBeNull()
      } finally {
        consoleError.mockRestore()
      }
    }
  )

  function prepareRetainedSpan() {
    const root = makeSpanNode('root')
    const other = makeSpanNode('other')
    other.spanData.parentSpanID = 'root'
    const replacement = deferred<SpanData>()
    searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    getTraceView.mockResolvedValue({
      traceID: 'trace-1',
      unplacedSpanCount: 0,
      spans: [root, other],
    })
    getSpan.mockImplementation((_traceID: string, spanID: string) =>
      spanID === 'root' ? Promise.resolve(root.spanData) : replacement.promise
    )
    setTestUrl('/traces/trace-1?span=root')
    return { root, other, replacement }
  }

  it('retains a valid log deep link when complete correlated logs precede the exact span', async () => {
    const exact = deferred<SpanData>()
    searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    getTraceView.mockResolvedValue(makeTraceData(0))
    getTraceLogSummaries.mockResolvedValue([
      makeTraceLog({ bodyPreview: 'requested log' }),
    ])
    getSpan.mockReturnValue(exact.promise)
    setTestUrl('/traces/trace-1?span=root&log=log-1')
    renderWithContexts(TracesPage)
    await waitFor(() =>
      expect(document.querySelector('tr[data-span-id="root"]')).not.toBeNull()
    )
    // This notice proves all settled tree/log effects have had time to run,
    // while the independent exact-span request is still outstanding.
    await screen.findByText('Loading selected span…')
    expect(new URLSearchParams(window.location.search).get('log')).toBe('log-1')
    exact.resolve(makeSpanNode('root').spanData)
    await screen.findByRole('table', { name: 'Span fields' })
    expect(new URLSearchParams(window.location.search).get('log')).toBe('log-1')
    expect(document.getElementById('span-log-log-1')).toBeInTheDocument()
  })

  it('retains a received event deep link while exact span fields are pending', async () => {
    const exact = deferred<SpanData>()
    const root = makeSpanNode('root')
    root.spanData.events = [
      {
        name: 'exact event',
        timestamp: 1n,
        attributes: [],
        droppedAttributesCount: 0,
      },
    ]
    searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    getTraceView.mockResolvedValue({
      traceID: 'trace-1',
      unplacedSpanCount: 0,
      spans: [root],
    })
    getSpan.mockReturnValue(exact.promise)
    setTestUrl('/traces/trace-1?span=root&event=0')
    renderWithContexts(TracesPage)
    await screen.findByText('Loading selected span…')
    expect(new URLSearchParams(window.location.search).get('event')).toBe('0')
    exact.resolve(root.spanData)
    await screen.findByRole('table', { name: 'Span fields' })
    expect(new URLSearchParams(window.location.search).get('event')).toBe('0')
  })

  it.each(['absent', 'different span', 'failure'] as const)(
    'normalizes log absence only from an owned completed response: %s',
    async result => {
      const exact = deferred<SpanData>()
      const logs = deferred<TraceLogSummary[]>()
      searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
      getStats.mockResolvedValue(makeStats())
      getTraceView.mockResolvedValue(makeTraceData(0))
      getTraceLogSummaries.mockReturnValue(logs.promise)
      getSpan.mockReturnValue(exact.promise)
      setTestUrl('/traces/trace-1?span=root&log=log-1')
      renderWithContexts(TracesPage)
      await screen.findByText('Loading selected span…')
      expect(new URLSearchParams(window.location.search).get('log')).toBe(
        'log-1'
      )
      if (result === 'failure') {
        logs.reject(new Error('logs failed'))
        exact.resolve(makeSpanNode('root').spanData)
        await screen.findByText('Could not load correlated logs.')
        expect(new URLSearchParams(window.location.search).get('log')).toBe(
          'log-1'
        )
      } else {
        logs.resolve(
          result === 'absent' ? [] : [makeTraceLog({ spanID: 'other' })]
        )
        await waitFor(() =>
          expect(new URLSearchParams(window.location.search).has('log')).toBe(
            false
          )
        )
        // Missing log correlation is known without waiting for exact fields.
        expect(screen.queryByRole('table', { name: 'Span fields' })).toBeNull()
      }
    }
  )

  it.each(['success', 'failure'] as const)(
    'ignores stale log %s while a returned A deep link is pending',
    async stale => {
      const old = deferred<TraceLogSummary[]>()
      const latest = deferred<TraceLogSummary[]>()
      searchTraceSummaries.mockResolvedValue([
        makeTraceSummary(),
        makeTraceSummary({ traceID: 'trace-2' }),
      ])
      getStats.mockResolvedValue(makeStats())
      getTraceView.mockImplementation(async (traceID: string) =>
        makeTraceData(0, traceID)
      )
      getSpan.mockImplementation(
        async (traceID: string, spanID: string) =>
          makeSpanNode(spanID, traceID).spanData
      )
      getTraceLogSummaries
        .mockReturnValueOnce(old.promise)
        .mockResolvedValueOnce([])
        .mockReturnValueOnce(latest.promise)
      setTestUrl('/traces/trace-1?span=root&log=log-1')
      renderWithContexts(TracesPage)
      await screen.findByRole('table', { name: 'Span fields' })
      navigate('/traces/trace-2?span=root')
      await waitFor(() => expect(getTraceLogSummaries).toHaveBeenCalledTimes(2))
      navigate('/traces/trace-1?span=root&log=log-1')
      await waitFor(() => expect(getTraceLogSummaries).toHaveBeenCalledTimes(3))
      expect(
        (getTraceLogSummaries.mock.calls[0]![1] as AbortSignal).aborted
      ).toBe(true)
      if (stale === 'success') old.resolve([])
      else old.reject(new Error('old log request failed'))
      await screen.findByText('Loading correlated logs…')
      expect(new URLSearchParams(window.location.search).get('log')).toBe(
        'log-1'
      )
      expect(screen.queryByText('Could not load correlated logs.')).toBeNull()
      latest.resolve([
        makeTraceLog({ bodyPreview: 'current deep-linked activity' }),
      ])
      const detail = screen.getByRole('region', { name: 'Span details' })
      await within(detail).findByText('current deep-linked activity')
      expect(new URLSearchParams(window.location.search).get('log')).toBe(
        'log-1'
      )
      expect(document.getElementById('span-log-log-1')).toBeInTheDocument()
    }
  )

  it.each(['pending', 'failed'] as const)(
    'settles trace-owned logs into retained A while same-trace B is %s',
    async outcome => {
      const { other, replacement } = prepareRetainedSpan()
      const logs = deferred<TraceLogSummary[]>()
      getTraceLogSummaries.mockReturnValue(logs.promise)
      renderWithContexts(TracesPage)
      const detail = screen.getByRole('region', { name: 'Span details' })
      const fields = await within(detail).findByRole('table', {
        name: 'Span fields',
      })
      await screen.findByText('Loading correlated logs…')
      navigate('/traces/trace-1?span=other')
      await waitFor(() => expect(getSpan).toHaveBeenCalledTimes(2))
      if (outcome === 'failed') {
        replacement.reject(new Error('B exact span unavailable'))
        await screen.findByText('B exact span unavailable')
      }
      logs.resolve([
        makeTraceLog({ bodyPreview: 'received A activity' }),
        makeTraceLog({
          logRef: 'B-log',
          spanID: 'other',
          bodyPreview: 'received B activity',
        }),
      ])
      await within(detail).findByText('received A activity')
      expect(within(detail).queryByText('received B activity')).toBeNull()
      expect(screen.queryByText('Loading correlated logs…')).toBeNull()
      expect(
        within(detail).getByText('Events').closest('summary')
      ).toHaveTextContent('[1]')
      expect(within(detail).getByRole('table', { name: 'Span fields' })).toBe(
        fields
      )
      expect(fields.querySelector('[data-field="name"]')).toHaveTextContent(
        'root'
      )
      expect(within(detail).getByText('· root')).toBeInTheDocument()
      const user = userEvent.setup()
      const bytes = '{"name":"root","exact":-0.0}'
      const write = vi
        .spyOn(navigator.clipboard, 'writeText')
        .mockResolvedValue()
      const fetch = vi.spyOn(globalThis, 'fetch').mockImplementation(
        async () =>
          new Response(bytes, {
            headers: { 'Content-Type': 'application/json' },
          })
      )
      await user.click(
        within(detail).getByRole('button', { name: 'Copy span' })
      )
      await waitFor(() => expect(write).toHaveBeenCalledWith(bytes))
      expect(fetch.mock.calls.at(-1)?.[0]).toBe(
        '/export/traces/trace-1?spanId=root'
      )
      fetch.mockRestore()
      write.mockRestore()
      if (outcome === 'pending') {
        replacement.resolve(other.spanData)
        await within(detail).findByText('received B activity')
        expect(within(detail).queryByText('received A activity')).toBeNull()
        expect(fields.querySelector('[data-field="name"]')).toHaveTextContent(
          'other'
        )
      }
    }
  )

  it('publishes log failure, retry and refresh into retained A independently of failed B', async () => {
    const { replacement } = prepareRetainedSpan()
    const logs = deferred<TraceLogSummary[]>()
    const retry = deferred<TraceLogSummary[]>()
    const refresh = deferred<TraceLogSummary[]>()
    getTraceLogSummaries
      .mockReturnValueOnce(logs.promise)
      .mockReturnValueOnce(retry.promise)
      .mockReturnValueOnce(refresh.promise)
    renderWithContexts(TracesPage)
    const detail = screen.getByRole('region', { name: 'Span details' })
    const fields = await within(detail).findByRole('table', {
      name: 'Span fields',
    })
    await screen.findByText('Loading correlated logs…')
    navigate('/traces/trace-1?span=other')
    await waitFor(() => expect(getSpan).toHaveBeenCalledTimes(2))
    replacement.reject(new Error('B exact span unavailable'))
    logs.reject(new Error('trace logs unavailable'))
    await within(detail).findByText('Could not load correlated logs.')
    expect(screen.queryByText('Loading correlated logs…')).toBeNull()
    await fireEvent.click(
      within(detail).getByRole('button', { name: 'Retry correlated logs' })
    )
    await waitFor(() => expect(getTraceLogSummaries).toHaveBeenCalledTimes(2))
    expect(screen.queryByText('Loading correlated logs…')).toBeNull()
    await screen.findByText('Loading correlated logs…')
    expect(getSpan).toHaveBeenCalledTimes(2)
    retry.resolve([
      makeTraceLog({ bodyPreview: 'retried A activity' }),
      makeTraceLog({
        logRef: 'B-log',
        spanID: 'other',
        bodyPreview: 'B activity',
      }),
    ])
    await within(detail).findByText('retried A activity')
    expect(within(detail).queryByText('B activity')).toBeNull()
    expect(
      within(detail).queryByText('Could not load correlated logs.')
    ).toBeNull()
    expect(screen.queryByText('Loading correlated logs…')).toBeNull()
    await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() => expect(getTraceLogSummaries).toHaveBeenCalledTimes(3))
    expect(within(detail).getByText('retried A activity')).toBeInTheDocument()
    expect(screen.queryByText('Loading correlated logs…')).toBeNull()
    await screen.findByText('Loading correlated logs…')
    refresh.resolve([makeTraceLog({ bodyPreview: 'refreshed A activity' })])
    await within(detail).findByText('refreshed A activity')
    expect(within(detail).queryByText('retried A activity')).toBeNull()
    expect(screen.queryByText('Loading correlated logs…')).toBeNull()
    expect(within(detail).getByRole('table', { name: 'Span fields' })).toBe(
      fields
    )
    expect(fields.querySelector('[data-field="name"]')).toHaveTextContent(
      'root'
    )
  })

  it('publishes retained activity only to its own trace when span IDs match', async () => {
    const { root, other } = prepareRetainedSpan()
    const refresh = deferred<TraceLogSummary[]>()
    const nextExact = deferred<SpanData>()
    getTraceView.mockImplementation(async (traceID: string) =>
      traceID === 'trace-1'
        ? { traceID, unplacedSpanCount: 0, spans: [root, other] }
        : makeTraceData(0, traceID)
    )
    getSpan.mockImplementation((traceID: string) =>
      traceID === 'trace-1' ? Promise.resolve(root.spanData) : nextExact.promise
    )
    getTraceLogSummaries
      .mockResolvedValueOnce([
        makeTraceLog({ bodyPreview: 'retained A activity' }),
      ])
      .mockReturnValueOnce(refresh.promise)
      .mockResolvedValueOnce([
        makeTraceLog({ bodyPreview: 'other trace activity' }),
      ])
    renderWithContexts(TracesPage)
    const detail = screen.getByRole('region', { name: 'Span details' })
    const fields = await within(detail).findByRole('table', {
      name: 'Span fields',
    })
    await within(detail).findByText('retained A activity')
    await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() => expect(getTraceLogSummaries).toHaveBeenCalledTimes(2))
    const oldSignal = getTraceLogSummaries.mock.calls[1]![1] as AbortSignal
    navigate('/traces/trace-2?span=root')
    await waitFor(() => expect(getTraceLogSummaries).toHaveBeenCalledTimes(3))
    expect(oldSignal.aborted).toBe(false)
    refresh.resolve([
      makeTraceLog({ bodyPreview: 'refreshed retained activity' }),
    ])
    await screen.findByText('Loading selected span…')
    await within(detail).findByText('refreshed retained activity')
    expect(within(detail).queryByText('retained A activity')).toBeNull()
    expect(within(detail).queryByText('other trace activity')).toBeNull()
    expect(screen.queryByText('Loading correlated logs…')).toBeNull()
    expect(fields.querySelector('[data-field="traceId"]')).toHaveTextContent(
      'trace-1'
    )
    nextExact.resolve(makeSpanNode('root', 'trace-2').spanData)
    await within(detail).findByText('other trace activity')
    expect(oldSignal.aborted).toBe(true)
    expect(within(detail).queryByText('refreshed retained activity')).toBeNull()
    expect(fields.querySelector('[data-field="traceId"]')).toHaveTextContent(
      'trace-2'
    )
  })
})

function makeTraceSummary(overrides: Partial<TraceSummary> = {}): TraceSummary {
  return {
    traceID: 'trace-1',
    hasRootSpan: true,
    rootSpan: { name: 'GET /orders', serviceName: 'orders-service' },
    startTime: 1_700_000_000_000_000_000n,
    durationNs: 1_000_000n,
    spanCount: 1,
    errorCount: 0,
    ...overrides,
  }
}

function makeStats(): Stats {
  return {
    traces: {
      traceCount: 1,
      spanCount: 1,
      serviceCount: 1,
      errorCount: 0,
      lastReceived: null,
    },
    logs: { logCount: 0, errorCount: 0, lastReceived: null },
    metrics: { metricCount: 0, dataPointCount: 0, lastReceived: null },
    rejections: [],
  }
}

function makeSpanNode(id: string, traceID = 'trace-1'): SpanNode {
  return {
    depth: 0,
    matched: true,
    spanData: {
      spanID: id,
      flags: 0,
      parentSpanID: null,
      traceID,
      name: id,
      kindCode: 2,
      startTime: 0n,
      endTime: 1_000_000n,
      statusCode: 'Ok',
      statusCodeValue: 1,
      events: [],
      links: [],
      attributes: [],
      resource: { attributes: [], droppedAttributesCount: 0 },
      scope: {
        name: '',
        version: '',
        attributes: [],
        droppedAttributesCount: 0,
      },
      traceState: '',
      kind: 'Server',
      droppedAttributesCount: 0,
      droppedEventsCount: 0,
      droppedLinksCount: 0,
      statusMessage: '',
    },
  }
}

function makeTraceData(
  unplacedSpanCount: number,
  traceID = 'trace-1',
  spanID = 'root'
): TraceData {
  return {
    traceID,
    unplacedSpanCount,
    spans: [makeSpanNode(spanID, traceID)],
  }
}

function makeTraceLog(
  overrides: Partial<TraceLogSummary> = {}
): TraceLogSummary {
  return {
    logRef: 'log-1',
    timestamp: 2n,
    spanID: 'root',
    severityText: 'INFO',
    severityNumber: 9,
    serviceName: 'orders',
    eventName: '',
    bodyPreview: '',
    ...overrides,
  }
}

function deferred<T>() {
  let resolve!: (value: T) => void
  let reject!: (reason?: unknown) => void
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise
    reject = rejectPromise
  })
  return { promise, resolve, reject }
}

async function renderSelectedTrace(unplacedSpanCount: number) {
  searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
  getStats.mockResolvedValue(makeStats())
  getTraceView.mockResolvedValue(makeTraceData(unplacedSpanCount))
  setTestUrl('/traces/trace-1')
  renderWithContexts(TracesPage)
  // Wait on detail content rather than coupling this helper to the banner.
  await waitFor(() =>
    expect(document.querySelector('tr[data-span-id="root"]')).not.toBeNull()
  )
  await screen.findByRole('table', { name: 'Span fields' })
}

beforeEach(() => {
  searchTraceSummaries.mockReset()
  getStats.mockReset()
  getTraceView.mockReset()
  getSpan.mockReset()
  getSpan.mockImplementation(async (traceID: string, spanID: string) => {
    const trace = await Promise.resolve(
      getTraceView.mock.results.at(-1)?.value
    ).catch(() => null)
    return (
      (trace?.traceID === traceID ? trace.spans : []).find(
        (node: SpanNode) => node.spanData.spanID === spanID
      )?.spanData ?? makeSpanNode(spanID, traceID).spanData
    )
  })
  getTraceColumnPreviews.mockReset()
  getTraceColumnPreviews.mockResolvedValue([])
  getTraceLogSummaries.mockReset()
  getTraceLogSummaries.mockResolvedValue([])
  getTraceAttributeDefinitions.mockReset()
  getTraceAttributeDefinitions.mockResolvedValue([])
  clearTraces.mockReset()
  clearTraces.mockResolvedValue(undefined)
  deleteTraces.mockReset()
  deleteTraces.mockResolvedValue(undefined)
})

/** Collapses the template's line-wrapped whitespace into single spaces so
 *  assertions read (and match) as one sentence. */
function normalizedText(el: HTMLElement): string {
  return el.textContent!.replace(/\s+/g, ' ').trim()
}

describe('TracesPage unplaced spans banner', () => {
  it('opens a route-selected trace and span outside the filtered list on a fresh mount', async () => {
    searchTraceSummaries.mockResolvedValue([])
    getStats.mockResolvedValue(makeStats())
    const other = makeSpanNode('other')
    const selected = makeSpanNode('selected')
    selected.matched = false
    getTraceView.mockResolvedValue({
      traceID: 'trace-1',
      unplacedSpanCount: 0,
      spans: [other, selected],
    })
    setTestUrl('/traces/trace-1?span=selected&q=name%20%3D%20%22excluded%22')
    renderWithContexts(TracesPage)
    await waitFor(() =>
      expect(
        document.querySelector('tr[data-span-id="selected"]')
      ).not.toBeNull()
    )
    expect(new URLSearchParams(window.location.search).get('span')).toBe(
      'selected'
    )
    expect(new URLSearchParams(window.location.search).get('q')).toBe(
      'name = "excluded"'
    )
  })
  it('allows exporting the selected trace when all its spans are unplaced', async () => {
    searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    getTraceView.mockResolvedValue({
      traceID: 'trace-1',
      spans: [],
      unplacedSpanCount: 2,
    })
    setTestUrl('/traces/trace-1')
    renderWithContexts(TracesPage)
    const header = await screen.findByRole('region', {
      name: 'Trace waterfall',
    })
    const button = within(header).getByRole('button', {
      name: 'Download trace',
    })
    expect(button).toBeEnabled()
    const title = header.querySelector('.pane-header__title')!
    expect(
      within(button.closest<HTMLElement>('.pane-header__meta-controls')!)
        .getAllByRole('button')
        .at(-1)
    ).toBe(button)
    expect(
      title.compareDocumentPosition(button) & Node.DOCUMENT_POSITION_FOLLOWING
    ).not.toBe(0)
    expect(
      screen.getAllByRole('button', { name: 'Download trace' })
    ).toHaveLength(1)
    expect(await screen.findByRole('alert')).toHaveTextContent(
      '2 spans are missing'
    )
  })

  it('queries the list with null bounds for the default All selection', async () => {
    await renderSelectedTrace(0)
    expect(searchTraceSummaries).toHaveBeenCalledWith(
      null,
      null,
      undefined,
      undefined,
      { field: 'startTime', direction: 'desc' },
      expect.any(AbortSignal)
    )
    expect(getTraceLogSummaries).toHaveBeenCalledWith(
      'trace-1',
      expect.any(AbortSignal)
    )
  })

  it('renders no banner when every span was placed', async () => {
    await renderSelectedTrace(0)
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it('renders the banner with singular copy for exactly one unplaced span', async () => {
    await renderSelectedTrace(1)
    const alert = await screen.findByRole('alert')
    expect(normalizedText(alert)).toContain('1 span is missing from this trace')
    expect(normalizedText(alert)).not.toContain('spans are missing')
  })

  it('renders the banner with plural copy for more than one unplaced span', async () => {
    await renderSelectedTrace(3)
    const alert = await screen.findByRole('alert')
    expect(normalizedText(alert)).toContain(
      '3 spans are missing from this trace'
    )
    expect(normalizedText(alert)).not.toContain('3 span is')
  })
})

describe('TracesPage independent list loading', () => {
  it('starts and publishes a submitted tree search without waiting for the list, and keeps match metadata with that tree', async () => {
    await renderSelectedTrace(0)
    const oldRow = screen.getByRole('row', { name: /^root —/ })
    const list = deferred<TraceSummary[]>()
    const tree = deferred<TraceData>()
    searchTraceSummaries.mockReturnValueOnce(list.promise)
    getTraceView.mockReturnValueOnce(tree.promise)
    navigate('/traces/trace-1?span=root&q=name%20%3D%20%22filtered%22')
    await waitFor(() => expect(searchTraceSummaries).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(getTraceView).toHaveBeenCalledTimes(2))
    expect(getTraceView.mock.calls[1]![1]).toEqual(
      searchTraceSummaries.mock.calls[1]![2]
    )
    expect(oldRow).not.toHaveClass('waterfall-row--matched')
    expect(screen.getByRole('row', { name: /^root —/ })).toBe(oldRow)
    const next = makeTraceData(0)
    next.spans[0]!.spanData.name = 'filtered tree'
    tree.resolve(next)
    const replacement = await screen.findByRole('row', {
      name: /^filtered tree —/,
    })
    expect(replacement).toHaveClass('waterfall-row--matched')
    list.reject(new Error('filtered list unavailable'))
    await screen.findByText('Error: filtered list unavailable')
    expect(screen.getByRole('row', { name: /^filtered tree —/ })).toBe(
      replacement
    )
    expect(getTraceView).toHaveBeenCalledTimes(2)
  })

  it('starts each known trace immediately and resets its shared 200ms notice without replacing the old tree', async () => {
    await renderSelectedTrace(0)
    await screen.findByRole('table', { name: 'Span fields' })
    const row = screen.getByRole('row', { name: /^root —/ })
    const first = deferred<TraceData>()
    const second = deferred<TraceData>()
    getTraceView
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(second.promise)
    getSpan.mockImplementation(
      async (traceID: string, spanID: string) =>
        makeSpanNode(spanID, traceID).spanData
    )
    vi.useFakeTimers()
    const advance = (ms: number) => act(() => vi.advanceTimersByTimeAsync(ms))
    try {
      navigate('/traces/missing-B')
      await tick()
      expect(getTraceView).toHaveBeenCalledTimes(2)
      expect(screen.queryByText('Loading selected trace…')).toBeNull()
      await advance(199)
      expect(screen.queryByText('Loading selected trace…')).toBeNull()
      await advance(1)
      expect(screen.getByText('Loading selected trace…')).toBeInTheDocument()
      navigate('/traces/missing-C')
      await tick()
      expect(getTraceView).toHaveBeenCalledTimes(3)
      expect((getTraceView.mock.calls[1]![2] as AbortSignal).aborted).toBe(true)
      expect(screen.queryByText('Loading selected trace…')).toBeNull()
      expect(screen.getByRole('row', { name: /^root —/ })).toBe(row)
      await advance(199)
      expect(screen.queryByText('Loading selected trace…')).toBeNull()
      await advance(1)
      expect(screen.getByText('Loading selected trace…')).toBeInTheDocument()
      second.resolve(makeTraceData(0, 'missing-C', 'current'))
      await advance(0)
      expect(screen.queryByText('Loading selected trace…')).toBeNull()
      expect(
        screen.getByRole('row', { name: /^current —/ })
      ).toBeInTheDocument()
      first.resolve(makeTraceData(0, 'missing-B', 'stale'))
      await advance(200)
      expect(screen.queryByText('Loading selected trace…')).toBeNull()
      expect(document.querySelector('tr[data-span-id="stale"]')).toBeNull()
    } finally {
      vi.useRealTimers()
    }
  })

  it.each(['pending', 'failed'] as const)(
    'starts route-owned tree, logs and exact Details while the list is %s',
    async outcome => {
      const list = deferred<TraceSummary[]>()
      const tree = deferred<TraceData>()
      searchTraceSummaries.mockReturnValue(list.promise)
      getStats.mockResolvedValue(makeStats())
      getTraceView.mockReturnValue(tree.promise)
      getTraceLogSummaries.mockResolvedValue([
        makeTraceLog({ bodyPreview: 'independently received activity' }),
      ])
      getSpan.mockResolvedValue(makeSpanNode('root').spanData)
      setTestUrl('/traces/trace-1?span=root&log=log-1')
      renderWithContexts(TracesPage)
      await waitFor(() => expect(getTraceView).toHaveBeenCalledTimes(1))
      await screen.findByText('independently received activity')
      expect(new URLSearchParams(window.location.search).get('log')).toBe(
        'log-1'
      )
      tree.resolve(makeTraceData(0))
      await screen.findByRole('row', { name: /^root —/ })
      if (outcome === 'failed') {
        list.reject(new Error('list unavailable'))
        await screen.findByText('Error: list unavailable')
      }
      expect(getTraceView).toHaveBeenCalledTimes(1)
      expect(getTraceLogSummaries).toHaveBeenCalledTimes(1)
      expect(
        screen.getByRole('table', { name: 'Span fields' })
      ).toBeInTheDocument()
      expect(screen.queryByText('No traces in this time range')).toBeNull()
    }
  )

  it.each(['success', 'failure'] as const)(
    'supersedes tree/log %s promptly for a missing selection during a pending list refresh',
    async outcome => {
      const oldTree = deferred<TraceData>()
      const oldLogs = deferred<TraceLogSummary[]>()
      const nextTree = deferred<TraceData>()
      const nextLogs = deferred<TraceLogSummary[]>()
      const list = deferred<TraceSummary[]>()
      searchTraceSummaries
        .mockResolvedValueOnce([makeTraceSummary()])
        .mockReturnValueOnce(list.promise)
      getStats.mockResolvedValue(makeStats())
      getTraceView
        .mockReturnValueOnce(oldTree.promise)
        .mockReturnValueOnce(nextTree.promise)
      getTraceLogSummaries
        .mockReturnValueOnce(oldLogs.promise)
        .mockReturnValueOnce(nextLogs.promise)
      getSpan.mockImplementation(
        async (traceID: string, spanID: string) =>
          makeSpanNode(spanID, traceID).spanData
      )
      setTestUrl('/traces/trace-1?span=root')
      renderWithContexts(TracesPage)
      await screen.findByText('GET /orders')
      await waitFor(() => expect(getTraceView).toHaveBeenCalledTimes(1))
      // Both the list refresh and the missing selection must supersede the old
      // activation, without waiting for the refreshed list to settle.
      const refreshTree = deferred<TraceData>()
      const refreshLogs = deferred<TraceLogSummary[]>()
      getTraceView
        .mockReset()
        .mockReturnValueOnce(refreshTree.promise)
        .mockReturnValueOnce(nextTree.promise)
      getTraceLogSummaries
        .mockReset()
        .mockReturnValueOnce(refreshLogs.promise)
        .mockReturnValueOnce(nextLogs.promise)
      await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
      await waitFor(() => expect(searchTraceSummaries).toHaveBeenCalledTimes(2))
      navigate('/traces/missing-trace?span=next&log=next-log')
      await waitFor(() =>
        expect(getTraceView).toHaveBeenCalledWith(
          'missing-trace',
          undefined,
          expect.any(AbortSignal),
          expect.any(Array)
        )
      )
      expect((getTraceView.mock.calls[0]![2] as AbortSignal).aborted).toBe(true)
      expect(
        (getTraceLogSummaries.mock.calls[0]![1] as AbortSignal).aborted
      ).toBe(true)
      if (outcome === 'success') {
        oldTree.resolve(makeTraceData(0, 'trace-1', 'stale'))
        refreshTree.resolve(makeTraceData(0, 'trace-1', 'stale-refresh'))
        oldLogs.resolve([])
        refreshLogs.resolve([])
      } else {
        oldTree.reject(new Error('stale tree failure'))
        refreshTree.reject(new Error('stale refresh failure'))
        oldLogs.reject(new Error('stale logs failure'))
        refreshLogs.reject(new Error('stale refresh logs failure'))
      }
      await screen.findByText('Loading selected trace…')
      expect(
        screen.queryByText('Could not load the selected trace.')
      ).toBeNull()
      expect(screen.queryByText('Could not load correlated logs.')).toBeNull()
      expect(window.location.search).toBe('?span=next&log=next-log')
      nextTree.resolve(makeTraceData(0, 'missing-trace', 'next'))
      nextLogs.resolve([
        makeTraceLog({
          logRef: 'next-log',
          spanID: 'next',
          bodyPreview: 'current activity',
        }),
      ])
      await screen.findByRole('row', { name: /^next —/ })
      await within(
        screen.getByRole('region', { name: 'Span details' })
      ).findByText('current activity')
      expect(window.location.search).toBe('?span=next&log=next-log')
    }
  )

  it.each(['success', 'failure'] as const)(
    'ignores an old A tree %s after A→missing B→A while the list is pending',
    async outcome => {
      const list = deferred<TraceSummary[]>()
      const old = deferred<TraceData>()
      const middle = deferred<TraceData>()
      const latest = deferred<TraceData>()
      searchTraceSummaries.mockReturnValue(list.promise)
      getStats.mockResolvedValue(makeStats())
      getTraceView
        .mockReturnValueOnce(old.promise)
        .mockReturnValueOnce(middle.promise)
        .mockReturnValueOnce(latest.promise)
      getSpan.mockImplementation(
        async (traceID: string, spanID: string) =>
          makeSpanNode(spanID, traceID).spanData
      )
      setTestUrl('/traces/trace-1?span=root')
      renderWithContexts(TracesPage)
      await waitFor(() => expect(getTraceView).toHaveBeenCalledTimes(1))
      navigate('/traces/missing-trace?span=other')
      await waitFor(() => expect(getTraceView).toHaveBeenCalledTimes(2))
      navigate('/traces/trace-1?span=current')
      await waitFor(() => expect(getTraceView).toHaveBeenCalledTimes(3))
      expect((getTraceView.mock.calls[0]![2] as AbortSignal).aborted).toBe(true)
      if (outcome === 'success')
        old.resolve(makeTraceData(0, 'trace-1', 'stale'))
      else old.reject(new Error('stale A failure'))
      await screen.findByText('Loading selected trace…')
      expect(window.location.search).toBe('?span=current')
      expect(
        screen.queryByText('Could not load the selected trace.')
      ).toBeNull()
      latest.resolve(makeTraceData(0, 'trace-1', 'current'))
      await screen.findByRole('row', { name: /^current —/ })
      list.resolve([makeTraceSummary()])
      await screen.findByText('GET /orders')
      expect(getTraceView).toHaveBeenCalledTimes(3)
      expect(document.querySelector('tr[data-span-id="stale"]')).toBeNull()
    }
  )

  it.each(['pending', 'failed'] as const)(
    'publishes the trace list and opens its tree while statistics are %s',
    async outcome => {
      const stats = deferred<Stats>()
      searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
      getStats.mockReturnValue(stats.promise)
      getTraceView.mockResolvedValue(makeTraceData(0))
      setTestUrl('/traces')
      renderWithContexts(TracesPage)
      await screen.findByText('GET /orders')
      await waitFor(() =>
        expect(document.querySelector('tr[data-span-id="root"]')).not.toBeNull()
      )
      if (outcome === 'failed') {
        stats.reject(new Error('statistics failed'))
        await new Promise(resolve => setTimeout(resolve, 0))
      }
      expect(screen.queryByText('Loading traces…')).toBeNull()
      expect(screen.queryByText(/statistics failed/)).toBeNull()
    }
  )

  it.each(['success', 'failure'] as const)(
    'retains the displayed tree during direct search and ignores stale %s',
    async outcome => {
      await renderSelectedTrace(0)
      const tree = document.querySelector('tr[data-span-id="root"]')
      const old = deferred<TraceSummary[]>()
      const current = deferred<TraceSummary[]>()
      searchTraceSummaries
        .mockReturnValueOnce(old.promise)
        .mockReturnValueOnce(current.promise)
      navigate('/traces/trace-1?span=root&q=name%20%3D%20%22old%22')
      await waitFor(() => expect(searchTraceSummaries).toHaveBeenCalledTimes(2))
      navigate('/traces/trace-1?span=root&q=name%20%3D%20%22current%22')
      await waitFor(() => expect(searchTraceSummaries).toHaveBeenCalledTimes(3))
      expect(searchTraceSummaries.mock.calls[1]![5].aborted).toBe(true)
      if (outcome === 'success') old.resolve([])
      else old.reject(new Error('stale search failed'))
      await screen.findByText('Loading traces…')
      expect(document.querySelector('tr[data-span-id="root"]')).toBe(tree)
      expect(screen.queryByText(/stale search failed/)).toBeNull()
      current.reject(new Error('current search failed'))
      await screen.findByText('Error: current search failed')
      expect(document.querySelector('tr[data-span-id="root"]')).toBe(tree)
      expect(screen.queryByText('Loading traces…')).toBeNull()
    }
  )
})

describe('TracesPage trace detail lifecycle', () => {
  it('does not republish retained A with new activity selection before the returned exact-span activation succeeds', async () => {
    const old = makeTraceData(0)
    old.spans[0]!.spanData.name = 'retained A'
    old.spans[0]!.spanData.events = ['first event', 'second event'].map(
      (name, index) => ({
        name,
        timestamp: BigInt(index),
        attributes: [],
        droppedAttributesCount: 0,
      })
    )
    const middleTree = deferred<TraceData>()
    const latestTree = deferred<TraceData>()
    const middleSpan = deferred<SpanData>()
    const latestSpan = deferred<SpanData>()
    searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    getTraceView
      .mockResolvedValueOnce(old)
      .mockReturnValueOnce(middleTree.promise)
      .mockReturnValueOnce(latestTree.promise)
    getSpan
      .mockResolvedValueOnce(old.spans[0]!.spanData)
      .mockReturnValueOnce(middleSpan.promise)
      .mockReturnValueOnce(latestSpan.promise)
    setTestUrl('/traces/trace-1?span=root&event=0')
    renderWithContexts(TracesPage)
    const detail = screen.getByRole('region', { name: 'Span details' })
    const fields = await within(detail).findByRole('table', {
      name: 'Span fields',
    })
    const secondEvent = within(
      document.getElementById('span-event-1')!
    ).getByRole('button')
    expect(secondEvent).toHaveAttribute('aria-expanded', 'false')
    navigate('/traces/missing-trace?span=root')
    await waitFor(() => expect(getSpan).toHaveBeenCalledTimes(2))
    navigate('/traces/trace-1?span=root&event=1')
    await waitFor(() => expect(getSpan).toHaveBeenCalledTimes(3))
    expect((getSpan.mock.calls[1]![2] as AbortSignal).aborted).toBe(true)
    middleSpan.resolve({
      ...makeSpanNode('root', 'missing-trace').spanData,
      name: 'stale B',
    })
    await screen.findByText('Loading selected trace…')
    expect(fields.querySelector('[data-field="name"]')).toHaveTextContent(
      'retained A'
    )
    expect(secondEvent).toHaveAttribute('aria-expanded', 'false')
    expect(within(detail).queryByText('stale B')).toBeNull()
    latestSpan.resolve({ ...old.spans[0]!.spanData, name: 'fresh A' })
    await within(detail).findByText('fresh A')
    expect(secondEvent).toHaveAttribute('aria-expanded', 'true')
    expect(window.location.search).toBe('?span=root&event=1')
    latestTree.resolve(old)
    await waitFor(() =>
      expect(screen.queryByText('Loading selected trace…')).toBeNull()
    )
    expect(getSpan).toHaveBeenCalledTimes(3)
  })

  it.each(['pending', 'failed'] as const)(
    'loads new columns for the displayed tree while a different trace is %s',
    async outcome => {
      const next = deferred<TraceData>()
      const preview = deferred<TraceColumnPreview[]>()
      const root = makeSpanNode('root')
      const old = {
        traceID: 'trace-1',
        unplacedSpanCount: 0,
        availableColumns: [{ source: 'span', key: 'new-key', count: 1 }],
        spans: [{ ...root, spanData: { ...root.spanData, columnValues: [] } }],
      }
      searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
      getStats.mockResolvedValue(makeStats())
      getTraceView.mockResolvedValueOnce(old).mockReturnValueOnce(next.promise)
      getSpan.mockResolvedValue(root.spanData)
      getTraceColumnPreviews.mockReturnValue(preview.promise)
      const consoleError = vi
        .spyOn(console, 'error')
        .mockImplementation(() => {})
      try {
        setTestUrl('/traces/trace-1?span=root')
        renderWithContexts(TracesPage)
        const row = await screen.findByRole('row', { name: /^root …/ })
        navigate('/traces/missing-trace')
        await waitFor(() => expect(getTraceView).toHaveBeenCalledTimes(2))
        if (outcome === 'failed') {
          next.reject(new Error('replacement unavailable'))
          await screen.findByText('Could not load the selected trace.')
        }
        await fireEvent.click(
          screen.getByRole('checkbox', { name: 'span: new-key', hidden: true })
        )
        await waitFor(() =>
          expect(getTraceColumnPreviews).toHaveBeenCalledTimes(1)
        )
        expect(getTraceColumnPreviews).toHaveBeenCalledWith(
          'trace-1',
          expect.arrayContaining([{ source: 'span', key: 'new-key' }]),
          expect.any(AbortSignal)
        )
        expect(within(row).queryByText('—')).toBeNull()
        preview.resolve([
          {
            spanID: 'root',
            source: 'span',
            key: 'new-key',
            values: [{ valueType: 'int64', preview: '9223372036854775807' }],
          },
        ])
        await within(row).findByText('9223372036854775807')
        expect(window.location.pathname).toBe('/traces/missing-trace')
      } finally {
        consoleError.mockRestore()
      }
    }
  )

  it.each(['pending', 'failed'] as const)(
    'keeps retained Details metadata, activity, disclosures and exact on-demand actions coherent through a %s replacement',
    async outcome => {
      const old = makeTraceData(0)
      const root = old.spans[0]!
      root.salvaged = true
      root.cyclePoint = true
      root.spanData.events = [
        {
          name: 'old event',
          timestamp: 100_000n,
          attributes: [],
          droppedAttributesCount: 0,
        },
      ]
      root.spanData.links = [
        {
          traceID: 'linked-trace',
          spanID: 'linked-span',
          traceState: '',
          flags: 0,
          attributes: [],
          droppedAttributesCount: 0,
          summary: { target: { name: 'old link', serviceName: 'worker' } },
        },
      ]
      const nextTree = deferred<TraceData>()
      const nextSpan = deferred<SpanData>()
      const latest = makeTraceData(0, 'trace-3', 'latest-root')
      latest.spans[0]!.spanData.events = [
        {
          name: 'latest event',
          timestamp: 1n,
          attributes: [],
          droppedAttributesCount: 0,
        },
      ]
      searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
      getStats.mockResolvedValue(makeStats())
      getTraceView.mockImplementation((traceID: string) =>
        traceID === 'trace-1'
          ? Promise.resolve(old)
          : traceID === 'trace-3'
            ? Promise.resolve(latest)
            : nextTree.promise
      )
      getSpan.mockImplementation((traceID: string) =>
        traceID === 'trace-1'
          ? Promise.resolve(root.spanData)
          : traceID === 'trace-3'
            ? Promise.resolve(latest.spans[0]!.spanData)
            : nextSpan.promise
      )
      getTraceLogSummaries.mockImplementation(async (traceID: string) => [
        makeTraceLog({
          bodyPreview:
            traceID === 'trace-1' ? 'old activity' : 'replacement activity',
          spanID: traceID === 'trace-3' ? 'latest-root' : 'root',
        }),
      ])
      const user = userEvent.setup()
      const write = vi
        .spyOn(navigator.clipboard, 'writeText')
        .mockResolvedValue()
      const bytes =
        '{"resourceSpans":[{"resource":{"attributes":[{"key":"large","value":{"intValue":"9223372036854775807"}}]},"scopeSpans":[{"scope":{"name":"complete-scope"},"spans":[{"traceId":"trace-1","spanId":"root","attributes":[{"key":"precise","value":{"doubleValue":1.234567890123456789}},{"key":"zero","value":{"doubleValue":-0.0}},{"key":"nested","value":{"kvlistValue":{"values":[{"key":"large","value":{"intValue":"-9223372036854775808"}}]}}}],"events":[{"name":"complete event"}],"links":[{"traceId":"linked-trace","spanId":"linked-span"}]}]}]}]}\n'
      const fetch = vi.spyOn(globalThis, 'fetch').mockImplementation(
        async url =>
          new Response(
            url === '/api/features' ? '{"evals":{"enabled":true}}' : bytes,
            {
              headers: { 'Content-Type': 'application/json' },
            }
          )
      )
      const exports = () =>
        fetch.mock.calls.filter(([url]) => String(url).startsWith('/export/'))
      const createObjectURL = vi.fn((_blob: Blob) => 'blob:trace-retained')
      const revokeObjectURL = vi.fn()
      const NativeURL = URL
      vi.stubGlobal(
        'URL',
        class extends NativeURL {
          static createObjectURL = createObjectURL
          static revokeObjectURL = revokeObjectURL
        }
      )
      const downloads: string[] = []
      const click = vi
        .spyOn(HTMLAnchorElement.prototype, 'click')
        .mockImplementation(function (this: HTMLAnchorElement) {
          downloads.push(this.download)
        })
      const consoleError = vi
        .spyOn(console, 'error')
        .mockImplementation(() => {})
      try {
        setTestUrl('/traces/trace-1?span=root&event=0')
        renderWithContexts(TracesPage)
        const detail = screen.getByRole('region', { name: 'Span details' })
        const fields = await within(detail).findByRole('table', {
          name: 'Span fields',
        })
        const activity = await within(detail).findByText('old activity')
        const linkHeader = within(detail).getByText('Links').closest('summary')!
        const linkSection = linkHeader.closest('details')!
        if (!linkSection.open) await fireEvent.click(linkHeader)
        expect(linkSection.open).toBe(true)
        const rootLink = within(detail).getByRole('link', {
          name: 'Open root span in trace trace-1',
        })
        expect(exports()).toHaveLength(0)
        navigate('/traces/missing-trace?span=root&log=log-1')
        await waitFor(() => expect(getTraceView).toHaveBeenCalledTimes(2))
        if (outcome === 'failed') {
          nextTree.reject(new Error('tree replacement failed'))
          nextSpan.reject(new Error('span replacement failed'))
          await screen.findByText('Could not load the selected trace.')
          await within(detail).findByText('span replacement failed')
        }
        expect(within(detail).getByRole('table', { name: 'Span fields' })).toBe(
          fields
        )
        expect(within(detail).getByText('old activity')).toBe(activity)
        expect(within(detail).queryByText('replacement activity')).toBeNull()
        expect(within(detail).getByText('· root')).toBeInTheDocument()
        expect(within(detail).getByText(/Cycle detected:/)).toBeInTheDocument()
        expect(rootLink).toHaveAttribute('href', '/traces/trace-1?span=root')
        expect(linkSection.open).toBe(true)
        expect(
          within(linkSection).getByText('old link (worker)')
        ).toBeInTheDocument()
        expect(
          within(detail).getByText('Events').closest('summary')
        ).toHaveTextContent('[2]')
        await user.click(
          within(detail).getByRole('button', { name: 'Copy span' })
        )
        await waitFor(() => expect(write).toHaveBeenCalledWith(bytes))
        expect(exports().at(-1)?.[0]).toBe('/export/traces/trace-1?spanId=root')
        await user.click(screen.getByRole('button', { name: 'Download trace' }))
        await waitFor(() => expect(createObjectURL).toHaveBeenCalledTimes(1))
        expect(exports().at(-1)?.[0]).toBe('/export/traces/trace-1')
        expect(await createObjectURL.mock.calls[0]![0].text()).toBe(bytes)
        expect(downloads).toEqual(['trace-trace-1.json'])
        expect(exports()).toHaveLength(2)
        navigate('/traces/trace-3?span=latest-root')
        await within(detail).findByText('latest event')
        await within(detail).findByText('replacement activity')
        expect(
          fields.querySelector('[data-field="traceId"]')
        ).toHaveTextContent('trace-3')
        expect(within(detail).queryByText('old activity')).toBeNull()
        expect(within(detail).queryByText(/Cycle detected:/)).toBeNull()
        expect(
          within(detail).getByRole('link', {
            name: 'Open root span in trace trace-3',
          })
        ).toHaveAttribute('href', '/traces/trace-3?span=latest-root')
        expect(exports()).toHaveLength(2)
      } finally {
        fetch.mockRestore()
        write.mockRestore()
        click.mockRestore()
        consoleError.mockRestore()
        vi.unstubAllGlobals()
      }
    }
  )

  describe.each(['pending', 'failed'] as const)(
    'retained waterfall with a %s replacement',
    outcome => {
      it.each(['span', 'event', 'log', 'column'] as const)(
        'selects the displayed trace through its retained %s handler',
        async action => {
          const next = deferred<TraceData>()
          const old = makeTraceData(0)
          const child = makeSpanNode('child')
          child.depth = 1
          child.spanData.parentSpanID = 'root'
          child.spanData.events = [
            {
              name: 'retained event',
              timestamp: 300_000n,
              attributes: [],
              droppedAttributesCount: 0,
            },
          ]
          child.spanData.statusCodeValue = 2
          child.spanData.statusCode = 'Error'
          child.spanData.statusMessage = 'retained status'
          old.spans.push(child)
          saveColumns([{ source: 'span', key: 'status.message', field: true }])
          searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
          getStats.mockResolvedValue(makeStats())
          getTraceView.mockImplementation((traceID: string) =>
            traceID === 'trace-1' ? Promise.resolve(old) : next.promise
          )
          getSpan.mockImplementation(async (traceID: string, spanID: string) =>
            traceID === 'trace-1'
              ? old.spans.find(node => node.spanData.spanID === spanID)!
                  .spanData
              : makeSpanNode(spanID, traceID).spanData
          )
          getTraceLogSummaries.mockImplementation(async (traceID: string) =>
            traceID === 'trace-1'
              ? [
                  makeTraceLog({
                    spanID: 'child',
                    timestamp: 800_000n,
                    bodyPreview: 'retained log',
                  }),
                ]
              : []
          )
          const consoleError = vi
            .spyOn(console, 'error')
            .mockImplementation(() => {})
          setTestUrl('/traces/trace-1?span=root')
          renderWithContexts(TracesPage)
          const row = await screen.findByRole('row', {
            name: /^child retained status/,
          })
          navigate('/traces/missing-trace?span=root')
          await waitFor(() => expect(getTraceView).toHaveBeenCalledTimes(2))
          if (outcome === 'failed') {
            next.reject(new Error('replacement unavailable'))
            await screen.findByText('Could not load the selected trace.')
          }
          if (action === 'span') await fireEvent.click(row)
          else if (action === 'column')
            await fireEvent.click(
              within(row).getByRole('button', {
                name: 'Show span field: status.message in details',
              })
            )
          else
            await fireEvent.click(
              within(row).getByRole('button', {
                name: action === 'event' ? /1 event/ : /1 log/,
              })
            )
          await waitFor(() =>
            expect(window.location.pathname).toBe('/traces/trace-1')
          )
          const query = new URLSearchParams(window.location.search)
          expect(query.get('span')).toBe('child')
          expect(query.get('event')).toBe(action === 'event' ? '0' : null)
          expect(query.get('log')).toBe(action === 'log' ? 'log-1' : null)
          const detail = screen.getByRole('region', { name: 'Span details' })
          const fields = await within(detail).findByRole('table', {
            name: 'Span fields',
          })
          await waitFor(() =>
            expect(
              fields.querySelector('[data-field="spanId"]')
            ).toHaveTextContent('child')
          )
          if (action === 'column')
            await waitFor(() =>
              expect(
                fields.querySelector('[data-field="status.message"]')
              ).toHaveClass('detail-property--revealed')
            )
          expect(
            screen.queryByText('Could not load the selected trace.')
          ).toBeNull()
          expect(getSpan.mock.calls.at(-1)?.slice(0, 2)).toEqual([
            'trace-1',
            'child',
          ])
          consoleError.mockRestore()
        }
      )
    }
  )

  it.each(['success', 'failure'] as const)(
    'ignores a late unmounted tree/log %s, including URL writes',
    async outcome => {
      const tree = deferred<TraceData>()
      const logs = deferred<TraceLogSummary[]>()
      searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
      getStats.mockResolvedValue(makeStats())
      getTraceView.mockReturnValue(tree.promise)
      getTraceLogSummaries.mockReturnValue(logs.promise)
      getSpan.mockResolvedValue(makeSpanNode('requested').spanData)
      const consoleError = vi
        .spyOn(console, 'error')
        .mockImplementation(() => {})
      setTestUrl('/traces/trace-1?span=requested&event=0')
      const mounted = renderWithContexts(TracesPage)
      await waitFor(() => expect(getTraceView).toHaveBeenCalledTimes(1))
      mounted.unmount()
      expect((getTraceView.mock.calls[0]![2] as AbortSignal).aborted).toBe(true)
      expect(
        (getTraceLogSummaries.mock.calls[0]![1] as AbortSignal).aborted
      ).toBe(true)
      if (outcome === 'success') {
        tree.resolve(makeTraceData(0, 'trace-1', 'late'))
        logs.resolve([])
      } else {
        tree.reject(new Error('late unmounted tree failure'))
        logs.reject(new Error('late unmounted log failure'))
      }
      await new Promise(resolve => setTimeout(resolve, 0))
      expect(window.location.search).toBe('?span=requested&event=0')
      expect(consoleError).not.toHaveBeenCalled()
      consoleError.mockRestore()
    }
  )

  it.each([
    'ABCDEF0123456789ABCDEF0123456789',
    'abcdef01-2345-6789-abcd-ef0123456789',
  ])('opens canonical Details for trace route %s', async routeID => {
    const traceID = 'abcdef0123456789abcdef0123456789'
    const spanID = 'abcdef0123456789'
    const trace = makeTraceData(0, traceID, spanID)
    searchTraceSummaries.mockResolvedValue([makeTraceSummary({ traceID })])
    getStats.mockResolvedValue(makeStats())
    getTraceView.mockResolvedValue(trace)
    getSpan.mockResolvedValue(trace.spans[0]!.spanData)
    setTestUrl(`/traces/${routeID}?span=${spanID}`)
    renderWithContexts(TracesPage)
    await waitFor(() =>
      expect(document.querySelector('[data-field="spanId"]')).not.toBeNull()
    )
    expect(getSpan).toHaveBeenCalledWith(
      traceID,
      spanID,
      expect.any(AbortSignal)
    )
    expect(screen.queryByRole('alert')).not.toBeInTheDocument()
  })

  it.each(['reactivation', 'refresh'] as const)(
    'reloads linked-target availability on trace %s',
    async transition => {
      searchTraceSummaries.mockImplementation(async () => [
        makeTraceSummary(),
        makeTraceSummary({ traceID: 'trace-2' }),
      ])
      getStats.mockResolvedValue(makeStats())
      getTraceView.mockImplementation(async traceID =>
        makeTraceData(0, traceID)
      )
      let available = false
      getSpan.mockImplementation(async (traceID, spanID) => ({
        ...makeSpanNode(spanID, traceID).spanData,
        links: [
          {
            traceID: 'linked-trace',
            spanID: 'linked-span',
            traceState: '',
            attributes: [],
            droppedAttributesCount: 0,
            flags: 0,
            summary: {
              target: available
                ? { name: 'Arrived target', serviceName: 'worker' }
                : null,
            },
          },
        ],
      }))
      setTestUrl('/traces/trace-1?span=root')
      renderWithContexts(TracesPage)
      await screen.findByText('(unavailable)')
      if (transition === 'reactivation') {
        navigate('/traces/trace-2?span=root')
        await waitFor(() =>
          expect(getSpan).toHaveBeenCalledWith(
            'trace-2',
            'root',
            expect.any(AbortSignal)
          )
        )
      }
      available = true
      if (transition === 'reactivation') navigate('/traces/trace-1?span=root')
      else
        await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
      await screen.findByText('Arrived target (worker)')
      expect(
        getSpan.mock.calls.filter(([id]) => id === 'trace-1')
      ).toHaveLength(2)
    }
  )

  it('keeps same-span-ID Details and activity isolated across traces during a tree handoff', async () => {
    const nextTree = deferred<TraceData>()
    const old = makeTraceData(0)
    const next = makeTraceData(0, 'trace-2')
    next.spans[0]!.spanData.statusCodeValue = 2
    next.spans[0]!.spanData.statusCode = 'Error'
    next.spans[0]!.spanData.statusMessage = 'new trace Details'
    next.spans[0]!.spanData.events = [
      {
        name: 'new event',
        timestamp: 1n,
        attributes: [],
        droppedAttributesCount: 0,
      },
    ]
    searchTraceSummaries.mockResolvedValue([
      makeTraceSummary(),
      makeTraceSummary({ traceID: 'trace-2' }),
    ])
    getStats.mockResolvedValue(makeStats())
    getTraceView.mockImplementation(traceID =>
      traceID === 'trace-1' ? Promise.resolve(old) : nextTree.promise
    )
    getTraceLogSummaries.mockImplementation(async traceID =>
      traceID === 'trace-1' ? [makeTraceLog()] : []
    )
    getSpan.mockImplementation(
      async traceID => (traceID === 'trace-1' ? old : next).spans[0]!.spanData
    )
    setTestUrl('/traces/trace-1?span=root')
    renderWithContexts(TracesPage)
    await waitFor(() =>
      expect(document.querySelector('[data-field="spanId"]')).not.toBeNull()
    )
    navigate('/traces/trace-2?span=root&event=0')
    await screen.findByText('new trace Details')
    expect(getSpan).toHaveBeenCalledWith(
      'trace-2',
      'root',
      expect.any(AbortSignal)
    )
    expect(
      screen.getByText('Events', { exact: true }).closest('summary')
    ).toHaveTextContent('Events [1]')
    nextTree.resolve(next)
    await waitFor(() =>
      expect(window.location.search).toBe('?span=root&event=0')
    )
  })

  it('starts known-span Details concurrently with the tree without an immediate loading replacement', async () => {
    const tree = deferred<TraceData>()
    const detail = deferred<SpanData>()
    searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    getTraceView.mockReturnValue(tree.promise)
    getSpan.mockReturnValue(detail.promise)
    setTestUrl('/traces/trace-1?span=root')
    renderWithContexts(TracesPage)
    await waitFor(() =>
      expect(getSpan).toHaveBeenCalledWith(
        'trace-1',
        'root',
        expect.any(AbortSignal)
      )
    )
    expect(screen.queryByText('Loading selected span…')).not.toBeInTheDocument()
    detail.resolve({ ...makeSpanNode('root').spanData, name: 'Exact details' })
    await screen.findByText('Exact details')
    tree.resolve(makeTraceData(0))
    await waitFor(() =>
      expect(document.querySelector('tr[data-span-id="root"]')).not.toBeNull()
    )
    expect(getSpan).toHaveBeenCalledTimes(1)
  })

  it('cancels superseded span requests and ignores a late result', async () => {
    const root = deferred<SpanData>()
    const child = deferred<SpanData>()
    const trace = makeTraceData(0)
    trace.spans.push(makeSpanNode('child'))
    searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    getTraceView.mockResolvedValue(trace)
    getSpan.mockImplementation((_traceID, spanID) =>
      spanID === 'root' ? root.promise : child.promise
    )
    setTestUrl('/traces/trace-1?span=root')
    renderWithContexts(TracesPage)
    await waitFor(() =>
      expect(document.querySelector('tr[data-span-id="child"]')).not.toBeNull()
    )
    const rootSignal = getSpan.mock.calls[0]![2] as AbortSignal
    await fireEvent.click(screen.getByRole('row', { name: /^child —/ }))
    await waitFor(() =>
      expect(getSpan).toHaveBeenCalledWith(
        'trace-1',
        'child',
        expect.any(AbortSignal)
      )
    )
    expect(rootSignal.aborted).toBe(true)
    child.resolve({
      ...makeSpanNode('child').spanData,
      statusCodeValue: 2,
      statusCode: 'Error',
      statusMessage: 'current Details',
    })
    await screen.findByText('current Details')
    root.resolve({
      ...makeSpanNode('root').spanData,
      statusCodeValue: 2,
      statusCode: 'Error',
      statusMessage: 'stale Details',
    })
    await waitFor(() =>
      expect(screen.queryByText('stale Details')).not.toBeInTheDocument()
    )
    expect(screen.getByText('current Details')).toBeInTheDocument()
  })

  it('reuses exact span Details from the bounded selection cache', async () => {
    const trace = makeTraceData(0)
    trace.spans.push(makeSpanNode('child'))
    searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    getTraceView.mockResolvedValue(trace)
    getSpan.mockImplementation(
      async (_traceID, spanID) => makeSpanNode(spanID).spanData
    )
    setTestUrl('/traces/trace-1?span=root')
    renderWithContexts(TracesPage)
    await waitFor(() =>
      expect(document.querySelector('tr[data-span-id="child"]')).not.toBeNull()
    )
    await waitFor(() =>
      expect(
        screen.queryByText('Loading selected span…')
      ).not.toBeInTheDocument()
    )
    await fireEvent.click(screen.getByRole('row', { name: /^child —/ }))
    await waitFor(() =>
      expect(getSpan).toHaveBeenCalledWith(
        'trace-1',
        'child',
        expect.any(AbortSignal)
      )
    )
    await waitFor(() =>
      expect(
        screen.queryByText('Loading selected span…')
      ).not.toBeInTheDocument()
    )
    const before = getSpan.mock.calls.length
    await fireEvent.click(screen.getByRole('row', { name: /^root —/ }))
    await waitFor(() =>
      expect(
        screen.queryByText('Loading selected span…')
      ).not.toBeInTheDocument()
    )
    expect(getSpan).toHaveBeenCalledTimes(before)
  })

  it('preserves valid event and log deep links until current trace detail loads', async () => {
    const trace = makeTraceData(0)
    trace.spans[0]!.spanData.events = [
      {
        name: 'ready',
        timestamp: 1n,
        attributes: [],
        droppedAttributesCount: 0,
      },
    ]
    const logs: TraceLogSummary[] = [
      {
        logRef: 'log-1',
        timestamp: 2n,
        spanID: 'root',
        severityText: 'INFO',
        severityNumber: 9,
        serviceName: 'orders',
        eventName: '',
        bodyPreview: '',
      },
    ]
    searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    getTraceView.mockResolvedValue(trace)
    getTraceLogSummaries.mockResolvedValue(logs)
    getSpan.mockResolvedValue(trace.spans[0]!.spanData)
    setTestUrl('/traces/trace-1?span=root&log=log-1')

    renderWithContexts(TracesPage)

    await waitFor(() =>
      expect(
        screen.getByText('Events', { exact: true }).closest('summary')
      ).toHaveTextContent('Events [2]')
    )
    expect(window.location.search).toBe('?span=root&log=log-1')
  })

  it('prefers a resolved event when both activity selectors are valid', async () => {
    const trace = makeTraceData(0)
    trace.spans[0]!.spanData.events = [
      {
        name: 'ready',
        timestamp: 1n,
        attributes: [],
        droppedAttributesCount: 0,
      },
    ]
    searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    getTraceView.mockResolvedValue(trace)
    getSpan.mockResolvedValue(trace.spans[0]!.spanData)
    getTraceLogSummaries.mockResolvedValue([makeTraceLog()])
    setTestUrl('/traces/trace-1?span=root&event=0&log=log-1')

    renderWithContexts(TracesPage)

    await waitFor(() =>
      expect(window.location.search).toBe('?span=root&event=0')
    )
  })

  it('preserves a resolved log when a competing numeric event is stale', async () => {
    searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    getTraceView.mockResolvedValue(makeTraceData(0))
    getTraceLogSummaries.mockResolvedValue([makeTraceLog()])
    setTestUrl('/traces/trace-1?span=root&event=99&log=log-1')

    renderWithContexts(TracesPage)

    await waitFor(() =>
      expect(window.location.search).toBe('?span=root&log=log-1')
    )
  })

  it('removes both competing selectors when neither resolves', async () => {
    searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    getTraceView.mockResolvedValue(makeTraceData(0))
    getTraceLogSummaries.mockResolvedValue([
      makeTraceLog({ spanID: 'other-span' }),
    ])
    setTestUrl('/traces/trace-1?span=root&event=99&log=log-1')

    renderWithContexts(TracesPage)

    await waitFor(() => expect(window.location.search).toBe('?span=root'))
  })

  it('removes a selected log that is not owned by the selected span', async () => {
    searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    getTraceView.mockResolvedValue(makeTraceData(0))
    getTraceLogSummaries.mockResolvedValue([
      {
        logRef: 'dangling-log',
        timestamp: 2n,
        spanID: 'missing-span',
        severityText: '',
        severityNumber: 0,
        serviceName: '',
        eventName: '',
        bodyPreview: '',
      } satisfies TraceLogSummary,
    ])
    setTestUrl('/traces/trace-1?span=root&log=dangling-log')

    renderWithContexts(TracesPage)

    await waitFor(() => expect(window.location.search).toBe('?span=root'))
  })

  it.each(['1junk', '1.5', '-1', '9007199254740992', ''])(
    'removes malformed event %j without clearing a valid log selection',
    async malformedEvent => {
      searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
      getStats.mockResolvedValue(makeStats())
      getTraceView.mockResolvedValue(makeTraceData(0))
      getTraceLogSummaries.mockResolvedValue([
        {
          logRef: 'log-1',
          timestamp: 2n,
          spanID: 'root',
          severityText: '',
          severityNumber: 0,
          serviceName: '',
          eventName: '',
          bodyPreview: '',
        } satisfies TraceLogSummary,
      ])
      setTestUrl(`/traces/trace-1?span=root&event=${malformedEvent}&log=log-1`)

      renderWithContexts(TracesPage)

      await waitFor(() =>
        expect(window.location.search).toBe('?span=root&log=log-1')
      )
    }
  )

  it('retains the displayed trace and its actions while the next trace loads', async () => {
    const nextDetail = deferred<TraceData>()
    searchTraceSummaries.mockResolvedValue([
      makeTraceSummary(),
      makeTraceSummary({
        traceID: 'trace-2',
        rootSpan: { name: 'Trace two', serviceName: 'orders-service' },
      }),
    ])
    getStats.mockResolvedValue(makeStats())
    getTraceView.mockImplementation((traceID: string) =>
      traceID === 'trace-1'
        ? Promise.resolve(makeTraceData(0))
        : nextDetail.promise
    )
    setTestUrl('/traces/trace-1')
    renderWithContexts(TracesPage)
    await waitFor(() =>
      expect(document.querySelector('tr[data-span-id="root"]')).not.toBeNull()
    )

    await fireEvent.click(
      await screen.findByRole('button', { name: /^Trace two\b/ })
    )

    await waitFor(() =>
      expect(screen.getByText('Loading selected trace…')).toBeVisible()
    )
    expect(document.querySelector('tr[data-span-id="root"]')).not.toBeNull()
    expect(
      screen.queryByRole('button', { name: 'Delete this trace' })
    ).toBeInTheDocument()

    nextDetail.resolve(makeTraceData(0, 'trace-2', 'next-root'))
    await waitFor(() =>
      expect(
        document.querySelector('tr[data-span-id="next-root"]')
      ).not.toBeNull()
    )
  })

  it('ignores a pending trace success after switching traces', async () => {
    const firstDetail = deferred<TraceData>()
    const nextDetail = deferred<TraceData>()
    searchTraceSummaries.mockResolvedValue([
      makeTraceSummary(),
      makeTraceSummary({
        traceID: 'trace-2',
        rootSpan: { name: 'Trace two', serviceName: 'orders-service' },
      }),
    ])
    getStats.mockResolvedValue(makeStats())
    getTraceView.mockImplementation((traceID: string) =>
      traceID === 'trace-1' ? firstDetail.promise : nextDetail.promise
    )
    setTestUrl('/traces/trace-1?span=old-span')
    renderWithContexts(TracesPage)
    await waitFor(() => expect(getTraceView).toHaveBeenCalledTimes(1))

    await fireEvent.click(
      await screen.findByRole('button', { name: /^Trace two\b/ })
    )
    await waitFor(() => expect(getTraceView).toHaveBeenCalledTimes(2))
    firstDetail.resolve(makeTraceData(0, 'trace-1', 'old-span'))

    await waitFor(() =>
      expect(screen.getByText('Loading selected trace…')).toBeVisible()
    )
    expect(window.location.pathname).toBe('/traces/trace-2')
    expect(window.location.search).toBe('')
    expect(document.querySelector('tr[data-span-id="old-span"]')).toBeNull()

    nextDetail.resolve(makeTraceData(0, 'trace-2', 'next-span'))
    await waitFor(() => expect(window.location.search).toBe('?span=next-span'))
  })

  it.each(['success', 'failure'] as const)(
    'ignores pending detail %s after all traces are deleted',
    async outcome => {
      const confirm = vi.spyOn(window, 'confirm').mockReturnValue(true)
      const pendingDetail = deferred<TraceData>()
      let detailSignal: AbortSignal | undefined
      searchTraceSummaries
        .mockResolvedValueOnce([makeTraceSummary()])
        .mockResolvedValueOnce([])
      getStats.mockResolvedValue(makeStats())
      getTraceView.mockImplementation(
        (
          _traceID: string,
          _queryTree: QueryNode | undefined,
          signal: AbortSignal
        ) => {
          detailSignal = signal
          return pendingDetail.promise
        }
      )
      const consoleError = vi
        .spyOn(console, 'error')
        .mockImplementation(() => {})
      setTestUrl('/traces/trace-1?span=old-span')
      renderWithContexts(TracesPage)
      await waitFor(() => expect(detailSignal).toBeDefined())

      await fireEvent.click(
        screen.getByRole('button', { name: 'Delete all traces' })
      )
      await waitFor(() => expect(window.location.pathname).toBe('/traces'))
      expect(detailSignal?.aborted).toBe(true)

      if (outcome === 'success') {
        pendingDetail.resolve(makeTraceData(0, 'trace-1', 'old-span'))
      } else {
        pendingDetail.reject(new Error('late failure'))
      }
      await new Promise(resolve => setTimeout(resolve))

      expect(window.location.search).toBe('')
      expect(document.querySelector('tr[data-span-id="old-span"]')).toBeNull()
      expect(consoleError).not.toHaveBeenCalled()
      consoleError.mockRestore()
      confirm.mockRestore()
    }
  )

  it('silently aborts an outstanding detail request when unmounted', async () => {
    searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())

    let detailSignal: AbortSignal | undefined
    const abortError = new DOMException('', 'AbortError')
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    getTraceView.mockImplementation(
      (
        _traceID: string,
        _queryTree: QueryNode | undefined,
        signal: AbortSignal
      ) =>
        new Promise((_resolve, reject) => {
          detailSignal = signal
          signal.addEventListener('abort', () => reject(abortError))
        })
    )

    setTestUrl('/traces/trace-1?span=span-1')
    const page = renderWithContexts(TracesPage)
    await waitFor(() => expect(detailSignal).toBeDefined())
    expect(window.location.search).toBe('?span=span-1')

    page.unmount()

    await new Promise(resolve => setTimeout(resolve, 0))
    expect(detailSignal?.aborted).toBe(true)
    expect(consoleError).not.toHaveBeenCalled()
    expect(window.location.search).toBe('?span=span-1')
    consoleError.mockRestore()
  })

  it('keeps logs independent of waterfall failure and cancels them when no displayed owner remains', async () => {
    const pendingLogs = deferred<TraceLogSummary[]>()
    const nextDetail = deferred<TraceData>()
    getSpan.mockReturnValue(deferred<SpanData>().promise)
    const error = new Error('spans failed')
    let spansSignal: AbortSignal | undefined
    let logsSignal: AbortSignal | undefined
    searchTraceSummaries.mockResolvedValue([
      makeTraceSummary(),
      makeTraceSummary({
        traceID: 'trace-2',
        rootSpan: { name: 'Trace two', serviceName: 'orders-service' },
      }),
    ])
    getStats.mockResolvedValue(makeStats())
    getTraceView.mockImplementation(
      (
        traceID: string,
        _queryTree: QueryNode | undefined,
        signal: AbortSignal
      ) => {
        if (traceID === 'trace-2') return nextDetail.promise
        spansSignal = signal
        return Promise.reject(error)
      }
    )
    getTraceLogSummaries.mockImplementation(
      (traceID: string, signal: AbortSignal) => {
        if (traceID === 'trace-2') return Promise.resolve([])
        logsSignal = signal
        return pendingLogs.promise
      }
    )
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    setTestUrl('/traces/trace-1?span=old-span')
    renderWithContexts(TracesPage)

    await waitFor(() =>
      expect(consoleError).toHaveBeenCalledWith(
        'Failed to fetch trace detail:',
        error
      )
    )
    expect(spansSignal).not.toBe(logsSignal)
    expect(spansSignal?.aborted).toBe(true)
    expect(logsSignal?.aborted).toBe(false)
    expect(window.location.search).toBe('?span=old-span')
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Could not load the selected trace.'
    )
    expect(screen.queryByRole('table', { name: 'Span fields' })).toBeNull()

    await fireEvent.click(
      await screen.findByRole('button', { name: /^Trace two\b/ })
    )
    await waitFor(() =>
      expect(screen.getByText('Loading selected trace…')).toBeVisible()
    )
    expect(logsSignal?.aborted).toBe(true)

    pendingLogs.reject(new Error('late sibling failure'))
    await new Promise(resolve => setTimeout(resolve))

    expect(screen.getByText('Loading selected trace…')).toBeVisible()
    expect(document.querySelector('tr[data-span-id="old-span"]')).toBeNull()
    expect(consoleError).toHaveBeenCalledTimes(1)

    nextDetail.resolve(makeTraceData(0, 'trace-2', 'next-span'))
    await waitFor(() =>
      expect(
        document.querySelector('tr[data-span-id="next-span"]')
      ).not.toBeNull()
    )
    consoleError.mockRestore()
  })

  it('publishes pending correlated logs after waterfall failure when the exact span is available', async () => {
    const tree = deferred<TraceData>()
    const logs = deferred<TraceLogSummary[]>()
    searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    getTraceView.mockReturnValue(tree.promise)
    getTraceLogSummaries.mockReturnValue(logs.promise)
    getSpan.mockResolvedValue(makeSpanNode('root').spanData)
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    setTestUrl('/traces/trace-1?span=root')
    renderWithContexts(TracesPage)
    const fields = await screen.findByRole('table', { name: 'Span fields' })
    tree.reject(new Error('tree unavailable'))
    await screen.findByText('Could not load the selected trace.')
    const signal = getTraceLogSummaries.mock.calls[0]![1] as AbortSignal
    expect(signal.aborted).toBe(false)
    expect(await screen.findByText('Loading correlated logs…')).toBeVisible()
    logs.resolve([makeTraceLog({ bodyPreview: 'available activity' })])
    await screen.findByText('available activity')
    expect(screen.getByText('Events').closest('summary')).toHaveTextContent(
      '[1]'
    )
    expect(screen.queryByText('Loading correlated logs…')).toBeNull()
    expect(screen.getByRole('table', { name: 'Span fields' })).toBe(fields)
    consoleError.mockRestore()
  })

  it('retries a log failure without refetching the successful exact span or failed waterfall', async () => {
    searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    getTraceView.mockRejectedValue(new Error('tree unavailable'))
    getSpan.mockResolvedValue(makeSpanNode('root').spanData)
    getTraceLogSummaries
      .mockRejectedValueOnce(new Error('logs unavailable'))
      .mockResolvedValueOnce([
        makeTraceLog({ bodyPreview: 'retried activity' }),
      ])
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    setTestUrl('/traces/trace-1?span=root')
    renderWithContexts(TracesPage)
    const fields = await screen.findByRole('table', { name: 'Span fields' })
    await screen.findByText('Could not load correlated logs.')
    expect(screen.getByText('Events').closest('summary')).not.toHaveTextContent(
      '[0]'
    )
    await fireEvent.click(
      screen.getByRole('button', { name: 'Retry correlated logs' })
    )
    await screen.findByText('retried activity')
    expect(screen.queryByText('Could not load correlated logs.')).toBeNull()
    expect(getSpan).toHaveBeenCalledTimes(1)
    expect(getTraceView).toHaveBeenCalledTimes(1)
    expect(screen.getByRole('table', { name: 'Span fields' })).toBe(fields)
    consoleError.mockRestore()
  })

  it('publishes the exact span and waterfall while correlated logs are pending, then adds logs', async () => {
    const logs = deferred<TraceLogSummary[]>()
    const tree = deferred<TraceData>()
    searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    getTraceView.mockReturnValue(tree.promise)
    getTraceLogSummaries.mockReturnValue(logs.promise)
    getSpan.mockResolvedValue(makeSpanNode('root').spanData)
    setTestUrl('/traces/trace-1?span=root')
    renderWithContexts(TracesPage)
    const fields = await screen.findByRole('table', { name: 'Span fields' })
    expect(fields).toHaveTextContent('root')
    const events = screen.getByText('Events').closest('summary')!
    expect(events).not.toHaveTextContent('[0]')
    expect(events).not.toHaveAttribute('aria-disabled', 'true')
    expect(screen.queryByText('No events recorded for this span')).toBeNull()
    expect(await screen.findByText('Loading correlated logs…')).toBeVisible()
    tree.resolve(makeTraceData(0))
    await waitFor(() =>
      expect(document.querySelector('tr[data-span-id="root"]')).not.toBeNull()
    )
    expect(screen.getByRole('table', { name: 'Span fields' })).toBe(fields)
    logs.resolve([makeTraceLog({ bodyPreview: 'correlated activity' })])
    await waitFor(() => expect(events).toHaveTextContent('[1]'))
    expect(
      within(events.closest('details')!).getByText('correlated activity')
    ).toBeInTheDocument()
    expect(screen.queryByText('Loading correlated logs…')).toBeNull()
  })

  it('does not discard available span data or confirm zero events when the log request fails', async () => {
    searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    getTraceView.mockResolvedValue(makeTraceData(0))
    getTraceLogSummaries.mockRejectedValue(new Error('logs failed'))
    setTestUrl('/traces/trace-1?span=root')
    renderWithContexts(TracesPage)
    await screen.findByRole('table', { name: 'Span fields' })
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Could not load correlated logs.'
    )
    expect(document.querySelector('tr[data-span-id="root"]')).not.toBeNull()
    expect(screen.getByText('Events').closest('summary')).not.toHaveTextContent(
      '[0]'
    )
    expect(screen.queryByText('No events recorded for this span')).toBeNull()
  })

  it('retains received activity and section availability during a same-trace log refresh', async () => {
    const replacement = deferred<TraceLogSummary[]>()
    getTraceLogSummaries
      .mockResolvedValueOnce([makeTraceLog({ bodyPreview: 'old activity' })])
      .mockReturnValueOnce(replacement.promise)
    await renderSelectedTrace(0)
    const events = screen.getByText('Events').closest('summary')!
    await waitFor(() => expect(events).toHaveTextContent('[1]'))
    const section = events.closest('details')!
    const activity = within(section).getByText('old activity')
    const spanFields = screen.getByRole('table', { name: 'Span fields' })
    await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() => expect(getTraceLogSummaries).toHaveBeenCalledTimes(2))
    expect(events).toHaveTextContent('[1]')
    expect(events).not.toHaveAttribute('aria-disabled', 'true')
    expect(within(section).getByText('old activity')).toBe(activity)
    expect(await screen.findByText('Loading correlated logs…')).toBeVisible()
    replacement.resolve([makeTraceLog({ bodyPreview: 'new activity' })])
    await within(section).findByText('new activity')
    expect(screen.getByRole('table', { name: 'Span fields' })).toBe(spanFields)
    expect(within(section).queryByText('old activity')).toBeNull()
    expect(screen.queryByText('Loading correlated logs…')).toBeNull()
  })

  it('rejects old correlated logs after A→B→A', async () => {
    const old = deferred<TraceLogSummary[]>()
    const latest = deferred<TraceLogSummary[]>()
    searchTraceSummaries.mockResolvedValue([
      makeTraceSummary(),
      makeTraceSummary({ traceID: 'trace-2' }),
    ])
    getStats.mockResolvedValue(makeStats())
    getTraceView.mockImplementation(async (traceID: string) =>
      makeTraceData(0, traceID)
    )
    getSpan.mockImplementation(
      async (traceID: string, spanID: string) =>
        makeSpanNode(spanID, traceID).spanData
    )
    getTraceLogSummaries
      .mockReturnValueOnce(old.promise)
      .mockResolvedValueOnce([])
      .mockReturnValueOnce(latest.promise)
    setTestUrl('/traces/trace-1?span=root')
    renderWithContexts(TracesPage)
    await waitFor(() => expect(getTraceLogSummaries).toHaveBeenCalledTimes(1))
    const signal = getTraceLogSummaries.mock.calls[0]![1] as AbortSignal
    navigate('/traces/trace-2?span=root')
    await waitFor(() => expect(getTraceLogSummaries).toHaveBeenCalledTimes(2))
    navigate('/traces/trace-1?span=root')
    await waitFor(() => expect(getTraceLogSummaries).toHaveBeenCalledTimes(3))
    expect(signal.aborted).toBe(true)
    old.resolve([makeTraceLog({ bodyPreview: 'stale activity' })])
    expect(await screen.findByText('Loading correlated logs…')).toBeVisible()
    expect(screen.queryByText('stale activity')).toBeNull()
    latest.resolve([makeTraceLog({ bodyPreview: 'current activity' })])
    await waitFor(() =>
      expect(screen.getAllByText('current activity').length).toBeGreaterThan(0)
    )
    expect(screen.queryByText('stale activity')).toBeNull()
  })

  it('reports ordinary detail request failures', async () => {
    searchTraceSummaries.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    const error = new Error('detail failed')
    getTraceView.mockRejectedValue(error)
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})

    setTestUrl('/traces/trace-1?span=span-1')
    renderWithContexts(TracesPage)

    await waitFor(() =>
      expect(consoleError).toHaveBeenCalledWith(
        'Failed to fetch trace detail:',
        error
      )
    )
    expect(window.location.search).toBe('?span=span-1')
    expect(screen.getByRole('alert')).toHaveTextContent(
      'Could not load the selected trace.'
    )
    consoleError.mockRestore()
  })
})
