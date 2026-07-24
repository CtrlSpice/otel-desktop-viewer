import { describe, expect, it } from 'vitest'
import {
  downscaleExpBuckets,
  floorDiv,
  foldBelowCutoff,
  mergeExplicitHistogramVectors,
  padLeftToOffset,
  sumBucketVectors,
} from '@/components/metrics/utils/histogram-merge'
import { histQuantile } from '@/components/metrics/utils/histogram-quantile'

describe('floorDiv', () => {
  it('negative remainder rounds toward -inf', () => {
    expect(floorDiv(-7, 2)).toBe(-4)
  })
  it('positive remainder', () => {
    expect(floorDiv(7, 2)).toBe(3)
  })
})

describe('sumBucketVectors', () => {
  it('sums element-wise', () => {
    const v = sumBucketVectors([
      [1, 2, 3],
      [4, 5, 6],
      [7, 8, 9],
    ])
    expect(v).toEqual([12, 15, 18])
  })
  it('zero-pads mismatched lengths', () => {
    expect(
      sumBucketVectors([
        [1, 2, 3],
        [4, 5],
      ])![2]
    ).toBe(3)
  })
  it('empty input returns null', () => {
    expect(sumBucketVectors([])).toBeNull()
  })
})

describe('downscaleExpBuckets', () => {
  it('levels=0 is identity', () => {
    const r = downscaleExpBuckets([10, 20, 30], 5, 0)
    expect(r.offset).toBe(5)
    expect(r.counts).toEqual([10, 20, 30])
  })
  it('levels=1 halves resolution at offset 0', () => {
    const r = downscaleExpBuckets([10, 20, 30, 40], 0, 1)
    expect(r.offset).toBe(0)
    expect(r.counts).toEqual([30, 70])
  })
  it('conserves mass', () => {
    const r = downscaleExpBuckets([3, 7, 11, 13, 17, 19, 23, 29], -2, 2)
    const sum = r.counts.reduce((a, b) => a + b, 0)
    expect(sum).toBe(122)
  })
  it('composes with sum_bucket_vectors', () => {
    const down = downscaleExpBuckets([10, 20, 30, 40], 0, 1).counts
    const merged = sumBucketVectors([down, [15, 35]])
    expect(merged).toEqual([45, 105])
  })
})

describe('foldBelowCutoff', () => {
  it('null cutoff is no-op', () => {
    const r = foldBelowCutoff([10, 20, 30], 5, null)
    expect(r.counts).toEqual([10, 20, 30])
    expect(r.folded).toBe(0)
  })
  it('folds at first bucket', () => {
    const r = foldBelowCutoff([10, 20, 30], 5, 5)
    expect(r.counts).toEqual([20, 30])
    expect(r.offset).toBe(6)
    expect(r.folded).toBe(10)
  })
  it('conserves mass', () => {
    const r = foldBelowCutoff([3, 7, 11, 13, 17, 19], 0, 2)
    const sum = r.folded + r.counts.reduce((a, b) => a + b, 0)
    expect(sum).toBe(70)
  })
})

describe('padLeftToOffset', () => {
  it('pads by 2', () => {
    const r = padLeftToOffset([10, 20, 30], 5, 3)
    expect(r).toEqual([0, 0, 10, 20, 30])
  })
  it('preserves mass', () => {
    const r = padLeftToOffset([3, 7, 11, 13], 10, 6)!
    const sum = r.reduce((a, b) => a + b, 0)
    expect(sum).toBe(34)
  })
})

describe('mergeExplicitHistogramVectors', () => {
  it('merges and computes quantile', () => {
    const merged = mergeExplicitHistogramVectors([
      { bounds: [1, 2, 5, 10], counts: [0, 50, 50, 0, 0] },
      { bounds: [1, 2, 5, 10], counts: [0, 30, 50, 20, 0] },
    ])
    expect(histQuantile(merged.bounds, merged.counts, 0.5)).toBeCloseTo(2.6, 9)
  })
  it('throws on bounds mismatch', () => {
    expect(() =>
      mergeExplicitHistogramVectors([
        { bounds: [1, 2], counts: [1, 2, 3] },
        { bounds: [1, 5], counts: [1, 2, 3] },
      ])
    ).toThrow()
  })
})
