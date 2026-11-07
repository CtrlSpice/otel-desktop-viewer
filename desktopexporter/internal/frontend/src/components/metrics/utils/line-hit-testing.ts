import { bisector } from 'd3-array'
import type { ChartPoint, ChartTimeseries } from '@/types/metric-chart-types'
import { isAggregateLineKey } from './aggregation'
import {
  insideLineGap,
  insideLineGapAtX,
  LINE_POINT_HIT_RADIUS,
  pointWithinHitRadius,
} from './gapped-line'

export type PlotPointer = Readonly<{ x: number; y: number }>
export type LineScales = Readonly<{
  x: (date: Date) => number
  y: (value: number) => number
}>
type Line = Readonly<{ key: string; data: readonly ChartPoint[] }>
type LineHit = Readonly<{
  seriesKey: string
  point: ChartPoint
  distance: number
}>
export type PointSelectionMode = 'source' | 'rate' | null

/** Match each series on its own timestamp grid, excluding gap interiors. */
export function nearestPointAt<T extends ChartPoint>(
  points: readonly T[],
  date: Date
): T | undefined {
  if (points.length === 0 || insideLineGap(points, date)) return undefined
  const time = date.getTime()
  const i = bisector((point: ChartPoint) => point.date.getTime()).left(
    points,
    time
  )
  const lo = Math.max(0, i - 1)
  const hi = Math.min(points.length - 1, i)
  return Math.abs(points[lo]!.date.getTime() - time) <=
    Math.abs(points[hi]!.date.getTime() - time)
    ? points[lo]
    : points[hi]
}

function pointDistance(
  point: ChartPoint,
  pointer: PlotPointer,
  scales: LineScales
): number {
  return Math.hypot(
    scales.x(point.date) - pointer.x,
    scales.y(point.value) - pointer.y
  )
}

/** Real point targets take precedence over gap rejection and tooltip snapping. */
export function pointAtPointer(
  points: readonly ChartPoint[],
  pointer: PlotPointer,
  scales: LineScales,
  tooltipDate: Date | null
): ChartPoint | undefined {
  const hit = pointWithinHitRadius(
    points,
    pointer.x,
    pointer.y,
    scales.x,
    scales.y
  )
  if (hit) return hit
  if (insideLineGapAtX(points, pointer.x, scales.x)) return undefined
  return tooltipDate ? nearestPointAt(points, tooltipDate) : undefined
}

export function hoveredPointAtPointer(
  lines: readonly Line[],
  pointer: PlotPointer,
  scales: LineScales,
  tooltipDate: Date
): LineHit | null {
  return lines.reduce<LineHit | null>((nearest, series) => {
    const point = pointAtPointer(series.data, pointer, scales, tooltipDate)
    if (!point) return nearest
    const distance = pointDistance(point, pointer, scales)
    return !nearest || distance < nearest.distance
      ? { seriesKey: series.key, point, distance }
      : nearest
  }, null)
}

/** Find a selectable sample without reading callbacks or component state. */
export function selectablePointAtPointer(
  lines: readonly Line[],
  pointer: PlotPointer,
  scales: LineScales,
  mode: PointSelectionMode,
  lineKey?: string
): LineHit | null {
  if (mode === null) return null
  return lines.reduce<LineHit | null>((nearest, series) => {
    if (isAggregateLineKey(series.key) || (lineKey && series.key !== lineKey))
      return nearest
    const inGap = insideLineGapAtX(series.data, pointer.x, scales.x)
    return series.data.reduce<LineHit | null>((closest, point) => {
      const selectable =
        mode === 'rate'
          ? point.timestampNs !== undefined
          : point.sourceDatapointID !== undefined
      if (!selectable) return closest
      const distance = pointDistance(point, pointer, scales)
      if (
        !Number.isFinite(distance) ||
        (inGap && distance > LINE_POINT_HIT_RADIUS)
      )
        return closest
      return !closest || distance < closest.distance
        ? { seriesKey: series.key, point, distance }
        : closest
    }, nearest)
  }, null)
}

/** Choose the raw series closest to a value on its own timestamp grid. */
export function seriesKeyAtValue(
  series: readonly ChartTimeseries[],
  date: Date,
  value: number
): string | null {
  const match = series.reduce<{ key: string; distance: number } | null>(
    (nearest, line) => {
      if (isAggregateLineKey(line.key)) return nearest
      const point = nearestPointAt(line.points, date)
      if (!point || !Number.isFinite(point.value)) return nearest
      const distance = Math.abs(point.value - value)
      return distance < (nearest?.distance ?? Infinity)
        ? { key: line.key, distance }
        : nearest
    },
    null
  )
  return match?.key ?? null
}

/** Resolve an unlabelled tooltip datum against raw series, preserving tie order. */
export function seriesKeyForChartPoint(
  series: readonly ChartTimeseries[],
  data: ChartPoint
): string | null {
  const time = data.date.getTime()
  if (Number.isFinite(data.value)) {
    const valueMatch = series.reduce<{ key: string; distance: number } | null>(
      (nearest, line) => {
        if (isAggregateLineKey(line.key)) return nearest
        return line.points.reduce((closest, point) => {
          if (point.date.getTime() !== time) return closest
          const distance = Math.abs(point.value - data.value)
          return distance < (closest?.distance ?? Infinity)
            ? { key: line.key, distance }
            : closest
        }, nearest)
      },
      null
    )
    if (valueMatch) return valueMatch.key
    const nearestValueKey = seriesKeyAtValue(series, data.date, data.value)
    if (nearestValueKey !== null) return nearestValueKey
  }

  const timeMatch = series.reduce<{ key: string; distance: number } | null>(
    (nearest, line) => {
      if (isAggregateLineKey(line.key)) return nearest
      return line.points.reduce((closest, point) => {
        const distance = Math.abs(point.date.getTime() - time)
        return distance < (closest?.distance ?? Infinity)
          ? { key: line.key, distance }
          : closest
      }, nearest)
    },
    null
  )
  return timeMatch?.key ?? null
}

/** Exact identity wins; ambiguous millisecond/value matches cannot select a sample. */
export function sourcePointForChartPoint(
  points: readonly ChartPoint[],
  data: ChartPoint
): ChartPoint | null {
  if (data.sourceDatapointID) {
    return (
      points.find(
        point => point.sourceDatapointID === data.sourceDatapointID
      ) ?? null
    )
  }
  if (data.timestampNs !== undefined) {
    return points.find(point => point.timestampNs === data.timestampNs) ?? null
  }
  const sameMillisecond = points.filter(
    point => point.date.getTime() === data.date.getTime()
  )
  if (sameMillisecond.length === 1) return sameMillisecond[0]!
  const sameValue = sameMillisecond.filter(point => point.value === data.value)
  return sameValue.length === 1 ? sameValue[0]! : null
}
