import { describe, expect, it } from 'vitest'
import { buildHistogramHeatmapData } from './histogram-heatmap-data'
import {
  buildHeatmapContours,
  heatmapBlobRadius,
  heatmapContourPath,
  type HeatmapContour,
} from './heatmap-blobs'

function model(columns: number[][]) {
  return buildHistogramHeatmapData(
    columns.map((counts, index) => ({
      kind: 'histogram' as const,
      timestamp: BigInt(index) * 1_000_000_000n,
      seriesRef: '',
      bounds: Array.from(
        { length: counts.length - 1 },
        (_, index) => index + 1
      ),
      counts,
      totals: {
        count: counts.reduce((sum, count) => sum + count, 0),
        sum: undefined,
        min: undefined,
        max: undefined,
      },
    }))
  )
}

function area(contour: HeatmapContour) {
  return contour.rings.reduce(
    (total, ring) =>
      total +
      ring.reduce((sum, point, index) => {
        const next = ring[(index + 1) % ring.length]!
        return sum + point.x * next.y - next.x * point.y
      }, 0) /
        2,
    0
  )
}

function contains(contour: HeatmapContour, x: number, y: number) {
  let inside = false
  for (const ring of contour.rings) {
    for (const [index, point] of ring.entries()) {
      const next = ring[(index + 1) % ring.length]!
      if (
        point.y > y !== next.y > y &&
        x < point.x + ((y - point.y) * (next.x - point.x)) / (next.y - point.y)
      )
        inside = !inside
    }
  }
  return inside
}

describe('heatmap count contours', () => {
  it('paints nested count regions low to high while retaining each cell once', () => {
    const data = model([
      [9, 1],
      [5, 9],
    ])
    const contours = buildHeatmapContours(data, [1, 5, 9])
    expect(contours.map(contour => contour.band)).toEqual([1, 2, 3])
    expect(contours.map(area)).toEqual([4, 3, 2])
    expect(
      contours.map(contour => contour.cells.map(cell => cell.datum.count))
    ).toEqual([[1], [5], [9, 9]])
    expect(contours.flatMap(contour => contour.cells)).toHaveLength(
      data.cells.length
    )
    expect(
      new Set(
        contours.flatMap(contour => contour.cells.map(cell => cell.datum))
      )
    ).toEqual(new Set(data.cells))
  })

  it('preserves count order at every rotated two-, three- and four-band junction', () => {
    for (let assignment = 0; assignment < 256; assignment++) {
      const data = model(
        Array.from({ length: 2 }, (_, x) =>
          Array.from(
            { length: 2 },
            (_, y) => 1 + ((assignment >> (2 * (2 * x + y))) & 3)
          )
        )
      )
      const contours = buildHeatmapContours(data, [1, 2, 3, 4])
      expect(contours.map(contour => contour.band)).toEqual(
        [...new Set(data.cells.map(cell => cell.count))].sort((a, b) => a - b)
      )
      for (const contour of contours) {
        expect(area(contour)).toBe(
          data.cells.filter(cell => cell.count >= contour.band).length
        )
        for (const cell of data.cells) {
          const x = data.columns.findIndex(
            column => column.key === cell.columnKey
          )
          const y = data.rows.findIndex(row => row.key === cell.bucketKey)
          expect(contains(contour, x + 0.5, y + 0.5)).toBe(
            cell.count >= contour.band
          )
        }
      }
    }
  })

  it('keeps holes, missing columns and diagonal-only contacts empty', () => {
    for (let mask = 0; mask < 512; mask++) {
      const columns = Array.from({ length: 3 }, (_, x) =>
        Array.from({ length: 3 }, (_, y) => (mask >> (x * 3 + y)) & 1)
      )
      const data = model(columns)
      const contours = buildHeatmapContours(data, [1])
      expect(contours.reduce((sum, contour) => sum + area(contour), 0)).toBe(
        data.cells.length
      )
      for (const contour of contours) {
        for (let x = 0; x < 3; x++)
          for (let y = 0; y < 3; y++)
            expect(contains(contour, x + 0.5, y + 0.5)).toBe(
              columns[x]![2 - y] === 1
            )
        expect(heatmapContourPath(contour, 0.2, 0.1)).not.toMatch(
          /NaN|Infinity/
        )
      }
    }
    const [diagonal] = buildHeatmapContours(
      model([
        [1, 0],
        [0, 1],
      ]),
      [1]
    )
    expect(diagonal!.rings).toHaveLength(2)
    const [hole] = buildHeatmapContours(
      model([
        [1, 1, 1],
        [1, 0, 1],
        [1, 1, 1],
      ]),
      [1]
    )
    expect(hole!.rings).toHaveLength(2)
    expect(area(hole!)).toBe(8)
  })

  it('rounds internal concave turns toward the lower-count layer', () => {
    // In screen order: low/middle above middle/high. The middle threshold
    // surrounds the low corner; the high threshold sits on top of the middle.
    const contours = buildHeatmapContours(
      model([
        [2, 1],
        [3, 2],
      ]),
      [1, 2, 3]
    )
    const middle = contours[1]!
    const high = contours[2]!
    for (const pitch of [4, 8, 12, 24, 40]) {
      const radius = heatmapBlobRadius(pitch, pitch)
      expect(heatmapContourPath(middle, pitch, pitch)).toContain(
        `L${pitch - radius},${pitch}Q${pitch},${pitch} ${pitch},${pitch - radius}`
      )
      expect(heatmapContourPath(high, pitch, pitch)).toContain(
        `Q${pitch},${pitch} ${pitch + radius},${pitch}`
      )
    }
  })

  it('retains the exterior curve and its clearance around empty buckets', () => {
    const contour: HeatmapContour = {
      band: 1,
      cells: [],
      rings: [
        [
          { x: 0, y: 0, internal: false },
          { x: 2, y: 0, internal: false },
          { x: 2, y: 1, internal: false },
          { x: 1, y: 1, internal: false },
          { x: 1, y: 2, internal: false },
          { x: 0, y: 2, internal: false },
        ],
      ],
    }
    const path = heatmapContourPath(contour, 40, 40)
    expect(path).toContain('Q0,0 2,0')
    expect(path).toContain('L41.5,39.5Q39.5,39.5 39.5,41.5')
    expect(path.match(/Q/g)).toHaveLength(6)
    expect(heatmapContourPath(contour, 0, 40)).toBe('')
  })

  it('uses the same rounding at every density and fits tiny cells', () => {
    for (const size of [8, 16, 28, 48]) {
      expect(heatmapBlobRadius(size, 200)).toBe(2)
      expect(heatmapBlobRadius(200, size)).toBe(2)
    }
    expect(heatmapBlobRadius(0.2, 200)).toBe(0.1)
    expect(heatmapBlobRadius(200, 0.2)).toBe(0.1)
    expect(heatmapBlobRadius(0, 200)).toBe(0)
  })
})
