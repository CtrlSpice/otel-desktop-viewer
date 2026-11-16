import { describe, expect, it } from 'vitest'
import { histogramCardPosition } from './histogram-card-position'

const size = {
  width: 600,
  height: 320,
  cardWidth: 100,
  cardHeight: 50,
  plot: { left: 20, top: 20, width: 560, height: 260 },
}
const selected = { left: 250, top: 150, width: 30, height: 130 }

function covers(position: { left: number; top: number }, bar: typeof selected) {
  return (
    position.left < bar.left + bar.width &&
    position.left + size.cardWidth > bar.left &&
    position.top < bar.top + bar.height &&
    position.top + size.cardHeight > bar.top
  )
}

describe('histogram pinned card positioning', () => {
  it('sits above an isolated bar with clearance from its measured height', () => {
    const position = histogramCardPosition({
      ...size,
      selected,
      bars: [selected],
    })
    expect(position.left + size.cardWidth / 2).toBe(
      selected.left + selected.width / 2
    )
    expect(position.top + size.cardHeight).toBe(selected.top - 12)
  })

  it('chooses the clear side when a tall neighbour blocks the other side', () => {
    const tall = { left: 150, top: 20, width: 95, height: 260 }
    const bars = [selected, tall]
    const position = histogramCardPosition({ ...size, selected, bars })
    expect(position.left).toBe(selected.left + selected.width + 12)
    expect(bars.some(bar => covers(position, bar))).toBe(false)
  })

  it('moves above adjacent bars when neither side has clear space at the anchor', () => {
    const bars = [
      selected,
      { left: 20, top: 100, width: 225, height: 180 },
      { left: 285, top: 100, width: 295, height: 180 },
    ]
    const position = histogramCardPosition({ ...size, selected, bars })
    expect(bars.some(bar => covers(position, bar))).toBe(false)
    expect(position.top + size.cardHeight).toBeLessThanOrEqual(88)
  })

  it('stays inside a narrow scrolled viewport when no clear placement exists', () => {
    const position = histogramCardPosition({
      ...size,
      width: 180,
      cardWidth: 160,
      plot: { left: -180, top: 20, width: 600, height: 260 },
      selected: { ...selected, left: 145, top: 20, height: 260 },
      bars: [{ left: -180, top: 20, width: 600, height: 260 }],
    })
    expect(position.left).toBeGreaterThanOrEqual(8)
    expect(position.left + 160).toBeLessThanOrEqual(172)
    expect(position.top).toBeGreaterThanOrEqual(20)
    expect(position.top + size.cardHeight).toBeLessThanOrEqual(280)
  })
})
