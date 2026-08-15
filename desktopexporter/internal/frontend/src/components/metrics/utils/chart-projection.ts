// Projection: backend MetricTimeseries → layerchart-shaped
// ChartTimeseries. Lives in a plain .ts module (not the chart's
// <script module>) so the metric view context, which is also a .ts
// module, can call it -- svelte-check won't resolve exports from
// .svelte files into a .ts importer.

import type {
  GaugeDataPoint,
  MetricTimeseries,
  SumDataPoint,
} from '@/types/api-types'
import type { ChartPoint, ChartTimeseries } from '@/types/metric-chart-types'

/**
 * Project backend-grouped MetricTimeseries into the {date, value}
 * shape layerchart wants. The grouping itself is already done -- the
 * backend emits one MetricTimeseries per (metric, attribute-set), and
 * timeseries arrive ordered "newest activity first" (latest dp
 * timestamp desc). We preserve that order so positional colour
 * assignment in the legend matches the chart line colour 1:1.
 *
 * Datapoints arrive timestamp-desc inside each timeseries, and layerchart's
 * LineChart wants monotonically-increasing x, so this walks each series
 * backwards. It used to build forwards and sort, which re-ordered data the
 * store had already ordered.
 */
export function timeseriesToChartTimeseries(timeseries: MetricTimeseries[]): {
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
    // Walked backwards, because the store sends datapoints newest-first and the
    // chart wants oldest-first. That order is not incidental -- the datapoint
    // list and "last seen" both read datapoints[0] as the newest -- so the
    // chart adapts rather than the wire.
    //
    // This used to build the array forwards and then sort it, calling
    // getTime() twice per comparison on data the store had already ordered:
    // roughly 23,000 points per Gauge sorted into the order they arrived in,
    // reversed. Iterating from the end is the same result at no cost.
    for (let i = ts.datapoints.length - 1; i >= 0; i--) {
      const dp = ts.datapoints[i]!
      if (dp.metricType !== 'Gauge' && dp.metricType !== 'Sum') continue
      const typed = dp as GaugeDataPoint | SumDataPoint
      const value = typed.doubleValue ?? typed.intValue ?? 0
      points.push({
        // The store sends epoch milliseconds; dividing the nanosecond BigInt
        // here cost one division per datapoint for no added precision.
        date: new Date(dp.timestampMs),
        value,
        // Cumulative Sums only; a Gauge has no interval to describe.
        delta: typed.metricType === 'Sum' ? (typed.delta ?? null) : null,
        isReset: typed.metricType === 'Sum' ? (typed.isReset ?? null) : null,
      })
    }
    chartTimeseries.push({
      key: ts.attributesKey,
      label: ts.attributesKey === '' ? 'default' : ts.attributesKey,
      points,
    })
    keys.push(ts.attributesKey)
  }

  return { chartTimeseries, keys }
}
