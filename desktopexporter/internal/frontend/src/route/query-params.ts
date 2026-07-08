export const SPAN_PARAM = 'span'

export const METRIC_VIEW_PARAMS = ['agg', 'htab', 'hscope', 'dp'] as const
export type MetricViewParam = (typeof METRIC_VIEW_PARAMS)[number]

/** Query keys scoped to a selected trace or metric item (not shared across signals). */
export const SIGNAL_ITEM_QUERY_PARAMS = [
  SPAN_PARAM,
  ...METRIC_VIEW_PARAMS,
] as const
