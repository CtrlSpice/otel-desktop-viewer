import { describe, expect, it } from 'vitest'
import {
  computeHeatmapChartHeight,
  computeHeatmapLayout,
  computeHeatmapPlotHeight,
  HEATMAP_MAX_PLOT_HEIGHT,
  MIN_HEATMAP_CELL_PX,
  heatmapNeedsHorizontalScroll,
} from '@/components/metrics/utils/heatmap-layout'

describe('computeHeatmapLayout', () => {
  const insetX = 144

  it('returns empty layout when there are no columns', () => {
    expect(
      computeHeatmapLayout({
        containerWidth: 400,
        plotInsetX: insetX,
        columnCount: 0,
      })
    ).toMatchObject({ cellSize: 0, chartWidth: 400 })
  })

  it('fills available width on sparse heatmaps (no max cell cap)', () => {
    const layout = computeHeatmapLayout({
      containerWidth: 800,
      plotInsetX: insetX,
      columnCount: 5,
    })
    expect(layout.cellSize).toBe((800 - insetX) / 5)
    expect(layout.plotWidth).toBe(800 - insetX)
    expect(layout.chartWidth).toBe(800)
    expect(layout.columnPitch).toBe(layout.cellSize)
    expect(heatmapNeedsHorizontalScroll(layout, 800)).toBe(false)
  })

  it('scales columns down smoothly until the minimum cell width', () => {
    const layout = computeHeatmapLayout({
      containerWidth: 500,
      plotInsetX: insetX,
      columnCount: 20,
    })
    expect(layout.cellSize).toBe((500 - insetX) / 20)
    expect(layout.plotWidth).toBe(500 - insetX)
    expect(heatmapNeedsHorizontalScroll(layout, 500)).toBe(false)
  })

  it('scrolls at the minimum cell width when columns still overflow', () => {
    const layout = computeHeatmapLayout({
      containerWidth: 400,
      plotInsetX: insetX,
      columnCount: 100,
    })
    expect(layout.cellSize).toBe(MIN_HEATMAP_CELL_PX)
    expect(layout.chartWidth).toBe(100 * MIN_HEATMAP_CELL_PX + insetX)
    expect(heatmapNeedsHorizontalScroll(layout, 400)).toBe(true)
  })

  it('does not scroll when columns fit in the container', () => {
    const layout = computeHeatmapLayout({
      containerWidth: 800,
      plotInsetX: insetX,
      columnCount: 5,
    })
    expect(heatmapNeedsHorizontalScroll(layout, 800)).toBe(false)
  })

  it('caps plot height to the global max', () => {
    expect(
      computeHeatmapPlotHeight({
        maxPlotHeight: 400,
      })
    ).toBe(HEATMAP_MAX_PLOT_HEIGHT)

    expect(
      computeHeatmapPlotHeight({
        maxPlotHeight: 100,
      })
    ).toBe(100)
  })

  it('builds chart height from plot height and insets', () => {
    expect(computeHeatmapChartHeight(160, 96)).toBe(256)
  })
})
