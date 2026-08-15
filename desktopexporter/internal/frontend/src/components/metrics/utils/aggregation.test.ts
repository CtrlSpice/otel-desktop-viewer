import { describe, expect, it } from 'vitest'
import {
  aggregateRate,
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

describe('aggregateRate over a cumulative series', () => {
  // The store differences the counter and sends the per-interval activity;
  // this reads it. It used to difference the points here instead -- points that
  // had already been through the M4 election, so consecutive chart points were
  // frequently not consecutive datapoints and the difference spanned the gap.
  //
  // The fixtures make the distinction visible: `value` stays a running total,
  // and `delta` is deliberately *not* the difference between neighbouring
  // values. Anything computing from `value` gets a different answer.
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
    const { series } = aggregateRate([{ key: 'a', label: 'a', points }], {
      cumulative: true,
      bucketCount: 1,
    })

    // 7 + 3 = 10 over the bucket. Differencing the values would give 40 + 60.
    const total = series[0]!.points.reduce((a, p) => a + p.value, 0)
    const bucketSeconds =
      (points[2]!.date.getTime() - points[1]!.date.getTime()) / 1000
    expect(total * bucketSeconds).toBeCloseTo(10, 9)
  })

  it('reports a reset the store flagged', () => {
    const points = [
      cumulativePoint(0, 100, null),
      cumulativePoint(1, 140, 40),
      // The counter restarted: the store sends the reading itself as the
      // interval's activity and says so.
      cumulativePoint(2, 5, 5, true),
    ]
    const { resets } = aggregateRate([{ key: 'a', label: 'a', points }], {
      cumulative: true,
      bucketCount: 2,
    })
    expect(resets.get('a')).toBeDefined()
    expect(resets.get('a')!.length).toBeGreaterThan(0)
  })

  it('leaves a delta series alone', () => {
    const points = [
      { date: new Date(0), value: 3 },
      { date: new Date(1000), value: 4 },
    ]
    const { series } = aggregateRate([{ key: 'a', label: 'a', points }], {
      cumulative: false,
      bucketCount: 1,
    })
    // Nothing dropped and nothing differenced: a Delta Sum's value already is
    // the interval's activity.
    expect(series[0]!.points.length).toBeGreaterThan(0)
  })
})
