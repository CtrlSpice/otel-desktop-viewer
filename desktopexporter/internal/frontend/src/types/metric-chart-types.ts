// Pure types shared between the metric chart components and modules
// that build chart data (the metric view context, the LTTB downsampler,
// the legend). Kept here because .svelte files can't be imported by
// plain .ts under svelte-check, so a typed-shared neutral location
// matters.

import type { Attribute } from '@/types/api-types'

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
 * One per-attribute timeseries in the legend. The caller is expected
 * to have already grouped its datapoints into these timeseries
 * entries and to pass them in the same order as the chart renders
 * them, so the n-th legend row's swatch colour matches the n-th
 * line on the chart. Checked rows use `ctx.timeseriesColorByKey.get(key)`
 * (a colour from the stem-rotated pool); unchecked rows use neutral.
 */
export type LegendTimeseries = {
  /** Stable identifier for this attribute set, used as the bind key.
   * In practice this is the `attributesKey` (canonical "key=value|..."
   * string) the backend materialises on every datapoint and bucket-
   * series point as `attrs_canonical`. The same encoding is used for
   * Gauge/Sum, Histogram, and ExponentialHistogram timeseries, so a
   * single legend implementation covers all metric types. */
  key: string
  /** Attributes that distinguish this timeseries from siblings. May
   * be empty for a metric whose datapoints carry no attributes. */
  attributes: Attribute[]
  /** Optional sample count or other small annotation shown after the
   * attribute pairs. Purely informational; not bound. */
  badge?: string
}
