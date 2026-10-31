// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { fireEvent } from '@testing-library/svelte'
import { tick } from 'svelte'
import ServiceConnectionDetails from './ServiceConnectionDetails.svelte'
import { telemetryAPI } from '@/services/telemetry-service'
import { renderWithContexts } from '@/test/render-helpers'
import {
  discoveryRequests,
  edge,
  evidence,
  store,
} from '@/test/service-map-fixtures'
import type { ServiceConnectionSpan } from '@/services/discovery-types'

afterEach(() => {
  vi.restoreAllMocks()
  vi.useRealTimers()
})

function mountEvidence() {
  const { requests, read } = discoveryRequests<ServiceConnectionSpan>()
  vi.spyOn(telemetryAPI, 'discoverServiceConnectionSpans').mockImplementation(
    read
  )
  const props = {
    connection: edge(),
    options: { startTime: '1000000000', endTime: '2000000000' },
    revision: 0,
  }
  const view = renderWithContexts(ServiceConnectionDetails, props)
  return { view, requests, props }
}

it.each(['Next', 'Retry'])(
  'retries the failed next page from the displayed offset using %s',
  async action => {
    const { view, requests } = mountEvidence()
    requests[0].resolve({ items: [evidence('first page')], truncated: true })
    const link = await view.findByRole('link', { name: /first page/ })
    await fireEvent.click(view.getByRole('button', { name: 'Next' }))
    requests[1].reject(new Error('Page unavailable'))
    await view.findByRole('alert')
    expect(view.getByText('Page 1')).toBeInTheDocument()
    expect(view.getByRole('button', { name: 'Previous' })).toBeDisabled()
    expect(view.getByRole('link', { name: /first page/ })).toBe(link)
    await fireEvent.click(view.getByRole('button', { name: action }))
    expect(requests.map(request => request.options.offset)).toEqual([0, 25, 25])
    requests[2].resolve({
      items: [evidence('second page', '3')],
      truncated: false,
    })
    await view.findByText('Page 2')
    await fireEvent.click(view.getByRole('button', { name: 'Previous' }))
    expect(requests[3].options.offset).toBe(0)
  }
)

it.each(['pending', 'failed'])(
  'pages retained evidence with its own connection and scope while replacement is %s',
  async outcome => {
    const { view, requests, props } = mountEvidence()
    requests[0].resolve({
      items: [evidence('worker evidence')],
      truncated: true,
    })
    await view.findByRole('link', { name: /worker evidence/ })
    await view.rerender({
      componentProps: {
        ...props,
        connection: edge(9, store),
        options: { startTime: '3000000000', endTime: '4000000000' },
        revision: 1,
      },
    })
    expect(requests[1].options).toMatchObject({
      targetIdentity: 'store',
      offset: 0,
    })
    if (outcome === 'failed') {
      requests[1].reject(new Error('Store unavailable'))
      await view.findByRole('alert')
    }
    const next = view.getByRole('button', { name: 'Next' })
    expect(next).toBeEnabled()
    await fireEvent.click(next)
    expect(requests[2].options).toEqual({ ...requests[0].options, offset: 25 })
    expect(requests[1].signal?.aborted).toBe(false)
    requests[2].resolve({
      items: [evidence('worker page two', '3')],
      truncated: false,
    })
    await view.findByRole('link', { name: /worker page two/ })
    expect(
      view.container.querySelector(
        '[data-field="Child service"] .detail-property__body'
      )
    ).toHaveTextContent('worker')
    if (outcome === 'failed')
      await fireEvent.click(view.getByRole('button', { name: 'Retry' }))
    const replacement = outcome === 'failed' ? requests[3] : requests[1]
    expect(replacement.options).toEqual(requests[1].options)
    replacement.resolve({
      items: [evidence('store evidence', '4')],
      truncated: false,
    })
    await view.findByRole('link', { name: /store evidence/ })
    expect(view.getByText('Page 1')).toBeInTheDocument()
    expect(
      view.container.querySelector(
        '[data-field="Parent-child connections"] .detail-property__value'
      )
    ).toHaveTextContent('9')
    expect(view.queryByRole('link', { name: /worker page two/ })).toBeNull()
  }
)

it.each(['success', 'failure'])(
  'ignores late page %s when a successful replacement or unmount cancels it',
  async outcome => {
    const { view, requests, props } = mountEvidence()
    requests[0].resolve({
      items: [evidence('worker evidence')],
      truncated: true,
    })
    await view.findByRole('link', { name: /worker evidence/ })
    await view.rerender({
      componentProps: { ...props, connection: edge(9, store), revision: 1 },
    })
    await fireEvent.click(view.getByRole('button', { name: 'Next' }))
    expect(requests).toHaveLength(3)
    requests[1].resolve({
      items: [evidence('store evidence')],
      truncated: true,
    })
    await view.findByRole('link', { name: /store evidence/ })
    expect(requests[2].signal?.aborted).toBe(true)
    if (outcome === 'success')
      requests[2].resolve({ items: [evidence('stale page')], truncated: false })
    else requests[2].reject(new Error('Stale page error'))
    await tick()
    expect(view.queryByText(/stale page/i)).toBeNull()
    expect(view.queryByRole('alert')).toBeNull()
    await fireEvent.click(view.getByRole('button', { name: 'Next' }))
    expect(requests[3].options.targetIdentity).toBe('store')
    view.unmount()
    expect(requests[3].signal?.aborted).toBe(true)
    if (outcome === 'success')
      requests[3].resolve({ items: [evidence('unmounted')], truncated: false })
    else requests[3].reject(new Error('Unmounted page error'))
    await tick()
    expect(view.container).toBeEmptyDOMElement()
  }
)

it('starts immediately, resets the 200ms notice on supersession, and replaces successful empty evidence', async () => {
  vi.useFakeTimers()
  const { view, requests, props } = mountEvidence()
  expect(requests).toHaveLength(1)
  await vi.advanceTimersByTimeAsync(199)
  expect(view.queryByText('Loading connected spans…')).toBeNull()
  await vi.advanceTimersByTimeAsync(1)
  expect(view.getByText('Loading connected spans…')).toBeInTheDocument()
  requests[0].resolve({ items: [evidence('retained')], truncated: true })
  await vi.advanceTimersByTimeAsync(0)
  expect(view.queryByText('Loading connected spans…')).toBeNull()
  await view.rerender({
    componentProps: { ...props, connection: edge(9, store), revision: 1 },
  })
  await vi.advanceTimersByTimeAsync(199)
  await view.rerender({ componentProps: { ...props, revision: 2 } })
  expect(requests[1].signal?.aborted).toBe(true)
  requests[1].reject(new Error('Stale replacement'))
  await vi.advanceTimersByTimeAsync(199)
  expect(view.queryByText('Loading connected spans…')).toBeNull()
  expect(view.queryByRole('alert')).toBeNull()
  await vi.advanceTimersByTimeAsync(1)
  expect(view.getByText('Loading connected spans…')).toBeInTheDocument()
  requests[2].resolve({ items: [], truncated: false })
  await vi.advanceTimersByTimeAsync(0)
  expect(view.queryByRole('link', { name: /retained/ })).toBeNull()
  expect(
    view.getByText('No connected spans match this search and time range.')
  ).toBeInTheDocument()
  expect(view.queryByText('Loading connected spans…')).toBeNull()
  view.unmount()
})
