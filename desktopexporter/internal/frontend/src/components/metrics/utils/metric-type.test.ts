import { describe, expect, it } from 'vitest'
import { metricTypeCardBadge } from './metric-type'

describe('metric type badges', () => {
  it.each([
    ['Gauge', 'badge-info'],
    ['Sum', 'badge-secondary'],
    ['Histogram', 'badge-rose'],
    ['ExponentialHistogram', 'badge-warning'],
  ])('colours the %s type badge with %s', (metricType, tone) => {
    expect(metricTypeCardBadge(metricType, null, false).className).toContain(
      tone
    )
  })

  it.each(['Empty', 'Unknown', 'constructor'])(
    'keeps %s neutral',
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
