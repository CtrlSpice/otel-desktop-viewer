// Pure type + projection helpers for the gauge/sum line chart. Lives
// outside MetricTimeSeriesChart.svelte so plain .ts modules (notably
// the metric view context) can import them without dragging Svelte's
// component resolver -- pure tsc can't read .svelte files, and these
// helpers have no runtime dependency on Svelte.

import type {
  GaugeDataPoint,
  MetricTimeseries,
  SumDataPoint,
} from '@/types/api-types'

export type ChartPoint = { date: Date; value: number }

/**
 * One per-attribute timeseries projected for layerchart. The
 * MetricTimeSeriesChart component renders one line per timeseries
 * that's still in `visibleKeys`; hidden timeseries are skipped
 * entirely (no greyed-out ghost line) so the chart stays readable
 * when the user is iteratively narrowing the visible set.
 */
export type ChartTimeseries = {
  /** Stable per-timeseries id. The canonical "key=value|..." string
   * from MetricTimeseries.attributesKey. The legend uses the same
   * key, so checking/unchecking maps 1:1. */
  key: string
  /** Human label for the layerchart series. The chart's tooltip
   * surfaces this. We feed in the same canonical attribute string
   * so the tooltip and the legend agree. */
  label: string
  points: ChartPoint[]
}

/**
 * Project backend-grouped MetricTimeseries into the {date, value}
 * shape layerchart wants. The grouping itself is already done -- the
 * backend emits one MetricTimeseries per (metric, attribute-set), and
 * timeseries arrive ordered "newest activity first" (latest dp
 * timestamp desc). We preserve that order so positional colour
 * assignment in the legend matches the chart line colour 1:1.
 *
 * Datapoints arrive timestamp-desc inside each timeseries; we re-sort
 * ascending here because layerchart's LineChart expects
 * monotonically-increasing x values to draw a connected line.
 */
export function timeseriesToChartTimeseries(
  timeseries: MetricTimeseries[]
): {
  chartTimeseries: ChartTimeseries[]
  /** Convenience: same `key` strings the timeseries have, in the
   * same order. Caller can seed `visibleKeys` from this without
   * having to map over `chartTimeseries`. */
  keys: string[]
} {
  const chartTimeseries: ChartTimeseries[] = []
  const keys: string[] = []

  for (const ts of timeseries) {
    const points: ChartPoint[] = []
    for (const dp of ts.datapoints) {
      if (dp.metricType !== 'Gauge' && dp.metricType !== 'Sum') continue
      const typed = dp as GaugeDataPoint | SumDataPoint
      const value = typed.doubleValue ?? typed.intValue ?? 0
      const t = Number(dp.timestamp / 1_000_000n)
      points.push({ date: new Date(t), value })
    }
    points.sort((a, b) => a.date.getTime() - b.date.getTime())
    chartTimeseries.push({
      key: ts.attributesKey,
      label: ts.attributesKey === '' ? 'default' : ts.attributesKey,
      points,
    })
    keys.push(ts.attributesKey)
  }

  return { chartTimeseries, keys }
}
