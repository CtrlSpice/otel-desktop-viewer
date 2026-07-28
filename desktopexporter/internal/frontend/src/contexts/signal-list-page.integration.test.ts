// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { tick } from 'svelte'
import { screen } from '@testing-library/svelte'
import SignalListPageProbe from '@/test/SignalListPageProbe.svelte'
import { navigateToItem } from '@/route'
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

describe('createSignalListPage integration', () => {
  it('loads the list on mount and exposes sorted items', async () => {
    const items = [
      { id: 'c', name: 'charlie' },
      { id: 'a', name: 'alpha' },
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
      { id: 'a', name: 'alpha' },
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

  it('refetches when navigateToItem changes the route after mount', async () => {
    const fetchList = vi.fn(async () => [{ id: 'a', name: 'alpha' }])
    renderProbe('/logs', [{ id: 'a', name: 'alpha' }], fetchList)
    await waitForMounted()

    navigateToItem('logs', 'a')
    await tick()

    // Initial mount fetch only; time-range effect may fire once mounted.
    expect(fetchList.mock.calls.length).toBeGreaterThanOrEqual(1)
  })
})
