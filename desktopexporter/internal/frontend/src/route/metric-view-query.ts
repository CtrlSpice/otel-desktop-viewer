import { navigateCurrentRoute, readRoute, withoutParams } from './router'
import { METRIC_VIEW_PARAMS, type MetricViewParam } from './query-params'

export type { MetricViewParam }
export { METRIC_VIEW_PARAMS }

export type TimeseriesMetricViewQuery = {
  kind: 'timeseries'
  agg: string | null
  dp: string | null
}

export type HistogramMetricViewQuery = {
  kind: 'histogram'
  htab: 'heatmap' | 'quantiles' | 'histogram'
  hscope: 'window' | 'bucket'
  dp: string | null
}

export type MetricViewQuery =
  | TimeseriesMetricViewQuery
  | HistogramMetricViewQuery

export type MetricViewParseContext = {
  isHistogramKind: boolean
  allowedAggs: readonly string[]
  datapointIds: ReadonlySet<string>
}

const HTAB_VALUES = ['heatmap', 'quantiles', 'histogram'] as const
const HSCOPE_VALUES = ['window', 'bucket'] as const

/**
 * Validates the `dp` query param against known datapoint ids.
 *
 * @param query - raw route query
 * @param datapointIds - ids present on the current metric
 * @returns validated datapoint id, or `null`
 *
 * @remarks Stale ids from shared links become `null`.
 */
function parseDatapointParam(
  query: Record<string, string>,
  datapointIds: ReadonlySet<string>
): string | null {
  const dp = query.dp || null
  return dp && datapointIds.has(dp) ? dp : null
}

/**
 * Returns a raw string when it is in the allowed set.
 *
 * @param raw - query value
 * @param allowed - permitted values
 * @returns matched value, or `null`
 *
 * @example `('rate', ['raw', 'rate'])` → `'rate'`
 */
function parseOptionalMember(
  raw: string | undefined,
  allowed: readonly string[]
): string | null {
  return raw && allowed.includes(raw) ? raw : null
}

/**
 * Returns a raw string when allowed, otherwise a fallback default.
 *
 * @param raw - query value
 * @param allowed - permitted values
 * @param fallback - value used when `raw` is missing or invalid
 * @returns matched or fallback value
 *
 * @example invalid `htab` → `'heatmap'`
 */
function parseEnumMember<T extends string>(
  raw: string | undefined,
  allowed: readonly T[],
  fallback: T
): T {
  return raw && allowed.includes(raw as T) ? (raw as T) : fallback
}

/**
 * Builds the histogram branch of {@link MetricViewQuery}.
 *
 * @param query - raw route query
 * @param dp - validated datapoint id
 * @returns histogram metric view union member
 *
 * @remarks Ignores `agg`; cross-kind keys are not carried into the result.
 */
function parseHistogramMetricViewQuery(
  query: Record<string, string>,
  dp: string | null
): HistogramMetricViewQuery {
  return {
    kind: 'histogram',
    htab: parseEnumMember(query.htab, HTAB_VALUES, 'heatmap'),
    hscope: parseEnumMember(query.hscope, HSCOPE_VALUES, 'window'),
    dp,
  }
}

/**
 * Builds the timeseries branch of {@link MetricViewQuery}.
 *
 * @param query - raw route query
 * @param allowedAggs - aggregation views permitted for the current metric
 * @param dp - validated datapoint id
 * @returns timeseries metric view union member
 *
 * @remarks Ignores `htab` and `hscope`; cross-kind keys are not carried into the result.
 */
function parseTimeseriesMetricViewQuery(
  query: Record<string, string>,
  allowedAggs: readonly string[],
  dp: string | null
): TimeseriesMetricViewQuery {
  return {
    kind: 'timeseries',
    agg: parseOptionalMember(query.agg, allowedAggs),
    dp,
  }
}

/**
 * Parses and validates metric sub-view params for the current metric kind.
 *
 * @param query - raw route query
 * @param ctx - metric shape and validation context
 * @returns validated {@link MetricViewQuery}
 *
 * @remarks Branch is chosen from `ctx.isHistogramKind`, not from which keys are present.
 */
export function parseMetricViewQuery(
  query: Record<string, string>,
  ctx: MetricViewParseContext
): MetricViewQuery {
  const dp = parseDatapointParam(query, ctx.datapointIds)
  return ctx.isHistogramKind
    ? parseHistogramMetricViewQuery(query, dp)
    : parseTimeseriesMetricViewQuery(query, ctx.allowedAggs, dp)
}

/**
 * Serializes a metric sub-view union to URL query keys for its kind.
 *
 * @param q - validated metric sub-view
 * @returns partial query param record
 *
 * @remarks Drops `kind` and omits null fields.
 *
 * @example `{ kind: 'timeseries', agg: null, dp: 'x' }` → `{ dp: 'x' }`
 */
export function metricViewQueryToParams(
  q: MetricViewQuery
): Partial<Record<MetricViewParam, string>> {
  const { kind: _, ...fields } = q
  return Object.fromEntries(
    Object.entries(fields).filter(([, v]) => v != null)
  ) as Partial<Record<MetricViewParam, string>>
}

/**
 * Replaces metric params in a route query while preserving everything else.
 *
 * @param routeQuery - full existing query object
 * @param q - validated metric sub-view
 * @returns new query object
 *
 * @example Keeps `start` and `end` while swapping `agg`, `htab`, `hscope`, and `dp`.
 */
export function mergeRouteQueryWithMetricView(
  routeQuery: Record<string, string>,
  q: MetricViewQuery
): Record<string, string> {
  return {
    ...withoutParams(routeQuery, METRIC_VIEW_PARAMS),
    ...metricViewQueryToParams(q),
  }
}

/**
 * Writes the full metric sub-view to the current route query.
 *
 * @param q - validated metric sub-view
 * @param opts.push - use `pushState` instead of `replaceState`
 *
 * @remarks Replaces all metric params atomically and preserves non-metric query keys.
 */
export function setMetricViewQuery(
  q: MetricViewQuery,
  opts: { push?: boolean } = {}
): void {
  const query = mergeRouteQueryWithMetricView(readRoute().query, q)
  navigateCurrentRoute(query, { replace: !opts.push })
}
