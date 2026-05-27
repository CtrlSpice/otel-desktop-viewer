import { describe, expect, it } from 'vitest'
import {
  bucketExtents,
  bucketQuantileLinear,
  bucketQuantileLoglin,
  expHistQuantile,
  expNegBuckets,
  expPosBuckets,
  histBuckets,
  histQuantile,
  interpLinear,
  interpLoglin,
} from '@/components/metrics/utils/histogram-quantile'

describe('interp kernels', () => {
  it('linear midpoint', () => {
    expect(interpLinear(0, 10, 0, 100, 50)).toBeCloseTo(5, 9)
  })
  it('loglin geometric midpoint', () => {
    expect(interpLoglin(1, 100, 0, 100, 50)).toBeCloseTo(10, 9)
  })
  it('loglin falls back when lo=0', () => {
    expect(interpLoglin(0, 10, 0, 100, 50)).toBeCloseTo(5, 9)
  })
})

describe('hist_quantile', () => {
  it('p50 on bucket boundary', () => {
    expect(
      histQuantile([1, 2, 5, 10], [0, 50, 50, 0, 0], 0.5)
    ).toBeCloseTo(2, 9)
  })
  it('p95 in unbounded tail', () => {
    expect(
      histQuantile([1, 2, 5, 10], [0, 10, 20, 30, 40], 0.95)
    ).toBeCloseTo(10, 9)
  })
  it('empty bounds returns null', () => {
    expect(histQuantile([], [], 0.5)).toBeNull()
  })
})

describe('exp_hist_quantile', () => {
  it('positive-only p50', () => {
    expect(expHistQuantile(0, 0, [], 0, 0, [50, 50], 0.5)).toBeCloseTo(2, 9)
  })
  it('zero-only p50', () => {
    expect(expHistQuantile(0, 0, [], 100, 0, [], 0.5)).toBeCloseTo(0, 9)
  })
  it('symmetric three-region p50 in zero bucket', () => {
    expect(expHistQuantile(0, 0, [10, 10], 20, 0, [10, 10], 0.5)).toBeCloseTo(
      0,
      9
    )
  })
  it('reference dataset p95', () => {
    expect(
      expHistQuantile(
        2,
        0,
        [],
        0,
        6,
        [1200, 3800, 4200, 2100, 720, 280, 70, 22, 6, 2],
        0.95
      )
    ).toBeCloseTo(6.349604207872798, 6)
  })
  it('all-zero counts returns null', () => {
    expect(expHistQuantile(0, 0, [], 0, 0, [], 0.5)).toBeNull()
  })
})

describe('hist_buckets shape', () => {
  const bounds = [1, 2, 5, 10]
  const counts = [10, 20, 30, 40, 50]
  const buckets = histBuckets(bounds, counts)

  it('clamps first bucket', () => {
    expect(buckets[0]!.lo).toBeCloseTo(1, 9)
    expect(buckets[0]!.hi).toBeCloseTo(1, 9)
  })
  it('clamps last bucket', () => {
    expect(buckets[4]!.lo).toBeCloseTo(10, 9)
    expect(buckets[4]!.hi).toBeCloseTo(10, 9)
  })
  it('inner bucket bounds', () => {
    expect(buckets[2]!.lo).toBeCloseTo(2, 9)
    expect(buckets[2]!.hi).toBeCloseTo(5, 9)
  })
})

describe('exp bucket builders', () => {
  it('exp_pos_buckets', () => {
    const b = expPosBuckets(0, 0, [10, 20, 30])
    expect(b[0]!.hi).toBeCloseTo(2, 9)
    expect(b[2]!.lo).toBeCloseTo(4, 9)
  })
  it('exp_neg_buckets reverses counts', () => {
    const b = expNegBuckets(0, 0, [10, 20, 30])
    expect(b[0]!.lo).toBeCloseTo(-8, 9)
    expect(b[0]!.cnt).toBe(30)
    expect(b[2]!.cnt).toBe(10)
  })
})

describe('end-to-end merged quantile', () => {
  it('hist_quantile on sum_bucket_vectors', () => {
    const merged = [0, 80, 100, 20, 0]
    expect(histQuantile([1, 2, 5, 10], merged, 0.5)).toBeCloseTo(2.6, 9)
  })
})

describe('bucketExtents', () => {
  it('derives min/max from populated explicit buckets', () => {
    expect(
      bucketExtents(histBuckets([10, 20, 50], [5, 0, 10, 0]))
    ).toEqual({ min: 10, max: 50 })
  })

  it('uses populated exp bucket bounds instead of OTLP summary fields', () => {
    const scale = 3
    const base = Math.pow(2, Math.pow(2, -scale))
    const center = 830
    const centerIdx = Math.floor(Math.log(center) / Math.log(base))
    const offset = centerIdx - 4
    const counts = [1, 2, 40, 80, 40, 20, 10, 5, 2]
    const extents = bucketExtents(expPosBuckets(scale, offset, counts))
    const lo = Math.pow(base, offset)
    expect(extents?.min).toBeCloseTo(lo, 0)
    expect(extents?.min).toBeGreaterThan(166)
    expect(extents?.max).toBeCloseTo(Math.pow(base, offset + counts.length), 0)
  })
})
