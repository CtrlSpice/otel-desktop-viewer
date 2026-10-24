import { bisectRight } from 'd3-array'
import type {
  HeatmapDatum,
  HistogramHeatmapData,
} from './histogram-heatmap-data'

type Point = { x: number; y: number }
type GridCell = { x: number; y: number; band: number; datum: HeatmapDatum }
type Edge = { start: Point; end: Point; direction: number }

export type HeatmapBlob = {
  key: string
  band: number
  cells: HeatmapDatum[]
  rings: Point[][]
}

const directions = [
  [1, 0],
  [0, 1],
  [-1, 0],
  [0, -1],
] as const
const pointKey = (point: Point) => `${point.x}:${point.y}`

const BLOB_DETAIL_STEPS = [
  { minCellSize: 40, radius: 18, gap: 4, liner: 1, joinColours: false },
  { minCellSize: 24, radius: 12, gap: 3, liner: 1, joinColours: false },
  { minCellSize: 12, radius: 6, gap: 2, liner: 0.75, joinColours: false },
  { minCellSize: 0, radius: 2, gap: 1, liner: 0.5, joinColours: true },
] as const

/** Use one detail level across the plot, based on rendered cell dimensions. */
export function heatmapBlobDetail(columnWidth: number, rowHeight: number) {
  const size = Math.max(0, Math.min(columnWidth, rowHeight))
  const detail = BLOB_DETAIL_STEPS.find(step => size >= step.minCellSize)!
  return { ...detail, liner: Math.min(detail.liner, size / 8) }
}

function heatmapGrid(
  model: HistogramHeatmapData,
  thresholds: readonly number[]
): Map<string, GridCell> {
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
  return grid
}

/** Four-connected regions, optionally joined across colours. Retain original cells. */
export function buildHeatmapBlobs(
  model: HistogramHeatmapData,
  thresholds: readonly number[],
  joinColours = false
): HeatmapBlob[] {
  const grid = heatmapGrid(model, thresholds)

  const visited = new Set<string>()
  const blobs: HeatmapBlob[] = []
  for (const [key, seed] of grid) {
    if (visited.has(key)) continue
    visited.add(key)
    const queue = [seed]
    const edges: Edge[] = []
    for (let index = 0; index < queue.length; index++) {
      const cell = queue[index]!
      for (const [dx, dy] of directions) {
        const neighbourKey = pointKey({ x: cell.x + dx, y: cell.y + dy })
        const neighbour = grid.get(neighbourKey)
        if (
          neighbour &&
          (joinColours || neighbour.band === seed.band) &&
          !visited.has(neighbourKey)
        ) {
          visited.add(neighbourKey)
          queue.push(neighbour)
        }
      }

      // Clockwise edges keep the populated region on the right.
      const { x, y } = cell
      const corners = [
        { x, y },
        { x: x + 1, y },
        { x: x + 1, y: y + 1 },
        { x, y: y + 1 },
      ]
      const neighbours = [
        [0, -1],
        [1, 0],
        [0, 1],
        [-1, 0],
      ] as const
      for (let side = 0; side < 4; side++) {
        const [dx, dy] = neighbours[side]!
        const neighbour = grid.get(pointKey({ x: x + dx, y: y + dy }))
        if (neighbour && (joinColours || neighbour.band === seed.band)) continue
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
    const rings: Point[][] = []
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
      rings.push(
        ring.filter((point, index) => {
          const before = ring[(index + ring.length - 1) % ring.length]!
          const after = ring[(index + 1) % ring.length]!
          return (
            (point.x - before.x) * (after.y - point.y) !==
            (point.y - before.y) * (after.x - point.x)
          )
        })
      )
    }
    blobs.push({
      key,
      band: seed.band,
      cells: queue.map(cell => cell.datum),
      rings,
    })
  }
  return blobs
}

type HeatmapColourContours = {
  corners: { key: string; band: number; datum: HeatmapDatum; path: string }[]
  regions: { band: number; path: string }[]
}

/** Round shared two-colour turns without cutting holes in the occupied grid.
 * Three equal quadrants surround one different band: paint the small corner
 * wedge in the surrounding colour, over the intact cells. Both colours then
 * share one curve, also used to clip their inner outlines. Leave empty,
 * diagonal-only and multi-colour junctions alone. */
export function heatmapColourContours(
  model: HistogramHeatmapData,
  thresholds: readonly number[],
  columnWidth: number,
  rowHeight: number
): HeatmapColourContours {
  const contours: HeatmapColourContours = { corners: [], regions: [] }
  if (columnWidth <= 0 || rowHeight <= 0) return contours
  const detail = heatmapBlobDetail(columnWidth, rowHeight)
  if (!detail.joinColours) return contours
  const radius = Math.min(detail.radius, columnWidth / 2, rowHeight / 2)
  const grid = heatmapGrid(model, thresholds)
  for (const topLeft of grid.values()) {
    const { x, y } = topLeft
    const topRight = grid.get(pointKey({ x: x + 1, y }))
    const bottomRight = grid.get(pointKey({ x: x + 1, y: y + 1 }))
    const bottomLeft = grid.get(pointKey({ x, y: y + 1 }))
    if (!topRight || !bottomRight || !bottomLeft) continue
    const quadrants = [topLeft, topRight, bottomRight, bottomLeft]
    const odd = quadrants.findIndex((cell, index) => {
      const other = quadrants[(index + 1) % 4]!.band
      return (
        cell.band !== other &&
        quadrants[(index + 2) % 4]!.band === other &&
        quadrants[(index + 3) % 4]!.band === other
      )
    })
    if (odd < 0) continue
    const vertex = { x: (x + 1) * columnWidth, y: (y + 1) * rowHeight }
    const dx = odd === 0 || odd === 3 ? -radius : radius
    const dy = odd < 2 ? -radius : radius
    contours.corners.push({
      key: pointKey({ x: x + 1, y: y + 1 }),
      band: quadrants[(odd + 1) % 4]!.band,
      // Keep hover and selection on the original rectangular cell footprint.
      datum: quadrants[odd]!.datum,
      path: `M${vertex.x},${vertex.y}L${vertex.x + dx},${vertex.y}Q${vertex.x},${vertex.y} ${vertex.x},${vertex.y + dy}Z`,
    })
  }

  const roundedVertices = new Set(contours.corners.map(corner => corner.key))
  const pathsByBand = new Map<number, string>()
  for (const blob of buildHeatmapBlobs(model, thresholds)) {
    const path = blob.rings
      .map(ring => {
        let path = ''
        for (let index = 0; index < ring.length; index++) {
          const point = ring[index]!
          const previous = ring[(index + ring.length - 1) % ring.length]!
          const next = ring[(index + 1) % ring.length]!
          const corner = roundedVertices.has(pointKey(point)) ? radius : 0
          const x = point.x * columnWidth
          const y = point.y * rowHeight
          const startX = x - Math.sign(point.x - previous.x) * corner
          const startY = y - Math.sign(point.y - previous.y) * corner
          const endX = x + Math.sign(next.x - point.x) * corner
          const endY = y + Math.sign(next.y - point.y) * corner
          path += `${index === 0 ? 'M' : 'L'}${startX},${startY}`
          if (corner > 0) path += `Q${x},${y} ${endX},${endY}`
        }
        return `${path}Z`
      })
      .join('')
    pathsByBand.set(blob.band, (pathsByBand.get(blob.band) ?? '') + path)
  }
  contours.regions = Array.from(pathsByBand, ([band, path]) => ({ band, path }))
  return contours
}

/** Round the inset perimeter, keeping inward curves within the visual gutter. */
export function heatmapBlobPath(
  blob: HeatmapBlob,
  columnWidth: number,
  rowHeight: number,
  grid?: { columns: number; rows: number }
): string {
  if (columnWidth <= 0 || rowHeight <= 0) return ''
  const detail = heatmapBlobDetail(columnWidth, rowHeight)
  const inset = Math.min(detail.gap / 2, columnWidth / 4, rowHeight / 4)
  const edgeInset = Math.min(detail.gap, columnWidth / 4, rowHeight / 4)
  const radius = Math.min(
    detail.radius,
    (columnWidth - inset * 2) / 2,
    (rowHeight - inset * 2) / 2
  )
  return blob.rings
    .map(ring => {
      const points = ring.map((point, index) => {
        const previous = ring[(index + ring.length - 1) % ring.length]!
        const next = ring[(index + 1) % ring.length]!
        // Interior neighbours each supply half the gap. The plot boundary
        // has no neighbour, so it supplies the full clearance itself.
        const insetX =
          grid && (point.x === 0 || point.x === grid.columns)
            ? edgeInset
            : inset
        const insetY =
          grid && (point.y === 0 || point.y === grid.rows) ? edgeInset : inset
        return {
          x:
            point.x * columnWidth -
            insetX *
              (Math.sign(point.y - previous.y) + Math.sign(next.y - point.y)),
          y:
            point.y * rowHeight +
            insetY *
              (Math.sign(point.x - previous.x) + Math.sign(next.x - point.x)),
        }
      })
      let path = ''
      for (let index = 0; index < points.length; index++) {
        const point = points[index]!
        const previous = points[(index + points.length - 1) % points.length]!
        const next = points[(index + 1) % points.length]!
        const incoming = { x: point.x - previous.x, y: point.y - previous.y }
        const outgoing = { x: next.x - point.x, y: next.y - point.y }
        const incomingLength = Math.hypot(incoming.x, incoming.y)
        const outgoingLength = Math.hypot(outgoing.x, outgoing.y)
        const convex = incoming.x * outgoing.y - incoming.y * outgoing.x > 0
        const corner = Math.min(radius, incomingLength / 2, outgoingLength / 2)
        // Match the outer rounding. An inward quadratic advances corner / 4
        // toward the empty quadrant on both axes, so widen the local gutter
        // rather than painting into an empty bucket. Straight edges taper
        // between the corner clearances; the regular inset stays unchanged.
        const clearance = convex ? 0 : Math.max(0, corner / 4 - inset)
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
