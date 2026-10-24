import { describe, expect, it } from 'vitest'
import {
  heatmapCardPosition,
  type HeatmapCardObstacle,
} from './heatmap-card-position'

const size = {
  width: 600,
  height: 300,
  cardWidth: 100,
  cardHeight: 40,
  columnLeft: 250,
  columnWidth: 20,
  plot: { left: 20, top: 20, width: 560, height: 260 },
}

function overlap(
  position: { left: number; top: number },
  cells: HeatmapCardObstacle[]
) {
  return cells.reduce(
    (sum, cell) =>
      sum +
      Math.max(
        0,
        Math.min(position.left + size.cardWidth, cell.left + cell.width) -
          Math.max(position.left, cell.left)
      ) *
        Math.max(
          0,
          Math.min(position.top + size.cardHeight, cell.top + cell.height) -
            Math.max(position.top, cell.top)
        ),
    0
  )
}

describe('heatmap card placement', () => {
  it('stays at the selected cell height even when another row is empty', () => {
    const cells = [{ left: 270, top: 20, width: 310, height: 100 }]
    const position = heatmapCardPosition({ ...size, cells, anchorY: 60 })
    expect(position).toEqual({ left: 138, top: 40 })
    expect(heatmapCardPosition({ ...size, cells, anchorY: 275 }).top).toBe(240)
  })

  it('moves into an empty row beside the column without covering data', () => {
    const cells = [
      { left: 20, top: 20, width: 560, height: 100 },
      { left: 20, top: 160, width: 560, height: 120 },
    ]
    const position = heatmapCardPosition({ ...size, cells })
    expect(position).toEqual({ left: 282, top: 120 })
    expect(overlap(position, cells)).toBe(0)
  })

  it('uses the quieter side but stays immediately beside its column', () => {
    const cells = [{ left: 270, top: 20, width: 310, height: 260 }]
    const position = heatmapCardPosition({ ...size, cells })
    expect(position.left + size.cardWidth).toBe(size.columnLeft - 12)
    expect(position.top).toBe(130)
    expect(overlap(position, cells)).toBe(0)
  })

  it('does not wander to a distant free corner when both adjacent sides are busy', () => {
    const cells = [{ left: 130, top: 20, width: 260, height: 260 }]
    const position = heatmapCardPosition({ ...size, cells })
    expect(position).toEqual({ left: 282, top: 130 })
  })

  it('chooses the least covered row when no empty area is large enough', () => {
    const cells = [
      { left: 20, top: 20, width: 560, height: 100 },
      { left: 282, top: 120, width: 10, height: 40 },
      { left: 138, top: 120, width: 20, height: 40 },
      { left: 20, top: 160, width: 560, height: 120 },
    ]
    const position = heatmapCardPosition({ ...size, cells })
    expect(position).toEqual({ left: 282, top: 120 })
    expect(overlap(position, cells)).toBe(400)
  })

  it('flips at the right edge and clamps around the column in narrow panes', () => {
    expect(
      heatmapCardPosition({ ...size, columnLeft: 550, cells: [] }).left
    ).toBe(438)
    const position = heatmapCardPosition({
      ...size,
      width: 180,
      cardWidth: 160,
      columnLeft: 80,
      plot: { left: 20, top: 20, width: 140, height: 260 },
      cells: [],
    })
    expect(position.left).toBe(12)
    expect(position.left + 160).toBeLessThanOrEqual(180 - 8)
  })
})
