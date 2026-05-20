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
import type {
  ChartPoint,
  ChartTimeseries,
} from '@/types/metric-chart-types'
import { downsampleLTTB } from '@/components/metrics/utils/metric-downsample'

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
  timeseries: MetricTimeseries[],
  opts?: { downsampleTo?: number },
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
    let points: ChartPoint[] = []
    for (const dp of ts.datapoints) {
      if (dp.metricType !== 'Gauge' && dp.metricType !== 'Sum') continue
      const typed = dp as GaugeDataPoint | SumDataPoint
      const value = typed.doubleValue ?? typed.intValue ?? 0
      const t = Number(dp.timestamp / 1_000_000n)
      points.push({ date: new Date(t), value })
    }
    points.sort((a, b) => a.date.getTime() - b.date.getTime())
    if (opts?.downsampleTo) {
      points = downsampleLTTB(points, opts.downsampleTo)
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
