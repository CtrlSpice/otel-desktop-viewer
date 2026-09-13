import { describe, expect, it } from 'vitest'
import { metricTypeBadgeTone, metricTypeStem } from './metric-type'

describe('metric type lookups', () => {
  it('returns configured stems and badge tones', () => {
    expect(metricTypeStem('Sum')).toBe('pine')
    expect(metricTypeBadgeTone('Histogram')).toBe('badge-rose')
  })

  it('uses neutral fallbacks for unknown and prototype-named types', () => {
    for (const metricType of ['Empty', 'toString', 'constructor']) {
      expect(metricTypeStem(metricType)).toBe('foam')
      expect(metricTypeBadgeTone(metricType)).toBe('badge-neutral')
    }
  })
})
