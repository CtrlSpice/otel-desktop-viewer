import { curveLinear, line, type CurveFactory } from 'd3-shape'

type Point = { date: Date; value: number; breakBefore?: true }

export const LINE_POINT_HIT_RADIUS = 6

function lineSections(points: readonly Point[]): Point[][] {
  const sections: Point[][] = []
  let start = 0
  for (let i = 0; i < points.length; i++) {
    const point = points[i]!
    const finite = Number.isFinite(point.value)
    if (point.breakBefore || !finite) {
      if (i > start) sections.push(points.slice(start, i))
      start = i
    }
    if (!finite) start = i + 1
  }
  if (start < points.length) sections.push(points.slice(start))
  return sections
}

/** Keep both real endpoints. A break starts a new path, not an undefined point. */
export function gappedLinePath(
  points: readonly Point[],
  x: (date: Date) => number,
  y: (value: number) => number,
  curve: CurveFactory = curveLinear
): string {
  const draw = line<Point>()
    .x(p => x(p.date))
    .y(p => y(p.value))
    .curve(curve)
  return lineSections(points)
    .map(section => {
      if (section.length !== 1) return draw(section) ?? ''
      const point = section[0]!
      return `M${x(point.date)},${y(point.value)}l0,0`
    })
    .join('')
}

/** Clip ink, not coordinates, to leave a stroke-width opening at each break. */
export function gappedLineClips(
  points: readonly Point[],
  x: (date: Date) => number,
  strokeWidth: number,
  left: number,
  right: number
): { x: number; width: number }[] {
  const clips: { x: number; width: number }[] = []
  let start = left
  for (let i = 1; i < points.length; i++) {
    if (!points[i]!.breakBefore) continue
    const middle = (x(points[i - 1]!.date) + x(points[i]!.date)) / 2
    const end = Math.min(right, middle - strokeWidth / 2)
    if (end > start) clips.push({ x: start, width: end - start })
    start = Math.max(start, middle + strokeWidth / 2)
  }
  if (right > start) clips.push({ x: start, width: right - start })
  return clips
}

/** A real endpoint is inspectable; a time strictly inside a gap has no value. */
export function insideLineGap(points: readonly Point[], date: Date): boolean {
  return insideLineGapAtX(points, date.getTime(), date => date.getTime())
}

function firstPointAtOrAfterX(
  points: readonly Point[],
  pointerX: number,
  x: (date: Date) => number
): number {
  let low = 0
  let high = points.length
  while (low < high) {
    const mid = Math.floor((low + high) / 2)
    if (x(points[mid]!.date) < pointerX) low = mid + 1
    else high = mid
  }
  return low
}

/** Compare drawing coordinates directly: Date would truncate fractional milliseconds. */
export function insideLineGapAtX(
  points: readonly Point[],
  pointerX: number,
  x: (date: Date) => number
): boolean {
  const low = firstPointAtOrAfterX(points, pointerX, x)
  return (
    low > 0 &&
    low < points.length &&
    points[low]!.breakBefore === true &&
    x(points[low - 1]!.date) < pointerX &&
    pointerX < x(points[low]!.date)
  )
}

/** A circular hit target, including the gap-facing side of a real endpoint. */
export function pointWithinHitRadius<T extends Point>(
  points: readonly T[],
  pointerX: number,
  pointerY: number,
  x: (date: Date) => number,
  y: (value: number) => number
): T | undefined {
  let nearest: T | undefined
  let distance = LINE_POINT_HIT_RADIUS
  const start = firstPointAtOrAfterX(
    points,
    pointerX - LINE_POINT_HIT_RADIUS,
    x
  )
  for (let i = start; i < points.length; i++) {
    const point = points[i]!
    const px = x(point.date)
    if (px > pointerX + LINE_POINT_HIT_RADIUS) break
    const candidateDistance = Math.hypot(
      px - pointerX,
      y(point.value) - pointerY
    )
    if (candidateDistance <= distance) {
      nearest = point
      distance = candidateDistance
    }
  }
  return nearest
}
