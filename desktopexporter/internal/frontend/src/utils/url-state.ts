// URL <-> selection helpers for shareable signal links.
//
// A URL travels with a DuckDB snapshot, so it carries the selected item id
// (per signal), the trace span sub-selection, and the active time window. The
// router path is the source of truth for the selected item; the query string
// holds the span and time range. All writes go through here so callers don't
// have to remember to preserve the rest of the URL.

import { router } from 'tinro5'

export type SignalName = 'traces' | 'metrics' | 'logs'

const SIGNAL_BASE: Record<SignalName, string> = {
  traces: '/traces',
  metrics: '/metrics',
  logs: '/logs',
}

const SPAN_PARAM = 'span'

/** Snapshot of the current query as a plain object we can safely mutate. */
function currentQuery(): Record<string, string> {
  return { ...(router.location.query.get() as Record<string, string>) }
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
 * query is preserved; the span sub-selection is item-scoped so it is dropped on
 * an item change.
 */
export function navigateToItem(
  signal: SignalName,
  id: string | null,
  opts: { replace?: boolean } = {}
): void {
  const query = currentQuery()
  delete query[SPAN_PARAM]
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

/** Set or clear the trace span sub-selection. Path-preserving, replaceState. */
export function setSpanInQuery(spanID: string | null): void {
  if (spanID) {
    router.location.query.set(SPAN_PARAM, spanID)
  } else {
    router.location.query.delete(SPAN_PARAM)
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
