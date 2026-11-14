import { describe, expect, it } from 'vitest'
import { sparklineGeometry } from './sparkline-geometry'

describe('sparkline geometry', () => {
  it('keeps gaps and source coordinates while producing stroke-width clip openings', () => {
    const points = Object.freeze(
      [0, 990, 2010, 64000].map((time, index) => {
        const point = { date: new Date(time), value: 0 }
        return Object.freeze(
          index === 2 ? { ...point, breakBefore: true as const } : point
        )
      })
    )
    const result = sparklineGeometry(points, 96, 18, 1.5)
    expect(result.path).toBe('M0,9L1.485,9M3.015,9L96,9')
    expect(result.clips[1]!.x - result.clips[0]!.width).toBe(1.5)
    expect(sparklineGeometry(points, 96, 18, 1.5)).toEqual(result)
    expect(points.map(point => point.date.getTime())).toEqual([
      0, 990, 2010, 64000,
    ])
  })

  it('fits negative values within the stroke inset and breaks across non-finite samples', () => {
    const points = [-5, 5, NaN, -5].map((value, i) => ({
      date: new Date(i),
      value,
    }))
    expect(sparklineGeometry(points, 96, 18, 2).path).toBe(
      'M0,17L32,1M96,17l0,0'
    )
  })

  it('handles empty, singleton, flat, and coincident-time previews', () => {
    expect(sparklineGeometry([], 96, 18, 1.5)).toEqual({ path: '', clips: [] })
    const point = { date: new Date(1), value: 0 }
    expect(sparklineGeometry([point], 96, 18, 1.5).path).toBe('M 0 9 L 96 9')
    expect(sparklineGeometry([point, point], 96, 18, 1.5).path).toBe('M0,9L0,9')
  })
})
