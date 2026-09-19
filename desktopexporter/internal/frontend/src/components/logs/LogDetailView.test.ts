// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import LogDetailView from './LogDetailView.svelte'
import type { LogData } from '@/types/api-types'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

function makeLog(overrides: Partial<LogData> = {}): LogData {
  return {
    id: 'log-1',
    timestamp: 1_700_000_000_000_000_000n,
    observedTimestamp: 1_700_000_000_000_000_000n,
    traceID: 'trace-abc',
    spanID: 'span-xyz',
    severityText: 'INFO',
    severityNumber: 9,
    body: { kind: 'string', value: 'hello' },
    resource: {
      attributes: [
        { key: 'service.name', value: { kind: 'string', value: 'checkout' } },
      ],
      droppedAttributesCount: 0,
    },
    scope: {
      name: 'checkout',
      version: '1.0',
      attributes: [],
      droppedAttributesCount: 0,
    },
    attributes: [],
    droppedAttributesCount: 0,
    flags: 0,
    eventName: '',
    ...overrides,
  }
}

function renderLog(log: LogData) {
  setTestUrl('/logs/log-1?start=0&end=1&span=stale-span&event=3')
  return renderWithContexts(LogDetailView, { log })
}

describe('LogDetailView trace correlation', () => {
  it('links trace and span ids with span in the href', () => {
    renderLog(makeLog())
    const traceLink = screen.getByRole('link', { name: 'trace-abc' })
    const spanLink = screen.getByRole('link', { name: 'span-xyz' })
    expect(traceLink).toHaveAttribute(
      'href',
      '/traces/trace-abc?start=0&end=1&span=span-xyz'
    )
    expect(spanLink).toHaveAttribute(
      'href',
      '/traces/trace-abc?start=0&end=1&span=span-xyz'
    )
  })

  it('drops stale trace state when navigating to the correlated span', async () => {
    renderLog(makeLog())
    const historyLength = window.history.length

    await userEvent.click(screen.getByRole('link', { name: 'span-xyz' }))

    expect(window.location.pathname).toBe('/traces/trace-abc')
    expect(window.location.search).toBe('?start=0&end=1&span=span-xyz')
    expect(window.history.length).toBe(historyLength + 1)
  })

  it('renders span id as plain text when trace id is missing', () => {
    renderLog(makeLog({ traceID: null }))
    expect(screen.queryByRole('link', { name: 'span-xyz' })).toBeNull()
    expect(screen.getByText('span-xyz')).toBeInTheDocument()
  })
})
