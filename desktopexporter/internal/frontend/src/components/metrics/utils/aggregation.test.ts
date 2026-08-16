import { describe, expect, it } from 'vitest'
import {
  availableAggregationViews,
  defaultAggregationViewFor,
  seriesStatsFromPoints,
} from '@/components/metrics/utils/aggregation'
import type { ChartPoint } from '@/types/metric-chart-types'

describe('seriesStatsFromPoints against a thinned series', () => {
  // This documents *why* the store computes these instead. The function is
  // correct for what it is given; what it is given is a sample.
  it('under-reports the total in proportion to the thinning', () => {
    const every = Array.from({ length: 1000 }, (_, i) => ({
      date: new Date(i),
      value: 1,
    }))
    const thinned = every.filter((_, i) => i % 10 === 0)

    expect(seriesStatsFromPoints(every).total).toBe(1000)
    // A tenth of the points means a tenth of the total -- and `total` is an
    // offered badge for Sum + Delta + raw, so this reached the screen.
    expect(seriesStatsFromPoints(thinned).total).toBe(100)
  })

  it('reports an average biased by which points survived', () => {
    // A series that is mostly 1 with a single spike. Keeping the spike while
    // dropping most of the ordinary points moves the mean a long way.
    const every = [
      ...Array.from({ length: 999 }, (_, i) => ({
        date: new Date(i),
        value: 1,
      })),
      { date: new Date(999), value: 1000 },
    ]
    const keptSpike = [every[0]!, every[500]!, every[999]!]

    expect(seriesStatsFromPoints(every).avg).toBeCloseTo(1.999, 5)
    expect(seriesStatsFromPoints(keptSpike).avg).toBeCloseTo(334.0, 5)
  })
})

describe('the default aggregation view agrees with the offered ones', () => {
  // The two rules disagreed on exactly one shape, and the disagreement was
  // invisible where it mattered: f1.driver.championship_points is a Sum,
  // Cumulative, non-monotonic. It defaulted to 'rate' while its own menu
  // offered only raw / sum / avg, so no tab rendered as active and every row
  // sparkline drew nothing -- the rate of a series' first bucket is null by
  // definition, and each of those series had exactly one bucket.
  const shapes: {
    name: string
    metricType: string
    temporality: string
    isMonotonic: boolean | null
    seriesCount: number
  }[] = [
    {
      name: 'cumulative monotonic Sum',
      metricType: 'Sum',
      temporality: 'Cumulative',
      isMonotonic: true,
      seriesCount: 3,
    },
    {
      name: 'cumulative non-monotonic Sum',
      metricType: 'Sum',
      temporality: 'Cumulative',
      isMonotonic: false,
      seriesCount: 3,
    },
    {
      name: 'cumulative Sum of unknown monotonicity',
      metricType: 'Sum',
      temporality: 'Cumulative',
      isMonotonic: null,
      seriesCount: 3,
    },
    {
      name: 'delta Sum',
      metricType: 'Sum',
      temporality: 'Delta',
      isMonotonic: true,
      seriesCount: 3,
    },
    {
      name: 'single-series cumulative monotonic Sum',
      metricType: 'Sum',
      temporality: 'Cumulative',
      isMonotonic: true,
      seriesCount: 1,
    },
    {
      name: 'Gauge',
      metricType: 'Gauge',
      temporality: 'Unspecified',
      isMonotonic: null,
      seriesCount: 3,
    },
    {
      name: 'single-series Gauge',
      metricType: 'Gauge',
      temporality: 'Unspecified',
      isMonotonic: null,
      seriesCount: 1,
    },
    {
      name: 'Histogram',
      metricType: 'Histogram',
      temporality: 'Delta',
      isMonotonic: null,
      seriesCount: 3,
    },
  ]

  for (const s of shapes) {
    it(`offers the default it picks for a ${s.name}`, () => {
      const available = availableAggregationViews(
        s.metricType,
        s.temporality,
        s.isMonotonic,
        s.seriesCount
      )
      const chosen = defaultAggregationViewFor(
        s.metricType,
        s.temporality,
        s.isMonotonic,
        s.seriesCount
      )
      expect(available).toContain(chosen)
    })
  }

  it('only reaches for rate when the counter can actually be differenced', () => {
    // Non-monotonic means it may legitimately fall, and rate reads a fall as a
    // counter restart -- so the rate of one would report resets that never
    // happened.
    expect(defaultAggregationViewFor('Sum', 'Cumulative', false, 3)).toBe('raw')
    expect(defaultAggregationViewFor('Sum', 'Cumulative', null, 3)).toBe('raw')
    expect(defaultAggregationViewFor('Sum', 'Cumulative', true, 3)).toBe('rate')
  })
})
