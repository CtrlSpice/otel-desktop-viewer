import { describe, expect, it } from 'vitest'
import {
  availableAggregationViews,
  defaultAggregationViewFor,
} from '@/components/metrics/utils/aggregation'
import type { ChartPoint } from '@/types/metric-chart-types'

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
