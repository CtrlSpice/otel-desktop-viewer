import { describe, expect, it } from 'vitest'
import { buildHistogramHeatmapData } from './histogram-heatmap-data'
import {
  buildHeatmapBlobs,
  heatmapBlobDetail,
  heatmapBlobPath,
  type HeatmapBlob,
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

function area(blob: HeatmapBlob) {
  return blob.rings.reduce(
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

describe('heatmap blobs', () => {
  it('joins touching cells by colour band while retaining individual counts and identities', () => {
    const data = model([
      [1, 2],
      [2, 3],
    ])
    const blobs = buildHeatmapBlobs(data, [1, 3])
    expect(blobs).toHaveLength(2)
    expect(blobs.map(blob => blob.cells.length)).toEqual([3, 1])
    expect(new Set(blobs.flatMap(blob => blob.cells))).toEqual(
      new Set(data.cells)
    )
    expect(blobs.map(area)).toEqual([3, 1])
    expect(blobs[0]!.cells.map(cell => cell.count).sort()).toEqual([1, 2, 2])
  })

  it('keeps diagonal neighbours and empty columns separate', () => {
    expect(
      buildHeatmapBlobs(
        model([
          [1, 0],
          [0, 1],
        ]),
        [1]
      )
    ).toHaveLength(2)
    expect(buildHeatmapBlobs(model([[1], [0], [1]]), [1])).toHaveLength(2)
  })

  it('preserves a hole surrounded by one colour', () => {
    const [blob] = buildHeatmapBlobs(
      model([
        [1, 1, 1],
        [1, 0, 1],
        [1, 1, 1],
      ]),
      [1]
    )
    expect(blob!.rings).toHaveLength(2)
    expect(area(blob!)).toBe(8)
    const path = heatmapBlobPath(blob!, 20, 40)
    expect(path.match(/M/g)).toHaveLength(2)
    expect(path.match(/Z/g)).toHaveLength(2)
    expect(path.match(/Q/g)).toHaveLength(8)
  })

  it('rounds inward turns without entering an empty bucket', () => {
    const blob: HeatmapBlob = {
      key: 'elbow',
      band: 0,
      cells: [],
      rings: [
        [
          { x: 0, y: 0 },
          { x: 2, y: 0 },
          { x: 2, y: 1 },
          { x: 1, y: 1 },
          { x: 1, y: 2 },
          { x: 0, y: 2 },
        ],
      ],
    }
    for (const pitch of [0.2, 4, 12, 24, 40]) {
      const path = heatmapBlobPath(blob, pitch, pitch)
      expect(path.match(/Q/g)).toHaveLength(6)
      if (pitch === 40) {
        // Both turns use an 18px curve; the inward control recedes just enough
        // to keep the missing lower-right bucket unpainted.
        expect(path).toContain('Q2,2 20,2')
        expect(path).toContain('L53.5,35.5Q35.5,35.5 35.5,53.5')
      }
      // Sample the rendered quadratic boundaries, including the inward turn.
      const segments = path.matchAll(
        /[ML]([\d.e-]+),([\d.e-]+)Q([\d.e-]+),([\d.e-]+) ([\d.e-]+),([\d.e-]+)/g
      )
      for (const segment of segments) {
        const [sx, sy, cx, cy, ex, ey] = segment.slice(1).map(Number) as [
          number,
          number,
          number,
          number,
          number,
          number,
        ]
        for (let step = 0; step <= 100; step++) {
          const t = step / 100
          const x = (1 - t) ** 2 * sx + 2 * (1 - t) * t * cx + t ** 2 * ex
          const y = (1 - t) ** 2 * sy + 2 * (1 - t) * t * cy + t ** 2 * ey
          expect(x > pitch + 1e-9 && y > pitch + 1e-9).toBe(false)
        }
      }
    }
  })

  it('traces every small-grid topology without losing cells or filling holes', () => {
    for (let mask = 0; mask < 512; mask++) {
      const columns = Array.from({ length: 3 }, (_, x) =>
        Array.from({ length: 3 }, (_, y) => (mask >> (x * 3 + y)) & 1)
      )
      const data = model(columns)
      const blobs = buildHeatmapBlobs(data, [1])
      expect(
        blobs.reduce((sum, blob) => sum + area(blob), 0),
        `mask ${mask}`
      ).toBe(data.cells.length)
      expect(new Set(blobs.flatMap(blob => blob.cells))).toEqual(
        new Set(data.cells)
      )
      for (const blob of blobs) {
        expect(heatmapBlobPath(blob, 0.2, 0.1)).not.toMatch(/NaN|Infinity/)
      }
    }
  })

  it('insets only the perimeter of a solid region and handles unmeasured charts', () => {
    const [blob] = buildHeatmapBlobs(
      model([
        [1, 1],
        [1, 1],
      ]),
      [1]
    )
    expect(blob!.rings[0]).toHaveLength(4)
    const path = heatmapBlobPath(blob!, 20, 40)
    expect(path.match(/Q/g)).toHaveLength(4)
    expect(path).toContain('39')
    expect(path).toContain('79')
    expect(heatmapBlobPath(blob!, 0, 40)).toBe('')
  })

  it.each([
    { pitch: 8, gap: 1 },
    { pitch: 16, gap: 2 },
    { pitch: 28, gap: 3 },
    { pitch: 48, gap: 4 },
  ])(
    'keeps full edge clearance and shared gaps at $pitch px density',
    ({ pitch, gap }) => {
      const blobs = buildHeatmapBlobs(model([[1], [2]]), [2])
      const grid = { columns: 2, rows: 1 }
      const bounds = blobs.map(blob => {
        const coordinates = [
          ...heatmapBlobPath(blob, pitch, pitch, grid).matchAll(
            /([\d.]+),([\d.]+)/g
          ),
        ]
        const xs = coordinates.map(match => Number(match[1]))
        const ys = coordinates.map(match => Number(match[2]))
        return {
          left: Math.min(...xs),
          right: Math.max(...xs),
          top: Math.min(...ys),
          bottom: Math.max(...ys),
        }
      })
      expect(bounds[0]).toEqual({
        left: gap,
        right: pitch - gap / 2,
        top: gap,
        bottom: pitch - gap,
      })
      expect(bounds[1]).toEqual({
        left: pitch + gap / 2,
        right: pitch * 2 - gap,
        top: gap,
        bottom: pitch - gap,
      })
    }
  )

  it('uses the narrower dimension and keeps the liner from consuming tiny cells', () => {
    for (const size of [0.2, 8, 16, 28, 48]) {
      const detail = heatmapBlobDetail(size, 200)
      expect(detail).toEqual(heatmapBlobDetail(200, size))
      expect(detail.liner * 2).toBeLessThanOrEqual(size / 4)
    }
    expect(heatmapBlobDetail(0, 200).liner).toBe(0)
  })
})
