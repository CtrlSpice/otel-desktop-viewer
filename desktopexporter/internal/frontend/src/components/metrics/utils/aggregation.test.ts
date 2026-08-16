import { describe, expect, it } from 'vitest'
import {
  aggregateSelectedAndAll,
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

describe('the pooled rate line over a cumulative series', () => {
  // The per-series views come from the store now. The pooled Selected / All
  // lines are still merged here, because they depend on the legend selection --
  // and they read the store's per-interval deltas rather than differencing the
  // points they were handed, which have been through the M4 election so
  // consecutive chart points are frequently not consecutive datapoints.
  //
  // The fixtures make that visible: `value` stays a running total and `delta`
  // is deliberately not the difference between neighbouring values, so anything
  // computing from `value` gets a different answer.
  function cumulativePoint(
    seconds: number,
    value: number,
    delta: number | null,
    isReset = false
  ): ChartPoint {
    return { date: new Date(seconds * 1000), value, delta, isReset }
  }

  it('rates the store deltas, not the differences between values', () => {
    const points = [
      // No predecessor, so no interval: dropped rather than counted as zero.
      cumulativePoint(0, 100, null),
      cumulativePoint(1, 140, 7),
      cumulativePoint(2, 200, 3),
    ]
    const series = [{ key: 'a', label: 'a', points }]
    const { lines } = aggregateSelectedAndAll(series, series, 'rate', {
      cumulative: true,
      bucketCount: 1,
    })

    expect(lines.length).toBeGreaterThan(0)
    const total = lines[0]!.points.reduce((sum, p) => sum + p.value, 0)
    // 7 + 3 = 10 over the bucket. Differencing the values would give 40 + 60.
    const bucketSeconds =
      (points[2]!.date.getTime() - points[1]!.date.getTime()) / 1000
    expect(total * bucketSeconds).toBeCloseTo(10, 9)
  })

  it('leaves a delta series alone', () => {
    const points = [
      { date: new Date(0), value: 3 },
      { date: new Date(1000), value: 4 },
    ]
    const series = [{ key: 'a', label: 'a', points }]
    const { lines } = aggregateSelectedAndAll(series, series, 'rate', {
      cumulative: false,
      bucketCount: 1,
    })
    // Nothing dropped and nothing differenced: a Delta Sum's value already is
    // the interval's activity.
    expect(lines[0]!.points.length).toBeGreaterThan(0)
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
    { name: 'cumulative monotonic Sum', metricType: 'Sum', temporality: 'Cumulative', isMonotonic: true, seriesCount: 3 },
    { name: 'cumulative non-monotonic Sum', metricType: 'Sum', temporality: 'Cumulative', isMonotonic: false, seriesCount: 3 },
    { name: 'cumulative Sum of unknown monotonicity', metricType: 'Sum', temporality: 'Cumulative', isMonotonic: null, seriesCount: 3 },
    { name: 'delta Sum', metricType: 'Sum', temporality: 'Delta', isMonotonic: true, seriesCount: 3 },
    { name: 'single-series cumulative monotonic Sum', metricType: 'Sum', temporality: 'Cumulative', isMonotonic: true, seriesCount: 1 },
    { name: 'Gauge', metricType: 'Gauge', temporality: 'Unspecified', isMonotonic: null, seriesCount: 3 },
    { name: 'single-series Gauge', metricType: 'Gauge', temporality: 'Unspecified', isMonotonic: null, seriesCount: 1 },
    { name: 'Histogram', metricType: 'Histogram', temporality: 'Delta', isMonotonic: null, seriesCount: 3 },
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
    expect(
      defaultAggregationViewFor('Sum', 'Cumulative', false, 3)
    ).toBe('raw')
    expect(
      defaultAggregationViewFor('Sum', 'Cumulative', null, 3)
    ).toBe('raw')
    expect(
      defaultAggregationViewFor('Sum', 'Cumulative', true, 3)
    ).toBe('rate')
  })
})
