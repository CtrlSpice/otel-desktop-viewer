// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { screen, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import type { TraceSummary } from '@/types/api-types'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import AssociatedTrace from './AssociatedTrace.svelte'

const { getTraceSummary } = vi.hoisted(() => ({ getTraceSummary: vi.fn() }))
vi.mock('@/services/telemetry-service', () => ({
  telemetryAPI: { getTraceSummary },
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
  setTestUrl('/logs/selected?start=1&end=2&event=3')
})

describe('AssociatedTrace', () => {
  it('reuses the trace summary and opens its associated span without losing the time range', async () => {
    getTraceSummary.mockResolvedValue(summary)
    renderWithContexts(AssociatedTrace, {
      traceID: summary.traceID,
      spanID: '0000000000000001',
    })
    const card = await screen.findByRole('button', { name: /POST \/checkout/ })
    expect(card).toHaveClass('signal-row')
    expect(card).toHaveTextContent('api')
    expect(card).toHaveTextContent('5 spans')
    expect(card).toHaveTextContent('2 err')
    expect(getTraceSummary).toHaveBeenCalledWith(
      summary.traceID,
      expect.any(AbortSignal)
    )
    await userEvent.click(card)
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
    expect(
      await screen.findByRole('button', { name: /New trace/ })
    ).toBeInTheDocument()
    expect(oldSignal.aborted).toBe(true)
    resolveOld(summary)
    await waitFor(() =>
      expect(
        screen.queryByRole('button', { name: /POST \/checkout/ })
      ).toBeNull()
    )
  })
})
