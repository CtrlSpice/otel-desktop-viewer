import type { MetricSeriesViewData } from '@/types/api-types'
import type { ChartPoint, ChartTimeseries } from '@/types/metric-chart-types'

/**
 * Map one reduced raw Sum datapoint onto the store's synthetic Rate grid.
 *
 * The source id is used only to recover the exact raw nanosecond timestamp.
 * Rate points do not represent individual datapoints and therefore cannot
 * carry that id themselves. Keeping the lookup on `timeseries.datapoints`
 * also means a raw-only detail datapoint omitted by reduction cannot be paired
 * with an unrelated bucket merely because its millisecond coordinate is near.
 */
export function rateBucketStartForSourceDatapoint(
  timeseries: Pick<MetricSeriesViewData, 'datapoints' | 'views'>,
  datapointID: string
): bigint | undefined {
  const datapoint = timeseries.datapoints.find(
    candidate => candidate.id === datapointID
  )
  if (datapoint?.metricType !== 'Sum' || !timeseries.views?.length) {
    return undefined
  }

  // Views arrive ordered by bucket start. Find the greatest start at or before
  // the raw timestamp: the store uses half-open [start, nextStart) buckets.
  let low = 0
  let high = timeseries.views.length
  while (low < high) {
    const middle = Math.floor((low + high) / 2)
    if (timeseries.views[middle]!.bucketStart <= datapoint.timestamp) {
      low = middle + 1
    } else {
      high = middle
    }
  }

  const bucket = timeseries.views[low - 1]
  // A populated bucket with a null rate is the first cumulative bucket. The
  // chart omits it because no preceding reading exists, so there is no honest
  // point to select.
  if (!bucket || bucket.sampleCount === 0 || bucket.rate === null) {
    return undefined
  }
  return bucket.bucketStart
}

/**
 * Project backend-grouped MetricSeriesViewData into the {date, value}
 * shape layerchart wants. The grouping itself is already done -- the
 * backend emits one MetricSeriesViewData per (metric, attribute-set), and
 * timeseries arrive ordered "newest activity first" (latest dp
 * timestamp desc). We preserve that order so positional colour
 * assignment in the legend matches the chart line colour 1:1.
 *
 * Datapoints arrive newest first; layerchart requires increasing timestamps.
 *
 * @returns Projected chart series and their keys. `keys` preserves the input
 * order so callers can seed `visibleKeys` without mapping over the series.
 */
export function timeseriesToChartTimeseries(
  timeseries: MetricSeriesViewData[],
  /** How to name a series for a human. `seriesRef` is an opaque id, so it
   *  identifies a line but cannot label one -- a tooltip showing it reads
   *  as a uuid. Resolving the label needs every series of the metric (to know
   *  which resource attributes distinguish them), which this function is not
   *  always given, so the caller supplies it. */
  labelFor?: (ts: MetricSeriesViewData) => string
) {
  const chartTimeseries: ChartTimeseries[] = []
  const keys: string[] = []

  for (const ts of timeseries) {
    const points: ChartPoint[] = []
    // Reverse traversal preserves wire order while producing oldest-first points.
    for (let i = ts.datapoints.length - 1; i >= 0; i--) {
      const dp = ts.datapoints[i]!
      if (dp.metricType !== 'Gauge' && dp.metricType !== 'Sum') continue
      // Chart coordinates are IEEE-754 numbers. Received integer measurements
      // stay bigint on the datapoint; only this display projection approximates.
      const value =
        dp.doubleValue ?? (dp.intValue === null ? 0 : Number(dp.intValue))
      points.push({
        date: new Date(dp.timestampMs),
        value,
        timestampNs: dp.timestamp,
        sourceDatapointID: dp.id,
        // Cumulative Sums only; a Gauge has no interval to describe. Exact
        // integral deltas stay bigint until this display-number projection.
        delta:
          dp.metricType === 'Sum' && dp.delta != null ? Number(dp.delta) : null,
        isReset: dp.metricType === 'Sum' ? (dp.isReset ?? null) : null,
      })
    }
    chartTimeseries.push({
      key: ts.seriesRef,
      label: labelFor?.(ts) ?? ts.seriesRef,
      points,
    })
    keys.push(ts.seriesRef)
  }

  return { chartTimeseries, keys }
}
