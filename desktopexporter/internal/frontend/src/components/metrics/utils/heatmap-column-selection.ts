import { QUANTILE_LABELS } from '@/components/metrics/utils/histogram-aggregation'
import type { HistogramSlicePoint } from '@/components/metrics/utils/histogram-aggregation'
import type { HistogramTotals } from '@/components/metrics/utils/histogram-aggregation'
import type { SelectionLegendRow } from '@/components/metrics/Charts/ChartSelectionLegend.svelte'
import { formatMetricValue } from '@/components/metrics/utils/format-metric-value'

export type HeatmapColumnSelection = {
  timestampMs: number
  totals: HistogramTotals
  quantiles: Record<string, number | null>
}

/** Column groups for histogram distribution stats (heatmap + bar chart).
 *  Layout: count/sum | min/max | scale/zeros (exp only) | p50/p95 | p99 */
export function histogramDistributionLegendColumns(opts: {
  totals: HistogramTotals
  quantiles: Record<string, number | null>
  unit?: string
  expHistogram?: { scale: number; zeroCount: number }
}): SelectionLegendRow[][] {
  const unitSuffix = opts.unit?.trim() ? ` ${opts.unit.trim()}` : ''
  const formatValue = (value: number) =>
    `${formatMetricValue(value)}${unitSuffix}`
  const neutral = 'var(--color-base-content)'

  const volume: SelectionLegendRow[] = [
    {
      key: 'count',
      color: neutral,
      label: 'count',
      valueText: String(opts.totals.count),
    },
    {
      key: 'sum',
      color: neutral,
      label: 'sum',
      valueText: formatValue(opts.totals.sum),
    },
  ]

  const range: SelectionLegendRow[] = [
    {
      key: 'min',
      color: neutral,
      label: 'min',
      valueText: formatValue(opts.totals.min),
    },
    {
      key: 'max',
      color: neutral,
      label: 'max',
      valueText: formatValue(opts.totals.max),
    },
  ]

  const columns: SelectionLegendRow[][] = [volume, range]

  if (opts.expHistogram) {
    columns.push([
      {
        key: 'scale',
        color: neutral,
        label: 'scale',
        valueText: String(opts.expHistogram.scale),
      },
      {
        key: 'zeros',
        color: neutral,
        label: 'zeros',
        valueText: String(opts.expHistogram.zeroCount),
      },
    ])
  }

  const midQuantiles: SelectionLegendRow[] = []
  let p99: SelectionLegendRow | null = null
  for (const { key, label } of QUANTILE_LABELS) {
    const value = opts.quantiles[key]
    const row: SelectionLegendRow = {
      key,
      color: neutral,
      label,
      valueText:
        value === null || value === undefined ? '—' : formatValue(value),
    }
    if (key === '0.99') {
      p99 = row
    } else {
      midQuantiles.push(row)
    }
  }

  columns.push(midQuantiles)
  if (p99) columns.push([p99])

  return columns
}

/** Sticky legend rows for a merged heatmap bucket selection. */
export function histogramColumnSelectionLegendRows(
  sel: HeatmapColumnSelection,
  unit: string
): SelectionLegendRow[][] {
  return histogramDistributionLegendColumns({
    totals: sel.totals,
    quantiles: sel.quantiles,
    unit,
  })
}

/** Quantiles tab: count, min, max on one line — no sum or quantile rows. */
export function quantileMergedSelectionLegendRows(
  sel: HeatmapColumnSelection,
  unit: string
): SelectionLegendRow[] {
  const unitSuffix = unit.trim() ? ` ${unit.trim()}` : ''
  const formatValue = (value: number) =>
    `${formatMetricValue(value)}${unitSuffix}`

  return [
    {
      key: 'totals-inline',
      color: 'var(--color-base-content)',
      label: '',
      valueText: `count: ${sel.totals.count} | min: ${formatValue(sel.totals.min)} | max: ${formatValue(sel.totals.max)}`,
    },
  ]
}

export function heatmapColumnSelectionAt(
  series: readonly HistogramSlicePoint[],
  timestampNs: bigint,
  temporality: string
): HeatmapColumnSelection | null {
  const idx = series.findIndex(s => s.timestamp === timestampNs)
  if (idx < 0) return null

  const slice = series[idx]!
  // The store's numbers. Computing them here meant rebuilding this bucket's
  // list and walking it once per quantile, for a value the response already
  // carried.
  const quantiles: Record<string, number | null> = {}
  for (const { key } of QUANTILE_LABELS) {
    quantiles[key] = slice.quantiles?.[key] ?? null
  }

  return {
    timestampMs: Number(timestampNs / 1_000_000n),
    totals: slice.totals,
    quantiles,
  }
}

/**
 * Where a selected heatmap column ends, exclusive by one nanosecond.
 *
 * The next column's own start, not the selected start plus a width. Columns are
 * cut in local time, so they are not all the same number of nanoseconds wide: a
 * local day is 23, 24 or 25 hours across a DST transition. Deriving a single
 * width -- the smallest gap, say -- and adding it would fetch 23 hours of a
 * 25-hour column and silently lose the other two, which is the defect fetching
 * the column exists to remove.
 *
 * One nanosecond short because the store filters `timestamp >= start and
 * timestamp <= end` while it cuts buckets half-open; ending on the next
 * boundary pulls that column's first reading in and counts it in both columns.
 *
 * The last column has no next one and borrows the preceding gap, which errs by
 * ending early rather than reaching into a column that does not exist. A lone
 * column has no gap to borrow and takes the window's end, which is what a single
 * column spans by definition.
 */
export function heatmapColumnEndNs(
  columnStarts: readonly bigint[],
  selectedStartNs: bigint,
  windowEndNs: bigint
): bigint | null {
  const sorted = [...new Set(columnStarts)].sort((a, b) =>
    a < b ? -1 : a > b ? 1 : 0
  )
  const i = sorted.indexOf(selectedStartNs)
  if (i >= 0 && i + 1 < sorted.length) return sorted[i + 1]! - 1n
  if (sorted.length >= 2) {
    const lastGap = sorted[sorted.length - 1]! - sorted[sorted.length - 2]!
    if (lastGap > 0n) return selectedStartNs + lastGap - 1n
  }
  return windowEndNs > selectedStartNs ? windowEndNs : null
}
