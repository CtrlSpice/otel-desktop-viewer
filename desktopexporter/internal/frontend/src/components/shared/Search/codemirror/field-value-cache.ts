/** How many values to fetch per field. One request, then local filtering. */
export const FETCH_LIMIT = 500

export type FieldValueCache = {
  /** The field's distinct values, fetched at most once per session. */
  values: (field: string) => Promise<string[]>
}

/**
 * One cached fetch per field and editor session. Concurrent callers share the
 * promise; failures are evicted. Values refresh when the editor is recreated.
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
        error => {
          // Evicted by identity: a later fetch may already have replaced this
          // entry, and deleting unconditionally would drop that one instead.
          if (inFlight.get(field) === fetched) inFlight.delete(field)
          throw error
        }
      )
      inFlight.set(field, fetched)
      return fetched
    },
  }
}
