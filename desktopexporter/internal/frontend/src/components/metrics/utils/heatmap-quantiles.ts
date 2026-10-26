import {
  QUANTILE_LABELS,
  type HistogramSlicePoint,
} from './histogram-aggregation'
import type { HistogramHeatmapData } from './histogram-heatmap-data'
import { graphSwatch } from '@/themes/graph-inks'

export const HISTOGRAM_QUANTILE_DASH = '6 4'

// Graph inks, rather than text-corrected status colours. Keep percentile colours
// consistent between histogram/heatmap views and Day/Night.
const quantileColors = {
  '0.5': graphSwatch('shin-ryoku:6')!.color,
  '0.95': graphSwatch('daikokuten:2')!.color,
  '0.99': graphSwatch('benzaiten:2')!.color,
}

export function histogramQuantileColor(key: string): string {
  return key === '0.5'
    ? quantileColors['0.5']
    : key === '0.95'
      ? quantileColors['0.95']
      : quantileColors['0.99']
}

/** Project the store's merged-distribution quantiles onto the heatmap's
 * categorical bucket rows. This does not recompute or average quantiles. */
export function heatmapQuantilePaths(
  points: readonly HistogramSlicePoint[],
  model: HistogramHeatmapData,
  columnWidth: number,
  plotHeight: number,
  activeKeys: ReadonlySet<string>
) {
  if (model.rows.length === 0 || columnWidth <= 0 || plotHeight <= 0) return []
  const rowHeight = plotHeight / model.rows.length
  const rowIndex = new Map(model.rows.map((row, index) => [row.key, index]))
  return QUANTILE_LABELS.filter(({ key }) => activeKeys.has(key)).flatMap(
    ({ key, label }) => {
      let path = ''
      let connected = false
      let end: { x: number; y: number } | null = null
      for (let index = 0; index < model.columns.length; index++) {
        const point = points[index]!
        const value = point.quantiles?.[key]
        const rows = model.rowsByColumn.get(model.columns[index]!.key) ?? []
        // Prefer the explicit zero region if exponential bounds overlap it.
        const zero =
          point.kind === 'expHistogram' &&
          value != null &&
          Math.abs(value) <= point.zeroThreshold
            ? rows.find(row => row.key.startsWith('exponential:zero:'))
            : undefined
        const row =
          value != null && Number.isFinite(value)
            ? (zero ?? rows.find(row => value >= row.lo && value <= row.hi))
            : undefined
        const order = row ? rowIndex.get(row.key) : undefined
        if (!row || order === undefined || value == null) {
          connected = false
          continue
        }
        // An open-ended or exact-zero bucket has no finite within-row scale.
        let fraction = 0.5
        if (
          Number.isFinite(row.lo) &&
          Number.isFinite(row.hi) &&
          row.hi > row.lo
        ) {
          const logarithmic =
            point.kind === 'expHistogram' && row.lo * row.hi > 0 && value !== 0
          fraction = logarithmic
            ? (Math.log(Math.abs(value)) - Math.log(Math.abs(row.lo))) /
              (Math.log(Math.abs(row.hi)) - Math.log(Math.abs(row.lo)))
            : (value - row.lo) / (row.hi - row.lo)
          fraction = Math.max(0, Math.min(1, fraction))
        }
        const y = (order + 1 - fraction) * rowHeight
        const x = index * columnWidth
        end = { x: x + columnWidth, y }
        path += connected
          ? `V${y}H${x + columnWidth}`
          : `M${x},${y}H${x + columnWidth}`
        connected = true
      }
      return path && end
        ? [{ key, label, path, end, dash: HISTOGRAM_QUANTILE_DASH }]
        : []
    }
  )
}
