// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/svelte'
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
  it('keeps the displayed summary and link through pending and failed replacements', async () => {
    getTraceSummary.mockResolvedValueOnce(summary)
    const view = renderWithContexts(AssociatedTrace, {
      traceID: summary.traceID,
      spanID: null,
    })
    const title = await screen.findByText('POST /checkout')
    const link = screen.getByRole('link')
    let reject!: (reason: Error) => void
    getTraceSummary.mockImplementationOnce(
      () =>
        new Promise((_resolve, fail) => {
          reject = fail
        })
    )
    await view.rerender({
      componentProps: { traceID: 'replacement', spanID: null },
    })
    expect(screen.getByText('POST /checkout')).toBe(title)
    expect(link).toHaveAttribute(
      'href',
      `/traces/${summary.traceID}?start=1&end=2`
    )
    expect(screen.queryByText('Loading trace context…')).toBeNull()
    await screen.findByText('Loading trace context…')
    reject(new Error('offline'))
    await screen.findByRole('alert')
    expect(screen.getByText('POST /checkout')).toBe(title)
    getTraceSummary.mockResolvedValueOnce({
      ...summary,
      traceID: 'replacement',
      rootSpan: { name: 'Replacement', serviceName: 'worker' },
    })
    await fireEvent.click(screen.getByRole('button', { name: 'Retry' }))
    await screen.findByText('Replacement')
    expect(screen.getByRole('link')).toBe(link)
    expect(link).toHaveAttribute('href', '/traces/replacement?start=1&end=2')
  })
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
    const link = screen.getByRole('link')
    await screen.findByText('INSERT order')
    const context = screen.getByRole('region', { name: 'Trace context' })
    expect(screen.getAllByRole('link')).toHaveLength(1)
    expect(screen.getByRole('link')).toBe(link)
    expect(context.lastElementChild).toBe(link)
    expect(screen.getByRole('heading', { name: 'Trace context' })).toHaveClass(
      'text-sm',
      'font-normal',
      'text-subtle'
    )
    expect(link.querySelector('[data-field="traceId"]')).toHaveTextContent(
      summary.traceID
    )
    expect(link.querySelector('[data-field="spanId"]')).toHaveTextContent(
      '0000000000000001'
    )
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

  it('shows compact missing-span IDs in one exact-span link without losing the time range', async () => {
    getTraceSummary.mockResolvedValue(summary)
    renderWithContexts(AssociatedTrace, {
      traceID: summary.traceID,
      spanID: '0000000000000001',
    })
    expect(await screen.findByText('(unavailable)')).toBeVisible()
    expect(screen.getByText('POST /checkout')).toBeVisible()
    expect(screen.getByText('Trace summary')).toBeVisible()
    expect(screen.getByLabelText('Trace summary')).toHaveTextContent(
      'Duration: 1.000 s'
    )
    expect(screen.queryByText(/Span details unavailable/)).toBeNull()
    expect(screen.getAllByTitle(summary.traceID)).toHaveLength(1)
    expect(screen.getAllByTitle('0000000000000001')).toHaveLength(1)
    const spanLink = screen.getByRole('link', { name: /^Open span/ })
    expect(screen.getAllByRole('link')).toHaveLength(1)
    expect(spanLink.querySelector('a')).toBeNull()
    expect(spanLink).toHaveAccessibleName(
      `Open span 0000000000000001 in trace ${summary.traceID} (unavailable)`
    )
    expect(spanLink.querySelector('[data-field="traceId"]')).toHaveTextContent(
      summary.traceID
    )
    expect(spanLink.querySelector('[data-field="spanId"]')).toHaveTextContent(
      '0000000000000001'
    )
    expect(spanLink).toHaveAttribute(
      'href',
      `/traces/${summary.traceID}?start=1&end=2&span=0000000000000001`
    )
    expect(getTraceSummary).toHaveBeenCalledWith(
      summary.traceID,
      expect.any(AbortSignal)
    )
    await userEvent.click(spanLink)
    expect(window.location.pathname).toBe(`/traces/${summary.traceID}`)
    const query = new URLSearchParams(window.location.search)
    expect(query.get('span')).toBe('0000000000000001')
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
    expect(screen.queryByText('(unavailable)')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
    expect(await screen.findByText('(unavailable)')).toBeInTheDocument()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(
      screen.getByRole('link', {
        name: `Open trace ${summary.traceID} (unavailable)`,
      })
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
    await screen.findByText('(unavailable)')
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

  it('keeps failed span lookups distinct from absence and updates in place after retry', async () => {
    getSpanTimings
      .mockRejectedValueOnce(new Error('offline'))
      .mockResolvedValueOnce([
        {
          traceID: summary.traceID,
          spanID: '0000000000000001',
          name: 'Arrived span',
          serviceName: 'worker',
          startTime: summary.startTime,
          endTime: summary.startTime + 10n,
          statusCodeValue: 0,
        } satisfies SpanTiming,
      ])
    getTraceSummary.mockResolvedValue(summary)
    renderWithContexts(AssociatedTrace, {
      traceID: summary.traceID,
      spanID: '0000000000000001',
    })
    const region = screen.getByRole('region', { name: 'Trace context' })
    expect(screen.queryByText('(unavailable)')).toBeNull()
    expect(await screen.findByRole('alert')).toHaveTextContent(
      'Unable to load the associated span.'
    )
    expect(screen.queryByText('(unavailable)')).toBeNull()
    expect(screen.queryByText('POST /checkout')).toBeNull()
    await userEvent.click(screen.getByRole('button', { name: 'Retry' }))
    await screen.findByText('Arrived span')
    expect(screen.getByRole('region', { name: 'Trace context' })).toBe(region)
    expect(screen.queryByRole('alert')).toBeNull()
    expect(screen.queryByText('(unavailable)')).toBeNull()
  })

  it('updates a compact missing reference in place when correlation props change', async () => {
    getTraceSummary.mockResolvedValue(null)
    const view = renderWithContexts(AssociatedTrace, {
      traceID: summary.traceID,
      spanID: null,
    })
    const region = screen.getByRole('region', { name: 'Trace context' })
    await screen.findByText('(unavailable)')
    const link = screen.getByRole('link')
    getTraceSummary.mockResolvedValue({ ...summary, traceID: 'arrived-trace' })
    await view.rerender({
      componentProps: { traceID: 'arrived-trace', spanID: null },
    })
    await screen.findByText('POST /checkout')
    expect(screen.getByRole('region', { name: 'Trace context' })).toBe(region)
    expect(screen.getByRole('link')).toBe(link)
    expect(region.lastElementChild).toBe(link)
    expect(link).toHaveAccessibleName('Open trace arrived-trace')
    expect(link).toHaveAttribute('href', '/traces/arrived-trace?start=1&end=2')
    expect(link.querySelector('[data-field="spanId"]')).toBeNull()
    expect(screen.queryByText('(unavailable)')).toBeNull()
    expect(screen.queryByTitle(summary.traceID)).toBeNull()
  })

  it('retains exact received IDs and native modified-click behavior in the combined link', async () => {
    getTraceSummary.mockResolvedValue(null)
    const traceID = 'A'.repeat(32)
    const spanID = 'B'.repeat(16)
    renderWithContexts(AssociatedTrace, { traceID, spanID })
    const link = screen.getByRole('link')
    expect(link).toHaveAccessibleName(`Open span ${spanID} in trace ${traceID}`)
    expect(link).toHaveAttribute(
      'href',
      `/traces/${traceID}?start=1&end=2&span=${spanID}`
    )
    const originalURL = window.location.href
    expect(await fireEvent.click(link, { ctrlKey: true })).toBe(true)
    expect(window.location.href).toBe(originalURL)
    await screen.findByText('(unavailable)')
    expect(screen.getByRole('link')).toBe(link)
    expect(screen.getByTitle(traceID).closest('a')).toBe(link)
    expect(screen.getByTitle(spanID).closest('a')).toBe(link)
    expect(await fireEvent.click(link, { metaKey: true })).toBe(true)
    expect(window.location.href).toBe(originalURL)
  })

  it('retains the original missing span target when the trace has no recorded root', async () => {
    getTraceSummary.mockResolvedValue({
      ...summary,
      hasRootSpan: false,
      rootSpan: undefined,
    })
    renderWithContexts(AssociatedTrace, {
      traceID: summary.traceID,
      spanID: 'missing-span',
    })
    await screen.findByText('(unavailable)')
    expect(screen.getByText('No root span')).toBeVisible()
    expect(screen.getByText('Trace summary')).toBeVisible()
    expect(screen.queryByText('POST /checkout')).toBeNull()
    expect(screen.getByRole('link')).toHaveAttribute(
      'href',
      `/traces/${summary.traceID}?start=1&end=2&span=missing-span`
    )
  })
})
