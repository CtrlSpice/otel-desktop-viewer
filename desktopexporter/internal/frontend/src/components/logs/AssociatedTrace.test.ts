// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import type { SpanTiming, TraceSummary } from '@/types/api-types'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import AssociatedTrace from './AssociatedTrace.svelte'

const { getTraceSummary, getSpanTimings } = vi.hoisted(() => ({
  getTraceSummary: vi.fn(),
  getSpanTimings: vi.fn(),
}))
vi.mock('@/services/telemetry-service', () => ({
  telemetryAPI: { getTraceSummary, getSpanTimings },
}))

const summary: TraceSummary = {
  traceID: '00000000000000000000000000000011',
  hasRootSpan: true,
  rootSpan: { name: 'POST /checkout', serviceName: 'api' },
  startTime: 1700000000000000000n,
  durationNs: 1000000000n,
  spanCount: 5,
  errorCount: 2,
}

beforeEach(() => {
  getTraceSummary.mockReset()
  getSpanTimings.mockReset().mockResolvedValue([])
  setTestUrl('/logs/selected?start=1&end=2&event=3')
})

describe('AssociatedTrace', () => {
  it('summarises the linked span rather than the root or whole trace', async () => {
    getSpanTimings.mockResolvedValue([
      {
        traceID: summary.traceID,
        spanID: '0000000000000001',
        name: 'INSERT order',
        serviceName: 'database',
        startTime: summary.startTime + 100000000n,
        endTime: summary.startTime + 125000000n,
        statusCodeValue: 2,
      } satisfies SpanTiming,
    ])
    renderWithContexts(AssociatedTrace, {
      traceID: summary.traceID,
      spanID: '0000000000000001',
    })
    await screen.findByText('INSERT order')
    const context = screen.getByRole('region', { name: 'Trace context' })
    expect(context).toHaveTextContent('(database)')
    expect(context).not.toHaveTextContent('1 span')
    expect(context.querySelector('button')).toBeNull()
    expect(context).toHaveTextContent('Duration: 25.000 ms')
    expect(context).toHaveTextContent('20.100')
    expect(context).toHaveTextContent('0000000000000001')
    expect(context).toHaveTextContent(summary.traceID)
    expect(getTraceSummary).not.toHaveBeenCalled()
    expect(getSpanTimings).toHaveBeenCalledWith(
      [{ traceID: summary.traceID, spanID: '0000000000000001' }],
      expect.any(AbortSignal)
    )
    await userEvent.click(screen.getByRole('link', { name: /^Open span/ }))
    expect(window.location.pathname).toBe(`/traces/${summary.traceID}`)
    expect(new URLSearchParams(window.location.search).get('span')).toBe(
      '0000000000000001'
    )
  })

  it('labels the trace fallback and opens the trace independently without losing the time range', async () => {
    getTraceSummary.mockResolvedValue(summary)
    renderWithContexts(AssociatedTrace, {
      traceID: summary.traceID,
      spanID: '0000000000000001',
    })
    await screen.findByText('POST /checkout')
    expect(
      screen.getByText('Span details unavailable. Showing trace summary.')
    ).toBeVisible()
    const traceLink = screen.getByRole('link', { name: /^Open trace/ })
    const spanLink = screen.getByRole('link', { name: /^Open span/ })
    expect(traceLink).toHaveAttribute(
      'href',
      `/traces/${summary.traceID}?start=1&end=2`
    )
    expect(spanLink).toHaveAttribute(
      'href',
      `/traces/${summary.traceID}?start=1&end=2&span=0000000000000001`
    )
    expect(getTraceSummary).toHaveBeenCalledWith(
      summary.traceID,
      expect.any(AbortSignal)
    )
    await userEvent.click(traceLink)
    expect(window.location.pathname).toBe(`/traces/${summary.traceID}`)
    const query = new URLSearchParams(window.location.search)
    expect(query.has('span')).toBe(false)
    expect(query.get('start')).toBe('1')
    expect(query.get('end')).toBe('2')
    expect(query.has('event')).toBe(false)
  })

  it('distinguishes a missing stored trace from a failed request and allows retry', async () => {
    getTraceSummary
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce(null)
    renderWithContexts(AssociatedTrace, {
      traceID: summary.traceID,
      spanID: null,
    })
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Unable to load the associated trace.'
    )
    await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(
      await screen.findByText('Associated trace is not stored.')
    ).toBeInTheDocument()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(
      screen.getByRole('link', { name: `Open trace ${summary.traceID}` })
    ).toHaveAttribute(
      'href',
      expect.stringContaining(`/traces/${summary.traceID}`)
    )
    expect(screen.queryByRole('button', { name: /POST/ })).toBeNull()
  })

  it('discards stale requests when the selected log points to another trace', async () => {
    let resolveOld!: (trace: TraceSummary) => void
    getTraceSummary
      .mockImplementationOnce(
        () =>
          new Promise<TraceSummary>(resolve => {
            resolveOld = resolve
          })
      )
      .mockResolvedValueOnce({
        ...summary,
        traceID: '00000000000000000000000000000022',
        rootSpan: { name: 'New trace', serviceName: 'worker' },
      })
    const view = renderWithContexts(AssociatedTrace, {
      traceID: summary.traceID,
      spanID: null,
    })
    await waitFor(() => expect(getTraceSummary).toHaveBeenCalledTimes(1))
    const oldSignal = getTraceSummary.mock.calls[0]![1] as AbortSignal
    await view.rerender({
      componentProps: {
        traceID: '00000000000000000000000000000022',
        spanID: null,
      },
    })
    expect(await screen.findByText('New trace')).toBeInTheDocument()
    expect(oldSignal.aborted).toBe(true)
    resolveOld(summary)
    await waitFor(() => expect(screen.queryByText('POST /checkout')).toBeNull())
  })

  it('discards stale span results when the linked span changes within the same trace', async () => {
    let resolveOld!: (spans: SpanTiming[]) => void
    getSpanTimings
      .mockImplementationOnce(
        () =>
          new Promise<SpanTiming[]>(resolve => {
            resolveOld = resolve
          })
      )
      .mockResolvedValueOnce([])
    getTraceSummary.mockResolvedValue(summary)
    const view = renderWithContexts(AssociatedTrace, {
      traceID: summary.traceID,
      spanID: '0000000000000001',
    })
    await waitFor(() => expect(getSpanTimings).toHaveBeenCalledTimes(1))
    const oldSignal = getSpanTimings.mock.calls[0]![1] as AbortSignal
    await view.rerender({
      componentProps: {
        traceID: summary.traceID,
        spanID: '0000000000000002',
      },
    })
    await screen.findByText('POST /checkout')
    expect(oldSignal.aborted).toBe(true)
    resolveOld([
      {
        traceID: summary.traceID,
        spanID: '0000000000000001',
        name: 'Stale span',
        serviceName: 'old',
        startTime: summary.startTime,
        endTime: summary.startTime + 1n,
        statusCodeValue: 0,
      },
    ])
    await waitFor(() => expect(screen.queryByText('Stale span')).toBeNull())
    expect(getTraceSummary).toHaveBeenCalledTimes(1)
  })
})
