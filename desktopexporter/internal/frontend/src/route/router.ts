// Client router: read/write URL state via the History API.

import { SIGNAL_ITEM_QUERY_PARAMS } from './query-params'

export type SignalName = 'traces' | 'metrics' | 'logs'

export type Route = {
  path: string
  query: Record<string, string>
}

function signalPath(signal: SignalName): string {
  return `/${signal}`
}

type RouteListener = () => void

const routeListeners = new Set<RouteListener>()

/**
 * Splits a URL into pathname and query params.
 *
 * @param href - full or relative URL string
 * @returns `{ path, query }` parsed from the href
 *
 * @example `/traces/abc?span=1` → `{ path: '/traces/abc', query: { span: '1' } }`
 */
export function parseRoute(href: string): Route {
  const url = new URL(href, 'http://local')
  return { path: url.pathname, query: Object.fromEntries(url.searchParams) }
}

/**
 * Builds a URL search string from a query record.
 *
 * @param query - flat string key/value map
 * @returns search string such as `?a=1&b=2`, or an empty string
 *
 * @remarks Empty, null, and undefined values are omitted.
 *
 * @example `{ a: '1' }` → `?a=1`
 */
export function buildSearch(query: Record<string, string>): string {
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
 * Applies one or more query param updates without mutating the input.
 *
 * @param query - source query object
 * @param patch - param keys mapped to new values
 * @returns new query object
 *
 * @remarks Patch values of `null` or `undefined` clear those keys. A single-key patch is fine.
 */
export function withQueryPatch(
  query: Record<string, string>,
  patch: Record<string, string | null | undefined>
): Record<string, string> {
  const next = { ...query }
  for (const [name, value] of Object.entries(patch)) {
    const v = value ?? null
    if (v) next[name] = v
    else delete next[name]
  }
  return next
}

/**
 * Drops listed query keys without mutating the input.
 *
 * @param query - source query object
 * @param params - param keys to remove
 * @returns new query object
 *
 * @example Used to clear item-scoped params when the selected trace or metric changes.
 */
export function withoutParams(
  query: Record<string, string>,
  params: readonly string[]
): Record<string, string> {
  const next = { ...query }
  for (const param of params) delete next[param]
  return next
}

/**
 * Extracts the selected item id from a signal path.
 *
 * @param signal - traces, metrics, or logs
 * @param path - current URL pathname
 * @returns decoded item id, or `null` for the bare list path
 *
 * @example `('traces', '/traces/abc')` → `'abc'`
 * @example `('traces', '/traces')` → `null`
 */
export function signalIdFromPath(
  signal: SignalName,
  path: string
): string | null {
  const prefix = `${signalPath(signal)}/`
  if (!path.startsWith(prefix)) return null
  const segment = path.slice(prefix.length).split('/')[0]
  return segment ? decodeURIComponent(segment) : null
}

/**
 * Returns the current browser URL as path and query.
 *
 * @returns live {@link Route}
 *
 * @remarks Reads `window.location`; no history writes.
 */
export function readRoute(): Route {
  return parseRoute(window.location.href)
}

function notifyRouteListeners(): void {
  for (const listener of [...routeListeners]) listener()
}

if (typeof window !== 'undefined') {
  window.addEventListener('popstate', notifyRouteListeners)
}

/**
 * Registers for URL change notifications.
 *
 * @param onChange - callback invoked on subscribe and on every route change
 * @returns unsubscribe function
 *
 * @remarks Also fires on browser back/forward via `popstate`.
 */
export function subscribeToRoute(onChange: () => void): () => void {
  routeListeners.add(onChange)
  onChange()
  return () => {
    routeListeners.delete(onChange)
  }
}

/**
 * Updates the browser history entry for the current tab.
 *
 * @param to - destination URL (path plus optional search)
 * @param opts.replace - use `replaceState` instead of `pushState`
 *
 * @remarks Notifies route listeners after writing history.
 */
export function navigate(to: string, opts: { replace?: boolean } = {}): void {
  history[opts.replace ? 'replaceState' : 'pushState'](null, '', to)
  notifyRouteListeners()
}

/**
 * Navigates on the current path with an updated query.
 *
 * @param query - full query object for the current pathname
 * @param opts.replace - use `replaceState` instead of `pushState`
 *
 * @remarks Reads the live route, then delegates to {@link navigate}.
 */
export function navigateCurrentRoute(
  query: Record<string, string>,
  opts: { replace?: boolean } = {}
): void {
  const route = readRoute()
  navigate(route.path + buildSearch(query), opts)
}

/**
 * Navigates to a signal item or bare list path.
 *
 * @param signal - traces, metrics, or logs
 * @param id - item id, or `null` for the list path
 * @param opts.replace - use `replaceState` instead of `pushState`
 *
 * @remarks Clears all signal item-scoped params (span, metric view) and preserves the time window.
 */
export function navigateToItem(
  signal: SignalName,
  id: string | null,
  opts: { replace?: boolean } = {}
): void {
  const route = readRoute()
  const query = withoutParams(route.query, SIGNAL_ITEM_QUERY_PARAMS)
  const base = signalPath(signal)
  const path = id ? `${base}/${encodeURIComponent(id)}` : base
  navigate(path + buildSearch(query), { replace: opts.replace ?? false })
}

/**
 * Navigates to the bare signal list path.
 *
 * @param signal - traces, metrics, or logs
 * @param opts.replace - use `replaceState` instead of `pushState`
 *
 * @example `navigateToSignal('traces')` → `/traces?start=...&end=...`
 */
export function navigateToSignal(
  signal: SignalName,
  opts: { replace?: boolean } = {}
): void {
  navigateToItem(signal, null, opts)
}
