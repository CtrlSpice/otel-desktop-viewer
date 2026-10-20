// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { screen, waitFor, within } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'svelte'
import TraceCard from './TraceCard.svelte'
import type { TraceSummary } from '@/types/api-types'
import { formatDurationParts, formatTimestampParts } from '@/utils/time'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

function makeTrace(overrides: Partial<TraceSummary> = {}): TraceSummary {
  return {
    traceID: 'trace-1',
    hasRootSpan: true,
    rootSpan: { name: 'GET /api/orders', serviceName: 'orders-service' },
    startTime: 1_700_000_000_000_000_000n,
    durationNs: 1_500_000_000n,
    spanCount: 4,
    errorCount: 0,
    ...overrides,
  }
}

function renderCard(props: ComponentProps<typeof TraceCard>) {
  setTestUrl('/traces')
  return renderWithContexts(TraceCard, props)
}

describe('TraceCard', () => {
  it("renders the root span's name and service", () => {
    renderCard({ trace: makeTrace() })
    const button = screen.getByRole('button')
    expect(within(button).getByText('GET /api/orders')).toBeInTheDocument()
    expect(screen.getByText('orders-service')).toBeInTheDocument()
    expect(
      screen.queryByRole('group', { name: /Service breakdown/ })
    ).not.toBeInTheDocument()
    expect(within(button).getByText('4 spans')).toBeInTheDocument()
    expect(screen.queryByText('(4 spans)')).not.toBeInTheDocument()
  })

  it('falls back to a placeholder title when the trace has no root span', () => {
    renderCard({
      trace: makeTrace({ hasRootSpan: false, rootSpan: undefined }),
    })
    expect(screen.getByText('No root span')).toBeInTheDocument()
  })

  it('keeps the full trace ID alongside the service label instead of in a footer', () => {
    const traceID = '1234567890abcdef1234567890abcdef'
    renderCard({ trace: makeTrace({ traceID }) })
    const row = document.querySelector('.ball-pit')
    expect(row).toHaveTextContent('orders-service')
    expect(row?.querySelector('.identifier')).toHaveAttribute('title', traceID)
    expect(row).toHaveTextContent(traceID)
    expect(screen.getByRole('button').querySelector('.identifier')).toBeNull()
    expect(document.querySelector('.signal-row__meta-row')).toBeNull()
  })

  it('renders the formatted start time and duration', () => {
    const trace = makeTrace()
    renderCard({ trace })
    const startParts = formatTimestampParts(
      trace.startTime,
      'local',
      'milliseconds'
    )
    // SAFETY: makeTrace constructs this test fixture with a bigint durationNs value.
    const durationParts = formatDurationParts(trace.durationNs as bigint)
    expect(screen.getByText('Duration:')).toBeInTheDocument()
    expect(screen.getByText(startParts.value)).toBeInTheDocument()
    expect(screen.getByText(durationParts.value)).toBeInTheDocument()
    expect(screen.getByText(durationParts.unit)).toBeInTheDocument()
  })

  it('renders the span and error counts', () => {
    renderCard({ trace: makeTrace({ spanCount: 4, errorCount: 2 }) })
    expect(
      screen.getByText(
        (_, el) =>
          el?.classList.contains('badge-count') === true &&
          el.textContent?.replace(/\s+/g, ' ').trim() === '2 err · 4 spans'
      )
    ).toBeInTheDocument()
    expect(screen.getByText('2 err')).toBeInTheDocument()
  })

  it('omits the error badge when there are no errors', () => {
    renderCard({ trace: makeTrace({ errorCount: 0 }) })
    expect(screen.queryByText(/err/)).not.toBeInTheDocument()
  })

  it('reflects the selected prop via aria-pressed', () => {
    renderCard({ trace: makeTrace(), selected: true })
    expect(screen.getByRole('button')).toHaveAttribute('aria-pressed', 'true')
  })

  it('is not pressed by default', () => {
    renderCard({ trace: makeTrace() })
    expect(screen.getByRole('button')).toHaveAttribute('aria-pressed', 'false')
  })

  it('calls onclick with the trace id when clicked', async () => {
    const onclick = vi.fn()
    renderCard({ trace: makeTrace({ traceID: 'trace-42' }), onclick })
    await userEvent.click(screen.getByRole('button'))
    expect(onclick).toHaveBeenCalledWith('trace-42')
  })

  it('calls onclick with the trace id on keyboard activation', async () => {
    const onclick = vi.fn()
    const user = userEvent.setup()
    renderCard({ trace: makeTrace({ traceID: 'trace-42' }), onclick })
    screen.getByRole('button').focus()
    await user.keyboard('{Enter}')
    expect(onclick).toHaveBeenCalledWith('trace-42')
  })

  it('previews one service at a time and restores the clicked choice after hover', async () => {
    const user = userEvent.setup()
    const onclick = vi.fn()
    renderCard({
      trace: makeTrace({
        serviceBreakdown: [
          { key: 'service:database', label: 'database', count: 2 },
          { key: 'service:orders-service', label: 'orders-service', count: 1 },
          { key: 'service:cache', label: 'cache', count: 1 },
        ],
      }),
      onclick,
    })
    const group = screen.getByRole('group', { name: 'Spans by service' })
    const choices = within(group).getAllByRole('button')
    expect(choices.map(button => button.getAttribute('aria-label'))).toEqual([
      'orders-service: 1 span, 25% of total',
      'database: 2 spans, 50% of total',
      'cache: 1 span, 25% of total',
    ])
    const [root, database, cache] = choices
    const label = () => group.querySelector('.breakdown-label')
    expect(root).toHaveAttribute('aria-pressed', 'true')
    expect(label()).toHaveTextContent('orders-service (1 span, 25%)')
    await user.hover(database)
    await waitFor(() =>
      expect(label()).toHaveTextContent('database (2 spans, 50%)')
    )
    await user.click(database)
    expect(database).toHaveAttribute('aria-pressed', 'true')
    await user.hover(cache)
    await waitFor(() =>
      expect(label()).toHaveTextContent('cache (1 span, 25%)')
    )
    await user.unhover(cache)
    expect(label()).toHaveTextContent('database (2 spans, 50%)')
    expect(group.querySelectorAll('.breakdown-label')).toHaveLength(1)
    expect(onclick).not.toHaveBeenCalled()
    expect(document.querySelector('button button')).toBeNull()
  })

  it('reveals services with keyboard focus and matches their bar colour', async () => {
    const user = userEvent.setup()
    renderCard({
      trace: makeTrace({
        serviceBreakdown: [
          { key: 'service:orders-service', label: 'orders-service', count: 3 },
          { key: 'service:cache', label: 'cache', count: 1 },
        ],
      }),
    })
    const group = screen.getByRole('group', { name: 'Spans by service' })
    const choices = within(group).getAllByRole('button')
    const bar = screen.getByRole('group', { name: /Service breakdown/ })
    const barColors = Array.from(bar.children, child => {
      const probe = document.createElement('span')
      probe.style.backgroundColor = (
        child as HTMLElement
      ).style.getPropertyValue('--part-color')
      return probe.style.backgroundColor
    })
    const label = () => group.querySelector('.breakdown-label')
    const dot = () => label()?.querySelector<HTMLElement>('.breakdown-dot')
    await user.tab()
    await user.tab()
    expect(choices[0]).toHaveFocus()
    expect(label()).toHaveTextContent('orders-service (3 spans, 75%)')
    expect(dot()?.style.backgroundColor).toBe(barColors[0])
    await user.tab()
    expect(choices[1]).toHaveFocus()
    expect(label()).toHaveTextContent('cache (1 span, 25%)')
    expect(dot()?.style.backgroundColor).toBe(barColors[1])
    await user.keyboard('{Enter}')
    expect(choices[1]).toHaveAttribute('aria-pressed', 'true')
  })
})
