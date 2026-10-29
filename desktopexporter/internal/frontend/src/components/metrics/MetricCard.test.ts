// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import type { ComponentProps } from 'svelte'
import MetricCard from './MetricCard.svelte'
import type { MetricSummary } from '@/types/api-types'
import { formatRelativeTime, formatTimestampParts } from '@/utils/time'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

function makeMetric(overrides: Partial<MetricSummary> = {}): MetricSummary {
  return {
    metricRef: 'metric-1',
    name: 'http.server.duration',
    description: 'Duration of HTTP server requests',
    unit: 'ms',
    metricType: 'Gauge',
    aggregationTemporality: null,
    aggregationTemporalityCode: null,
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
    expect(screen.getByText('orders-service')).toBeInTheDocument()
  })

  it('keeps descriptions out of summary cards without leaving an empty row', () => {
    const { container } = renderCard({
      metric: makeMetric({ description: 'Duration of HTTP server requests' }),
    })
    expect(
      screen.queryByText('Duration of HTTP server requests')
    ).not.toBeInTheDocument()
    expect(container.querySelector('.signal-row__lead')).toBeNull()
  })

  it('renders the last value with its unit when available', () => {
    renderCard({ metric: makeMetric({ lastValue: 42.5, unit: 'ms' }) })
    expect(screen.getByText('Last value:')).toBeInTheDocument()
    const expectedValue = new Intl.NumberFormat(undefined, {
      maximumFractionDigits: 6,
    }).format(42.5)
    expect(screen.getByText(expectedValue)).toBeInTheDocument()
    expect(screen.getByText('ms')).toBeInTheDocument()
    expect(screen.getByText('Last value:').parentElement).toHaveTextContent(
      `${expectedValue} ms`
    )
    expect(screen.queryByText('Units:')).not.toBeInTheDocument()
  })

  it('omits a unit-only row or type cluster when there is no last value', () => {
    const { container } = renderCard({
      metric: makeMetric({ lastValue: null, unit: 'By' }),
    })
    expect(screen.getByText('Gauge', { exact: true })).toBeInTheDocument()
    expect(screen.queryByText('By')).not.toBeInTheDocument()
    expect(screen.queryByText('Last value:')).not.toBeInTheDocument()
    expect(screen.queryByText('Units:')).not.toBeInTheDocument()
    expect(container.querySelector('.signal-row__meta-row')).toBeNull()
  })

  it('does not invent a histogram point value or an empty metadata row', () => {
    const { container } = renderCard({
      metric: makeMetric({
        metricType: 'Histogram',
        aggregationTemporality: 'delta',
        lastValue: null,
        unit: 's',
      }),
    })
    expect(screen.getByText('Histogram', { exact: true })).toHaveAttribute(
      'title',
      'Histogram · Delta'
    )
    expect(
      container.querySelector('.metric-card__classification')
    ).not.toHaveTextContent('Δ')
    expect(screen.queryByText('s', { exact: true })).not.toBeInTheDocument()
    expect(screen.queryByText('Last value:')).not.toBeInTheDocument()
    expect(screen.queryByText('Units:')).not.toBeInTheDocument()
    expect(container.querySelector('.signal-row__meta-row')).toBeNull()
  })

  it('keeps a received zero last value with its unit', () => {
    renderCard({ metric: makeMetric({ lastValue: 0, unit: 's' }) })
    expect(screen.getByText('Last value:').parentElement).toHaveTextContent(
      /Last value:\s*0\s+s/
    )
  })

  it('renders the plain metric type summary', () => {
    renderCard({
      metric: makeMetric({
        metricType: 'Sum',
        aggregationTemporality: 'delta',
        isMonotonic: true,
      }),
    })
    expect(screen.getByText('Sum', { exact: true })).toHaveAttribute(
      'title',
      'Sum · Delta · Monotonic'
    )
    expect(screen.queryByText(/Δ|↗/)).not.toBeInTheDocument()
  })

  it.each([
    ['Histogram', 'Histogram'],
    ['ExponentialHistogram', 'Exp. Histogram'],
  ] as const)('keeps the full %s name on hover for %s', (metricType, label) => {
    renderCard({ metric: makeMetric({ metricType }) })
    expect(screen.getByText(label, { exact: true })).toHaveAttribute(
      'title',
      metricType
    )
  })

  it('places Last seen and Last value on the shared bottom row with the exact date on hover', () => {
    const metric = makeMetric()
    renderCard({ metric })
    const lastSeenParts = formatTimestampParts(
      metric.lastSeen!,
      'local',
      'nanoseconds'
    )
    expect(screen.getByText('Last seen:')).toBeInTheDocument()
    expect(
      screen.getByText(
        formatRelativeTime(metric.lastSeen!, BigInt(Date.now()) * 1_000_000n)
      )
    ).toBeInTheDocument()
    const date = document.querySelector('.signal-date')
    expect(date).toHaveAttribute(
      'title',
      `Last seen: ${lastSeenParts.value} ${lastSeenParts.unit}`
    )
    expect(date).not.toHaveTextContent(lastSeenParts.unit)
    expect(document.querySelector('.signal-row__date')).toContainElement(
      screen.getByText('Last value:')
    )
    expect(document.querySelector('.signal-row__meta-row')).toBeNull()
    expect(
      document.querySelector('.signal-row__body')?.lastElementChild
    ).toContainElement(date as HTMLElement)
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
    renderCard({ metric: makeMetric({ metricRef: 'metric-42' }), onclick })
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
          el.textContent?.replace(/\s+/g, ' ').trim() ===
            '[3 of 21 series | 120 pts]'
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
          el.textContent?.replace(/\s+/g, ' ').trim() ===
            '[21 series | 120 pts]'
      )
    ).toBeInTheDocument()
    expect(screen.queryByText(/of 21 series/)).not.toBeInTheDocument()
  })

  it.each([
    {
      seriesCount: 1,
      seriesCardinality: 1,
      dataPointCount: 30,
      label: '[1 series | 30 pts]',
    },
    {
      seriesCount: 1,
      seriesCardinality: 3,
      dataPointCount: 30,
      label: '[1 of 3 series | 30 pts]',
    },
    {
      seriesCount: 0,
      seriesCardinality: 3,
      dataPointCount: 0,
      label: '[0 of 3 series | 0 pts]',
    },
    {
      seriesCount: 0,
      seriesCardinality: 0,
      dataPointCount: 0,
      label: '[0 series | 0 pts]',
    },
  ])('uses one bracketed count group: $label', ({ label, ...counts }) => {
    const { container } = renderCard({ metric: makeMetric(counts) })
    const count = container.querySelector('.badge-count')
    expect(container.querySelectorAll('.badge-count')).toHaveLength(1)
    expect(count?.textContent?.replace(/\s+/g, ' ').trim()).toBe(label)
    expect(count).toHaveAttribute(
      'title',
      expect.stringContaining(`${counts.dataPointCount} datapoints in range`)
    )
    expect(count?.querySelector('.badge')).toBeNull()
  })
})
