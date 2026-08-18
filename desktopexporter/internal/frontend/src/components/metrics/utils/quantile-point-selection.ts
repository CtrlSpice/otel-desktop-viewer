import { QUANTILE_LABELS } from '@/components/metrics/utils/histogram-aggregation'
import type { HistogramSlicePoint } from '@/components/metrics/utils/histogram-aggregation'
import {
  heatmapColumnSelectionAt,
  type HeatmapColumnSelection,
} from '@/components/metrics/utils/heatmap-column-selection'

export type QuantileSeriesSelection = {
  seriesKey: string
  quantiles: Record<string, number | null>
}

export type QuantilePointSelection = {
  timestampMs: number
  series: QuantileSeriesSelection[]
  merged: HeatmapColumnSelection | null
}

/**
 * The per-series distribution for one heatmap column.
 *
 * `columnSlices` holds at most one slice per series, each already merged by the
 * store over exactly the column's time range. It used to be every series slice
 * in the window, looked up by matching the clicked timestamp -- which found a
 * slice every time and was wrong every time a column was wider than one
 * per-series bucket. Both grids snap to the same ladder and its rungs divide
 * evenly, so a column start is always a bucket start; but a 30s column holds
 * three 10s buckets, and the match returned the first of them. The reader saw
 * one third of the column they clicked, with nothing to say so, and the error
 * was largest exactly when it mattered -- a spike beginning after a column's
 * first bucket showed p95 1 against the column's true 10.
 *
 * So there is no timestamp lookup here any more. The caller fetches the column
 * and this reads it.
 */
export function quantilePointSelectionAt(
  columnSlices: readonly HistogramSlicePoint[],
  mergedBucketSeries: readonly HistogramSlicePoint[],
  timestampNs: bigint,
  visibleKeys: Set<string> | null,
  temporality: string
): QuantilePointSelection | null {
  const visible =
    visibleKeys === null
      ? [...new Set(columnSlices.map(s => s.attributesKey))].sort()
      : [...visibleKeys].sort()

  const series: QuantileSeriesSelection[] = []
  for (const seriesKey of visible) {
    const slice = columnSlices.find(s => s.attributesKey === seriesKey)
    if (!slice) continue
    // Read from the store's response; see sliceQuantileValue for why this is
    // not computed here.
    const quantiles: Record<string, number | null> = {}
    for (const { key } of QUANTILE_LABELS) {
      quantiles[key] = slice.quantiles?.[key] ?? null
    }
    series.push({ seriesKey, quantiles })
  }

  if (series.length === 0) return null

  return {
    timestampMs: Number(timestampNs / 1_000_000n),
    series,
    merged: heatmapColumnSelectionAt(
      mergedBucketSeries,
      timestampNs,
      temporality
    ),
  }
}
