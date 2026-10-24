import { describe, expect, it } from 'vitest'
import { chartStickyPosition } from './chart-sticky-position'

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
})
