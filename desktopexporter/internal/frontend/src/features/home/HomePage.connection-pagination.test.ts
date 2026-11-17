// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { fireEvent, waitFor, within } from '@testing-library/svelte'
import { tick } from 'svelte'
import { navigate } from '@/route'
import { api, edge, store, mountServiceMap } from '@/test/service-map-fixtures'

afterEach(() => vi.restoreAllMocks())

it.each([
  ['query', 'pending'],
  ['query', 'failed'],
  ['time range', 'pending'],
  ['time range', 'failed'],
])(
  'continues and retries the displayed connection page during a %s replacement that is %s',
  async (change, outcome) => {
    const { view, list, connections, refresh } = mountServiceMap()
    list[0].resolve({ items: [api], truncated: false })
    await waitFor(() => expect(connections).toHaveLength(1))
    connections[0].resolve({ items: [edge()], truncated: true })
    const more = await view.findByRole('button', { name: 'More connections' })
    if (change === 'query') {
      await fireEvent.input(
        view.getByRole('searchbox', { name: 'Search telemetry' }),
        { target: { value: 'search("new")' } }
      )
      await refresh()
    } else navigate('/?start=3000&end=4000')
    await waitFor(() => expect(connections).toHaveLength(2))
    if (outcome === 'failed') {
      list[1].reject(new Error('List unavailable'))
      connections[1].reject(new Error('Replacement unavailable'))
      await view.findByText('api: Error: Replacement unavailable')
    }
    expect(more).toBeEnabled()
    await fireEvent.click(more)
    expect(connections[2].options).toEqual({
      ...connections[0].options,
      offset: 1,
    })
    expect(connections[1].signal?.aborted).toBe(false)
    expect(more).toBeDisabled()
    connections[2].reject(new Error('Page unavailable'))
    const pageError = await view.findByText('api: Error: Page unavailable')
    expect(more).toBeEnabled()
    await fireEvent.click(
      within(pageError.parentElement!).getByRole('button', { name: 'Retry' })
    )
    expect(connections[3].options).toEqual(connections[2].options)
    connections[3].resolve({ items: [edge(2, store)], truncated: true })
    await view.findByRole('button', { name: /api → store: 2 / })
    expect(
      view.getByRole('button', { name: /api → worker: 1 / })
    ).toBeInTheDocument()
    await fireEvent.click(more)
    expect(connections[4].options).toEqual({
      ...connections[0].options,
      offset: 2,
    })
    let replacement = connections[1]
    if (outcome === 'failed') {
      const error = view.getByText('api: Error: Replacement unavailable')
      await fireEvent.click(
        within(error.parentElement!).getByRole('button', { name: 'Retry' })
      )
      expect(connections[5].options).toEqual(connections[1].options)
      replacement = connections[5]
    }
    expect(connections[4].signal?.aborted).toBe(false)
    replacement.resolve({ items: [edge(9)], truncated: true })
    await view.findByRole('button', { name: /api → worker: 9 / })
    expect(connections[4].signal?.aborted).toBe(true)
    expect(view.queryByRole('button', { name: /api → store/ })).toBeNull()
    connections[4].resolve({ items: [edge(3, store)], truncated: false })
    await tick()
    expect(view.queryByRole('button', { name: /api → store/ })).toBeNull()
    await fireEvent.click(more)
    expect(connections.at(-1)?.options).toEqual({
      ...replacement.options,
      offset: 1,
    })
  }
)

it.each(['success', 'failure'])(
  'ignores late connection page %s after refresh replacement and unmount',
  async outcome => {
    const { view, list, connections } = mountServiceMap()
    list[0].resolve({ items: [api], truncated: false })
    await waitFor(() => expect(connections).toHaveLength(1))
    connections[0].resolve({ items: [edge()], truncated: true })
    const more = await view.findByRole('button', { name: 'More connections' })
    await fireEvent.click(more)
    navigate('/?start=3000&end=4000')
    await waitFor(() => expect(connections).toHaveLength(3))
    expect(connections[1].signal?.aborted).toBe(false)
    connections[2].resolve({ items: [edge(9)], truncated: true })
    await view.findByRole('button', { name: /api → worker: 9 / })
    expect(connections[1].signal?.aborted).toBe(true)
    if (outcome === 'success')
      connections[1].resolve({ items: [edge(1, store)], truncated: false })
    else connections[1].reject(new Error('Stale page'))
    await tick()
    expect(view.queryByRole('button', { name: /api → store/ })).toBeNull()
    expect(view.queryByText(/Stale page/)).toBeNull()
    await fireEvent.click(more)
    expect(connections[3].options).toEqual({
      ...connections[2].options,
      offset: 1,
    })
    view.unmount()
    expect(connections[3].signal?.aborted).toBe(true)
    if (outcome === 'success')
      connections[3].resolve({ items: [edge(2, store)], truncated: false })
    else connections[3].reject(new Error('Unmounted page'))
    await tick()
    expect(view.container).toBeEmptyDOMElement()
  }
)
