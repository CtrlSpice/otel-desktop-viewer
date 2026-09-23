// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/svelte'
import TracesPage from './TracesPage.svelte'
import type {
  TraceSummary,
  TraceData,
  Stats,
  SpanNode,
  TraceLogSummary,
} from '@/types/api-types'
import type { QueryNode } from '@/components/shared/Search/queryTree'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

// TracesPage is the only place the unplaced-span warning banner is rendered
// -- the singular/plural copy and the `unplacedSpanCount > 0` gate both live
// inline in its template, not in an extracted helper. Mounting the whole
// page (with telemetryAPI stubbed) is the cheapest seam that actually
// exercises that markup: pulling the copy logic out into a testable
// function would be a production change the task asked us not to make, and
// nothing smaller than the page owns this conditional.
//
// The heavy pieces underneath (DrawerSearchPanel's CodeMirror editor, the
// virtual list) already mount successfully in jsdom elsewhere in this suite
// without special stand-ins (see DrawerSearchPanel.test.ts and
// SignalListDrawer.test.ts), so nothing extra is mocked here beyond the
// three telemetryAPI calls TracesPage itself drives.

const {
  searchTraces,
  getStats,
  searchSpans,
  getTraceLogs,
  getTraceAttributes,
  clearTraces,
  deleteTraces,
} = vi.hoisted(() => ({
  searchTraces: vi.fn(),
  getStats: vi.fn(),
  searchSpans: vi.fn(),
  getTraceLogs: vi.fn(),
  getTraceAttributes: vi.fn(),
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
      searchTraces,
      getStats,
      searchSpans,
      getTraceLogs,
      getTraceAttributes,
      clearTraces,
      deleteTraces,
    },
  }
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
    id: 'log-1',
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
  searchTraces.mockResolvedValue([makeTraceSummary()])
  getStats.mockResolvedValue(makeStats())
  searchSpans.mockResolvedValue(makeTraceData(unplacedSpanCount))
  setTestUrl('/traces/trace-1')
  renderWithContexts(TracesPage)
  // Wait for the detail fetch to land -- the waterfall's root row is proof
  // traceData is populated and the banner conditional has had its chance to
  // run, without coupling the wait to the banner itself.
  await waitFor(() =>
    expect(document.querySelector('tr[data-span-id="root"]')).not.toBeNull()
  )
}

beforeEach(() => {
  searchTraces.mockReset()
  getStats.mockReset()
  searchSpans.mockReset()
  getTraceLogs.mockReset()
  getTraceLogs.mockResolvedValue([])
  getTraceAttributes.mockReset()
  getTraceAttributes.mockResolvedValue([])
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
  it('queries the list with null bounds for the default All selection', async () => {
    await renderSelectedTrace(0)
    expect(searchTraces).toHaveBeenCalledWith(null, null)
    expect(getTraceLogs).toHaveBeenCalledWith(
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

describe('TracesPage trace detail lifecycle', () => {
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
        id: 'log-1',
        timestamp: 2n,
        spanID: 'root',
        severityText: 'INFO',
        severityNumber: 9,
        serviceName: 'orders',
        eventName: '',
        bodyPreview: '',
      },
    ]
    searchTraces.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    searchSpans.mockResolvedValue(trace)
    getTraceLogs.mockResolvedValue(logs)
    setTestUrl('/traces/trace-1?span=root&log=log-1')

    renderWithContexts(TracesPage)

    await waitFor(() =>
      expect(screen.getByRole('tab', { name: /Activity.*2/ })).toHaveAttribute(
        'aria-selected',
        'true'
      )
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
    searchTraces.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    searchSpans.mockResolvedValue(trace)
    getTraceLogs.mockResolvedValue([makeTraceLog()])
    setTestUrl('/traces/trace-1?span=root&event=0&log=log-1')

    renderWithContexts(TracesPage)

    await waitFor(() =>
      expect(window.location.search).toBe('?span=root&event=0')
    )
  })

  it('preserves a resolved log when a competing numeric event is stale', async () => {
    searchTraces.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    searchSpans.mockResolvedValue(makeTraceData(0))
    getTraceLogs.mockResolvedValue([makeTraceLog()])
    setTestUrl('/traces/trace-1?span=root&event=99&log=log-1')

    renderWithContexts(TracesPage)

    await waitFor(() =>
      expect(window.location.search).toBe('?span=root&log=log-1')
    )
  })

  it('removes both competing selectors when neither resolves', async () => {
    searchTraces.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    searchSpans.mockResolvedValue(makeTraceData(0))
    getTraceLogs.mockResolvedValue([makeTraceLog({ spanID: 'other-span' })])
    setTestUrl('/traces/trace-1?span=root&event=99&log=log-1')

    renderWithContexts(TracesPage)

    await waitFor(() => expect(window.location.search).toBe('?span=root'))
  })

  it('removes a selected log that is not owned by the selected span', async () => {
    searchTraces.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    searchSpans.mockResolvedValue(makeTraceData(0))
    getTraceLogs.mockResolvedValue([
      {
        id: 'dangling-log',
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
      searchTraces.mockResolvedValue([makeTraceSummary()])
      getStats.mockResolvedValue(makeStats())
      searchSpans.mockResolvedValue(makeTraceData(0))
      getTraceLogs.mockResolvedValue([
        {
          id: 'log-1',
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

  it('hides old detail and its delete action while the next trace loads', async () => {
    const nextDetail = deferred<TraceData>()
    searchTraces.mockResolvedValue([
      makeTraceSummary(),
      makeTraceSummary({
        traceID: 'trace-2',
        rootSpan: { name: 'Trace two', serviceName: 'orders-service' },
      }),
    ])
    getStats.mockResolvedValue(makeStats())
    searchSpans.mockImplementation((traceID: string) =>
      traceID === 'trace-1'
        ? Promise.resolve(makeTraceData(0))
        : nextDetail.promise
    )
    setTestUrl('/traces/trace-1')
    renderWithContexts(TracesPage)
    await waitFor(() =>
      expect(document.querySelector('tr[data-span-id="root"]')).not.toBeNull()
    )

    await fireEvent.click(screen.getByText('trace-2').closest('button')!)

    await waitFor(() =>
      expect(screen.getByText('Loading trace detail…')).toBeVisible()
    )
    expect(document.querySelector('tr[data-span-id="root"]')).toBeNull()
    expect(
      screen.queryByRole('button', { name: 'Delete this trace' })
    ).not.toBeInTheDocument()

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
    searchTraces.mockResolvedValue([
      makeTraceSummary(),
      makeTraceSummary({ traceID: 'trace-2' }),
    ])
    getStats.mockResolvedValue(makeStats())
    searchSpans.mockImplementation((traceID: string) =>
      traceID === 'trace-1' ? firstDetail.promise : nextDetail.promise
    )
    setTestUrl('/traces/trace-1?span=old-span')
    renderWithContexts(TracesPage)
    await waitFor(() => expect(searchSpans).toHaveBeenCalledTimes(1))

    await fireEvent.click(screen.getByText('trace-2').closest('button')!)
    await waitFor(() => expect(searchSpans).toHaveBeenCalledTimes(2))
    firstDetail.resolve(makeTraceData(0, 'trace-1', 'old-span'))

    await waitFor(() =>
      expect(screen.getByText('Loading trace detail…')).toBeVisible()
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
      const pendingDetail = deferred<TraceData>()
      let detailSignal: AbortSignal | undefined
      searchTraces
        .mockResolvedValueOnce([makeTraceSummary()])
        .mockResolvedValueOnce([])
      getStats.mockResolvedValue(makeStats())
      searchSpans.mockImplementation(
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
    }
  )

  it('silently aborts an outstanding detail request when unmounted', async () => {
    searchTraces.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())

    let detailSignal: AbortSignal | undefined
    let resolveAbortHandled!: () => void
    const abortHandled = new Promise<void>(resolve => {
      resolveAbortHandled = resolve
    })
    const abortError = new DOMException('', 'AbortError')
    Object.defineProperty(abortError, 'name', {
      get() {
        // isAbortError reads this only from fetchTraceDetail's catch branch.
        resolveAbortHandled()
        return 'AbortError'
      },
    })
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})
    searchSpans.mockImplementation(
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

    await abortHandled
    expect(detailSignal?.aborted).toBe(true)
    expect(consoleError).not.toHaveBeenCalled()
    expect(window.location.search).toBe('?span=span-1')
    consoleError.mockRestore()
  })

  it.each([
    { failedBranch: 'spans', lateSibling: 'failure' },
    { failedBranch: 'logs', lateSibling: 'success' },
  ] as const)(
    'aborts the shared request when $failedBranch fails before a late sibling $lateSibling',
    async ({ failedBranch, lateSibling }) => {
      const pendingSpans = deferred<TraceData>()
      const pendingLogs = deferred<TraceLogSummary[]>()
      const nextDetail = deferred<TraceData>()
      const error = new Error(`${failedBranch} failed`)
      let spansSignal: AbortSignal | undefined
      let logsSignal: AbortSignal | undefined
      searchTraces.mockResolvedValue([
        makeTraceSummary(),
        makeTraceSummary({
          traceID: 'trace-2',
          rootSpan: { name: 'Trace two', serviceName: 'orders-service' },
        }),
      ])
      getStats.mockResolvedValue(makeStats())
      searchSpans.mockImplementation(
        (
          traceID: string,
          _queryTree: QueryNode | undefined,
          signal: AbortSignal
        ) => {
          if (traceID === 'trace-2') return nextDetail.promise
          spansSignal = signal
          return failedBranch === 'spans'
            ? Promise.reject(error)
            : pendingSpans.promise
        }
      )
      getTraceLogs.mockImplementation(
        (traceID: string, signal: AbortSignal) => {
          if (traceID === 'trace-2') return Promise.resolve([])
          logsSignal = signal
          return failedBranch === 'logs'
            ? Promise.reject(error)
            : pendingLogs.promise
        }
      )
      const consoleError = vi
        .spyOn(console, 'error')
        .mockImplementation(() => {})
      setTestUrl('/traces/trace-1?span=old-span')
      renderWithContexts(TracesPage)

      await waitFor(() =>
        expect(consoleError).toHaveBeenCalledWith(
          'Failed to fetch trace detail:',
          error
        )
      )
      expect(spansSignal).toBe(logsSignal)
      expect(spansSignal?.aborted).toBe(true)
      expect(window.location.search).toBe('')
      expect(screen.getByText('Select a trace to view details')).toBeVisible()

      await fireEvent.click(screen.getByText('trace-2').closest('button')!)
      await waitFor(() =>
        expect(screen.getByText('Loading trace detail…')).toBeVisible()
      )

      if (lateSibling === 'failure') {
        pendingLogs.reject(new Error('late sibling failure'))
      } else {
        pendingSpans.resolve(makeTraceData(0, 'trace-1', 'old-span'))
      }
      await new Promise(resolve => setTimeout(resolve))

      expect(screen.getByText('Loading trace detail…')).toBeVisible()
      expect(document.querySelector('tr[data-span-id="old-span"]')).toBeNull()
      expect(consoleError).toHaveBeenCalledTimes(1)

      nextDetail.resolve(makeTraceData(0, 'trace-2', 'next-span'))
      await waitFor(() =>
        expect(
          document.querySelector('tr[data-span-id="next-span"]')
        ).not.toBeNull()
      )
      consoleError.mockRestore()
    }
  )

  it('reports ordinary detail request failures', async () => {
    searchTraces.mockResolvedValue([makeTraceSummary()])
    getStats.mockResolvedValue(makeStats())
    const error = new Error('detail failed')
    searchSpans.mockRejectedValue(error)
    const consoleError = vi.spyOn(console, 'error').mockImplementation(() => {})

    setTestUrl('/traces/trace-1?span=span-1')
    renderWithContexts(TracesPage)

    await waitFor(() =>
      expect(consoleError).toHaveBeenCalledWith(
        'Failed to fetch trace detail:',
        error
      )
    )
    expect(window.location.search).toBe('')
    consoleError.mockRestore()
  })
})
