import { describe, it, expect } from 'vitest'
import { quantilePointSelectionAt } from './quantile-point-selection'
import type { HistogramSlicePoint } from './histogram-aggregation'

function slice(
  attributesKey: string,
  timestampNs: bigint,
  p95: number
): HistogramSlicePoint {
  return {
    kind: 'histogram',
    timestamp: timestampNs,
    attributesKey,
    bounds: [1, 5, 10],
    counts: [10, 0, 0, 0],
    totals: { count: 10, sum: 5, min: 0.1, max: 0.9 },
    quantiles: { '0.5': 1, '0.95': p95 },
  }
}

const COLUMN_START = 1_000_000_000_000n

describe('quantilePointSelectionAt', () => {
  // The bug this replaced: the caller passed every per-series slice in the
  // window and the lookup matched on the clicked timestamp. A column is wider
  // than a per-series bucket, so that match found the column's *first* bucket
  // and reported it as the column. Slices now arrive already merged over the
  // column, and are read by series key alone.
  it('reads one slice per series without matching timestamps', () => {
    // Deliberately stamped away from the clicked timestamp: the store stamps a
    // single-bucket merge at the data's own start, which is not the column
    // boundary the reader clicked.
    const columnSlices = [
      slice('pod=a', COLUMN_START + 7n, 42),
      slice('pod=b', COLUMN_START + 9n, 99),
    ]
    const got = quantilePointSelectionAt(
      columnSlices,
      [slice('merged', COLUMN_START, 50)],
      COLUMN_START,
      null,
      'Delta'
    )
    expect(got).not.toBeNull()
    expect(got!.series.map(s => s.seriesKey)).toEqual(['pod=a', 'pod=b'])
    expect(got!.series[0]!.quantiles['0.95']).toBe(42)
    expect(got!.series[1]!.quantiles['0.95']).toBe(99)
  })

  it('reports only the visible series', () => {
    const columnSlices = [
      slice('pod=a', COLUMN_START, 42),
      slice('pod=b', COLUMN_START, 99),
    ]
    const got = quantilePointSelectionAt(
      columnSlices,
      [slice('merged', COLUMN_START, 50)],
      COLUMN_START,
      new Set(['pod=b']),
      'Delta'
    )
    expect(got!.series.map(s => s.seriesKey)).toEqual(['pod=b'])
  })

  it('skips a visible series the column holds nothing for', () => {
    const got = quantilePointSelectionAt(
      [slice('pod=a', COLUMN_START, 42)],
      [slice('merged', COLUMN_START, 50)],
      COLUMN_START,
      new Set(['pod=a', 'pod=gone']),
      'Delta'
    )
    expect(got!.series.map(s => s.seriesKey)).toEqual(['pod=a'])
  })

  it('is null when the column holds nothing at all', () => {
    const got = quantilePointSelectionAt(
      [],
      [slice('merged', COLUMN_START, 50)],
      COLUMN_START,
      null,
      'Delta'
    )
    expect(got).toBeNull()
  })
})
