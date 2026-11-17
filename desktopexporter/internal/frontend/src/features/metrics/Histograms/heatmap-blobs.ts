import { bisectRight } from 'd3-array'
import type {
  HeatmapDatum,
  HistogramHeatmapData,
} from './histogram-heatmap-data'

type Point = { x: number; y: number }
type GridCell = Point & { band: number; datum: HeatmapDatum }
type Edge = { start: Point; end: Point; direction: number }
type Corner = Point & { internal: boolean }

export type HeatmapContour = {
  band: number
  /** Only this band's original cells; higher bands own their own hit targets. */
  cells: GridCell[]
  /** Boundary of every occupied cell at or above this band. */
  rings: Corner[][]
}

const neighbours = [
  [0, -1],
  [1, 0],
  [0, 1],
  [-1, 0],
] as const
const pointKey = (point: Point) => `${point.x}:${point.y}`

/** One radius at every density, capped only to fit tiny cells. */
export function heatmapBlobRadius(columnWidth: number, rowHeight: number) {
  return Math.min(2, Math.max(0, Math.min(columnWidth, rowHeight)) / 2)
}

function traceRings(
  level: ReadonlyMap<string, GridCell>,
  occupied: ReadonlyMap<string, GridCell>
): Corner[][] {
  const edges: Edge[] = []
  for (const { x, y } of level.values()) {
    const corners = [
      { x, y },
      { x: x + 1, y },
      { x: x + 1, y: y + 1 },
      { x, y: y + 1 },
    ]
    // Clockwise edges keep the populated region on the right.
    for (const [side, [dx, dy]] of neighbours.entries()) {
      if (level.has(pointKey({ x: x + dx, y: y + dy }))) continue
      edges.push({
        start: corners[side]!,
        end: corners[(side + 1) % 4]!,
        direction: side,
      })
    }
  }

  const outgoing = new Map<string, Edge[]>()
  for (const edge of edges) {
    const key = pointKey(edge.start)
    const list = outgoing.get(key) ?? []
    list.push(edge)
    outgoing.set(key, list)
  }
  const remaining = new Set(edges)
  const rings: Corner[][] = []
  for (const start of edges) {
    if (!remaining.has(start)) continue
    const ring: Point[] = []
    let edge: Edge | undefined = start
    while (edge) {
      ring.push(edge.start)
      remaining.delete(edge)
      if (pointKey(edge.end) === pointKey(start.start)) break
      const candidates: Edge[] = outgoing.get(pointKey(edge.end)) ?? []
      // A right turn keeps diagonally touching boundaries separate.
      const direction: number = edge.direction
      edge = [1, 0, 3, 2].flatMap(turn =>
        candidates.filter(
          candidate =>
            remaining.has(candidate) &&
            candidate.direction === (direction + turn) % 4
        )
      )[0]
    }
    const corners: Corner[] = []
    for (const [index, point] of ring.entries()) {
      const before = ring[(index + ring.length - 1) % ring.length]!
      const after = ring[(index + 1) % ring.length]!
      if (
        (point.x - before.x) * (after.y - point.y) ===
        (point.y - before.y) * (after.x - point.x)
      )
        continue
      corners.push({
        ...point,
        internal: [
          { x: point.x - 1, y: point.y - 1 },
          { x: point.x, y: point.y - 1 },
          point,
          { x: point.x - 1, y: point.y },
        ].every(quadrant => occupied.has(pointKey(quadrant))),
      })
    }
    rings.push(corners)
  }
  return rings
}

/** Nested count regions, ordered low to high for painting. The first region
 * contains all occupied cells and also supplies the shared exterior clip. */
export function buildHeatmapContours(
  model: HistogramHeatmapData,
  thresholds: readonly number[]
): HeatmapContour[] {
  const columns = new Map(
    model.columns.map((column, index) => [column.key, index])
  )
  const rows = new Map(model.rows.map((row, index) => [row.key, index]))
  const grid = new Map<string, GridCell>()
  for (const datum of model.cells) {
    if (datum.count <= 0) continue
    const cell = {
      x: columns.get(datum.columnKey)!,
      y: rows.get(datum.bucketKey)!,
      band: bisectRight(thresholds, datum.count),
      datum,
    }
    grid.set(pointKey(cell), cell)
  }
  const bands = [...new Set([...grid.values()].map(cell => cell.band))].sort(
    (a, b) => a - b
  )
  return bands.map(band => {
    const level = new Map([...grid].filter(([, cell]) => cell.band >= band))
    return {
      band,
      cells: [...level.values()].filter(cell => cell.band === band),
      rings: traceRings(level, grid),
    }
  })
}

/** Internal turns follow the count region's winding. The existing exterior
 * clearance keeps rounded holes and missing buckets outside the painted area. */
export function heatmapContourPath(
  contour: HeatmapContour,
  columnWidth: number,
  rowHeight: number
): string {
  if (columnWidth <= 0 || rowHeight <= 0) return ''
  const radius = heatmapBlobRadius(columnWidth, rowHeight)
  return contour.rings
    .map(ring => {
      const points = ring.map(point => ({
        ...point,
        x: point.x * columnWidth,
        y: point.y * rowHeight,
      }))
      let path = ''
      for (const [index, point] of points.entries()) {
        const previous = points[(index + points.length - 1) % points.length]!
        const next = points[(index + 1) % points.length]!
        const incoming = { x: point.x - previous.x, y: point.y - previous.y }
        const outgoing = { x: next.x - point.x, y: next.y - point.y }
        const corner = Math.min(
          radius,
          Math.hypot(incoming.x, incoming.y) / 2,
          Math.hypot(outgoing.x, outgoing.y) / 2
        )
        const concave = incoming.x * outgoing.y - incoming.y * outgoing.x < 0
        const clearance = concave && !point.internal ? corner / 4 : 0
        const control = {
          x:
            point.x +
            clearance * (-Math.sign(incoming.y) - Math.sign(outgoing.y)),
          y:
            point.y +
            clearance * (Math.sign(incoming.x) + Math.sign(outgoing.x)),
        }
        const start = {
          x: control.x - Math.sign(incoming.x) * corner,
          y: control.y - Math.sign(incoming.y) * corner,
        }
        const end = {
          x: control.x + Math.sign(outgoing.x) * corner,
          y: control.y + Math.sign(outgoing.y) * corner,
        }
        path += `${index === 0 ? 'M' : 'L'}${start.x},${start.y}`
        if (corner > 0) path += `Q${control.x},${control.y} ${end.x},${end.y}`
      }
      return `${path}Z`
    })
    .join('')
}
