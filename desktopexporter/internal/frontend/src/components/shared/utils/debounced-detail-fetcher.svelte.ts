import { createResource } from '@/services/resources'

// Keep the displayed record until its replacement is ready. Only keyboard
// traversal is debounced; the resource rejects obsolete A → B → A responses.

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
  const resource = createResource<D>({
    errorMessage: opts.fallbackErrorMessage ?? 'Failed to load details',
  })

  function schedule(forKey: K, debounce = false) {
    void resource.load(signal => opts.fetch(forKey, signal), undefined, {
      delayMs: debounce ? delayMs : 0,
    })
  }

  return {
    select(next: K | null, debounce = false) {
      if (next === null) {
        key = null
        resource.clear()
        return
      }
      if (key !== null && opts.keysEqual(key, next)) return
      key = next
      schedule(next, debounce)
    },
    get data() {
      return resource.data ?? null
    },
    get loading() {
      return resource.pending
    },
    get requestKey() {
      return resource.requestKey
    },
    get error() {
      return resource.error
    },
    refresh() {
      const current = key
      if (current === null) return
      schedule(current)
    },
  }
}
