/** How many values to fetch per field. One request, then local filtering. */
export const FETCH_LIMIT = 500

export type FieldValueCache = {
  /** The field's distinct values, fetched at most once per session. */
  values: (field: string) => Promise<string[]>
}

/**
 * One fetch per field per editor session, shared by every completion source.
 *
 * @remarks
 * The store query behind this scans a column to rank values by frequency --
 * measured at ~11ms on a 250k-span store -- which is fine once and not fine
 * per keystroke. Sources therefore fetch the whole list here and filter it
 * locally as the user types.
 *
 * Shared rather than per-source because two sources want the same values: the
 * value position (`name = `) and bare-text discovery (`checkout` offering
 * `name = "checkout/pay"`). A cache inside each meant the same field could be
 * fetched twice in one session, which is harmless but dishonest about the
 * cost.
 *
 * The promise is cached rather than its result, so concurrent triggers share
 * one flight instead of racing two. A rejected fetch is evicted so the next
 * trigger retries: one unreachable moment should not disable completion for
 * the rest of the session.
 *
 * Values are read once and never refreshed, so telemetry arriving mid-session
 * is not offered until the editor is recreated. That is the deliberate trade
 * for not querying per keystroke; the name can still be typed by hand.
 */
export function createFieldValueCache(
  fetchValues: (
    signal: string,
    field: string,
    term: string,
    limit: number
  ) => Promise<string[]>,
  signal: string
): FieldValueCache {
  const inFlight = new Map<string, Promise<string[]>>()

  return {
    values(field) {
      const hit = inFlight.get(field)
      if (hit) return hit

      const fetched = fetchValues(signal, field, '', FETCH_LIMIT).catch(
        (err: unknown) => {
          // Evicted by identity: a later fetch may already have replaced this
          // entry, and deleting unconditionally would drop that one instead.
          if (inFlight.get(field) === fetched) inFlight.delete(field)
          throw err
        }
      )
      inFlight.set(field, fetched)
      return fetched
    },
  }
}
