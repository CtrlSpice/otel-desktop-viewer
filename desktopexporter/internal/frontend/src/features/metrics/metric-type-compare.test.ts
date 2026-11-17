import { describe, expect, it } from 'vitest'
import type { MetricSummary } from '@/types/api-types'
import { compareMetricType } from './metric-type-compare'

const sum = (
  aggregationTemporalityCode: number | null,
  isMonotonic: boolean | null = null
) => ({ metricType: 'Sum' as const, aggregationTemporalityCode, isMonotonic })

describe('metric Type axes', () => {
  it('orders Delta, Cumulative, explicit Unspecified and missing distinctly', () => {
    const sorted = [sum(null), sum(0), sum(2), sum(1)].sort(compareMetricType)
    expect(sorted.map(metric => metric.aggregationTemporalityCode)).toEqual([
      1,
      2,
      0,
      null,
    ])
    expect(
      [...sorted]
        .sort((a, b) => -compareMetricType(a, b))
        .map(metric => metric.aggregationTemporalityCode)
    ).toEqual([null, 0, 2, 1])
  })

  it('keeps false, true and absent monotonicity separate for Sum only', () => {
    expect(
      [sum(1), sum(1, true), sum(1, false)]
        .sort(compareMetricType)
        .map(metric => metric.isMonotonic)
    ).toEqual([false, true, null])
    const histogram = { ...sum(1, true), metricType: 'Histogram' as const }
    expect(
      compareMetricType(histogram, { ...histogram, isMonotonic: false })
    ).toBe(0)
  })

  it('ignores inapplicable Gauge axes and compares actual types first', () => {
    const gauge = { ...sum(1, true), metricType: 'Gauge' as const }
    expect(
      compareMetricType(gauge, {
        ...gauge,
        aggregationTemporalityCode: null,
        isMonotonic: false,
      })
    ).toBe(0)
    expect(compareMetricType(gauge, sum(1))).toBeLessThan(0)
  })

  it('treats runtime missing axes as missing rather than zero or false', () => {
    const missing = { metricType: 'Sum' } as MetricSummary
    expect(compareMetricType(missing, sum(0, false))).toBeGreaterThan(0)
  })
})
