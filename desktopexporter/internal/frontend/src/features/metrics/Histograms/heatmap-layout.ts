/** Minimum column width (px) — below this we scroll instead of shrinking further. */
export const MIN_HEATMAP_CELL_PX = 8

export type HeatmapLayout = {
  /** Column/cell width in px. */
  cellSize: number
  chartWidth: number
  /** Distance between column origins (for click + selection). */
  columnPitch: number
  /** Plot-area width consumed by the heatmap. */
  plotWidth: number
}

function plotWidth(columnCount: number, cellSize: number): number {
  if (columnCount <= 0) return 0
  return columnCount * cellSize
}

/** Cap on plot-area height (px) — heatmaps stay compact vs the full chart pane. */
export const HEATMAP_MAX_PLOT_HEIGHT = 160

function layoutForCellSize(opts: {
  cellSize: number
  plotInsetX: number
  columnCount: number
}): HeatmapLayout {
  const { cellSize, plotInsetX, columnCount } = opts
  const plotW = plotWidth(columnCount, cellSize)

  return {
    cellSize,
    chartWidth: plotW + plotInsetX,
    columnPitch: columnCount > 0 ? plotW / columnCount : 0,
    plotWidth: plotW,
  }
}

/** Plot height after content sizing + cap (always <= maxPlotHeight). */
export function computeHeatmapPlotHeight(opts: {
  maxPlotHeight: number
  maxPlotHeightCap?: number
}): number {
  const cap = opts.maxPlotHeightCap ?? HEATMAP_MAX_PLOT_HEIGHT
  return Math.min(Math.max(0, opts.maxPlotHeight), cap)
}

export function computeHeatmapChartHeight(
  plotHeight: number,
  plotInsetY: number
): number {
  return plotHeight + plotInsetY
}

/**
 * Fluid column sizing: divide available plot width evenly across columns
 * (sparse heatmaps fill the container), clamp at minCellPx, then scroll
 * when even min-width columns would overflow.
 */
export function computeHeatmapLayout(opts: {
  containerWidth: number
  plotInsetX: number
  columnCount: number
  minCellPx?: number
}): HeatmapLayout {
  const containerWidth = Math.max(opts.containerWidth, 1)
  const plotInsetX = Math.max(opts.plotInsetX, 0)
  const availablePlotWidth = Math.max(0, containerWidth - plotInsetX)
  const columnCount = opts.columnCount
  const minCellPx = opts.minCellPx ?? MIN_HEATMAP_CELL_PX

  const emptyLayout: HeatmapLayout = {
    cellSize: 0,
    chartWidth: containerWidth,
    columnPitch: 0,
    plotWidth: 0,
  }

  if (columnCount <= 0) return emptyLayout

  const idealCellSize = availablePlotWidth / columnCount
  const cellSize = idealCellSize >= minCellPx ? idealCellSize : minCellPx

  return layoutForCellSize({ plotInsetX, columnCount, cellSize })
}

/** True when the chart is wider than its container and should scroll. */
export function heatmapNeedsHorizontalScroll(
  layout: HeatmapLayout,
  containerWidth: number
): boolean {
  return layout.chartWidth > Math.max(containerWidth, 1)
}
