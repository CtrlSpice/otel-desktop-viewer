import { describe, expect, it } from 'vitest'
import { seriesStatsFromPoints } from '@/components/metrics/utils/aggregation'

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
