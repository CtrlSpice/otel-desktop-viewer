import { describe, expect, it } from 'vitest'
import {
  exponentialBucketIdentity,
  exponentialZeroBucketIdentity,
  exponentialZeroBucketLabel,
  formatHistogramBound,
  histogramRangeIdentity,
  indexedHistogramRangeIdentity,
  numberIdentity,
} from './histogram-bucket'

describe('histogram bucket presentation', () => {
  it('keeps special numbers stable in identities', () => {
    expect([
      numberIdentity(Number.NaN),
      numberIdentity(-0),
      numberIdentity(Infinity),
      numberIdentity(-Infinity),
    ]).toEqual(['nan', '-0', '+infinity', '-infinity'])
  })

  it('formats finite and unbounded histogram values', () => {
    expect([
      formatHistogramBound(-Infinity),
      formatHistogramBound(0),
      formatHistogramBound(0.001),
      formatHistogramBound(1.234),
      formatHistogramBound(Infinity),
    ]).toEqual(['-∞', '0', '1.0e-3', '1.23', '+∞'])
    expect(exponentialZeroBucketLabel(0.001)).toBe('[-1.0e-3, +1.0e-3]')
    expect(exponentialZeroBucketLabel(0)).toBe('0')
  })

  it('distinguishes logical and index-preserving bucket identities', () => {
    expect(histogramRangeIdentity(1, 1)).toBe('explicit:1:1')
    expect(indexedHistogramRangeIdentity(2, 1, 1)).toBe('explicit:2:1:1')
    expect(exponentialBucketIdentity(3, 'negative', -2)).toBe(
      'exponential:3:negative:-2'
    )
    expect(exponentialZeroBucketIdentity(0.001)).toBe('exponential:zero:0.001')
  })
})
