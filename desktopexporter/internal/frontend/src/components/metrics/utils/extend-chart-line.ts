type Point = { x: number; y: number }
type Bounds = { left: number; right: number; top: number; bottom: number }

/** Extend a screen-space guide to the plot edges without changing its direction. */
export function extendChartLine(from: Point, to: Point, bounds: Bounds) {
  if (
    ![from.x, from.y, to.x, to.y, ...Object.values(bounds)].every(
      Number.isFinite
    )
  )
    return null
  if (bounds.right <= bounds.left || bounds.bottom <= bounds.top) return null
  const dx = to.x - from.x
  const dy = to.y - from.y
  if (dx === 0 && dy === 0) return null

  let start = -Infinity
  let end = Infinity
  for (const [origin, delta, min, max] of [
    [from.x, dx, bounds.left, bounds.right],
    [from.y, dy, bounds.top, bounds.bottom],
  ] as const) {
    if (delta === 0) {
      if (origin < min || origin > max) return null
      continue
    }
    const a = (min - origin) / delta
    const b = (max - origin) / delta
    start = Math.max(start, Math.min(a, b))
    end = Math.min(end, Math.max(a, b))
  }
  if (start >= end) return null
  return {
    x1: from.x + start * dx,
    y1: from.y + start * dy,
    x2: from.x + end * dx,
    y2: from.y + end * dy,
  }
}
