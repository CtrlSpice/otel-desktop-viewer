// Debounce keyboard-driven selections and discard results for stale keys.
// Key comparison, rather than request cancellation, guards the displayed data.

export type DebouncedDetailFetcher<K, D> = {
  key: K | null
  readonly data: D | null
  readonly loading: boolean
  readonly error: string | null
  /** Refetch the current key; no-op when the key is null. */
  refresh(): void
}

export type CreateDebouncedDetailFetcherOptions<K, D> = {
  fetch: (key: K) => Promise<D>
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

  let key = $state<K | null>(null)
  let data = $state<D | null>(null)
  let loading = $state(false)
  let error = $state<string | null>(null)

  let pendingTimer: ReturnType<typeof setTimeout> | null = null

  function clear() {
    if (pendingTimer !== null) {
      clearTimeout(pendingTimer)
      pendingTimer = null
    }
    data = null
    loading = false
    error = null
  }

  // Check the captured key before starting and after resolving the request.
  function schedule(forKey: K) {
    if (pendingTimer !== null) clearTimeout(pendingTimer)
    loading = true
    error = null
    pendingTimer = setTimeout(() => {
      pendingTimer = null
      if (key === null || !opts.keysEqual(key, forKey)) return
      opts.fetch(forKey).then(
        result => {
          if (key === null || !opts.keysEqual(key, forKey)) return
          data = result
          loading = false
        },
        err => {
          if (key === null || !opts.keysEqual(key, forKey)) return
          error = err instanceof Error ? err.message : fallback
          data = null
          loading = false
        }
      )
    }, delayMs)
  }

  $effect(() => {
    const current = key
    if (current === null) {
      clear()
      return
    }
    schedule(current)
  })

  return {
    get key() {
      return key
    },
    set key(next: K | null) {
      key = next
    },
    get data() {
      return data
    },
    get loading() {
      return loading
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
