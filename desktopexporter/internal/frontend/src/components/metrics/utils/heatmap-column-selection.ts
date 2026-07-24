import {
  histogramQuantilesForDatapoint,
  histogramSliceToDatapoint,
  QUANTILE_LABELS,
} from '@/components/metrics/utils/histogram-aggregation'
import type { HistogramSlicePoint } from '@/components/metrics/utils/histogram-aggregation'
import type { HistogramTotals } from '@/components/metrics/utils/histogram-merge'
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
  let idx = series.findIndex(s => s.timestamp === timestampNs)
  if (idx < 0) {
    const targetMs = Number(timestampNs / 1_000_000n)
    idx = series.findIndex(s => Number(s.timestamp / 1_000_000n) === targetMs)
  }
  if (idx < 0) return null

  const slice = series[idx]!
  const dp = histogramSliceToDatapoint(slice, 'heatmap-column', temporality)
  const quantiles: Record<string, number | null> = {}
  for (const { key } of QUANTILE_LABELS) {
    quantiles[key] = histogramQuantilesForDatapoint(dp)[key] ?? null
  }

  return {
    timestampMs: Number(timestampNs / 1_000_000n),
    totals: slice.totals,
    quantiles,
  }
}
