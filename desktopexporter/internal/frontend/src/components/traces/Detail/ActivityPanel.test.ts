// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { screen, within, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import ActivityPanel from './ActivityPanel.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import type { EventData, TraceLogSummary } from '@/types/api-types'

const events: EventData[] = [
  {
    name: 'cache.hit',
    timestamp: 110n,
    attributes: [],
    droppedAttributesCount: 0,
  },
]

const logs: TraceLogSummary[] = [
  {
    logRef: 'log-1',
    timestamp: 90n,
    spanID: 'span-1',
    severityText: 'WARN',
    severityNumber: 13,
    serviceName: 'checkout',
    eventName: 'order.delayed',
    bodyPreview: 'preview only',
  },
]

describe('ActivityPanel', () => {
  it('uses summary cards for events and logs without changing log navigation', async () => {
    Object.defineProperty(Element.prototype, 'scrollIntoView', {
      configurable: true,
      value: vi.fn(),
    })
    setTestUrl('/traces/trace-1?span=span-1&log=log-1&start=1&end=2')
    const { container } = renderWithContexts(ActivityPanel, {
      events,
      logs,
      spanStartTime: 100n,
      selectedLogRef: 'log-1',
    })

    expect(screen.getByText('Span event', { exact: true })).toBeVisible()
    expect(
      screen
        .getByRole('region', { name: 'Log summary' })
        .querySelector('.signal-row__title')
    ).toHaveTextContent(/^checkout · Log$/)
    expect(container.querySelectorAll('details, summary')).toHaveLength(0)
    expect(
      screen.getByRole('region', { name: 'Event cache.hit' }).parentElement
    ).toBe(screen.getByRole('region', { name: 'Log summary' }).parentElement)
    const summary = screen.getByRole('region', { name: 'Log summary' })
    const card = within(summary).getByRole('button')
    expect(card).toHaveClass('signal-row')
    expect(card).toHaveAttribute('aria-pressed', 'true')
    expect(card).toHaveTextContent('checkout')
    expect(card).toHaveTextContent('WARN (13)')
    expect(card.querySelector('.signal-row__badge')).toHaveTextContent(
      'WARN (13)'
    )
    expect(within(card).getByText('preview only')).toBeVisible()
    expect(within(summary).getByText('order.delayed')).toBeVisible()
    expect(summary.querySelector('table, [data-field]')).toBeNull()
    await waitFor(() =>
      expect(Element.prototype.scrollIntoView).toHaveBeenCalledWith({
        block: 'start',
      })
    )

    expect(screen.queryByRole('link', { name: 'Open log' })).toBeNull()
    await userEvent.click(card)
    expect(window.location.pathname).toBe('/logs/log-1')
  })

  it('keeps negative event and log offsets and uses the shared severity fallback', () => {
    const negativeEvents: EventData[] = [{ ...events[0]!, timestamp: 0n }]
    const negativeLogs: TraceLogSummary[] = [
      {
        ...logs[0]!,
        timestamp: 0n,
        severityText: '',
        severityNumber: 5,
      },
    ]
    renderWithContexts(ActivityPanel, {
      events: negativeEvents,
      logs: negativeLogs,
      spanStartTime: 1_000_000_000n,
    })

    expect(screen.getByText('-1.000 s')).toBeVisible()
    expect(screen.getAllByText('(-1.000 s)')).toHaveLength(1)
    const summary = screen.getByRole('region', { name: 'Log summary' })
    const offset = within(summary).getByText('(-1.000 s)', { exact: true })
    expect(offset).toHaveClass('text-rp-subtle')
    expect(offset).not.toHaveClass('badge-count')
    expect(offset.closest('.signal-date')).not.toBeNull()
    expect(within(summary).getByRole('button')).toContainElement(offset)
    expect(summary).toHaveTextContent('DEBUG (5)')
    expect(summary).not.toHaveTextContent('TRACE')
  })

  it.each(['click', 'keyboard'] as const)(
    'opens the associated log from its summary by %s and preserves the time range',
    async interaction => {
      setTestUrl('/traces/trace-1?span=span-1&log=log-1&start=1&end=2')
      renderWithContexts(ActivityPanel, {
        events: [],
        logs,
        spanStartTime: 100n,
      })
      const card = within(
        screen.getByRole('region', { name: 'Log summary' })
      ).getByRole('button')
      expect(card).toHaveAttribute('aria-pressed', 'false')
      if (interaction === 'click') await userEvent.click(card)
      else {
        card.focus()
        await userEvent.keyboard('{Enter}')
      }
      expect(window.location.pathname + window.location.search).toBe(
        '/logs/log-1?start=1&end=2'
      )
    }
  )
})
