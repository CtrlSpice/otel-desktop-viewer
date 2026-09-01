import {
  navigateCurrentRoute,
  readRoute,
  withoutParams,
  type HistoryMode,
} from './router'
import { METRIC_VIEW_PARAMS, type MetricViewParam } from './query-params'

export type { MetricViewParam }
export { METRIC_VIEW_PARAMS }

export type TimeseriesMetricViewQuery = {
  kind: 'timeseries'
  agg: string | null
  dp: string | null
  series: string | null
  visible?: readonly string[]
}

export type HistogramMetricViewQuery = {
  kind: 'histogram'
  htab: 'heatmap' | 'quantiles' | 'histogram'
  hscope: 'window' | 'bucket'
  dp: string | null
  series: string | null
  visible?: readonly string[]
  quantiles?: readonly string[]
}

export type MetricViewQuery =
  TimeseriesMetricViewQuery | HistogramMetricViewQuery

export type MetricViewParseContext = {
  isHistogramKind: boolean
  allowedAggs: readonly string[]
  datapointIDs: ReadonlySet<string>
  /** Series ids present on the current metric. */
  seriesKeys: ReadonlySet<string>
  /** Quantile overlay keys supported by the histogram chart. */
  quantileKeys: ReadonlySet<string>
}

const HTAB_VALUES = ['heatmap', 'quantiles', 'histogram'] as const
const HSCOPE_VALUES = ['window', 'bucket'] as const

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
  query: Record<string, string>,
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
 * This is the durable half of a metric link. A series id is content-derived
 * from (stream, resource, labels), so it is the same across restarts and
 * re-ingests and survives retention deleting the specific datapoint a link was
 * built from -- which `dp` alone cannot.
 *
 * Still validated rather than trusted: a series can genuinely disappear when
 * its labels stop being reported, and a link naming one that no longer exists
 * should fall back to no selection rather than dangle.
 */
function parseSeriesParam(
  query: Record<string, string>,
  seriesKeys: ReadonlySet<string>
): string | null {
  const series = query.series || null
  return series && seriesKeys.has(series) ? series : null
}

function parseKnownSetParam(
  raw: string | undefined,
  known: ReadonlySet<string>
): readonly string[] | undefined {
  if (!raw) return undefined
  if (raw === '-') return []

  const values = [
    ...new Set(raw.split(',').filter(value => known.has(value))),
  ].sort()
  return values.length > 0 ? values : undefined
}

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
  dp: string | null,
  series: string | null,
  visible: readonly string[] | undefined,
  quantiles: readonly string[] | undefined
): HistogramMetricViewQuery {
  return {
    kind: 'histogram',
    htab: parseEnumMember(query.htab, HTAB_VALUES, 'heatmap'),
    hscope: parseEnumMember(query.hscope, HSCOPE_VALUES, 'window'),
    dp,
    series,
    ...(visible === undefined ? {} : { visible }),
    ...(quantiles === undefined ? {} : { quantiles }),
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
  dp: string | null,
  series: string | null,
  visible: readonly string[] | undefined
): TimeseriesMetricViewQuery {
  return {
    kind: 'timeseries',
    agg: parseOptionalMember(query.agg, allowedAggs),
    dp,
    series,
    ...(visible === undefined ? {} : { visible }),
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
  const dp = parseDatapointParam(query, ctx.datapointIDs)
  const series = parseSeriesParam(query, ctx.seriesKeys)
  const visible = parseKnownSetParam(query.visible, ctx.seriesKeys)
  return ctx.isHistogramKind
    ? parseHistogramMetricViewQuery(
        query,
        dp,
        series,
        visible,
        parseKnownSetParam(query.quantiles, ctx.quantileKeys)
      )
    : parseTimeseriesMetricViewQuery(
        query,
        ctx.allowedAggs,
        dp,
        series,
        visible
      )
}

function querySetsEqual(
  a: readonly string[] | undefined,
  b: readonly string[] | undefined
): boolean {
  if (a === undefined || b === undefined) return a === b
  if (a.length !== b.length) return false
  const left = [...a].sort()
  const right = [...b].sort()
  return left.every((value, index) => value === right[index])
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
  if (!querySetsEqual(a.visible, b.visible)) return false
  if (a.kind === 'histogram') {
    return (
      b.kind === 'histogram' &&
      a.htab === b.htab &&
      a.hscope === b.hscope &&
      querySetsEqual(a.quantiles, b.quantiles)
    )
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
  const params = Object.fromEntries(
    Object.entries({
      ...(q.kind === 'histogram'
        ? { htab: q.htab, hscope: q.hscope }
        : { agg: q.agg }),
      dp: q.dp,
      series: q.series,
    }).filter(([, value]) => value != null)
  ) as Partial<Record<MetricViewParam, string>>

  if (q.visible !== undefined) {
    params.visible =
      q.visible.length > 0 ? [...q.visible].sort().join(',') : '-'
  }
  if (q.kind === 'histogram' && q.quantiles !== undefined) {
    params.quantiles =
      q.quantiles.length > 0 ? [...q.quantiles].sort().join(',') : '-'
  }
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
