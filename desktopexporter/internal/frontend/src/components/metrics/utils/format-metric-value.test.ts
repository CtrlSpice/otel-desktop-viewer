import { describe, expect, it } from 'vitest'
import {
  formatMetricValue,
  formatMetricValuePlain,
  formatRateSlopeValue,
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
    expect(formatMetricValue(1500)).toBe('1.5k')
    expect(formatMetricValuePlain(1.25, { unit: 'ms' })).toBe('1.25 ms')
    expect(formatMetricValue(null)).toBe('')
    expect(formatMetricValuePlain(undefined)).toBe('')
  })
})
