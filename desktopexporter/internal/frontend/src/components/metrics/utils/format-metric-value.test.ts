import { describe, expect, it } from 'vitest'
import {
  formatMetricValue,
  formatMetricValuePlain,
  formatRateSlopeValue,
  formatMetricHoverValue,
  formatMetricCount,
  metricNumberPrecision,
} from './format-metric-value'

describe('special Metric values', () => {
  it.each([
    [-0, '-0'],
    [0, '0'],
    [NaN, 'NaN'],
    [Infinity, 'Infinity'],
    [-Infinity, '-Infinity'],
  ])('keeps %s readable as %s in compact and detail labels', (value, label) => {
    expect(formatMetricValue(value)).toBe(label)
    expect(formatMetricValuePlain(value)).toBe(label)
  })

  it('keeps the sign when adding a unit or formatting a rate slope', () => {
    expect(formatMetricValuePlain(-0, { unit: 'ms' })).toBe('-0 ms')
    expect(formatMetricValuePlain(-0, { unit: '1' })).toBe('-0')
    expect(formatRateSlopeValue(-0, 'By')).toBe('-0 By/s²')
  })

  it('keeps ordinary finite formatting and absent values', () => {
    expect(formatMetricValue(1500)).toBe('1,500')
    expect(formatMetricValuePlain(1.25, { unit: 'ms' })).toBe('1.25 ms')
    expect(formatMetricValue(null)).toBe('')
    expect(formatMetricValuePlain(undefined)).toBe('')
    expect(formatMetricValuePlain(1.23e30)).toBe('1.23e+30')
  })
})

describe('shared graph number format', () => {
  it.each([
    [1250, '1,250'],
    [0.0004, '0.0004'],
    [0.000001, '0.000001'],
    [0.000000999, '9.99 × 10⁻⁷'],
    [999000, '999,000'],
    [999999, '1 × 10⁶'],
    [1289254762, '1.29 × 10⁹'],
    [-1289254762, '-1.29 × 10⁹'],
    [1.2e-20, '1.2 × 10⁻²⁰'],
    [1.2e30, '1.2 × 10³⁰'],
    [Number.MIN_VALUE, '4.94 × 10⁻³²⁴'],
    [Number.MAX_VALUE, '1.8 × 10³⁰⁸'],
  ])('formats %s as %s without changing its unit', (value, expected) => {
    expect(formatMetricValue(value)).toBe(expected)
  })

  it('uses six significant digits on hover and groups integer counts exactly', () => {
    expect(formatMetricHoverValue(1289254762)).toBe('1.28925 × 10⁹')
    expect(formatMetricHoverValue(0.123456789)).toBe('0.123457')
    expect(formatMetricCount(1234567)).toBe('1,234,567')
    expect(formatMetricCount(18446744073709551615n)).toBe('18,446,744,073,709,551,615')
  })

  it('increases precision for distinct close bounds, without counting repeated bounds', () => {
    const values = [1.00001, 1.00002, 1.00002]
    const digits = metricNumberPrecision(values)
    expect(digits).toBe(6)
    expect(values.map(value => formatMetricValue(value, { maxSignificantDigits: digits })))
      .toEqual(['1.00001', '1.00002', '1.00002'])
    expect(metricNumberPrecision([1, 1 + Number.EPSILON])).toBe(17)
    expect(metricNumberPrecision([1, 2, Infinity])).toBe(3)
  })
})
