import { QUANTILE_LABELS } from '@/components/metrics/utils/histogram-aggregation'
import type { HistogramSlicePoint } from '@/components/metrics/utils/histogram-aggregation'
import {
  heatmapColumnSelectionAt,
  type HeatmapColumnSelection,
} from '@/components/metrics/utils/heatmap-column-selection'
import type {
  ChartPoint,
  ChartTimeseries,
  LayerChartPointClickDetail,
} from '@/types/metric-chart-types'

export type QuantileSeriesSelection = {
  seriesKey: string
  quantiles: Record<string, number | null>
}

export type QuantilePointSelection = {
  timestampMs: number
  series: QuantileSeriesSelection[]
  merged: HeatmapColumnSelection | null
}

export type QuantileChartPointSelection = {
  lineKey: string
  timestampNs: bigint
}

export function quantileChartPointSelection(
  detail: LayerChartPointClickDetail<ChartPoint>,
  timeseries: readonly ChartTimeseries[]
): QuantileChartPointSelection | null {
  const clickedPoint = detail.point
  if (clickedPoint?.seriesKey === undefined) return null
  const lineKey = clickedPoint.seriesKey

  if (detail.data.seriesKey === lineKey) {
    const timestampNs = detail.data.timestampNs
    return timestampNs === undefined ? null : { lineKey, timestampNs }
  }

  const projected = clickedPoint.data
  let timestampNs: bigint | null = null
  for (const series of timeseries) {
    if (series.key !== lineKey) continue
    for (const point of series.points) {
      if (
        point.date.getTime() !== projected.x.getTime() ||
        point.value !== projected.y ||
        point.timestampNs === undefined
      ) {
        continue
      }
      if (timestampNs !== null && timestampNs !== point.timestampNs) return null
      timestampNs = point.timestampNs
    }
  }
  return timestampNs === null ? null : { lineKey, timestampNs }
}

/** Reads each series distribution already merged over the selected column. */
export function quantilePointSelectionAt(
  columnSlices: readonly HistogramSlicePoint[],
  mergedBucketSeries: readonly HistogramSlicePoint[],
  timestampNs: bigint,
  visibleKeys: Set<string> | null
): QuantilePointSelection | null {
  const visible =
    visibleKeys === null
      ? [...new Set(columnSlices.map(s => s.seriesRef))].sort()
      : [...visibleKeys].sort()

  const series: QuantileSeriesSelection[] = []
  for (const seriesKey of visible) {
    const slice = columnSlices.find(s => s.seriesRef === seriesKey)
    if (!slice) continue
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
    merged: heatmapColumnSelectionAt(mergedBucketSeries, timestampNs),
  }
}
