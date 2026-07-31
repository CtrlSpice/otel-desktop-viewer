import {
  navigateCurrentRoute,
  readRoute,
  withQueryPatch,
  type HistoryMode,
} from './router'
import { EVENT_PARAM, SPAN_PARAM } from './query-params'

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
 * Returns the selected span event index from the live URL.
 *
 * @returns zero-based event index, or `null` when absent or invalid
 */
export function getEventFromQuery(): number | null {
  const raw = readRoute().query[EVENT_PARAM]
  if (!raw) return null
  const index = Number.parseInt(raw, 10)
  if (!Number.isFinite(index) || index < 0) return null
  return index
}

/**
 * Sets or clears the trace span param on the current route.
 *
 * @param spanID - span id, or `null` to clear
 * @param mode - {@link HistoryMode}; defaults to `'replace'` (param adjustment)
 *
 * @remarks Clears `event` when the span changes.
 */
export function setSpanInQuery(
  spanID: string | null,
  mode: HistoryMode = 'replace'
): void {
  const query = withQueryPatch(readRoute().query, {
    [SPAN_PARAM]: spanID,
    [EVENT_PARAM]: null,
  })
  navigateCurrentRoute(query, mode)
}

/**
 * Sets or clears the span event index on the current route.
 *
 * @param eventIndex - zero-based index, or `null` to clear
 * @param mode - {@link HistoryMode}; defaults to `'replace'`
 */
export function setEventInQuery(
  eventIndex: number | null,
  mode: HistoryMode = 'replace'
): void {
  const query = withQueryPatch(readRoute().query, {
    [EVENT_PARAM]: eventIndex === null ? null : String(eventIndex),
  })
  navigateCurrentRoute(query, mode)
}

/**
 * Selects a span and event on the current trace route.
 *
 * @param spanID - span to select in the waterfall/detail pane
 * @param eventIndex - zero-based index into `span.events`
 * @param mode - {@link HistoryMode}; defaults to `'push'`
 */
export function selectSpanEvent(
  spanID: string,
  eventIndex: number,
  mode: HistoryMode = 'push'
): void {
  const query = withQueryPatch(readRoute().query, {
    [SPAN_PARAM]: spanID,
    [EVENT_PARAM]: String(eventIndex),
  })
  navigateCurrentRoute(query, mode)
}
