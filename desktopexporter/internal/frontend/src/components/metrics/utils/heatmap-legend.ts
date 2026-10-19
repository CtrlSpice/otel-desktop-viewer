import type { HistogramSlicePoint } from '@/components/metrics/utils/histogram-aggregation'
import {
  computeHeatmapColorScale,
  type HeatmapLegendEntry,
} from '@/components/metrics/utils/heatmap-color-scale'

export type { HeatmapLegendEntry }

/** Count values that appear in heatmap cells (matches HistogramHeatmap data rules). */
export function collectHeatmapCounts(points: HistogramSlicePoint[]): number[] {
  const counts: number[] = []
  for (const pt of points) {
    if (pt.kind === 'histogram') {
      for (const count of pt.counts) {
        if (count === 0) continue
        counts.push(count)
      }
      continue
    }
    if (pt.zeroCount > 0) counts.push(pt.zeroCount)
    for (const count of pt.positiveCounts) {
      if (count === 0) continue
      counts.push(count)
    }
    for (const count of pt.negativeCounts) {
      if (count === 0) continue
      counts.push(count)
    }
  }
  return counts
}

export function computeHeatmapLegendEntries(
  points: HistogramSlicePoint[],
  theme: string
): HeatmapLegendEntry[] {
  const counts = collectHeatmapCounts(points)
  let maxCount = 0
  const distinctNonZero = new Set<number>()
  for (const count of counts) {
    if (count > maxCount) maxCount = count
    if (count > 0) distinctNonZero.add(count)
  }

  return computeHeatmapColorScale({
    maxCount,
    distinctNonZeroCount: distinctNonZero.size,
    theme,
  }).legendEntries
}
