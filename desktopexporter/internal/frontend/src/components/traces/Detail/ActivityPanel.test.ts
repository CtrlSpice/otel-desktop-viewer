// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/svelte'
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
    id: 'log-1',
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
  it('keeps separate event and log groups and shows useful log summary fields', async () => {
    Object.defineProperty(Element.prototype, 'scrollIntoView', {
      configurable: true,
      value: vi.fn(),
    })
    setTestUrl('/traces/trace-1?span=span-1&log=log-1&start=1&end=2')
    renderWithContexts(ActivityPanel, {
      events,
      logs,
      spanStartTime: 100n,
      selectedLogID: 'log-1',
    })

    expect(screen.getByText('Events').closest('summary')).toHaveTextContent('1')
    expect(screen.getByText('Logs').closest('summary')).toHaveTextContent('1')
    expect(
      screen.getByRole('table', { name: 'Log summary' })
    ).toHaveTextContent(
      /timestamp.*offset.*severity.*service.*event name.*body preview/s
    )
    expect(screen.getByText('preview only')).toBeVisible()

    const link = screen.getByRole('link', { name: 'Open log' })
    expect(link).toHaveAttribute('href', '/logs/log-1?start=1&end=2')
    await userEvent.click(link)
    expect(window.location.pathname).toBe('/logs/log-1')
  })

  it('formats negative event and log offsets and keeps received severity fields independent', () => {
    const negativeEvents: EventData[] = [{ ...events[0]!, timestamp: 0n }]
    const negativeLogs: TraceLogSummary[] = [
      {
        ...logs[0]!,
        timestamp: 0n,
        severityText: '',
        severityNumber: 0,
      },
    ]
    renderWithContexts(ActivityPanel, {
      events: negativeEvents,
      logs: negativeLogs,
      spanStartTime: 1_000_000_000n,
    })

    const badges = screen.getAllByText('-1.000 s')
    expect(badges).toHaveLength(3)
    const summary = screen.getByRole('table', { name: 'Log summary' })
    expect(summary).toHaveTextContent(/severity number.*0/s)
    expect(summary).not.toHaveTextContent('severity text')
    expect(summary).not.toHaveTextContent('TRACE')
  })
})
