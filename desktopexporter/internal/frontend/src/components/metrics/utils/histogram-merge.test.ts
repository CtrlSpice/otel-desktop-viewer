import { describe, expect, it } from 'vitest'
import {
  downscaleExpBuckets,
  mergeExpHistogramStreams,
  floorDiv,
  foldBelowCutoff,
  mergeExplicitHistogramVectors,
  padLeftToOffset,
  sumBucketVectors,
} from '@/components/metrics/utils/histogram-merge'

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
  it('merges counts positionally when the bounds agree', () => {
    const merged = mergeExplicitHistogramVectors([
      { bounds: [1, 2, 5, 10], counts: [0, 50, 50, 0, 0] },
      { bounds: [1, 2, 5, 10], counts: [0, 30, 50, 20, 0] },
    ])
    expect(merged.bounds).toEqual([1, 2, 5, 10])
    expect(merged.counts).toEqual([0, 80, 100, 20, 0])
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

describe('downscaleExpBuckets with an empty bucket array', () => {
  // The offset of an empty array is still a *source-scale* index. Returned
  // unrescaled it wins the min() that picks the alignment point, and the merge
  // then zero-pads out to an index no real bucket occupies -- correct counts,
  // unbounded memory.
  it('rescales the offset even when there are no counts', () => {
    expect(downscaleExpBuckets([], -7, 5)).toEqual({ offset: -1, counts: [] })
    // ...matching what a non-empty array at the same offset and scale gives.
    expect(downscaleExpBuckets([10, 20], -7, 5).offset).toBe(-1)
  })

  it('leaves the offset alone when there is no rescale to do', () => {
    expect(downscaleExpBuckets([], -7, 0)).toEqual({ offset: -7, counts: [] })
  })

  it('does not pad a merge out to an empty stream stale offset', () => {
    const merged = mergeExpHistogramStreams(
      [
        {
          scale: 5,
          zeroCount: 0,
          zeroThreshold: 0,
          positiveBucketOffset: -7,
          positiveBucketCounts: [],
          negativeBucketOffset: 0,
          negativeBucketCounts: [],
        },
        {
          scale: 0,
          zeroCount: 0,
          zeroThreshold: 0,
          positiveBucketOffset: 0,
          positiveBucketCounts: [1, 2],
          negativeBucketOffset: 0,
          negativeBucketCounts: [],
        },
      ],
      { count: 3, sum: 3, min: 1, max: 2 }
    )
    // Before the fix this returned offset -7 and nine buckets, seven of them
    // padding. The real data occupies two.
    expect(merged.positiveBucketCounts).toEqual([1, 2])
    expect(merged.positiveBucketOffset).toBe(0)
  })
})
