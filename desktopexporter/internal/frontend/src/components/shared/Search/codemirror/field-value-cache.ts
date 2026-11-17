import type { ReadMap } from '@/services/reads/map.svelte'

/** How many values to fetch per field. One request, then local filtering. */
export const FETCH_LIMIT = 500

export type FieldValueCache = {
  /** The field's distinct values, fetched at most once per session. */
  values: (field: string) => Promise<string[]>
}

/**
 * Keep received values only for this editor session. A later completion can
 * retry a failed read. The editor owns the resources and their cancellation.
 */
export function createFieldValueCache(
  fetchValues: (
    signal: string,
    field: string,
    term: string,
    limit: number,
    abort: AbortSignal
  ) => Promise<string[]>,
  signal: string,
  reads: ReadMap<string, string[]>
): FieldValueCache {
  return {
    async values(field) {
      const read = reads.for(field)
      if (!read.pending && read.data === undefined)
        void read.load(abort =>
          fetchValues(signal, field, '', FETCH_LIMIT, abort)
        )
      await read.completion
      if (read.error) throw new Error(read.error)
      return read.data ?? []
    },
  }
}
