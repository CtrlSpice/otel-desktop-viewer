import { describe, expect, it } from 'vitest'
import {
  metricTypeBadgeTone,
  metricTypeSeriesColor,
  metricTypeStem,
} from './metric-type'

describe('metric type lookups', () => {
  it('returns the configured metric stems and badge tones', () => {
    expect(metricTypeStem('Gauge')).toBe('foam')
    expect(metricTypeBadgeTone('Gauge')).toBe('badge-info')
    expect(metricTypeStem('Sum')).toBe('pine')
    expect(metricTypeBadgeTone('Sum')).toBe('badge-secondary')
    expect(metricTypeStem('Histogram')).toBe('rose')
    expect(metricTypeBadgeTone('Histogram')).toBe('badge-rose')
    expect(metricTypeStem('ExponentialHistogram')).toBe('gold')
    expect(metricTypeBadgeTone('ExponentialHistogram')).toBe('badge-warning')
  })

  it('uses neutral fallbacks for unknown and prototype-named types', () => {
    for (const metricType of ['Empty', 'love', 'toString', 'constructor']) {
      expect(metricTypeStem(metricType)).toBe('foam')
      expect(metricTypeBadgeTone(metricType)).toBe('badge-neutral')
      expect(metricTypeSeriesColor(metricType)).toBe('var(--color-neutral)')
    }
  })
})
