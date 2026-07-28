// Shared list-page orchestration for traces / logs / metrics pages.
//
// Owns the duplicated wiring: mount/fetch, sort, URL-driven selection with
// shared-link-safe auto-select, time-range refetch, stats polling refresh
// indicator, footer keyboard nav, and search-result override. Callers keep
// signal-specific sort fns, detail fetching, and Svelte snippets.
//
// File extension is `.svelte.ts` because we use `$state` / `$effect` inside.

import { onMount, onDestroy } from 'svelte'
import { getTimeContext } from '@/contexts/time-context.svelte'
import { getRouteContext } from '@/contexts/route-context.svelte'
import {
  navigateToItem,
  signalIdFromPath,
  type HistoryMode,
  type SignalName,
} from '@/route'
import type { SearchResultEvent } from '@/types/api-types'
import type { SearchEditorAPI } from '@/components/shared/Search/search-editor-api'
import {
  beginListUpdate,
  cancelPendingListUpdates,
  isLatestListUpdate,
} from '@/components/shared/utils/list-update-seq'

export type SortDirection = 'asc' | 'desc'

export type SignalListPageOptions<TItem> = {
  signal: SignalName
  getItemId: (item: TItem) => string
  fetchList: () => Promise<TItem[]>
  compare: (
    a: TItem,
    b: TItem,
    column: string,
    direction: SortDirection
  ) => number
  initialSort: { column: string; direction: SortDirection }
  /** Called after each poll interval; update polled stat counters here. */
  pollStats?: () => Promise<void>
  /** Derive refresh pulse + aside tip from baseline vs polled counters. */
  refreshFromStats?: () => { pulse: boolean; tip: string }
}

export type SignalListPage<TItem> = {
  readonly items: TItem[]
  readonly loading: boolean
  readonly error: string | null
  readonly mounted: boolean
  readonly sortColumn: string
  readonly sortDirection: SortDirection
  readonly sortedItems: TItem[]
  readonly selectedId: string | null
  readonly selectedIndex: number
  readonly selectedSummary: TItem | undefined
  readonly refreshPulse: boolean
  readonly refreshAsideTip: string
  searchEditorApi: SearchEditorAPI | null
  handleSortChange(value: string, direction: SortDirection): void
  selectItem(id: string, mode?: HistoryMode): void
  selectByOffset(delta: number): void
  selectFirst(): void
  selectLast(): void
  handleRefresh(): void
  handleSearchResults(event: SearchResultEvent): void
  runListFetch(): Promise<void>
}

const POLL_INTERVAL_MS = 3000

/** Index of the item whose id matches, or -1 when id is missing / not found. */
export function findItemIndexById<T>(
  items: readonly T[],
  id: string | null | undefined,
  getItemId: (item: T) => string
): number {
  if (!id) return -1
  return items.findIndex(item => getItemId(item) === id)
}

/** Clamps selectedIndex + delta into [0, length - 1]; -1 when nav is impossible. */
export function clampNavTargetIndex(
  selectedIndex: number,
  delta: number,
  length: number
): number {
  if (selectedIndex < 0 || length === 0) return -1
  return Math.max(0, Math.min(length - 1, selectedIndex + delta))
}

/** Row index to select when the URL id is stale or missing from the list. */
export function resolveFallbackIndex(
  lastValidIndex: number,
  listLength: number
): number {
  if (listLength === 0) return 0
  return Math.min(lastValidIndex, listLength - 1)
}

export function createSignalListPage<TItem>(
  opts: SignalListPageOptions<TItem>
): SignalListPage<TItem> {
  const timeContext = getTimeContext()
  const routeContext = getRouteContext()

  let items = $state<TItem[]>([])
  let loading = $state(true)
  let error = $state<string | null>(null)
  let mounted = $state(false)

  let sortColumn = $state(opts.initialSort.column)
  let sortDirection = $state<SortDirection>(opts.initialSort.direction)

  let searchEditorApi = $state<SearchEditorAPI | null>(null)
  let refreshPulse = $state(false)
  let refreshAsideTip = $state('')

  let lastValidIndex = $state(0)

  let selectedId = $derived(
    signalIdFromPath(opts.signal, routeContext.route.path)
  )

  let sortedItems = $derived.by(() => {
    const col = sortColumn
    const dir = sortDirection
    const rows = [...items]
    rows.sort((a, b) => opts.compare(a, b, col, dir))
    return rows
  })

  let selectedIndex = $derived(
    findItemIndexById(sortedItems, selectedId, opts.getItemId)
  )

  let selectedSummary = $derived(
    selectedId
      ? sortedItems.find(item => opts.getItemId(item) === selectedId)
      : undefined
  )

  function updateRefreshIndicator() {
    if (!opts.refreshFromStats) return
    const next = opts.refreshFromStats()
    refreshPulse = next.pulse
    refreshAsideTip = next.tip
  }

  // Guarded behind mounted + !loading so a URL-provided id (shared link) is
  // never replaced before the list has finished fetching.
  $effect(() => {
    if (!mounted || loading) return
    const id = selectedId
    const idx = findItemIndexById(sortedItems, id, opts.getItemId)
    if (idx >= 0) {
      lastValidIndex = idx
    } else if (sortedItems.length > 0) {
      const fallback =
        sortedItems[resolveFallbackIndex(lastValidIndex, sortedItems.length)]
      if (fallback) {
        navigateToItem(opts.signal, opts.getItemId(fallback), 'replace')
      }
    } else if (id) {
      navigateToItem(opts.signal, null, 'replace')
    }
  })

  $effect(() => {
    void timeContext.selection
    if (mounted) {
      void runListFetch()
    }
  })

  $effect(() => {
    if (!mounted || !opts.pollStats) return
    const id = setInterval(async () => {
      try {
        await opts.pollStats!()
        updateRefreshIndicator()
      } catch {
        /* polling failures are silent */
      }
    }, POLL_INTERVAL_MS)
    return () => clearInterval(id)
  })

  async function runListFetch() {
    const updateSeq = beginListUpdate(opts.signal)
    try {
      loading = true
      error = null
      const next = await opts.fetchList()
      if (!isLatestListUpdate(opts.signal, updateSeq)) return
      items = next
      updateRefreshIndicator()
    } catch (err) {
      if (!isLatestListUpdate(opts.signal, updateSeq)) return
      error = err instanceof Error ? err.message : 'Failed to load list'
    } finally {
      if (isLatestListUpdate(opts.signal, updateSeq)) loading = false
    }
  }

  function handleSortChange(value: string, direction: SortDirection) {
    sortColumn = value
    sortDirection = direction
  }

  function selectItem(id: string, mode: HistoryMode = 'push') {
    navigateToItem(opts.signal, id, mode)
  }

  function selectByOffset(delta: number) {
    const target = clampNavTargetIndex(selectedIndex, delta, sortedItems.length)
    if (target < 0 || target === selectedIndex) return
    const next = sortedItems[target]
    if (next) navigateToItem(opts.signal, opts.getItemId(next), 'replace')
  }

  function selectFirst() {
    const first = sortedItems[0]
    if (first) navigateToItem(opts.signal, opts.getItemId(first), 'replace')
  }

  function selectLast() {
    const last = sortedItems[sortedItems.length - 1]
    if (last) navigateToItem(opts.signal, opts.getItemId(last), 'replace')
  }

  function handleRefresh() {
    searchEditorApi?.clear()
    void runListFetch()
  }

  function handleSearchResults(event: SearchResultEvent) {
    if (event.signal !== opts.signal) return
    if (!isLatestListUpdate(event.signal, event.updateSeq)) return
    error = null
    items = event.results as TItem[]
    loading = false
  }

  onMount(() => {
    mounted = true
  })

  onDestroy(() => {
    cancelPendingListUpdates(opts.signal)
  })

  return {
    get items() {
      return items
    },
    get loading() {
      return loading
    },
    get error() {
      return error
    },
    get mounted() {
      return mounted
    },
    get sortColumn() {
      return sortColumn
    },
    get sortDirection() {
      return sortDirection
    },
    get sortedItems() {
      return sortedItems
    },
    get selectedId() {
      return selectedId
    },
    get selectedIndex() {
      return selectedIndex
    },
    get selectedSummary() {
      return selectedSummary
    },
    get refreshPulse() {
      return refreshPulse
    },
    get refreshAsideTip() {
      return refreshAsideTip
    },
    get searchEditorApi() {
      return searchEditorApi
    },
    set searchEditorApi(next: SearchEditorAPI | null) {
      searchEditorApi = next
    },
    handleSortChange,
    selectItem,
    selectByOffset,
    selectFirst,
    selectLast,
    handleRefresh,
    handleSearchResults,
    runListFetch,
  }
}
