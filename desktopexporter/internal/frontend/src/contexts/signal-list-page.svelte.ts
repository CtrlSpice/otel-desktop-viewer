// Shared list-page orchestration for traces / logs / metrics pages.
//
// Owns the duplicated wiring: mount/fetch, sort, URL-driven selection with
// shared-link-safe auto-select, time-range refetch, commit-driven refresh
// indicator and footer keyboard nav. Callers keep
// signal-specific sort fns, detail fetching, and Svelte snippets.
//
// File extension is `.svelte.ts` because we use `$state` / `$effect` inside.

import { onMount, onDestroy, untrack } from 'svelte'
import { getTimeContext } from '@/contexts/time-context.svelte'
import { getRouteContext } from '@/contexts/route-context.svelte'
import { getDataChanges } from '@/contexts/data-changes.svelte'
import { createRefreshCoordinator } from '@/services/refresh-coordinator'
import { createRead } from '@/services/reads'
import {
  navigateToItem,
  signalIdFromPath,
  type HistoryMode,
  type SignalName,
} from '@/route'
import type { SearchEditorAPI } from '@/components/shared/Search/search-editor-api'

export type SortDirection = 'asc' | 'desc'

export type SignalListPageOptions<TItem, TSortColumn extends string> = {
  signal: SignalName
  getItemID: (item: TItem) => string
  fetchList: (signal: AbortSignal) => Promise<TItem[]>
  compare: (
    a: TItem,
    b: TItem,
    column: TSortColumn,
    direction: SortDirection
  ) => number
  initialSort: { column: TSortColumn; direction: SortDirection }
  /** Keep a route-selected ID that is absent from the current filtered list. */
  preserveMissingSelection?: boolean
  /** Paged results already have a stable server order. */
  serverSorted?: boolean
  /** Called after committed changes; update freshness counters here. */
  refreshStats?: () => Promise<void>
  /** Defer freshness reads until the displayed list's baseline has settled. */
  statsPending?: () => boolean
  /** Derive refresh pulse + aside tip from baseline vs polled counters. */
  refreshFromStats?: () => { pulse: boolean; tip: string }
}

export type SignalListPage<TItem, TSortColumn extends string> = {
  readonly items: TItem[]
  readonly loading: boolean
  readonly requestKey: number
  readonly error: string | null
  readonly mounted: boolean
  readonly sortColumn: TSortColumn
  readonly sortDirection: SortDirection
  readonly sortedItems: TItem[]
  readonly selectedID: string | null
  readonly selectedIndex: number
  readonly selectedSummary: TItem | undefined
  readonly refreshPulse: boolean
  readonly refreshAsideTip: string
  searchEditorApi: SearchEditorAPI | null
  handleSortChange(value: TSortColumn, direction: SortDirection): void
  selectItem(id: string, mode?: HistoryMode): void
  selectByOffset(delta: number): void
  selectFirst(): void
  selectLast(): void
  handleRefresh(): void
  runListFetch(): Promise<void>
  /** Publish a page of the displayed reading without superseding its replacement. */
  publishItems(next: TItem[]): void
}

/** Index of the item whose id matches, or -1 when id is missing / not found. */
export function findItemIndexByID<T>(
  items: readonly T[],
  id: string | null | undefined,
  getItemID: (item: T) => string
): number {
  if (!id) return -1
  return items.findIndex(item => getItemID(item) === id)
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

export function createSignalListPage<TItem, TSortColumn extends string>(
  opts: SignalListPageOptions<TItem, TSortColumn>
): SignalListPage<TItem, TSortColumn> {
  const timeContext = getTimeContext()
  const routeContext = getRouteContext()
  const changes = getDataChanges()

  let items = $state<TItem[]>([])
  const list = createRead<TItem[]>({ errorMessage: 'Failed to load list' })
  let mounted = $state(false)
  const loading = $derived(!mounted || list.requestKey === 0 || list.pending)

  let sortColumn = $state(opts.initialSort.column)
  let sortDirection = $state<SortDirection>(opts.initialSort.direction)

  let searchEditorApi = $state<SearchEditorAPI | null>(null)
  let refreshPulse = $state(false)
  let refreshAsideTip = $state('')

  let lastValidIndex = $state(0)

  let selectedID = $derived(
    signalIdFromPath(opts.signal, routeContext.route.path)
  )

  let sortedItems = $derived.by(() => {
    if (opts.serverSorted) return items
    const col = sortColumn
    const dir = sortDirection
    const rows = [...items]
    rows.sort((a, b) => opts.compare(a, b, col, dir))
    return rows
  })

  let selectedIndex = $derived(
    findItemIndexByID(sortedItems, selectedID, opts.getItemID)
  )

  let selectedSummary = $derived(
    selectedID
      ? sortedItems.find(item => opts.getItemID(item) === selectedID)
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
    const id = selectedID
    const idx = findItemIndexByID(sortedItems, id, opts.getItemID)
    if (idx >= 0) {
      lastValidIndex = idx
    } else if (id && opts.preserveMissingSelection) {
      return
    } else if (sortedItems.length > 0) {
      const fallback =
        sortedItems[resolveFallbackIndex(lastValidIndex, sortedItems.length)]
      if (fallback) {
        navigateToItem(opts.signal, opts.getItemID(fallback), 'replace')
      }
    } else if (id) {
      navigateToItem(opts.signal, null, 'replace')
    }
  })

  $effect(() => {
    void timeContext.selection
    if (mounted) {
      untrack(() => void runListFetch())
    }
  })

  let statsRevision = $state(0)
  let refreshedStatsRevision = 0
  const statsRefresh = createRefreshCoordinator(async () => {
    if (loading || opts.statsPending?.()) return
    refreshedStatsRevision = statsRevision
    if (opts.refreshStats) {
      try {
        await opts.refreshStats()
        updateRefreshIndicator()
      } catch {
        /* Freshness failures do not replace displayed data. */
      }
    }
  })
  if (changes) onDestroy(changes.subscribe(() => statsRevision++))
  $effect(() => {
    if (
      statsRevision !== refreshedStatsRevision &&
      !loading &&
      !opts.statsPending?.()
    )
      void statsRefresh.request()
  })
  onDestroy(() => statsRefresh.dispose())

  async function runListFetch() {
    await list.load(opts.fetchList, next => {
      items = next
      updateRefreshIndicator()
    })
  }

  function handleSortChange(value: TSortColumn, direction: SortDirection) {
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
    if (next) navigateToItem(opts.signal, opts.getItemID(next), 'replace')
  }

  function selectFirst() {
    const first = sortedItems[0]
    if (first) navigateToItem(opts.signal, opts.getItemID(first), 'replace')
  }

  function selectLast() {
    const last = sortedItems[sortedItems.length - 1]
    if (last) navigateToItem(opts.signal, opts.getItemID(last), 'replace')
  }

  function handleRefresh() {
    void runListFetch()
  }

  onMount(() => {
    mounted = true
  })

  return {
    get items() {
      return items
    },
    get loading() {
      return loading
    },
    get requestKey() {
      return list.requestKey
    },
    get error() {
      return list.error
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
    get selectedID() {
      return selectedID
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
    runListFetch,
    publishItems(next) {
      items = next
    },
  }
}
