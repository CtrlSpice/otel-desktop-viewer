import {
  navigateCurrentRoute,
  readRoute,
  withQueryPatch,
} from './router'
import { SPAN_PARAM } from './query-params'

/**
 * Returns the trace span query param from the live URL.
 *
 * @returns span id, or `null` when absent
 *
 * @remarks Reads the current route only; no history writes.
 */
export function getSpanFromQuery(): string | null {
  return readRoute().query[SPAN_PARAM] || null
}

/**
 * Sets or clears the trace span param on the current route.
 *
 * @param spanID - span id, or `null` to clear
 * @param opts.push - use `pushState` for explicit span picks
 *
 * @remarks Defaults to `replaceState`.
 */
export function setSpanInQuery(
  spanID: string | null,
  opts: { push?: boolean } = {}
): void {
  const query = withQueryPatch(readRoute().query, { [SPAN_PARAM]: spanID })
  navigateCurrentRoute(query, { replace: !opts.push })
}
