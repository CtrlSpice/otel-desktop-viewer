// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { tick } from 'svelte'
import { screen, within } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import LogDetailView from './LogDetailView.svelte'
import type { LogData } from '@/types/api-types'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

function makeLog(overrides: Partial<LogData> = {}): LogData {
  return {
    logRef: 'log-1',
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
  it('offers scroll to top for a scrolled log without changing the route or log', async () => {
    const view = renderLog(makeLog())
    await tick()
    const viewport = view.container.querySelector<HTMLElement>(
      '.log-detail-panel__scroll'
    )!
    const scroll = vi.spyOn(viewport, 'scrollTo')
    const route = window.location.href
    expect(
      screen.queryByRole('button', { name: 'Scroll log to top' })
    ).not.toBeInTheDocument()
    viewport.scrollTop = 200
    viewport.dispatchEvent(new Event('scroll'))
    await tick()
    screen.getByRole('button', { name: 'Scroll log to top' }).click()
    expect(scroll).toHaveBeenCalledWith({ top: 0, behavior: 'smooth' })
    expect(window.location.href).toBe(route)
    expect(screen.getByRole('table', { name: 'Log fields' })).toHaveTextContent(
      'hello'
    )
    viewport.scrollTop = 0
    viewport.dispatchEvent(new Event('scroll'))
    await tick()
    expect(
      screen.queryByRole('button', { name: 'Scroll log to top' })
    ).not.toBeInTheDocument()
    scroll.mockRestore()
  })

  it('shows readable millisecond timestamps above unchanged raw nanoseconds', () => {
    const { container } = renderLog(
      makeLog({ timestamp: 1_700_000_000_123_456_789n })
    )
    const field = container.querySelector('[data-field="timeUnixNano"]')!
    expect(field.querySelector('.detail-property__value')!.textContent).toMatch(
      /\.123\s/
    )
    expect(
      field.querySelector('.detail-property__value')!.textContent
    ).not.toContain('.123456789')
    expect(
      field.querySelector('.detail-property__raw-timestamp')
    ).toHaveTextContent('1700000000123456789')
  })

  it('places the whole-record export in the main log header', () => {
    renderLog(makeLog())
    const header = screen.getByRole('region', { name: 'Log record' })
    const button = within(header).getByRole('button', { name: 'Export log' })
    expect(button).toBeEnabled()
    const title = within(header).getByText('checkout', { exact: true })
    expect(
      title.compareDocumentPosition(button) & Node.DOCUMENT_POSITION_FOLLOWING
    ).not.toBe(0)
    expect(screen.getAllByRole('button', { name: 'Export log' })).toHaveLength(
      1
    )
  })

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
