import { createRead } from '@/services/reads'

// Keep the displayed record until its replacement is ready. Only keyboard
// traversal is debounced; the read rejects obsolete A → B → A responses.

export type DebouncedDetailFetcher<K, D> = {
  select(key: K | null, debounce?: boolean): void
  readonly data: D | null
  readonly loading: boolean
  readonly requestKey: number
  readonly error: string | null
  /** Refetch the current key; no-op when the key is null. */
  refresh(): void
}

export type CreateDebouncedDetailFetcherOptions<K, D> = {
  fetch: (key: K, signal: AbortSignal) => Promise<D>
  keysEqual: (a: K, b: K) => boolean
  /** Quiet period after a key change, in milliseconds. */
  delayMs?: number
  fallbackErrorMessage?: string
}

export function createDebouncedDetailFetcher<K, D>(
  opts: CreateDebouncedDetailFetcherOptions<K, D>
): DebouncedDetailFetcher<K, D> {
  const delayMs = opts.delayMs ?? 150
  let key: K | null = null
  const read = createRead<D>({
    errorMessage: opts.fallbackErrorMessage ?? 'Failed to load details',
  })

  function schedule(forKey: K, debounce = false) {
    void read.load(signal => opts.fetch(forKey, signal), undefined, {
      delayMs: debounce ? delayMs : 0,
    })
  }

  return {
    select(next: K | null, debounce = false) {
      if (next === null) {
        key = null
        read.clear()
        return
      }
      if (key !== null && opts.keysEqual(key, next)) return
      key = next
      schedule(next, debounce)
    },
    get data() {
      return read.data ?? null
    },
    get loading() {
      return read.pending
    },
    get requestKey() {
      return read.requestKey
    },
    get error() {
      return read.error
    },
    refresh() {
      const current = key
      if (current === null) return
      schedule(current)
    },
  }
}
