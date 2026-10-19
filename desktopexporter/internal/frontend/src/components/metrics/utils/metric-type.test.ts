import { describe, expect, it } from 'vitest'
import { metricTypeCardBadge } from './metric-type'

describe('metric type badges', () => {
  it.each(['Gauge', 'Sum', 'Histogram', 'ExponentialHistogram', 'Empty'])(
    'keeps %s neutral rather than implying a series colour',
    metricType => {
      expect(metricTypeCardBadge(metricType, null, false).className).toContain(
        'badge-neutral'
      )
    }
  )

  it('retains semantic error colour for unspecified temporality', () => {
    const badge = metricTypeCardBadge('Sum', 'Unspecified', true)
    expect(badge.className).toContain('badge-error')
    expect(badge.label).toBe('Sum Unspecified ↗')
  })

  it('retains received type and temporality labels', () => {
    expect(
      metricTypeCardBadge('ExponentialHistogram', 'Delta', false)
    ).toMatchObject({
      label: 'ExpHist Δ',
      title: 'ExponentialHistogram · Delta',
    })
  })
})
