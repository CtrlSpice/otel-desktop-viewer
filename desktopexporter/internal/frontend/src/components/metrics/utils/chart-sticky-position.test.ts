import { describe, expect, it } from 'vitest'
import {
  chartStickyPosition,
  type ChartObstacle,
} from './chart-sticky-position'

function overlap(a: ChartObstacle, b: ChartObstacle) {
  return (
    Math.max(
      0,
      Math.min(a.left + a.width, b.left + b.width) - Math.max(a.left, b.left)
    ) *
    Math.max(
      0,
      Math.min(a.top + a.height, b.top + b.height) - Math.max(a.top, b.top)
    )
  )
}

describe('chartStickyPosition', () => {
  it('puts the card beside the selected value when there is room', () => {
    expect(chartStickyPosition({ x: 200, y: 180 }, 800, 500, 180, 80)).toEqual({
      left: 212,
      top: 140,
    })
  })
  it('flips left near the right edge', () => {
    expect(chartStickyPosition({ x: 780, y: 180 }, 800, 500, 180, 80)).toEqual({
      left: 588,
      top: 140,
    })
  })
  it('uses vertical room when neither side fits', () => {
    expect(chartStickyPosition({ x: 150, y: 100 }, 300, 500, 200, 80)).toEqual({
      left: 50,
      top: 112,
    })
    expect(chartStickyPosition({ x: 150, y: 480 }, 300, 500, 200, 80)).toEqual({
      left: 50,
      top: 388,
    })
  })
  it.each([
    { x: 0, y: 0 },
    { x: 800, y: 500 },
    { x: -200, y: 900 },
  ])('keeps the card inside the plot at %j', anchor => {
    const { left, top } = chartStickyPosition(anchor, 800, 500, 180, 80)
    expect(left).toBeGreaterThanOrEqual(8)
    expect(top).toBeGreaterThanOrEqual(8)
    expect(left + 180).toBeLessThanOrEqual(792)
    expect(top + 80).toBeLessThanOrEqual(492)
  })
  it('falls back to a safe corner without a plotted mark', () => {
    expect(chartStickyPosition(null, 800, 500, 180, 80)).toEqual({
      left: 612,
      top: 8,
    })
    expect(chartStickyPosition({ x: NaN, y: 2 }, 800, 500, 180, 80)).toEqual({
      left: 612,
      top: 8,
    })
  })
  it('keeps a non-negative inset when the card exceeds the available space', () => {
    expect(chartStickyPosition({ x: 10, y: 10 }, 20, 20, 200, 100)).toEqual({
      left: 8,
      top: 8,
    })
  })

  it.each([180, 260])(
    'avoids a measured minimum label and selected marker with a %ipx card',
    cardWidth => {
      const obstacles = [
        { left: 150, top: 188, width: 170, height: 28 },
        { left: 193, top: 173, width: 14, height: 14 },
      ]
      const preferred = chartStickyPosition(
        { x: 200, y: 180 },
        800,
        500,
        cardWidth,
        80
      )
      expect(
        overlap({ ...preferred, width: cardWidth, height: 80 }, obstacles[0]!)
      ).toBeGreaterThan(0)
      const placed = chartStickyPosition(
        { x: 200, y: 180 },
        800,
        500,
        cardWidth,
        80,
        obstacles
      )
      for (const obstacle of obstacles)
        expect(
          overlap({ ...placed, width: cardWidth, height: 80 }, obstacle)
        ).toBe(0)
      expect(placed.left).toBeGreaterThanOrEqual(8)
      expect(placed.left + cardWidth).toBeLessThanOrEqual(792)
      expect(placed.top).toBeGreaterThanOrEqual(8)
      expect(placed.top + 80).toBeLessThanOrEqual(492)
    }
  )

  it('retains its preferred position when nearby labels do not overlap', () => {
    const anchor = { x: 200, y: 180 }
    expect(
      chartStickyPosition(anchor, 800, 500, 180, 80, [
        { left: 400, top: 300, width: 80, height: 24 },
      ])
    ).toEqual(chartStickyPosition(anchor, 800, 500, 180, 80))
  })

  it('accepts local overlap instead of escaping across a crowded chart', () => {
    const anchor = { x: 400, y: 250 }
    const obstacle = { left: 150, top: 100, width: 500, height: 300 }
    const placed = chartStickyPosition(anchor, 1000, 700, 180, 80, [obstacle])
    expect(
      overlap({ ...placed, width: 180, height: 80 }, obstacle)
    ).toBeGreaterThan(0)
    expect(
      Math.hypot(
        Math.max(placed.left - anchor.x, anchor.x - placed.left - 180, 0),
        Math.max(placed.top - anchor.y, anchor.y - placed.top - 80, 0)
      )
    ).toBeLessThanOrEqual(48)
  })

  it('minimizes overlap deterministically when no clear position fits', () => {
    const obstacle = { left: 8, top: 8, width: 250, height: 284 }
    const anchor = { x: 68, y: 150 }
    const preferred = chartStickyPosition(anchor, 300, 300, 180, 80)
    const placed = chartStickyPosition(anchor, 300, 300, 180, 80, [obstacle])
    expect(
      overlap({ ...placed, width: 180, height: 80 }, obstacle)
    ).toBeLessThan(overlap({ ...preferred, width: 180, height: 80 }, obstacle))
    expect(placed).toEqual(
      chartStickyPosition(anchor, 300, 300, 180, 80, [obstacle])
    )
    expect(placed.left + 180).toBeLessThanOrEqual(292)
  })
})
