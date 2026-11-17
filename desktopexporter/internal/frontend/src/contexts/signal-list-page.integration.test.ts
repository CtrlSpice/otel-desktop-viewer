// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { tick } from 'svelte'
import { screen } from '@testing-library/svelte'
import SignalListPageProbe from '@/test/SignalListPageProbe.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

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

async function waitForListLoaded() {
  await waitForMounted()
  await vi.waitFor(() => {
    expect(screen.getByTestId('loading').textContent).toBe('false')
  })
}

describe('createSignalListPage integration', () => {
  it('loads the list on mount and exposes sorted items', async () => {
    const items = [
      { id: 'c', name: 'charlie' },
      { id: 'a', name: 'alfa' },
      { id: 'b', name: 'bravo' },
    ]
    const fetchList = vi.fn(async () => items)
    renderProbe('/logs', items, fetchList)
    await waitForListLoaded()

    expect(fetchList).toHaveBeenCalledTimes(1)
    expect(screen.getByTestId('item-count').textContent).toBe('3')
    expect(screen.getByTestId('item-ids').textContent).toBe('a,b,c')
  })

  it('derives selectedID from the URL path', async () => {
    renderProbe('/logs/log-b', [
      { id: 'log-a', name: 'a' },
      { id: 'log-b', name: 'b' },
    ])
    await waitForListLoaded()

    expect(screen.getByTestId('selected-id').textContent).toBe('log-b')
    expect(screen.getByTestId('selected-index').textContent).toBe('1')
  })

  it('refreshes the submitted list without submitting or clearing the editor draft', async () => {
    let page:
      | import('@/contexts/signal-list-page.svelte').SignalListPage<
          Item,
          'name'
        >
      | undefined
    const fetchList = vi.fn(async () => [{ id: 'a', name: 'alfa' }])
    const submit = vi.fn()
    const clear = vi.fn()
    setTestUrl('/logs')
    renderWithContexts(SignalListPageProbe, {
      fetchList,
      onContext: ctx => {
        page = ctx
      },
    })
    await waitForListLoaded()
    page!.searchEditorApi = { submit, clear }

    page!.handleRefresh()

    expect(submit).not.toHaveBeenCalled()
    expect(clear).not.toHaveBeenCalled()
    expect(fetchList).toHaveBeenCalledTimes(2)
  })

  it('refreshes the list when no search editor is attached', async () => {
    let page:
      | import('@/contexts/signal-list-page.svelte').SignalListPage<
          Item,
          'name'
        >
      | undefined
    const fetchList = vi.fn(async () => [{ id: 'a', name: 'alfa' }])
    setTestUrl('/logs')
    renderWithContexts(SignalListPageProbe, {
      fetchList,
      onContext: ctx => {
        page = ctx
      },
    })
    await waitForListLoaded()

    page!.handleRefresh()

    await vi.waitFor(() => expect(fetchList).toHaveBeenCalledTimes(2))
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
    await waitForListLoaded()
    await tick()

    // Stale id should trigger fallback navigation, not an immediate clobber mid-fetch.
    expect(window.location.pathname).toBe('/logs/other')
  })

  it('selectByOffset navigates with replace mode', async () => {
    const items = [
      { id: 'a', name: 'alfa' },
      { id: 'b', name: 'bravo' },
    ]
    let page:
      | import('@/contexts/signal-list-page.svelte').SignalListPage<
          Item,
          'name'
        >
      | undefined

    setTestUrl('/logs/a')
    renderWithContexts(SignalListPageProbe, {
      fetchList: async () => items,
      onContext: ctx => {
        page = ctx
      },
    })
    await waitForListLoaded()

    page!.selectByOffset(1)
    await tick()

    expect(window.location.pathname).toBe('/logs/b')
  })

  it('does not let a stale list fetch overwrite newer search results', async () => {
    let resolveSlowFetch!: (items: Item[]) => void
    let fetchCalls = 0
    const fetchList = vi.fn(async () => {
      fetchCalls++
      if (fetchCalls <= 1) {
        return [{ id: 'a', name: 'alfa' }]
      }
      return new Promise<Item[]>(resolve => {
        resolveSlowFetch = resolve
      })
    })

    let page:
      | import('@/contexts/signal-list-page.svelte').SignalListPage<
          Item,
          'name'
        >
      | undefined
    setTestUrl('/logs')
    renderWithContexts(SignalListPageProbe, {
      fetchList,
      onContext: ctx => {
        page = ctx
      },
    })
    await waitForListLoaded()

    const slowFetch = page!.runListFetch()
    fetchList.mockResolvedValueOnce([
      { id: 'search-only', name: 'search-only' },
    ])
    await page!.runListFetch()
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
      if (fetchCalls <= 1) {
        return [{ id: 'a', name: 'alfa' }]
      }
      return new Promise<Item[]>(resolve => {
        resolveSlowFetch = resolve
      })
    })

    let page:
      | import('@/contexts/signal-list-page.svelte').SignalListPage<
          Item,
          'name'
        >
      | undefined
    setTestUrl('/logs')
    renderWithContexts(SignalListPageProbe, {
      fetchList,
      onContext: ctx => {
        page = ctx
      },
    })
    await waitForListLoaded()

    const staleSearch = page!.runListFetch()
    const resolveSearch = resolveSlowFetch
    const slowFetch = page!.runListFetch()
    await tick()

    expect(screen.getByTestId('item-ids').textContent).toBe('a')

    resolveSlowFetch([
      { id: 'x', name: 'x-ray' },
      { id: 'y', name: 'yankee' },
    ])
    await slowFetch
    resolveSearch([{ id: 'stale-search', name: 'stale' }])
    await staleSearch
    await tick()

    expect(screen.getByTestId('item-ids').textContent).toBe('x,y')
  })
})
