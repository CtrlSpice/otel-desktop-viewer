// Pure type for the timeseries legend. Lives outside
// TimeseriesLegend.svelte so plain .ts modules (notably the metric
// view context) can import it without dragging Svelte's component
// resolver -- pure tsc can't read .svelte files.

import type { Attribute } from '@/types/api-types'

/**
 * One per-attribute timeseries in the chart. The caller is expected
 * to have already grouped its datapoints into these timeseries
 * entries and to pass them in the same order as the chart renders
 * them, so the n-th legend row's swatch colour matches the n-th
 * line on the chart. Checked rows use `ctx.timeseriesColorByKey.get(key)`
 * (a colour from the stem-rotated pool); unchecked rows use neutral.
 */
export type Timeseries = {
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
