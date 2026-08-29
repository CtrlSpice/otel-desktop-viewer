import { describe, expect, it } from 'vitest'
import type { HistogramSlicePoint } from '@/components/metrics/utils/histogram-aggregation'
import {
  buildHistogramHeatmapData,
  buildHistogramHeatmapDataWithStats,
} from './histogram-heatmap-data'

const BASE_NS = 1_700_000_000_000_000_000n

function explicitPoint(
  timestamp: bigint,
  bounds: number[],
  counts: number[]
): HistogramSlicePoint {
  return {
    kind: 'histogram',
    timestamp,
    attributesKey: '',
    bounds,
    counts,
    totals: {
      count: counts.reduce((total, count) => total + count, 0),
      sum: 0,
      min: 0,
      max: 0,
    },
  }
}

function exponentialPoint(
  timestamp: bigint,
  negativeCounts: number[],
  positiveCounts: number[],
  zeroCount = 0,
  zeroThreshold = 0.001
): HistogramSlicePoint {
  return {
    kind: 'expHistogram',
    timestamp,
    attributesKey: '',
    scale: 1,
    zeroThreshold,
    zeroCount,
    positiveOffset: 0,
    positiveCounts,
    negativeOffset: 0,
    negativeCounts,
    totals: {
      count:
        zeroCount +
        negativeCounts.reduce((total, count) => total + count, 0) +
        positiveCounts.reduce((total, count) => total + count, 0),
      sum: 0,
      min: -2,
      max: 2,
    },
  }
}

describe('buildHistogramHeatmapData', () => {
  it('retains only nonzero cells and counts for a 100x1000 grid', () => {
    const columnCount = 100
    const rowCount = 1000
    const bounds = Array.from({ length: rowCount - 1 }, (_, index) => index)
    const points = Array.from({ length: columnCount }, (_, columnIndex) => {
      const counts = Array<number>(rowCount).fill(0)
      counts[(columnIndex * 37) % rowCount] = columnIndex + 1
      return explicitPoint(
        BASE_NS + BigInt(columnIndex) * 1_000_000n,
        bounds.slice(),
        counts
      )
    })

    const { data: result, stats } = buildHistogramHeatmapDataWithStats(points)
    const retainedCountEntries = [...result.countByColumn.values()].reduce(
      (total, counts) => total + counts.size,
      0
    )

    expect(result.columns).toHaveLength(columnCount)
    expect(result.rows).toHaveLength(rowCount)
    expect(result.cells).toHaveLength(columnCount)
    expect(result.countByColumn.size).toBe(columnCount)
    expect(retainedCountEntries).toBe(columnCount)
    expect(result.cells.length).toBeLessThan(
      result.columns.length * result.rows.length
    )
    expect(result.maxCount).toBe(columnCount)
    expect(result.distinctNonZeroCount).toBe(columnCount)
    expect(stats).toEqual({
      explicitSchemaCount: 1,
      exponentialSchemaCount: 0,
      descriptorConstructionCount: rowCount,
    })
  })

  it('keeps duplicate display labels as distinct raw rows and omits zero intersections', () => {
    const result = buildHistogramHeatmapData([
      explicitPoint(BASE_NS, [1.001, 1.002, 1.003], [1, 0, 0, 4]),
    ])
    const duplicateRows = result.rows.filter(
      row => row.label === '(1.00, 1.00]'
    )

    expect(duplicateRows).toHaveLength(2)
    expect(new Set(duplicateRows.map(row => row.key)).size).toBe(2)
    expect(result.cells).toHaveLength(2)
    expect(result.countByColumn.get(result.columns[0]!.key)?.size).toBe(2)
    expect(
      result.countByColumn
        .get(result.columns[0]!.key)
        ?.get(duplicateRows[0]!.key)
    ).toBeUndefined()
  })

  it('unifies physical intervals across shifted explicit schemas', () => {
    const firstNs = BASE_NS + 100n
    const secondNs = BASE_NS + 900n
    const { data: result, stats } = buildHistogramHeatmapDataWithStats([
      explicitPoint(firstNs, [1, 2], [0, 1, 0]),
      explicitPoint(secondNs, [0, 1, 2], [0, 0, 2, 0]),
    ])

    expect(stats).toEqual({
      explicitSchemaCount: 2,
      exponentialSchemaCount: 0,
      descriptorConstructionCount: 7,
    })
    expect(result.rows).toHaveLength(5)
    expect(result.rows.filter(row => row.key === 'explicit:1:2')).toHaveLength(
      1
    )
    expect(
      result.countByColumn.get(result.columns[0]!.key)?.get('explicit:1:2')
    ).toBe(1)
    expect(
      result.countByColumn.get(result.columns[1]!.key)?.get('explicit:1:2')
    ).toBe(2)
    expect(result.columns.map(column => column.timestampNs)).toEqual([
      firstNs,
      secondNs,
    ])
    expect(new Set(result.columns.map(column => column.key)).size).toBe(2)
  })

  it('reuses exponential descriptors while retaining only nonzero cells', () => {
    const { data: result, stats } = buildHistogramHeatmapDataWithStats([
      exponentialPoint(BASE_NS, [0, 2], [0, 3]),
      exponentialPoint(BASE_NS + 1_000_000n, [0, 0], [4, 0]),
    ])

    expect(stats).toEqual({
      explicitSchemaCount: 0,
      exponentialSchemaCount: 1,
      descriptorConstructionCount: 5,
    })
    expect(result.rows).toHaveLength(5)
    expect(result.cells.map(cell => cell.bucketKey).sort()).toEqual([
      'exponential:1:negative:1',
      'exponential:1:positive:0',
      'exponential:1:positive:1',
    ])
    expect(
      [...result.countByColumn.values()].reduce(
        (total, counts) => total + counts.size,
        0
      )
    ).toBe(3)
    expect(
      result.rows.find(row => row.key === 'exponential:zero:0.001')?.label
    ).toBe('[-1.0e-3, +1.0e-3]')
  })

  it('describes a zero-threshold exponential bucket as exact zero', () => {
    const result = buildHistogramHeatmapData([
      exponentialPoint(BASE_NS, [], [], 1, 0),
    ])

    expect(result.rows).toEqual([
      { key: 'exponential:zero:0', label: '0', order: 0 },
    ])
  })
})
