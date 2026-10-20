import { describe, expect, it } from 'vitest'
import { metricTypeSummary } from './metric-type'

describe('metric type summaries', () => {
  it.each([
    ['Gauge', 'Gauge'],
    ['Sum', 'Sum'],
    ['Histogram', 'Histogram'],
    ['ExponentialHistogram', 'ExpHist'],
    ['Empty', 'Empty'],
    ['Unknown', 'Unknown'],
    ['constructor', 'constructor'],
  ])('labels %s as %s', (metricType, label) => {
    expect(metricTypeSummary(metricType, null, false)).toEqual({
      label,
      title: metricType,
    })
  })

  it('retains unspecified temporality and monotonicity', () => {
    const summary = metricTypeSummary('Sum', 'Unspecified', true)
    expect(summary.title).toContain('aggregationTemporality = Unspecified')
    expect(summary.label).toBe('Sum Unspecified ↗')
  })

  it('retains received type and temporality labels', () => {
    expect(
      metricTypeSummary('ExponentialHistogram', 'Delta', false)
    ).toMatchObject({
      label: 'ExpHist Δ',
      title: 'ExponentialHistogram · Delta',
    })
  })
})
