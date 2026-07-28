// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { tick } from 'svelte'
import { screen } from '@testing-library/svelte'
import SignalListPageProbe from '@/test/SignalListPageProbe.svelte'
import { navigateToItem } from '@/route'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import {
  beginListUpdate,
  cancelPendingListUpdates,
  isLatestListUpdate,
  resetListUpdateSeqForTests,
} from '@/components/shared/utils/list-update-seq'

type Item = { id: string; name: string }

function renderProbe(
  url: string,
  items: Item[],
  fetchList = vi.fn(async () => items)
) {
  setTestUrl(url)
  return renderWithContexts(SignalListPageProbe, { fetchList })
}

async function waitForMounted() {
  await vi.waitFor(() => {
    expect(screen.getByTestId('mounted').textContent).toBe('true')
  })
}

describe('createSignalListPage integration', () => {
  beforeEach(() => {
    resetListUpdateSeqForTests()
  })

  it('loads the list on mount and exposes sorted items', async () => {
    const items = [
      { id: 'c', name: 'charlie' },
      { id: 'a', name: 'alfa' },
      { id: 'b', name: 'bravo' },
    ]
    const fetchList = vi.fn(async () => items)
    renderProbe('/logs', items, fetchList)
    await waitForMounted()

    expect(fetchList).toHaveBeenCalledTimes(2)
    expect(screen.getByTestId('item-count').textContent).toBe('3')
    expect(screen.getByTestId('item-ids').textContent).toBe('a,b,c')
  })

  it('derives selectedId from the URL path', async () => {
    renderProbe('/logs/log-b', [{ id: 'log-a', name: 'a' }, { id: 'log-b', name: 'b' }])
    await waitForMounted()

    expect(screen.getByTestId('selected-id').textContent).toBe('log-b')
    expect(screen.getByTestId('selected-index').textContent).toBe('1')
  })

  it('does not clobber a shared-link id before the list finishes loading', async () => {
    let resolveFetch!: (items: Item[]) => void
    const fetchList = vi.fn(
      () =>
        new Promise<Item[]>(resolve => {
          resolveFetch = resolve
        })
    )

    setTestUrl('/logs/deep-link-id')
    renderWithContexts(SignalListPageProbe, { fetchList })

    expect(screen.getByTestId('loading').textContent).toBe('true')
    expect(window.location.pathname).toBe('/logs/deep-link-id')

    resolveFetch([{ id: 'other', name: 'other' }])
    await waitForMounted()
    await tick()

    // Stale id should trigger fallback navigation, not an immediate clobber mid-fetch.
    expect(window.location.pathname).toBe('/logs/other')
  })

  it('selectByOffset navigates with replace mode', async () => {
    const items = [
      { id: 'a', name: 'alfa' },
      { id: 'b', name: 'bravo' },
    ]
    let page: import('@/contexts/signal-list-page.svelte').SignalListPage<Item> | undefined

    setTestUrl('/logs/a')
    renderWithContexts(SignalListPageProbe, {
      fetchList: async () => items,
      onContext: ctx => {
        page = ctx
      },
    })
    await waitForMounted()

    page!.selectByOffset(1)
    await tick()

    expect(window.location.pathname).toBe('/logs/b')
  })

  it('does not let a stale list fetch overwrite newer search results', async () => {
    let resolveSlowFetch!: (items: Item[]) => void
    let fetchCalls = 0
    const fetchList = vi.fn(async () => {
      fetchCalls++
      if (fetchCalls <= 2) {
        return [{ id: 'a', name: 'alfa' }]
      }
      return new Promise<Item[]>(resolve => {
        resolveSlowFetch = resolve
      })
    })

    let page: import('@/contexts/signal-list-page.svelte').SignalListPage<Item> | undefined
    setTestUrl('/logs')
    renderWithContexts(SignalListPageProbe, {
      fetchList,
      onContext: ctx => {
        page = ctx
      },
    })
    await waitForMounted()

    const slowFetch = page!.runListFetch()
    const searchSeq = beginListUpdate('logs')
    page!.handleSearchResults({
      signal: 'logs',
      updateSeq: searchSeq,
      results: [{ id: 'search-only', name: 'search-only' }],
    } as unknown as import('@/types/api-types').SearchResultEvent)
    await tick()

    expect(screen.getByTestId('item-ids').textContent).toBe('search-only')

    resolveSlowFetch([
      { id: 'x', name: 'x-ray' },
      { id: 'y', name: 'yankee' },
    ])
    await slowFetch
    await tick()

    expect(screen.getByTestId('item-ids').textContent).toBe('search-only')
  })

  it('does not let a stale search overwrite a newer list fetch', async () => {
    let resolveSlowFetch!: (items: Item[]) => void
    let fetchCalls = 0
    const fetchList = vi.fn(async () => {
      fetchCalls++
      if (fetchCalls <= 2) {
        return [{ id: 'a', name: 'alfa' }]
      }
      return new Promise<Item[]>(resolve => {
        resolveSlowFetch = resolve
      })
    })

    let page: import('@/contexts/signal-list-page.svelte').SignalListPage<Item> | undefined
    setTestUrl('/logs')
    renderWithContexts(SignalListPageProbe, {
      fetchList,
      onContext: ctx => {
        page = ctx
      },
    })
    await waitForMounted()

    const staleSearchSeq = beginListUpdate('logs')
    const slowFetch = page!.runListFetch()
    page!.handleSearchResults({
      signal: 'logs',
      updateSeq: staleSearchSeq,
      results: [{ id: 'stale-search', name: 'stale' }],
    } as unknown as import('@/types/api-types').SearchResultEvent)
    await tick()

    expect(screen.getByTestId('item-ids').textContent).toBe('a')

    resolveSlowFetch([
      { id: 'x', name: 'x-ray' },
      { id: 'y', name: 'yankee' },
    ])
    await slowFetch
    await tick()

    expect(screen.getByTestId('item-ids').textContent).toBe('x,y')
  })

  it('does not let one signal\'s list update invalidate another signal\'s seq', () => {
    const logsSeq = beginListUpdate('logs')
    beginListUpdate('metrics')

    expect(isLatestListUpdate('logs', logsSeq)).toBe(true)
  })

  it('cancelPendingListUpdates invalidates in-flight ops for that signal only', () => {
    const logsSeq = beginListUpdate('logs')
    const metricsSeq = beginListUpdate('metrics')

    cancelPendingListUpdates('logs')

    expect(isLatestListUpdate('logs', logsSeq)).toBe(false)
    expect(isLatestListUpdate('metrics', metricsSeq)).toBe(true)
  })
})
