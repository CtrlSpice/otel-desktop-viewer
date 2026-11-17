import {
  navigateCurrentRoute,
  readRoute,
  withoutParams,
  type HistoryMode,
  type RouteQuery,
} from './router'
import { METRIC_VIEW_PARAMS, type MetricViewParam } from './query-params'
import type { AggregationView } from '@/features/metrics/Charts/aggregation'

export type { MetricViewParam }
export { METRIC_VIEW_PARAMS }

export type TimeseriesMetricViewQuery = {
  kind: 'timeseries'
  agg: AggregationView | null
  dp: string | null
  series: string | null
  /** Requested chart series, including IDs absent from the current response. */
  visible: string[] | null
}

export type HistogramMetricViewQuery = {
  kind: 'histogram'
  htab: 'heatmap' | 'histogram'
  dp: string | null
  series: string | null
  visible: string[] | null
}

export type MetricViewQuery =
  TimeseriesMetricViewQuery | HistogramMetricViewQuery

export type MetricViewParseContext = {
  isHistogramKind: boolean
  allowedAggs: readonly AggregationView[]
  datapointIDs: ReadonlySet<string>
  /** Series ids present on the current metric. */
  seriesKeys: ReadonlySet<string>
  /** Shared histogram-family preference when the URL does not name a tab. */
  defaultHistogramTab?: 'heatmap' | 'histogram'
}

const HTAB_VALUES = ['heatmap', 'histogram'] as const

/** Null uses local defaults; an explicit empty set must stay empty. */
export function parseMetricVisibleSeries(query: RouteQuery): string[] | null {
  if (query.visible === undefined) return null
  if (query.visible === 'none') return []
  return [
    ...new Set(
      query.visible
        .split(',')
        .map(key => key.trim())
        .filter(Boolean)
    ),
  ].sort()
}

export function metricVisibleSeriesToParam(keys: Iterable<string>): string {
  return [...new Set(keys)].sort().join(',') || 'none'
}

/**
 * Validates the `dp` query param against known datapoint ids.
 *
 * @param query - raw route query
 * @param datapointIDs - ids present on the current metric
 * @returns validated datapoint id, or `null`
 *
 * @remarks Stale ids from shared links become `null`. Datapoint ids are minted
 * per row and deleted by retention, so this happens routinely on any link more
 * than a retention window old -- which is why `series` exists alongside it.
 */
function parseDatapointParam(
  query: RouteQuery,
  datapointIDs: ReadonlySet<string>
): string | null {
  const dp = query.dp || null
  return dp && datapointIDs.has(dp) ? dp : null
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
/**
 * Validates the `series` query param against the metric's series ids.
 *
 * @param query - raw route query
 * @param seriesKeys - series ids present on the current metric
 * @returns validated series id, or `null`
 *
 * @remarks
 * This is the durable half of a metric link. A series id is a generated,
 * database-local reference that remains stable across re-ingests and survives
 * retention deleting the specific datapoint a link was built from -- which
 * `dp` alone cannot.
 *
 * Still validated rather than trusted: a series can genuinely disappear when
 * its labels stop being reported, and a link naming one that no longer exists
 * should fall back to no selection rather than dangle.
 */
function parseSeriesParam(
  query: RouteQuery,
  seriesKeys: ReadonlySet<string>
): string | null {
  const series = query.series || null
  return series && seriesKeys.has(series) ? series : null
}

function parseOptionalMember<T extends string>(
  raw: string | undefined,
  allowed: readonly T[]
): T | null {
  for (const member of allowed) {
    if (member === raw) return member
  }
  return null
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
  for (const member of allowed) {
    if (member === raw) return member
  }
  return fallback
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
  query: RouteQuery,
  dp: string | null,
  series: string | null,
  visible: string[] | null,
  defaultTab: 'heatmap' | 'histogram' = 'heatmap'
): HistogramMetricViewQuery {
  return {
    kind: 'histogram',
    htab:
      query.htab === 'quantiles'
        ? 'heatmap'
        : parseEnumMember(query.htab, HTAB_VALUES, defaultTab),
    dp,
    series,
    visible,
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
  query: RouteQuery,
  allowedAggs: readonly AggregationView[],
  dp: string | null,
  series: string | null,
  visible: string[] | null
): TimeseriesMetricViewQuery {
  return {
    kind: 'timeseries',
    agg: parseOptionalMember(query.agg, allowedAggs),
    dp,
    series,
    visible,
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
  query: RouteQuery,
  ctx: MetricViewParseContext
): MetricViewQuery {
  const dp = parseDatapointParam(query, ctx.datapointIDs)
  const series = parseSeriesParam(query, ctx.seriesKeys)
  const visible = parseMetricVisibleSeries(query)
  return ctx.isHistogramKind
    ? parseHistogramMetricViewQuery(
        query,
        dp,
        series,
        visible,
        ctx.defaultHistogramTab
      )
    : parseTimeseriesMetricViewQuery(
        query,
        ctx.allowedAggs,
        dp,
        series,
        visible
      )
}

/**
 * Field-by-field equality for metric sub-view unions.
 *
 * @param a - first metric sub-view
 * @param b - second metric sub-view
 * @returns true when kind and every kind-specific field match
 *
 * @remarks Structural comparison
 * not broken by key-order drift between parse and serialize
 */
export function metricViewQueriesEqual(
  a: MetricViewQuery,
  b: MetricViewQuery
): boolean {
  if (a.dp !== b.dp) return false
  if (a.series !== b.series) return false
  if (a.visible === null || b.visible === null) {
    if (a.visible !== b.visible) return false
  } else {
    const left = new Set(a.visible)
    const right = new Set(b.visible)
    if (left.size !== right.size || [...left].some(key => !right.has(key)))
      return false
  }
  if (a.kind === 'histogram') {
    return b.kind === 'histogram' && a.htab === b.htab
  }
  return b.kind === 'timeseries' && a.agg === b.agg
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
  const params: Partial<Record<MetricViewParam, string>> = {}

  if (q.kind === 'timeseries') {
    if (q.agg !== null) params.agg = q.agg
  } else {
    params.htab = q.htab
  }
  if (q.dp !== null) params.dp = q.dp
  if (q.series !== null) params.series = q.series
  if (q.visible !== null) params.visible = metricVisibleSeriesToParam(q.visible)

  return params
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
  routeQuery: RouteQuery,
  q: MetricViewQuery
): RouteQuery {
  return {
    ...withoutParams(routeQuery, METRIC_VIEW_PARAMS),
    ...metricViewQueryToParams(q),
  }
}

/**
 * Writes the full metric sub-view to the current route query.
 *
 * @param q - validated metric sub-view
 * @param mode - {@link HistoryMode}; defaults to `'replace'` (param adjustment)
 *
 * @remarks Replaces all metric params atomically and preserves non-metric query keys.
 */
export function setMetricViewQuery(
  q: MetricViewQuery,
  mode: HistoryMode = 'replace'
): void {
  const query = mergeRouteQueryWithMetricView(readRoute().query, q)
  navigateCurrentRoute(query, mode)
}
