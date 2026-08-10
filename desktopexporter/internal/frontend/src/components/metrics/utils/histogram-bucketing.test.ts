import { describe, expect, it } from 'vitest'
import {
  buildHistogramTimeMergedSeries,
  histogramBucketNs,
  histogramBucketStart,
  isHistogramAggregationError,
  type HistogramSlicePoint,
} from '@/components/metrics/utils/histogram-aggregation'
import type { HistogramDataPoint, MetricTimeseries } from '@/types/api-types'

const MS = 1_000_000n
const SEC = 1_000n * MS
const MIN = 60n * SEC
const HOUR = 60n * MIN
const DAY = 24n * HOUR

function dp(timestampNs: bigint, counts: number[]): HistogramDataPoint {
  return {
    metricType: 'Histogram',
    id: `dp-${timestampNs}`,
    timestamp: timestampNs,
    startTime: 0n,
    flags: 0,
    exemplars: [],
    count: counts.reduce((a, b) => a + b, 0),
    sum: 0,
    min: 0,
    max: 10,
    bucketCounts: counts,
    explicitBounds: [1, 2, 5],
    aggregationTemporality: 'Cumulative',
  } as unknown as HistogramDataPoint
}

function series(
  attributesKey: string,
  datapoints: HistogramDataPoint[]
): MetricTimeseries {
  return {
    attributesKey,
    attributes: [],
    datapoints,
  } as unknown as MetricTimeseries
}

describe('histogramBucketNs', () => {
  // The bug that made long windows unusable: a short burst of data viewed
  // much later had its bucket width set by the empty tail, collapsing every
  // datapoint into one column -- and worsening the further you zoomed out.
  it('sizes buckets from the data extent, not the empty tail of the window', () => {
    // Shaped like the race capture: a ~2s burst, viewed 11 days later.
    const dataStart = 1_000_000n * SEC
    const dataEnd = dataStart + 2n * SEC
    const windowEnd = dataStart + 11n * DAY
    const stamps: bigint[] = []
    for (let i = 0n; i <= 20n; i++) stamps.push(dataStart + i * 100n * MS)

    const withTail = histogramBucketNs(
      dataStart - HOUR,
      windowEnd,
      dataStart,
      100,
      dataEnd,
      stamps
    )

    // Old behaviour divided the whole 11-day window by 100, giving ~2.7h
    // buckets that swallowed all 2s of data in one column.
    const oldWidth = (windowEnd - dataStart) / 100n
    expect(oldWidth).toBeGreaterThan(HOUR)
    expect(withTail).toBeLessThan(MIN)

    // The data must span more than one column now.
    expect(
      histogramBucketStart(dataEnd, withTail) >
        histogramBucketStart(dataStart, withTail)
    ).toBe(true)
  })

  it('only ever returns ladder widths', () => {
    const ladder = new Set([
      SEC,
      5n * SEC,
      10n * SEC,
      30n * SEC,
      MIN,
      5n * MIN,
      10n * MIN,
      15n * MIN,
      30n * MIN,
      HOUR,
      3n * HOUR,
      6n * HOUR,
      12n * HOUR,
      DAY,
    ])
    const start = 1_000_000n * SEC
    for (const spanMinutes of [1n, 7n, 45n, 200n, 1_000n, 10_000n]) {
      const end = start + spanMinutes * MIN
      const w = histogramBucketNs(start, end, start, 100, end, [start, end])
      expect(ladder.has(w)).toBe(true)
    }
  })

  it('keeps the bucket count under the target', () => {
    const start = 1_000_000n * SEC
    for (const spanHours of [1n, 6n, 24n, 24n * 7n]) {
      const end = start + spanHours * HOUR
      const w = histogramBucketNs(start, end, start, 100, end, [start, end])
      expect(Number((end - start) / w)).toBeLessThanOrEqual(100)
    }
  })

  // The point of ladder widths: boundaries are absolute, so panning the
  // window does not reshuffle every column.
  it('produces boundaries that do not move when the window shifts', () => {
    const start = 1_000_000n * SEC
    const end = start + 3n * HOUR
    const shift = 137n * SEC // arbitrary, not a multiple of any ladder step

    const w1 = histogramBucketNs(start, end, start, 100, end, [start, end])
    const w2 = histogramBucketNs(
      start + shift,
      end + shift,
      start + shift,
      100,
      end + shift,
      [start + shift, end + shift]
    )
    expect(w1).toBe(w2)

    // A timestamp present in both windows lands in the same bucket.
    const probe = start + 2n * HOUR
    expect(histogramBucketStart(probe, w1)).toBe(
      histogramBucketStart(probe, w2)
    )
  })

  // Bucketing finer than the reporting interval invents empty columns
  // between real ones.
  it('never buckets finer than the data cadence', () => {
    const start = 1_000_000n * SEC
    const stamps: bigint[] = []
    for (let i = 0n; i < 20n; i++) stamps.push(start + i * MIN)
    const end = start + 20n * MIN

    const w = histogramBucketNs(start, end, start, 100, end, stamps)
    expect(w).toBeGreaterThanOrEqual(MIN)
  })

  // A pause in the capture must not drag the floor up to the gap's size.
  it('uses the median gap, so one long pause does not coarsen everything', () => {
    const start = 1_000_000n * SEC
    const stamps: bigint[] = []
    for (let i = 0n; i < 10n; i++) stamps.push(start + i * SEC)
    const resume = start + 6n * HOUR
    for (let i = 0n; i < 10n; i++) stamps.push(resume + i * SEC)
    const end = resume + 10n * SEC

    const w = histogramBucketNs(start, end, start, 100, end, stamps)
    // Mean gap would be ~20min; median is 1s, so we stay fine-grained
    // subject only to the bucket-count target.
    expect(w).toBeLessThanOrEqual(10n * MIN)
  })
})

describe('cumulative bucket merge', () => {
  const start = 1_000_000n * SEC

  function slicesFor(dps: HistogramDataPoint[]): HistogramSlicePoint[] {
    const out = buildHistogramTimeMergedSeries(
      [series('driver=alonso', dps)],
      start - MIN,
      start + DAY,
      100,
      'Cumulative'
    )
    if (isHistogramAggregationError(out)) throw new Error(out.message)
    return out.sort((a, b) => (a.timestamp < b.timestamp ? -1 : 1))
  }

  /** 250 points at 1s, counter climbing a steady +10 per point. The count
   *  target forces buckets wider than the 1s cadence, so several points
   *  share each bucket -- which is the situation the merge has to handle. */
  function steadyClimb(): HistogramDataPoint[] {
    const out: HistogramDataPoint[] = []
    for (let i = 0; i < 250; i++) {
      out.push(dp(start + BigInt(i) * SEC, [i * 10, 0, 0, 0]))
    }
    return out
  }

  // The bug: cumulative counts run since stream start, so returning the
  // bucket's latest point made every column show the running total. Under a
  // steady workload the columns climbed without bound; they should instead
  // be flat, because the same amount happened in each interval.
  it('shows flat columns for a steady workload, not a climbing total', () => {
    const slices = slicesFor(steadyClimb())
    expect(slices.length).toBeGreaterThan(5)

    // Ignore the first bucket: it has no earlier point to difference
    // against, so it legitimately carries the stream's history.
    const counts = slices
      .slice(1)
      .map(s => (s.kind === 'histogram' ? s.counts[0]! : NaN))

    const first = counts[0]!
    const last = counts[counts.length - 1]!
    expect(last).toBeCloseTo(first, 5)

    // And concretely: nothing like the old running total, which by the end
    // would have been in the thousands.
    expect(Math.max(...counts)).toBeLessThan(200)
  })

  // A single point has nothing to difference against; the stream's history
  // to that moment is genuine activity we just cannot place more precisely.
  it('passes a lone datapoint through unchanged', () => {
    const slices = slicesFor([dp(start, [7, 8, 9, 0])])
    const s = slices[0]!
    if (s.kind !== 'histogram') throw new Error('expected histogram')
    expect(s.counts).toEqual([7, 8, 9, 0])
  })

  // After a counter reset the newer point already counts from zero, so
  // differencing it against the pre-reset point would go negative.
  it('never emits negative counts when the counter resets', () => {
    const dps = steadyClimb()
    // Restart the counter a third of the way through.
    for (let i = 80; i < dps.length; i++) {
      dps[i] = dp(start + BigInt(i) * SEC, [(i - 80) * 10, 0, 0, 0])
    }

    const slices = slicesFor(dps)
    for (const s of slices) {
      if (s.kind !== 'histogram') continue
      expect(s.counts.every(c => c >= 0)).toBe(true)
      expect(s.totals.count).toBeGreaterThanOrEqual(0)
    }
  })
})

describe('histogramBucketStart timezone alignment', () => {
  // Flooring against the epoch aligns to UTC. A day-scale column then breaks
  // at UTC midnight wherever the viewer is, which reads as the wrong day for
  // anyone not on UTC.
  it('aligns day buckets to local midnight, not UTC midnight', () => {
    // 2026-03-15T04:30:00Z
    const ts = BigInt(Date.UTC(2026, 2, 15, 4, 30, 0)) * MS

    const utcStart = histogramBucketStart(ts, DAY, 'UTC')
    const localStart = histogramBucketStart(ts, DAY, 'local')

    // UTC alignment always lands exactly on a UTC midnight.
    expect(new Date(Number(utcStart / MS)).getUTCHours()).toBe(0)

    // Local alignment lands on local midnight. In a UTC runner the two
    // coincide, which is correct rather than a failure -- assert the
    // property, not a difference.
    const localDate = new Date(Number(localStart / MS))
    expect(localDate.getHours()).toBe(0)
    expect(localDate.getMinutes()).toBe(0)
    expect(localDate.getSeconds()).toBe(0)
  })

  it('never floors past the timestamp it is given', () => {
    const ts = BigInt(Date.UTC(2026, 6, 4, 17, 45, 13)) * MS
    for (const width of [SEC, MIN, HOUR, DAY]) {
      for (const tz of ['UTC', 'local'] as const) {
        const start = histogramBucketStart(ts, width, tz)
        expect(start).toBeLessThanOrEqual(ts)
        expect(ts - start).toBeLessThan(width)
      }
    }
  })

  // Two timestamps in the same local day must share a day bucket, which is
  // the whole point of aligning to the viewer's clock.
  it('groups timestamps from one local day into one bucket', () => {
    const morning = BigInt(new Date(2026, 4, 20, 9, 15, 0).getTime()) * MS
    const evening = BigInt(new Date(2026, 4, 20, 22, 45, 0).getTime()) * MS

    expect(histogramBucketStart(morning, DAY, 'local')).toBe(
      histogramBucketStart(evening, DAY, 'local')
    )
  })

  it('floors pre-epoch timestamps downward rather than toward zero', () => {
    const ts = BigInt(Date.UTC(1969, 11, 31, 22, 0, 0)) * MS
    for (const tz of ['UTC', 'local'] as const) {
      const start = histogramBucketStart(ts, HOUR, tz)
      expect(start).toBeLessThanOrEqual(ts)
      expect(ts - start).toBeLessThan(HOUR)
    }
  })
})
