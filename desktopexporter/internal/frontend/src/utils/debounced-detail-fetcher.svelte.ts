// A generic, debounced, race-guarded detail fetcher for list +
// detail-pane UIs.
//
// Problem we're solving: a list returns lightweight "summary" rows
// for display; the detail pane needs heavier data that we don't
// want to ship for every row. We fetch it on demand keyed by the
// user's selection. Two complications:
//
//   1. Keyboard navigation (hold the down arrow) would otherwise
//      fire one request per row. Debouncing collapses those into a
//      single fetch after the user settles.
//   2. Even after debouncing, a slow response for selection A can
//      arrive after the user has moved on to B. Without a guard
//      we'd paint A's content into B's pane. We capture the key at
//      fetch-start and drop the result if the current key no
//      longer matches.
//
// API shape: callers write to `key`, read `data`/`loading`/`error`.
// The helper takes a fetch callback and an equality predicate; it
// is intentionally agnostic to the key shape so a tuple-keyed
// metric selection works just as well as a string-keyed log id.
// `refresh()` triggers a re-fetch of the current key without
// having to null-then-set it (useful after a mutation).
//
// We deliberately do not use AbortController. This is a local-only
// tool: there's no bandwidth to save and no shared server resources
// to free up, so cancelling the in-flight request would buy nothing
// over the keysEqual race guard. If we ever grow expensive backend
// computation that's worth bailing out of mid-query, revisit.
//
// File extension is `.svelte.ts` because we use `$state` inside;
// Svelte runes are only legal in `.svelte` and `.svelte.ts` files.

export type DebouncedDetailFetcher<K, D> = {
  // The reactive selection. Set this when the user picks a row;
  // pass null to clear and reset the panel state.
  key: K | null
  // Reactive read-only outputs.
  readonly data: D | null
  readonly loading: boolean
  readonly error: string | null
  // Force a refetch of the current key (e.g., after a local
  // mutation invalidated the cached detail). No-op when key is null.
  refresh(): void
}

export type CreateDebouncedDetailFetcherOptions<K, D> = {
  // Called with the current key when the debounce timer fires.
  // Should resolve with the full detail payload, or reject.
  fetch: (key: K) => Promise<D>
  // Structural equality for keys. For string/number keys, `===`
  // wrapped in a function works; for composite metric keys the
  // caller supplies a deep equality.
  keysEqual: (a: K, b: K) => boolean
  // Quiet period (ms) before a fetch fires after the most recent
  // key change. 150ms is a reasonable default for keyboard nav:
  // long enough to coalesce holds, short enough that tap-to-pick
  // feels immediate.
  delayMs?: number
  // Message to surface when fetch rejects with a non-Error value.
  // Sensible default; callers can override per-signal.
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

  // Pending timer id; cleared whenever the key changes so the old
  // timer never gets to fire its stale fetch.
  let pendingTimer: ReturnType<typeof setTimeout> | null = null

  // Cancel any pending debounced fetch and reset detail state.
  // Called when key becomes null and on internal cleanup paths.
  function clear() {
    if (pendingTimer !== null) {
      clearTimeout(pendingTimer)
      pendingTimer = null
    }
    data = null
    loading = false
    error = null
  }

  // Schedule a fetch for the given key after delayMs. The captured
  // `forKey` is compared against the live `key` in both the timer
  // callback and the promise resolution to drop stale work.
  function schedule(forKey: K) {
    if (pendingTimer !== null) clearTimeout(pendingTimer)
    loading = true
    error = null
    pendingTimer = setTimeout(() => {
      pendingTimer = null
      // If the key changed between scheduling and firing, drop.
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

  // Watch key for changes. Setting to null clears immediately;
  // setting to a non-null value (re)schedules a debounced fetch.
  // This is the helper's one $effect; callers don't need their own.
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
