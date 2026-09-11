import type {
  ChartPoint,
  ChartTimeseries,
  LayerChartPointClickDetail,
} from '@/types/metric-chart-types'

export type TimeSeriesChartPointSelection = {
  seriesKey: string
  sourceDatapointID: string
}

export function timeSeriesChartPointSelection(
  detail: LayerChartPointClickDetail<ChartPoint>,
  timeseries: readonly ChartTimeseries[]
): TimeSeriesChartPointSelection | null {
  const clickedPoint = detail.point
  if (clickedPoint?.seriesKey === undefined) return null
  const seriesKey = clickedPoint.seriesKey

  if (detail.data.seriesKey === seriesKey) {
    const sourceDatapointID = detail.data.sourceDatapointID
    return sourceDatapointID === undefined
      ? null
      : { seriesKey, sourceDatapointID }
  }

  const projected = clickedPoint.data
  let sourceDatapointID: string | null = null
  for (const series of timeseries) {
    if (series.key !== seriesKey) continue
    for (const point of series.points) {
      if (
        point.date.getTime() !== projected.x.getTime() ||
        point.value !== projected.y ||
        point.sourceDatapointID === undefined
      ) {
        continue
      }
      if (
        sourceDatapointID !== null &&
        sourceDatapointID !== point.sourceDatapointID
      ) {
        return null
      }
      sourceDatapointID = point.sourceDatapointID
    }
  }
  return sourceDatapointID === null ? null : { seriesKey, sourceDatapointID }
}
