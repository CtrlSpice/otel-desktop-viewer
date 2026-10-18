import type { ChartStickySize } from './chart-sticky-position'

export type HeatmapCardObstacle = {
  left: number
  top: number
  width: number
  height: number
}

/** Choose a quiet vertical position immediately beside the selected time column. */
export function heatmapCardPosition({
  width,
  height,
  cardWidth,
  cardHeight,
  columnLeft,
  columnWidth,
  plot,
  cells,
  anchorY,
}: ChartStickySize & {
  columnLeft: number
  columnWidth: number
  plot: HeatmapCardObstacle
  cells: readonly HeatmapCardObstacle[]
  /** A picked cell fixes the vertical anchor; only the side may change. */
  anchorY?: number
}) {
  const inset = 8
  const gap = 12
  const minLeft = Math.min(
    Math.max(inset, plot.left),
    Math.max(inset, width - cardWidth - inset)
  )
  const maxLeft = Math.max(
    minLeft,
    Math.min(width - cardWidth - inset, plot.left + plot.width - cardWidth)
  )
  const minTop = Math.min(
    Math.max(inset, plot.top),
    Math.max(inset, height - cardHeight - inset)
  )
  const maxTop = Math.max(
    minTop,
    Math.min(height - cardHeight - inset, plot.top + plot.height - cardHeight)
  )
  const clampTop = (top: number) => Math.max(minTop, Math.min(maxTop, top))
  const preferredTop = clampTop(
    (anchorY ?? plot.top + plot.height / 2) - cardHeight / 2
  )
  const sides = [
    columnLeft + columnWidth + gap,
    columnLeft - gap - cardWidth,
  ].filter(left => left >= minLeft && left <= maxLeft)
  if (sides.length === 0) {
    sides.push(
      Math.max(
        minLeft,
        Math.min(maxLeft, columnLeft + (columnWidth - cardWidth) / 2)
      )
    )
  }

  let best = { left: sides[0]!, top: preferredTop }
  let bestOverlap = Infinity
  let bestDistance = Infinity
  for (const left of sides) {
    // Collapse equal grid rows before evaluating positions, keeping work
    // independent of the number of time columns during the vertical search.
    const rows = new Map<
      number,
      { top: number; height: number; width: number }
    >()
    for (const cell of cells) {
      const overlapWidth = Math.max(
        0,
        Math.min(left + cardWidth, cell.left + cell.width) -
          Math.max(left, cell.left)
      )
      if (overlapWidth === 0) continue
      const row = rows.get(cell.top)
      if (row) row.width += overlapWidth
      else
        rows.set(cell.top, {
          top: cell.top,
          height: cell.height,
          width: overlapWidth,
        })
    }
    const candidates = new Set(
      anchorY === undefined ? [preferredTop, minTop, maxTop] : [preferredTop]
    )
    for (const row of anchorY === undefined ? rows.values() : []) {
      candidates.add(clampTop(row.top - cardHeight))
      candidates.add(clampTop(row.top))
      candidates.add(clampTop(row.top + row.height - cardHeight))
      candidates.add(clampTop(row.top + row.height))
      candidates.add(clampTop(row.top - cardHeight - 4))
      candidates.add(clampTop(row.top + row.height + 4))
      candidates.add(clampTop(row.top + (row.height - cardHeight) / 2))
    }
    for (const top of candidates) {
      let overlap = 0
      for (const row of rows.values()) {
        overlap +=
          row.width *
          Math.max(
            0,
            Math.min(top + cardHeight, row.top + row.height) -
              Math.max(top, row.top)
          )
      }
      const distance = Math.abs(top - preferredTop)
      if (
        overlap < bestOverlap ||
        (overlap === bestOverlap && distance < bestDistance)
      ) {
        best = { left, top }
        bestOverlap = overlap
        bestDistance = distance
      }
    }
  }
  return best
}
