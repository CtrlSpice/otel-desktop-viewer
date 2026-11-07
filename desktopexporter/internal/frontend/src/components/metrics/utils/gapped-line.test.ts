import { describe, expect, it } from 'vitest'
import type { ChartPoint } from '@/types/metric-chart-types'
import { gappedLineClips, gappedLinePath, insideLineGap } from './gapped-line'

const x = (date: Date) => date.getTime()
const y = (value: number) => value
const points: ChartPoint[] = [
  { date: new Date(0), value: 0 },
  { date: new Date(10), value: 42 },
  { date: new Date(40), value: 45, breakBefore: true },
  { date: new Date(50), value: 0 },
]

describe('gap-preserving lines', () => {
  it('keeps both boundary points but never connects across the gap', () => {
    expect(gappedLinePath(points, x, y)).toBe('M0,0L10,42M40,45L50,0')
    expect(points.map(p => p.date.getTime())).toEqual([0, 10, 40, 50])
  })

  it('keeps isolated samples visible as zero-length round-capped strokes', () => {
    expect(gappedLinePath(points.slice(1, 3), x, y)).toBe(
      'M10,42l0,0M40,45l0,0'
    )
  })

  it('leaves a stroke-width opening when gap endpoints project less than a pixel apart', () => {
    const clips = gappedLineClips(points, date => x(date) / 100, 1.75, -2, 3)
    expect(clips).toEqual([
      { x: -2, width: 1.375 },
      { x: 1.125, width: 1.875 },
    ])
    expect(clips[1]!.x - (clips[0]!.x + clips[0]!.width)).toBe(1.75)
  })

  it('coalesces overlapping minimum-width openings without negative rectangles', () => {
    const dense = [0, 1, 2, 10].map((time, i): ChartPoint => {
      const point: ChartPoint = { date: new Date(time), value: time }
      if (i === 1 || i === 2) point.breakBefore = true
      return point
    })
    expect(gappedLineClips(dense, x, 4, 0, 10)).toEqual([
      { x: 3.5, width: 6.5 },
    ])
  })

  it('allows exact endpoints and continuous intervals, but not times inside a gap', () => {
    for (const time of [0, 5, 10, 40, 45, 50])
      expect(insideLineGap(points, new Date(time))).toBe(false)
    for (const time of [11, 25, 39])
      expect(insideLineGap(points, new Date(time))).toBe(true)
  })

  it('does not turn zero into a gap and still breaks on non-finite values', () => {
    expect(
      gappedLinePath(
        [
          { date: new Date(0), value: 0 },
          { date: new Date(1), value: 0 },
          { date: new Date(2), value: NaN },
          { date: new Date(3), value: 0 },
        ],
        x,
        y
      )
    ).toBe('M0,0L1,0M3,0l0,0')
  })
})
