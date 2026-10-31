// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { fireEvent, waitFor } from '@testing-library/svelte'
import { tick } from 'svelte'
import { navigate } from '@/route'
import {
  api,
  worker,
  edge,
  evidence,
  mountServiceMap,
} from '@/test/service-map-fixtures'

afterEach(() => vi.restoreAllMocks())

it('enables a ready service connection page while an unrelated sibling stays pending', async () => {
  const { view, list, connections, refresh } = mountServiceMap()
  list[0].resolve({ items: [api, worker], truncated: false })
  await waitFor(() => expect(connections).toHaveLength(2))
  connections[0].resolve({ items: [edge()], truncated: true })
  connections[1].resolve({ items: [], truncated: false })
  await waitFor(() =>
    expect(view.getByRole('button', { name: 'More connections' })).toBeEnabled()
  )
  await refresh()
  await waitFor(() => expect(connections).toHaveLength(4))
  await fireEvent.click(view.getByRole('button', { name: /^worker· test / }))
  await fireEvent.click(view.getByRole('button', { name: /^api· test / }))
  const more = view.getByRole('button', { name: 'More connections' })
  expect(more).toBeDisabled()
  connections[2].resolve({ items: [edge(9)], truncated: true })
  await view.findByRole('button', { name: /api → worker: 9 / })
  expect(more).toBeEnabled()
  expect(connections[3].signal?.aborted).toBe(false)
  await fireEvent.click(more)
  expect(connections).toHaveLength(5)
  expect(connections[4].options).toMatchObject({
    serviceIdentity: 'api',
    offset: 1,
  })
  expect(connections[3].signal?.aborted).toBe(false)
  expect(more).toBeDisabled()
  connections[4].resolve({ items: [], truncated: false })
  await waitFor(() =>
    expect(view.queryByRole('button', { name: 'More connections' })).toBeNull()
  )
})

it.each(['failure', 'empty'])(
  'retains connection inspection through service-list success until its own membership is replaced: %s',
  async outcome => {
    const { view, list, connections, spans, refresh } = mountServiceMap()
    list[0].resolve({ items: [api, worker], truncated: false })
    await waitFor(() => expect(connections).toHaveLength(2))
    for (const request of connections)
      request.resolve({ items: [edge()], truncated: false })
    await waitFor(() =>
      expect(
        view.container.querySelector('.lc-layout-svg-g-transform')
      ).toHaveAttribute('transform')
    )
    const target = await view.findByRole('button', { name: /api → worker: 1 / })
    await fireEvent.keyDown(target, { key: 'Enter' })
    spans[0].resolve({
      items: [evidence('retained evidence')],
      truncated: false,
    })
    const link = await view.findByRole('link', { name: /retained evidence/ })
    const table = view.getByRole('table', { name: 'Connection details' })
    await refresh()
    await waitFor(() => expect(connections).toHaveLength(4))
    list[1].resolve({ items: [api, worker], truncated: false })
    await waitFor(() =>
      expect(view.getByRole('button', { name: 'Refresh' })).toBeEnabled()
    )
    expect(view.getByRole('table', { name: 'Connection details' })).toBe(table)
    expect(view.getByRole('link', { name: /retained evidence/ })).toBe(link)
    expect(target).toHaveAttribute('aria-pressed', 'true')
    expect(spans).toHaveLength(1)
    if (outcome === 'failure') {
      for (const request of connections.slice(2))
        request.reject(new Error('Edges unavailable'))
      await waitFor(() => expect(view.getAllByRole('alert')).toHaveLength(2))
      expect(view.getByRole('table', { name: 'Connection details' })).toBe(
        table
      )
      expect(view.getByRole('link', { name: /retained evidence/ })).toBe(link)
    } else {
      connections[2].resolve({ items: [], truncated: false })
      await tick()
      expect(view.getByRole('table', { name: 'Connection details' })).toBe(
        table
      )
      connections[3].resolve({ items: [], truncated: false })
      await waitFor(() =>
        expect(view.queryByRole('button', { name: /api → worker/ })).toBeNull()
      )
      expect(
        view.queryByRole('table', { name: 'Connection details' })
      ).toBeNull()
      expect(view.queryByRole('link', { name: /retained evidence/ })).toBeNull()
      expect(
        view.getByRole('region', { name: 'Service map' })
      ).toHaveTextContent('0 connections')
    }
  }
)

it('reconciles a selected fresh edge and publishes its Details with successful evidence', async () => {
  const { view, list, connections, spans } = mountServiceMap()
  list[0].resolve({ items: [api], truncated: true })
  await waitFor(() => expect(connections).toHaveLength(1))
  connections[0].resolve({ items: [edge()], truncated: false })
  await waitFor(() =>
    expect(
      view.container.querySelector('.lc-layout-svg-g-transform')
    ).toHaveAttribute('transform')
  )
  await fireEvent.keyDown(
    await view.findByRole('button', { name: /api → worker: 1 / }),
    { key: 'Enter' }
  )
  spans[0].resolve({ items: [evidence('old evidence')], truncated: false })
  const oldLink = await view.findByRole('link', { name: /old evidence/ })
  const details = view.container.querySelector(
    '[data-field="Parent-child connections"] .detail-property__value'
  )
  expect(details).toHaveTextContent('1')
  await fireEvent.click(view.getByRole('button', { name: 'More services' }))
  list[1].resolve({ items: [worker], truncated: false })
  await waitFor(() => expect(connections).toHaveLength(2))
  connections[1].resolve({ items: [edge(9)], truncated: false })
  await view.findByRole('button', { name: /api → worker: 9 / })
  await waitFor(() => expect(spans).toHaveLength(2))
  expect(spans[1].options).toEqual(spans[0].options)
  expect(view.getByRole('link', { name: /old evidence/ })).toBe(oldLink)
  expect(details).toHaveTextContent('1')
  spans[1].reject(new Error('Evidence unavailable'))
  await view.findByText('Error: Evidence unavailable')
  expect(details).toHaveTextContent('1')
  await fireEvent.click(view.getByRole('button', { name: 'Retry' }))
  expect(spans).toHaveLength(3)
  spans[2].resolve({
    items: [evidence('fresh evidence', '3')],
    truncated: false,
  })
  await view.findByRole('link', { name: /fresh evidence/ })
  expect(details).toHaveTextContent('9')
  expect(view.queryByRole('link', { name: /old evidence/ })).toBeNull()
})

it.each(['query', 'time range'])(
  'inspects retained connections in their successful scope during pending and failed %s replacement',
  async change => {
    const { view, list, connections, spans, refresh } = mountServiceMap()
    list[0].resolve({ items: [api, worker], truncated: false })
    await waitFor(() => expect(connections).toHaveLength(2))
    connections[0].resolve({ items: [edge()], truncated: false })
    connections[1].resolve({ items: [edge()], truncated: false })
    await waitFor(() =>
      expect(
        view.container.querySelector('.lc-layout-svg-g-transform')
      ).toHaveAttribute('transform')
    )
    const target = await view.findByRole('button', { name: /api → worker: 1 / })
    if (change === 'query') {
      await fireEvent.input(
        view.getByRole('searchbox', { name: 'Search telemetry' }),
        { target: { value: 'search("new")' } }
      )
      await refresh()
    } else navigate('/?start=3000&end=4000')
    await waitFor(() => expect(connections).toHaveLength(4))
    await fireEvent.keyDown(target, { key: 'Enter' })
    expect(spans).toHaveLength(1)
    expect(spans[0].options).toMatchObject({
      startTime: '1000000000',
      endTime: '2000000000',
      queries: { traces: null, logs: null, metrics: null },
      serviceIdentity: 'api',
      targetIdentity: 'worker',
    })
    spans[0].resolve({
      items: [evidence('original evidence')],
      truncated: false,
    })
    const link = await view.findByRole('link', { name: /original evidence/ })
    list[1].reject(new Error('List unavailable'))
    for (const request of connections.slice(2))
      request.reject(new Error('Edges unavailable'))
    await view.findByText('Error: List unavailable')
    await fireEvent.keyDown(target, { key: 'Enter' })
    await tick()
    expect(
      spans.every(
        request =>
          request.options.startTime === '1000000000' &&
          request.options.queries?.traces === null
      )
    ).toBe(true)
    expect(view.getByRole('link', { name: /original evidence/ })).toBe(link)
  }
)
