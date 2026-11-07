import { extent } from 'd3-array'
import type { ChartPoint } from '@/types/metric-chart-types'
import { gappedLineClips, gappedLinePath } from './gapped-line'

/** SVG geometry only: no DOM measurements, component state, or source mutation. */
export function sparklineGeometry(
  points: readonly ChartPoint[],
  width: number,
  height: number,
  strokeWidth: number
) {
  if (points.length === 0) return { path: '', clips: [] }
  const [start = 0, end = start] = extent(points, point => point.date.getTime())
  const timeRange = end - start || 1
  const x = (date: Date) => ((date.getTime() - start) / timeRange) * width
  const [min = 0, max = min] = extent(points, point =>
    Number.isFinite(point.value) ? point.value : undefined
  )
  const valueRange = max - min
  const flat = !Number.isFinite(valueRange) || valueRange === 0
  const inset = strokeWidth / 2
  const innerHeight = Math.max(0, height - 2 * inset)
  const y = (value: number) =>
    flat ? height / 2 : inset + (1 - (value - min) / valueRange) * innerHeight

  return {
    path:
      points.length === 1
        ? `M 0 ${height / 2} L ${width} ${height / 2}`
        : gappedLinePath(points, x, y),
    clips: gappedLineClips(points, x, strokeWidth, 0, width),
  }
}
