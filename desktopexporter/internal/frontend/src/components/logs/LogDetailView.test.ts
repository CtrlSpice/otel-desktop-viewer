// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { tick } from 'svelte'
import { screen, within } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import LogDetailView from './LogDetailView.svelte'
import type { LogData } from '@/types/api-types'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

vi.mock('@/services/telemetry-service', async importOriginal => {
  const actual =
    await importOriginal<typeof import('@/services/telemetry-service')>()
  return {
    ...actual,
    telemetryAPI: {
      ...actual.telemetryAPI,
      getTraceSummary: vi.fn().mockResolvedValue(null),
    },
  }
})

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
  it.each([
    [1, 'neutral'],
    [5, 'success'],
    [9, 'info'],
    [13, 'warning'],
    [17, 'error'],
    [21, 'error'],
  ] as const)(
    'colours both received severity values for level %s',
    (severityNumber, colour) => {
      const { container } = renderLog(
        makeLog({
          traceID: null,
          spanID: null,
          severityNumber,
          severityText: 'received custom label',
        })
      )
      for (const name of ['severityText', 'severityNumber']) {
        const row = container.querySelector<HTMLElement>(
          `[data-field="${name}"]`
        )!
        expect(
          row.style.getPropertyValue('--detail-property-value-color')
        ).toBe(`var(--color-${colour})`)
      }
      expect(
        container.querySelector('[data-field="severityText"]')
      ).toHaveTextContent('received custom label')
    }
  )

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
    const button = within(header).getByRole('button', { name: 'Download log' })
    expect(
      within(button.closest<HTMLElement>('.pane-header__right')!)
        .getAllByRole('button')
        .at(-1)
    ).toBe(button)
    expect(button).toBeEnabled()
    expect(button).toHaveClass('header-icon-button')
    expect(button).toHaveTextContent('')
    const tabs = within(header).getByRole('tablist', { name: 'Detail view' })
    expect(within(header).queryByLabelText('Log timestamp')).toBeNull()
    expect(tabs.closest('.pane-header__top')).not.toBeNull()
    expect(header.querySelector('.pane-header__meta-row')).toBeNull()
    expect(header.querySelector('.badge')).toBeNull()
    expect(
      within(tabs).getByRole('tab', { name: 'Properties' })
    ).toHaveAttribute('aria-selected', 'true')
    expect(within(tabs).getByRole('tab', { name: 'JSON' })).toBeInTheDocument()
    const title = within(header).getByText('checkout', { exact: true })
    expect(
      title.compareDocumentPosition(button) & Node.DOCUMENT_POSITION_FOLLOWING
    ).not.toBe(0)
    expect(
      screen.getAllByRole('button', { name: 'Download log' })
    ).toHaveLength(1)
  })

  it('pins the shared associated trace above the scrolling details instead of a trace ID field', () => {
    const { container } = renderLog(makeLog())
    const headings = container.querySelectorAll(
      'summary.detail-bubble__heading'
    )
    expect(headings).toHaveLength(0)
    const fields = screen.getByRole('table', { name: 'Log fields' })
    const trace = screen.getByRole('region', { name: 'Associated trace' })
    expect(trace.closest('.log-detail-panel__scroll')).toBeNull()
    expect(fields.querySelector('[data-field="traceId"]')).toBeNull()
    expect(
      trace.compareDocumentPosition(fields) & Node.DOCUMENT_POSITION_FOLLOWING
    ).not.toBe(0)
    expect(trace.closest('details')).toBeNull()
    expect(
      screen.getByRole('table', { name: 'Log fields' }).closest('details')
    ).toBeNull()
    expect(container.querySelector('.waterfall-view')).toBeNull()
  })

  it('omits the associated trace when the log has no trace ID', () => {
    renderLog(makeLog({ traceID: null, spanID: null }))
    expect(screen.queryByRole('region', { name: 'Trace' })).toBeNull()
    expect(
      screen.queryByRole('region', { name: 'Associated trace' })
    ).toBeNull()
  })

  it('keeps the span field linked to the exact correlated span', () => {
    renderLog(makeLog())
    const spanLink = screen.getByRole('link', { name: 'span-xyz' })
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
