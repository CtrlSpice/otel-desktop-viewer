import {
  histogramQuantilesForDatapoint,
  histogramSliceToDatapoint,
  QUANTILE_LABELS,
} from '@/components/metrics/utils/histogram-aggregation'
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

function sliceAtTimestamp(
  slices: readonly HistogramSlicePoint[],
  timestampNs: bigint,
  attributesKey: string
): HistogramSlicePoint | null {
  let idx = slices.findIndex(
    s => s.timestamp === timestampNs && s.attributesKey === attributesKey
  )
  if (idx >= 0) return slices[idx]!
  const targetMs = Number(timestampNs / 1_000_000n)
  idx = slices.findIndex(
    s =>
      s.attributesKey === attributesKey &&
      Number(s.timestamp / 1_000_000n) === targetMs
  )
  return idx >= 0 ? slices[idx]! : null
}

export function quantilePointSelectionAt(
  perAttributeSlices: readonly HistogramSlicePoint[],
  mergedBucketSeries: readonly HistogramSlicePoint[],
  timestampNs: bigint,
  visibleKeys: Set<string> | null,
  temporality: string
): QuantilePointSelection | null {
  const visible =
    visibleKeys === null
      ? [...new Set(perAttributeSlices.map(s => s.attributesKey))].sort()
      : [...visibleKeys].sort()

  const series: QuantileSeriesSelection[] = []
  for (const seriesKey of visible) {
    const slice = sliceAtTimestamp(perAttributeSlices, timestampNs, seriesKey)
    if (!slice) continue
    const dp = histogramSliceToDatapoint(
      slice,
      `quantile:${seriesKey}`,
      temporality
    )
    const quantiles: Record<string, number | null> = {}
    const record = histogramQuantilesForDatapoint(dp)
    for (const { key } of QUANTILE_LABELS) {
      quantiles[key] = record[key] ?? null
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
