// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'svelte'
import MetricCard from './MetricCard.svelte'
import type { MetricSummary } from '@/types/api-types'
import { formatTimestampParts } from '@/utils/time'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

function makeMetric(overrides: Partial<MetricSummary> = {}): MetricSummary {
  return {
    id: 'metric-1',
    name: 'http.server.duration',
    description: 'Duration of HTTP server requests',
    unit: 'ms',
    metricType: 'Gauge',
    aggregationTemporality: null,
    isMonotonic: null,
    serviceName: 'orders-service',
    seriesCount: 3,
    seriesCardinality: 3,
    dataPointCount: 120,
    lastValue: 42.5,
    lastSeen: 1_700_000_000_000_000_000n,
    ...overrides,
  }
}

function renderCard(props: ComponentProps<typeof MetricCard>) {
  setTestUrl('/metrics')
  return renderWithContexts(MetricCard, props)
}

describe('MetricCard', () => {
  it('renders the metric name and service name', () => {
    renderCard({ metric: makeMetric() })
    expect(screen.getByText('http.server.duration')).toBeInTheDocument()
    expect(screen.getByRole('button')).toHaveTextContent('(orders-service)')
  })

  it('renders the description', () => {
    renderCard({
      metric: makeMetric({ description: 'Duration of HTTP server requests' }),
    })
    expect(
      screen.getByText('Duration of HTTP server requests')
    ).toBeInTheDocument()
  })

  it('renders the last value with its unit when available', () => {
    renderCard({ metric: makeMetric({ lastValue: 42.5, unit: 'ms' }) })
    expect(screen.getByText('Last value:')).toBeInTheDocument()
    const expectedValue = new Intl.NumberFormat(undefined, {
      maximumFractionDigits: 6,
    }).format(42.5)
    expect(screen.getByText(`${expectedValue} ms`)).toBeInTheDocument()
  })

  it('renders the unit alone when there is no last value', () => {
    renderCard({ metric: makeMetric({ lastValue: null, unit: 'By' }) })
    expect(screen.getByText('Units:')).toBeInTheDocument()
    expect(screen.getByText('By')).toBeInTheDocument()
    expect(screen.queryByText('Last value:')).not.toBeInTheDocument()
  })

  it('renders the metric type badge', () => {
    renderCard({
      metric: makeMetric({
        metricType: 'Sum',
        aggregationTemporality: 'delta',
        isMonotonic: true,
      }),
    })
    expect(screen.getByText('Sum Δ ↗')).toBeInTheDocument()
  })

  it('renders the formatted last-seen timestamp', () => {
    const metric = makeMetric()
    renderCard({ metric })
    const lastSeenParts = formatTimestampParts(
      metric.lastSeen,
      'local',
      'milliseconds'
    )
    expect(screen.getByText('Last seen:')).toBeInTheDocument()
    expect(screen.getByText(lastSeenParts.value)).toBeInTheDocument()
  })

  it('reflects the selected prop via aria-pressed', () => {
    renderCard({ metric: makeMetric(), selected: true })
    expect(screen.getByRole('button')).toHaveAttribute('aria-pressed', 'true')
  })

  it('is not pressed by default', () => {
    renderCard({ metric: makeMetric() })
    expect(screen.getByRole('button')).toHaveAttribute('aria-pressed', 'false')
  })

  it('calls onclick with the metric id when clicked', async () => {
    const onclick = vi.fn()
    renderCard({ metric: makeMetric({ id: 'metric-42' }), onclick })
    await userEvent.click(screen.getByRole('button'))
    expect(onclick).toHaveBeenCalledWith('metric-42')
  })
})

describe('MetricCard series counts', () => {
  // One number that changed meaning with the window read as data going
  // missing. Both are shown when they differ, and the pair is what says
  // "these series went quiet" rather than "these series are gone".
  it('shows both counts when the window holds fewer than the stream has', () => {
    renderWithContexts(MetricCard, {
      metric: makeMetric({ seriesCount: 3, seriesCardinality: 21 }),
      onclick: vi.fn(),
    })
    expect(
      screen.getByText(
        (_, el) =>
          el?.classList.contains('badge-count') === true &&
          el.textContent?.replace(/\s+/g, ' ').trim() === '3 of 21 series'
      )
    ).toBeInTheDocument()
  })

  // The unbounded-window case, where they agree: "21 of 21" is noise.
  it('shows one count when they agree', () => {
    renderWithContexts(MetricCard, {
      metric: makeMetric({ seriesCount: 21, seriesCardinality: 21 }),
      onclick: vi.fn(),
    })
    expect(
      screen.getByText(
        (_, el) =>
          el?.classList.contains('badge-count') === true &&
          el.textContent?.replace(/\s+/g, ' ').trim() === '21 series'
      )
    ).toBeInTheDocument()
    expect(screen.queryByText(/of 21 series/)).not.toBeInTheDocument()
  })
})
