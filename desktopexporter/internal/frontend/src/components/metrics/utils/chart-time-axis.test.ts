import { describe, expect, it } from 'vitest'
import { normalizeTimezone } from '@/utils/time'
import { formatChartAxisTime, getChartTimeRangeLabels } from './chart-time-axis'

const newYork = normalizeTimezone('America/New_York')
if (!newYork) throw new Error('Test runtime lacks America/New_York')

describe('named-zone chart time labels', () => {
  it('formats axis ticks in the selected named timezone', () => {
    expect(formatChartAxisTime(new Date('2024-01-15T08:30:00Z'), newYork)).toBe(
      '03:30:00'
    )
  })

  it('compares calendar days in the selected named timezone', () => {
    const labels = getChartTimeRangeLabels(
      new Date('2024-01-15T01:00:00Z').getTime(),
      new Date('2024-01-15T03:00:00Z').getTime(),
      newYork
    )
    expect(labels).toEqual({ start: 'January 14, 2024' })
  })
})
