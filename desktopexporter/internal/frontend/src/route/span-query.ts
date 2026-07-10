import {
  navigateCurrentRoute,
  readRoute,
  withQueryPatch,
  type HistoryMode,
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
 * @param mode - {@link HistoryMode}; defaults to `'replace'` (param adjustment)
 */
export function setSpanInQuery(
  spanID: string | null,
  mode: HistoryMode = 'replace'
): void {
  const query = withQueryPatch(readRoute().query, { [SPAN_PARAM]: spanID })
  navigateCurrentRoute(query, mode)
}
