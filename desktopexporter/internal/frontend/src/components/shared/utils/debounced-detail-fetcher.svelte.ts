import { onDestroy } from 'svelte'

// Keep the displayed record until its replacement is ready. Only keyboard
// traversal is debounced; request generations also guard A → B → A races.

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
  const fallback = opts.fallbackErrorMessage ?? 'Failed to load details'

  let key: K | null = null
  let data = $state<D | null>(null)
  let loading = $state(false)
  let requestKey = $state(0)
  let error = $state<string | null>(null)

  let pendingTimer: ReturnType<typeof setTimeout> | null = null
  let generation = 0
  let activeRequest: AbortController | null = null

  onDestroy(() => {
    ++generation
    activeRequest?.abort()
    if (pendingTimer !== null) clearTimeout(pendingTimer)
  })

  function clear() {
    ++generation
    activeRequest?.abort()
    activeRequest = null
    if (pendingTimer !== null) {
      clearTimeout(pendingTimer)
      pendingTimer = null
    }
    data = null
    loading = false
    error = null
  }

  function schedule(forKey: K, debounce = false) {
    const request = ++generation
    requestKey = request
    activeRequest?.abort()
    activeRequest = null
    if (pendingTimer !== null) clearTimeout(pendingTimer)
    loading = true
    error = null
    const run = () => {
      pendingTimer = null
      if (request !== generation) return
      activeRequest = new AbortController()
      opts.fetch(forKey, activeRequest.signal).then(
        result => {
          if (request !== generation) return
          data = result
          loading = false
        },
        err => {
          if (request !== generation) return
          error = err instanceof Error ? err.message : fallback
          loading = false
        }
      )
    }
    if (debounce) pendingTimer = setTimeout(run, delayMs)
    else run()
  }

  return {
    select(next: K | null, debounce = false) {
      if (next === null) {
        key = null
        clear()
        return
      }
      if (key !== null && opts.keysEqual(key, next)) return
      key = next
      schedule(next, debounce)
    },
    get data() {
      return data
    },
    get loading() {
      return loading
    },
    get requestKey() {
      return requestKey
    },
    get error() {
      return error
    },
    refresh() {
      const current = key
      if (current === null) return
      schedule(current)
    },
  }
}
