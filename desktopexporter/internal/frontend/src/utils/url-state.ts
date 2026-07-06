// URL <-> selection helpers for shareable signal links.
//
// A URL travels with a DuckDB snapshot, so it carries the selected item id
// (per signal), per-signal sub-selections (trace span; metric datapoint +
// histogram tab/scope + aggregation view), and the active time window. The
// router path is the source of truth for the selected item; the query string
// holds the sub-selections and time range. All writes go through here so
// callers don't have to remember to preserve the rest of the URL.
//
// History semantics: explicit picks (clicking an item/span/datapoint, switching
// a tab) push a history entry so back/forward steps through navigation; quick
// adjustments (aggregation, scope, time window, arrow-key scrubbing) replace.
// Callers opt into a push via `{ push: true }` / `{ replace: false }`.

import { router } from 'tinro5'

export type SignalName = 'traces' | 'metrics' | 'logs'

const SIGNAL_BASE: Record<SignalName, string> = {
  traces: '/traces',
  metrics: '/metrics',
  logs: '/logs',
}

const SPAN_PARAM = 'span'

// Metric sub-view query params (item-scoped: cleared when the metric changes).
export type MetricViewParam = 'agg' | 'htab' | 'hscope' | 'dp'
const METRIC_PARAMS: MetricViewParam[] = ['agg', 'htab', 'hscope', 'dp']

// Params that belong to the currently-selected item and must be dropped when
// the selected item changes (so a new trace/metric doesn't inherit the old
// one's sub-selection). Logs have no sub-selection.
const ITEM_SCOPED_PARAMS: Record<SignalName, string[]> = {
  traces: [SPAN_PARAM],
  metrics: [...METRIC_PARAMS],
  logs: [],
}

/** Snapshot of the current query as a plain object we can safely mutate. */
function currentQuery(): Record<string, string> {
  return { ...(router.location.query.get() as Record<string, string>) }
}

/**
 * Write a single query param, preserving the rest of the URL. `push` creates a
 * history entry (router.goto); otherwise it is a path-preserving replaceState.
 * Passing a null/empty value clears the param.
 */
function writeParam(
  name: string,
  value: string | null,
  opts: { push?: boolean } = {}
): void {
  if (opts.push) {
    const query = currentQuery()
    if (value) query[name] = value
    else delete query[name]
    router.goto((router.path ?? '/') + buildSearch(query), false)
    return
  }
  if (value) {
    router.location.query.set(name, value)
  } else {
    router.location.query.delete(name)
  }
}

function buildSearch(query: Record<string, string>): string {
  const params = new URLSearchParams()
  for (const [key, value] of Object.entries(query)) {
    if (value !== undefined && value !== null && value !== '') {
      params.set(key, String(value))
    }
  }
  const search = params.toString()
  return search ? `?${search}` : ''
}

/**
 * Parse the selected item id out of a signal path like `/traces/<id>`.
 * Returns null for the bare list path (`/traces`).
 */
export function signalIdFromPath(
  signal: SignalName,
  path: string
): string | null {
  const prefix = `${SIGNAL_BASE[signal]}/`
  if (!path.startsWith(prefix)) return null
  const segment = path.slice(prefix.length).split('/')[0]
  return segment ? decodeURIComponent(segment) : null
}

/**
 * Navigate to a signal item, or the bare list when `id` is null. The time-range
 * query is preserved; the signal's item-scoped sub-selections (trace span,
 * metric datapoint/tab/scope/aggregation) are dropped on an item change.
 */
export function navigateToItem(
  signal: SignalName,
  id: string | null,
  opts: { replace?: boolean } = {}
): void {
  const query = currentQuery()
  for (const param of ITEM_SCOPED_PARAMS[signal]) delete query[param]
  const base = SIGNAL_BASE[signal]
  const path = id ? `${base}/${encodeURIComponent(id)}` : base
  router.goto(path + buildSearch(query), opts.replace ?? false)
}

/** Navigate to a signal's list/tab, preserving the time-range query. */
export function navigateToSignal(
  signal: SignalName,
  opts: { replace?: boolean } = {}
): void {
  navigateToItem(signal, null, opts)
}

/**
 * Imperative read of the span sub-selection. Reactive consumers should derive
 * it from the router subscription's `query` instead.
 */
export function getSpanFromQuery(): string | null {
  const value = router.location.query.get(SPAN_PARAM)
  return typeof value === 'string' && value ? value : null
}

/**
 * Set or clear the trace span sub-selection. Defaults to replaceState; a span
 * picked by an explicit click should pass `{ push: true }` so back returns to
 * the prior span.
 */
export function setSpanInQuery(
  spanID: string | null,
  opts: { push?: boolean } = {}
): void {
  writeParam(SPAN_PARAM, spanID, opts)
}

// --- metric sub-view query ---
//
// Item-scoped params for the selected metric. Values are kept as raw strings
// here (url-state stays signal-agnostic); metric-view-context validates and
// coerces them against the metric's actual shape.

export type MetricViewQuery = {
  agg: string | null
  htab: string | null
  hscope: string | null
  dp: string | null
}

/** Imperative read of all metric sub-view params (for adopt-from-URL). */
export function readMetricViewQuery(): MetricViewQuery {
  const query = router.location.query.get() as Record<string, string>
  const val = (name: MetricViewParam) =>
    typeof query[name] === 'string' && query[name] ? query[name] : null
  return {
    agg: val('agg'),
    htab: val('htab'),
    hscope: val('hscope'),
    dp: val('dp'),
  }
}

/**
 * Set or clear one or more metric sub-view params in a single navigation,
 * preserving the rest of the URL. A datapoint click can move `dp` plus the
 * histogram `htab`/`hscope` at once, so batching keeps that a single history
 * entry. Tab/datapoint picks pass `{ push: true }` (navigational);
 * aggregation/scope adjustments use the default replaceState.
 */
export function setMetricViewParams(
  patch: Partial<Record<MetricViewParam, string | null>>,
  opts: { push?: boolean } = {}
): void {
  const query = currentQuery()
  for (const [name, value] of Object.entries(patch)) {
    if (value) query[name] = value
    else delete query[name]
  }
  if (opts.push) {
    router.goto((router.path ?? '/') + buildSearch(query), false)
  } else {
    router.location.query.replace(query)
  }
}

// --- time range query ---
//
// The time window is frozen to absolute start/end (ms) when written, so a
// shared snapshot reliably contains the selected item (see time-context).

export type TimeQuery = {
  start: number
  end: number
  tz: string | null
}

export function readTimeQuery(): TimeQuery | null {
  const query = router.location.query.get() as Record<string, string>
  const start = Number(query.start)
  const end = Number(query.end)
  if (
    !query.start ||
    !query.end ||
    !Number.isFinite(start) ||
    !Number.isFinite(end)
  ) {
    return null
  }
  return { start, end, tz: query.tz ?? null }
}

/** Merge the time window into the query (preserving span etc.), replaceState. */
export function writeTimeQuery(time: TimeQuery): void {
  const query = currentQuery()
  query.start = String(time.start)
  query.end = String(time.end)
  if (time.tz) query.tz = time.tz
  else delete query.tz
  router.location.query.replace(query)
}
