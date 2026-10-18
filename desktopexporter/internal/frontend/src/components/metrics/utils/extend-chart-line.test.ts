import { describe, expect, it } from 'vitest'
import { extendChartLine } from './extend-chart-line'

const bounds = { left: 0, right: 100, top: 0, bottom: 80 }

describe('extendChartLine', () => {
  it('extends a rising guide in both directions to the plot edges', () => {
    expect(extendChartLine({ x: 40, y: 50 }, { x: 60, y: 30 }, bounds)).toEqual(
      { x1: 10, y1: 80, x2: 90, y2: 0 }
    )
  })
  it('extends a falling guide without reversing its direction', () => {
    expect(extendChartLine({ x: 40, y: 30 }, { x: 60, y: 50 }, bounds)).toEqual(
      { x1: 10, y1: 0, x2: 90, y2: 80 }
    )
  })
  it('handles zero slope and coincident screen-space timestamps', () => {
    expect(extendChartLine({ x: 40, y: 30 }, { x: 60, y: 30 }, bounds)).toEqual(
      { x1: 0, y1: 30, x2: 100, y2: 30 }
    )
    expect(extendChartLine({ x: 40, y: 50 }, { x: 40, y: 30 }, bounds)).toEqual(
      { x1: 40, y1: 80, x2: 40, y2: 0 }
    )
  })
  it('does not draw a guide for a collapsed segment, invalid geometry, or a line outside the plot', () => {
    expect(
      extendChartLine({ x: 40, y: 30 }, { x: 40, y: 30 }, bounds)
    ).toBeNull()
    expect(
      extendChartLine({ x: NaN, y: 30 }, { x: 60, y: 50 }, bounds)
    ).toBeNull()
    expect(
      extendChartLine({ x: 40, y: 100 }, { x: 60, y: 100 }, bounds)
    ).toBeNull()
    expect(
      extendChartLine(
        { x: 40, y: 30 },
        { x: 60, y: 50 },
        { ...bounds, right: 0 }
      )
    ).toBeNull()
  })
})
