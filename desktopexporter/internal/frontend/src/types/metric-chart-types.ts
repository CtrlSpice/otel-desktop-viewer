// Pure types shared between the metric chart components and modules
// that build chart data (the metric view context, the LTTB downsampler,
// the legend). Kept here because .svelte files can't be imported by
// plain .ts under svelte-check, so a typed-shared neutral location
// matters.

import type { Attribute } from '@/types/api-types'

export type ChartPoint = {
  date: Date
  value: number
  /** Exact source or bucket timestamp when the chart's millisecond Date is a
   * lossy rendering coordinate. Cursor reconciliation and column activation use
   * this identity so two sub-millisecond points never collapse together. */
  timestampNs?: bigint
  /** Exact raw datapoint identity when this point still represents one source
   * sample. Synthetic aggregates deliberately omit it and remain read-only. */
  sourceDatapointID?: string
  /** Activity since the previous reading, computed by the store. Present only
   *  on Cumulative Sums, and null on a series' first point, which describes no
   *  interval. Derived views read this rather than differencing the points they
   *  were given -- those have been through the reduction, so consecutive chart
   *  points are frequently not consecutive datapoints. */
  delta?: number | null
  /** The counter restarted in this interval, as the store saw it. */
  isReset?: boolean | null
  /** Rate view only: slope of the drawn segment arriving at this point, from
   *  the store. The tangent overlay reads it instead of differencing points. */
  slope?: number | null
}

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
 * One per-attribute timeseries in the legend. The caller is expected
 * to have already grouped its datapoints into these timeseries
 * entries and to pass them in the same order as the chart renders
 * them, so the n-th legend row's swatch colour matches the n-th
 * line on the chart. Checked rows use `ctx.timeseriesColorByKey.get(key)`
 * (a colour from the stem-rotated pool); unchecked rows use neutral.
 */
export type LegendTimeseries = {
  /** Stable identifier for this series, used as the bind key. This is
   * `MetricTimeseries.attributesKey`, which is now the series id --
   * content-derived from (stream, resource, labels) rather than a rendering
   * of the labels. The same id covers Gauge/Sum, Histogram and
   * ExponentialHistogram, so one legend implementation serves all of them. */
  key: string
  /** Attributes that distinguish this timeseries from siblings. May
   * be empty for a metric whose datapoints carry no attributes.
   *
   * These are the datapoint labels, plus any resource attributes that differ
   * between the series of this metric. The resource ones are only present when
   * a metric spans several: two replicas of one service emit byte-identical
   * labels, so without something from the resource the legend would show rows
   * a user cannot tell apart -- worse than the single merged line that
   * splitting them replaced. Only the resource attributes that actually vary
   * are merged in, since a whole resource is ~15 attributes of
   * mostly-identical noise.
   *
   * Merging rather than carrying a separate field is deliberate: the series
   * table already renders a column per attribute key that differs across rows,
   * so a distinguishing host.name simply becomes a column. */
  attributes: Attribute[]
  /** Optional sample count or other small annotation shown after the
   * attribute pairs. Purely informational; not bound. */
  badge?: string
}
