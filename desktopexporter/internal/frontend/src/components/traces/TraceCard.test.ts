// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { screen, within } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'svelte'
import TraceCard from './TraceCard.svelte'
import type { TraceSummary } from '@/types/api-types'
import {
  formatDurationParts,
  formatRelativeTime,
  formatTimestampParts,
} from '@/utils/time'
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

function cardButton() {
  return screen.getByRole('button', { name: /^GET \/api\/orders/ })
}

describe('TraceCard', () => {
  it('shows a single service as a bar with its count in the shared tooltip', () => {
    renderCard({ trace: makeTrace() })
    const button = cardButton()
    expect(within(button).getByText('GET /api/orders')).toBeInTheDocument()
    const breakdown = screen.getByRole('group', { name: /^Spans by service/ })
    expect(
      within(breakdown).getByRole('img', { name: 'orders-service: 4 spans' })
    ).toBeInTheDocument()
    expect(breakdown.querySelectorAll('.breakdown-segment')).toHaveLength(1)
    expect(
      breakdown.querySelector('.breakdown-segment__fill')
    ).toBeInTheDocument()
    expect(breakdown.querySelector('.breakdown-label')).toBeNull()
    expect(within(breakdown).queryByRole('button')).not.toBeInTheDocument()
    expect(breakdown.querySelector('[role="tooltip"]')).toHaveTextContent(
      'orders-service [4]'
    )
    expect(within(button).getByText('[4 spans]')).toBeInTheDocument()
    expect(screen.queryByText('(4 spans)')).not.toBeInTheDocument()
  })

  it('falls back to a placeholder title when the trace has no root span', () => {
    renderCard({
      trace: makeTrace({ hasRootSpan: false, rootSpan: undefined }),
    })
    expect(screen.getByText('No root span')).toBeInTheDocument()
  })

  it('keeps trace IDs out of the segment tooltip and preserves exact selection', async () => {
    const traceID = '1234567890abcdef1234567890abcdef'
    const onclick = vi.fn()
    renderCard({ trace: makeTrace({ traceID }), onclick })
    expect(document.querySelector('[role="tooltip"]')).toHaveTextContent(
      'orders-service [4]'
    )
    await userEvent.click(cardButton())
    expect(onclick).toHaveBeenCalledWith(traceID)
    expect(screen.queryByText(traceID)).not.toBeInTheDocument()
    expect(document.querySelector('.identifier, .ball-pit')).toBeNull()
    expect(document.querySelector('.signal-row__date')).not.toHaveTextContent(
      traceID
    )
  })

  it('places relative Start and Duration together at the bottom with the exact date on hover', () => {
    const trace = makeTrace()
    renderCard({ trace })
    const startParts = formatTimestampParts(
      trace.startTime,
      'local',
      'nanoseconds'
    )
    // SAFETY: makeTrace constructs this test fixture with a bigint durationNs value.
    const durationParts = formatDurationParts(trace.durationNs as bigint)
    expect(screen.getByText('Duration:')).toBeInTheDocument()
    const relative = formatRelativeTime(
      trace.startTime,
      BigInt(Date.now()) * 1_000_000n
    )
    expect(screen.getByText(relative)).toBeInTheDocument()
    expect(screen.getByText(durationParts.value)).toBeInTheDocument()
    expect(screen.getByText(durationParts.unit)).toBeInTheDocument()
    const date = document.querySelector('.signal-date')
    expect(date).toHaveTextContent(`Start: ${relative}`)
    expect(date).toHaveAttribute(
      'title',
      `Start: ${startParts.value} ${startParts.unit}`
    )
    expect(date).not.toHaveTextContent(startParts.unit)
    expect(
      document.querySelector('.signal-row__body')?.lastElementChild
    ).toContainElement(date as HTMLElement)
    expect(document.querySelector('.signal-row__date')).toContainElement(
      screen.getByText('Duration:')
    )
    expect(document.querySelector('.signal-row__meta-row')).toBeNull()
  })

  it('preserves a received zero duration', () => {
    renderCard({ trace: makeTrace({ durationNs: 0n }) })
    expect(screen.getByText('Duration:').parentElement).toHaveTextContent(
      /Duration:\s*0\s*ns/
    )
  })

  it('does not invent a zero duration when timing is missing', () => {
    renderCard({ trace: makeTrace({ durationNs: null }) })
    expect(screen.queryByText('Duration:')).not.toBeInTheDocument()
    expect(cardButton()).toHaveTextContent('GET /api/orders')
  })

  it('renders the span and error counts', () => {
    renderCard({ trace: makeTrace({ spanCount: 4, errorCount: 2 }) })
    expect(
      screen.getByText(
        (_, el) =>
          el?.classList.contains('badge-count') === true &&
          el.textContent?.replace(/\s+/g, ' ').trim() === '[2 err | 4 spans]'
      )
    ).toBeInTheDocument()
    expect(document.querySelector('.badge-count .bg-error')).toBeInTheDocument()
  })

  it('omits the error label, dot and separator when there are no errors', () => {
    renderCard({ trace: makeTrace({ errorCount: 0 }) })
    expect(screen.queryByText(/err/)).not.toBeInTheDocument()
    expect(document.querySelector('.badge-count')).toHaveTextContent(
      '[4 spans]'
    )
    expect(document.querySelector('.badge-count .bg-error')).toBeNull()
    expect(document.querySelector('.badge-count')).not.toHaveTextContent('·')
  })

  it.each([
    { spanCount: 0, label: '[0 spans]' },
    { spanCount: 1, label: '[1 span]' },
  ])(
    'keeps the exact span count and singular form: $label',
    ({ spanCount, label }) => {
      renderCard({ trace: makeTrace({ spanCount }) })
      expect(document.querySelector('.badge-count')?.textContent?.trim()).toBe(
        label
      )
    }
  )

  it('reflects the selected prop via aria-pressed', () => {
    renderCard({ trace: makeTrace(), selected: true })
    expect(cardButton()).toHaveAttribute('aria-pressed', 'true')
  })

  it('is not pressed by default', () => {
    renderCard({ trace: makeTrace() })
    expect(cardButton()).toHaveAttribute('aria-pressed', 'false')
  })

  it('calls onclick with the trace id when clicked', async () => {
    const onclick = vi.fn()
    renderCard({ trace: makeTrace({ traceID: 'trace-42' }), onclick })
    await userEvent.click(cardButton())
    expect(onclick).toHaveBeenCalledWith('trace-42')
  })

  it('calls onclick with the trace id on keyboard activation', async () => {
    const onclick = vi.fn()
    const user = userEvent.setup()
    renderCard({ trace: makeTrace({ traceID: 'trace-42' }), onclick })
    cardButton().focus()
    await user.keyboard('{Enter}')
    expect(onclick).toHaveBeenCalledWith('trace-42')
  })

  it('previews service counts and selects the trace once on bar click, not pointerdown', async () => {
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
    const group = screen.getByRole('group', { name: /^Spans by service/ })
    const bar = within(group).getByRole('img', { name: 'database: 2 spans' })
    expect(
      [...group.querySelectorAll('[role="tooltip"]')].map(node =>
        node.textContent?.trim()
      )
    ).toEqual(['orders-service [1]', 'database [2]', 'cache [1]'])
    expect(group.querySelectorAll('.breakdown-segment')).toHaveLength(3)
    expect(group.querySelector('.breakdown-label, .legend-rows')).toBeNull()
    await user.hover(bar)
    expect(onclick).not.toHaveBeenCalled()
    await user.pointer({ target: bar, keys: '[MouseLeft>]' })
    expect(onclick).not.toHaveBeenCalled()
    await user.pointer({ keys: '[/MouseLeft]' })
    expect(onclick).toHaveBeenCalledExactlyOnceWith('trace-1')
    expect(cardButton()).toContainElement(bar)
    expect(document.querySelector('button button')).toBeNull()
  })

  it('exposes service counts on the card without adding keyboard stops', async () => {
    const user = userEvent.setup()
    const onclick = vi.fn()
    renderCard({
      trace: makeTrace({
        serviceBreakdown: [
          {
            key: 'service:orders-service',
            label: 'orders-service',
            count: 3,
          },
          { key: 'service:cache', label: 'cache', count: 1 },
        ],
      }),
      onclick,
    })
    const group = screen.getByRole('group', { name: /^Spans by service/ })
    expect(within(group).queryByRole('button')).not.toBeInTheDocument()
    await user.tab()
    expect(cardButton()).toHaveFocus()
    expect(cardButton()).toHaveAccessibleName(
      expect.stringContaining('orders-service: 3 spans')
    )
    await user.keyboard('{Enter}')
    expect(onclick).toHaveBeenCalledExactlyOnceWith('trace-1')
    await user.keyboard(' ')
    expect(onclick).toHaveBeenCalledTimes(2)
    await user.tab()
    expect(cardButton()).not.toHaveFocus()
    expect(group).not.toContainElement(document.activeElement as HTMLElement)
  })
})
