import { describe, expect, it } from 'vitest'
import type { ChartPoint, ChartTimeseries } from '@/types/metric-chart-types'
import { AGG_KEY_ALL } from './aggregation'
import {
  hoveredPointAtPointer,
  nearestPointAt,
  pointAtPointer,
  selectablePointAtPointer,
  seriesKeyAtValue,
  seriesKeyForChartPoint,
  sourcePointForChartPoint,
} from './line-hit-testing'

const scales = {
  x: (date: Date) => date.getTime() * 100,
  y: (value: number) => value,
}
const points = Object.freeze(
  [0, 1, 3, 4].map((time, i): ChartPoint => {
    const point = {
      date: new Date(time),
      value: 0,
      sourceDatapointID: `point-${i}`,
    }
    return Object.freeze(
      i === 2 ? { ...point, breakBefore: true as const } : point
    )
  })
)
const lines = Object.freeze([{ key: 'raw', data: points }])

describe('line hit testing without component state', () => {
  it('rejects a fractional gap interior even when the tooltip date snaps to an endpoint', () => {
    const pointer = { x: 150, y: 0 }
    expect(pointAtPointer(points, pointer, scales, new Date(1))).toBeUndefined()
    expect(
      hoveredPointAtPointer(lines, pointer, scales, new Date(1))
    ).toBeNull()
    expect(
      selectablePointAtPointer(lines, pointer, scales, 'source')
    ).toBeNull()
  })

  it('lets either boundary be hit from its gap-facing side, but not outside the circle', () => {
    for (const [x, point] of [
      [105, points[1]!],
      [295, points[2]!],
    ] as const) {
      expect(pointAtPointer(points, { x, y: 0 }, scales, new Date(1))).toBe(
        point
      )
      expect(
        selectablePointAtPointer(lines, { x, y: 0 }, scales, 'source')?.point
      ).toBe(point)
    }
    expect(
      pointAtPointer(points, { x: 105, y: 5 }, scales, new Date(1))
    ).toBeUndefined()
    expect(
      selectablePointAtPointer(lines, { x: 105, y: 5 }, scales, 'source')
    ).toBeNull()
  })

  it('uses each line’s timestamp grid and preserves first-series ties', () => {
    const other = {
      key: 'other',
      data: [{ date: new Date(2), value: 5, sourceDatapointID: 'other' }],
    }
    const candidates = [...lines, other, { ...other, key: 'tied' }]
    expect(
      hoveredPointAtPointer(candidates, { x: 150, y: 5 }, scales, new Date(1))
        ?.seriesKey
    ).toBe('other')
    expect(
      selectablePointAtPointer(candidates, { x: 150, y: 5 }, scales, 'source')
        ?.seriesKey
    ).toBe('other')
    expect(
      selectablePointAtPointer(
        candidates,
        { x: 150, y: 5 },
        scales,
        'source',
        'raw'
      )
    ).toBeNull()
    expect(nearestPointAt(points, new Date(0))).toBe(points[0])
    expect(nearestPointAt(points, new Date(2))).toBeUndefined()
    expect(nearestPointAt([], new Date(0))).toBeUndefined()
  })

  it('separates raw identity, rate bucket identity, and disabled selection', () => {
    const ratePoint = Object.freeze({
      date: new Date(1),
      value: 0,
      timestampNs: 1_000_123n,
    })
    const rateLine = { key: 'rate', data: [ratePoint] }
    const candidates = [{ key: AGG_KEY_ALL, data: points }, rateLine, ...lines]
    const pointer = { x: 100, y: 0 }
    expect(
      selectablePointAtPointer(candidates, pointer, scales, 'source')?.seriesKey
    ).toBe('raw')
    expect(
      selectablePointAtPointer(candidates, pointer, scales, 'rate')?.point
    ).toBe(ratePoint)
    expect(
      selectablePointAtPointer(candidates, pointer, scales, null)
    ).toBeNull()
    expect(
      selectablePointAtPointer(
        candidates,
        pointer,
        scales,
        'source',
        AGG_KEY_ALL
      )
    ).toBeNull()
    expect(
      selectablePointAtPointer(
        [
          {
            key: 'invalid',
            data: [
              { date: new Date(1), value: NaN, sourceDatapointID: 'invalid' },
            ],
          },
        ],
        pointer,
        scales,
        'source'
      )
    ).toBeNull()
  })

  it('disambiguates tooltip series by exact time/value, then value, then time', () => {
    const series: ChartTimeseries[] = [
      {
        key: AGG_KEY_ALL,
        label: '',
        points: [{ date: new Date(2), value: 20 }],
      },
      {
        key: 'a',
        label: '',
        points: [
          { date: new Date(0), value: 0 },
          { date: new Date(2), value: 10 },
        ],
      },
      {
        key: 'b',
        label: '',
        points: [
          { date: new Date(1), value: 20 },
          { date: new Date(2), value: 20 },
        ],
      },
    ]
    expect(
      seriesKeyForChartPoint(series, { date: new Date(2), value: 20 })
    ).toBe('b')
    expect(
      seriesKeyForChartPoint(series, { date: new Date(2), value: 15 })
    ).toBe('a')
    expect(
      seriesKeyForChartPoint(series, { date: new Date(3), value: 19 })
    ).toBe('b')
    expect(
      seriesKeyForChartPoint(series, { date: new Date(1), value: NaN })
    ).toBe('b')
    expect(seriesKeyAtValue(series, new Date(1), 19)).toBe('b')
  })

  it('preserves exact identities when millisecond coordinates collide and rejects ambiguity', () => {
    const sameDate = new Date(1)
    const samples = Object.freeze([
      Object.freeze({
        date: sameDate,
        value: 2,
        sourceDatapointID: 'a',
        timestampNs: 1_000_100n,
      }),
      Object.freeze({
        date: sameDate,
        value: 2,
        sourceDatapointID: 'b',
        timestampNs: 1_000_900n,
      }),
    ])
    expect(sourcePointForChartPoint(samples, samples[1]!)).toBe(samples[1])
    expect(
      sourcePointForChartPoint(samples, {
        date: sameDate,
        value: 2,
        timestampNs: 1_000_100n,
      })
    ).toBe(samples[0])
    expect(
      sourcePointForChartPoint(samples, { date: sameDate, value: 2 })
    ).toBeNull()
    expect(
      sourcePointForChartPoint(samples, {
        date: sameDate,
        value: 2,
        sourceDatapointID: 'missing',
      })
    ).toBeNull()
  })
})
