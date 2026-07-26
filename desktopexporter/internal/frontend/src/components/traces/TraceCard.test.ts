// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { screen, within } from '@testing-library/svelte'
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
    expect(button).toHaveTextContent('(orders-service)')
  })

  it('falls back to a placeholder title when the trace has no root span yet', () => {
    renderCard({
      trace: makeTrace({ hasRootSpan: false, rootSpan: undefined }),
    })
    expect(screen.getByText('No root span yet')).toBeInTheDocument()
  })

  it('renders the formatted start time and duration', () => {
    const trace = makeTrace()
    renderCard({ trace })
    const startParts = formatTimestampParts(
      trace.startTime,
      'local',
      'milliseconds'
    )
    const durationParts = formatDurationParts(trace.durationNs as bigint)
    expect(screen.getByText('Duration:')).toBeInTheDocument()
    expect(screen.getByText(startParts.value)).toBeInTheDocument()
    expect(screen.getByText(durationParts.value)).toBeInTheDocument()
    expect(screen.getByText(durationParts.unit)).toBeInTheDocument()
  })

  it('renders the span and error counts', () => {
    renderCard({ trace: makeTrace({ spanCount: 4, errorCount: 2 }) })
    expect(screen.getByText('4 spans')).toBeInTheDocument()
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
})
