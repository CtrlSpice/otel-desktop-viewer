// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { fireEvent } from '@testing-library/svelte'
import { tick } from 'svelte'
import { navigate } from '@/route'
import ServiceConnectionDetails from './ServiceConnectionDetails.svelte'
import { telemetryAPI } from '@/services/telemetry-service'
import { copyText } from '@/utils/clipboard'
import { renderWithContexts } from '@/test/render-helpers'
import {
  discoveryRequests,
  edge,
  evidence,
  store,
} from '@/test/service-map-fixtures'
import type { ServiceConnectionSpan } from '@/services/discovery-types'
import { homeQueries } from './home-search'
import { parseQuery } from '@/components/shared/Search/queryParser'

vi.mock('@/utils/clipboard', () => ({ copyText: vi.fn() }))

afterEach(() => {
  vi.restoreAllMocks()
  vi.mocked(copyText).mockReset()
  vi.useRealTimers()
})

function mountEvidence() {
  const { requests, read } = discoveryRequests<ServiceConnectionSpan>()
  vi.spyOn(telemetryAPI, 'discoverServiceConnectionSpans').mockImplementation(
    read
  )
  const props = {
    connection: edge(),
    options: {
      startTime: '1000000000',
      endTime: '2000000000',
      queries: homeQueries(parseQuery("search('A')", [])),
    },
    revision: 0,
  }
  const view = renderWithContexts(ServiceConnectionDetails, props)
  return { view, requests, props }
}

it.each(['success', 'failure'])(
  'cancels old-owner Copy on displayed replacement and ignores its late %s',
  async outcome => {
    const copies: Array<
      ReturnType<
        typeof Promise.withResolvers<{
          serviceName: string
          serviceNamespace: string
        }>
      > & { identity: string; signal?: AbortSignal }
    > = []
    vi.spyOn(telemetryAPI, 'getServiceLabels').mockImplementation(
      (identity, signal) => {
        const request = {
          ...Promise.withResolvers<{
            serviceName: string
            serviceNamespace: string
          }>(),
          identity,
          signal,
        }
        copies.push(request)
        return request.promise
      }
    )
    const { view, requests, props } = mountEvidence()
    requests[0].resolve({
      items: [evidence('worker evidence')],
      truncated: false,
    })
    await view.findByRole('link', { name: /worker evidence/ })
    expect(copies).toHaveLength(0)
    const button = view.getByRole('button', { name: 'Copy Child service' })
    await fireEvent.click(button)
    expect(copies[0].identity).toBe('worker')
    // Identical reduced labels must not hide a change of exact source identity.
    await view.rerender({
      componentProps: {
        ...props,
        connection: edge(9, { ...store, serviceName: 'worker' }),
        revision: 1,
      },
    })
    expect(copies[0].signal?.aborted).toBe(false)
    requests[1].reject(new Error('Replacement unavailable'))
    await view.findByRole('alert')
    expect(copies[0].signal?.aborted).toBe(false)
    await fireEvent.click(view.getByRole('button', { name: 'Retry' }))
    requests[2].resolve({
      items: [evidence('store evidence')],
      truncated: false,
    })
    await view.findByRole('link', { name: /store evidence/ })
    expect(view.getByRole('button', { name: 'Copy Child service' })).toBe(
      button
    )
    expect(copies[0].signal?.aborted).toBe(true)
    await fireEvent.click(button)
    expect(copies[1].identity).toBe('store')
    if (outcome === 'success')
      copies[0].resolve({
        serviceName: 'exact old worker',
        serviceNamespace: 'old',
      })
    else copies[0].reject(new Error('Old copy failed'))
    await tick()
    expect(copyText).not.toHaveBeenCalled()
    expect(button).toHaveAttribute('aria-busy', 'true')
    expect(view.queryByText('Copied Child service.')).toBeNull()
    expect(view.queryByText('Could not copy Child service.')).toBeNull()
    copies[1].resolve({
      serviceName: 'exact new store',
      serviceNamespace: 'new',
    })
    await view.findByText('Copied Child service.')
    expect(copyText).toHaveBeenCalledExactlyOnceWith('exact new store')
  }
)

it('completes exact Copy for the retained displayed service during replacement and aborts on unmount', async () => {
  const pending = Promise.withResolvers<{
    serviceName: string
    serviceNamespace: string
  }>()
  const labels = vi
    .spyOn(telemetryAPI, 'getServiceLabels')
    .mockReturnValue(pending.promise)
  const { view, requests, props } = mountEvidence()
  requests[0].resolve({
    items: [evidence('worker evidence')],
    truncated: false,
  })
  await view.findByRole('link', { name: /worker evidence/ })
  await view.rerender({
    componentProps: { ...props, connection: edge(9, store), revision: 1 },
  })
  const button = view.getByRole('button', { name: 'Copy Child service' })
  await fireEvent.click(button)
  expect(labels.mock.calls[0][0]).toBe('worker')
  expect(labels.mock.calls[0][1]?.aborted).toBe(false)
  pending.resolve({
    serviceName: 'complete retained worker',
    serviceNamespace: '',
  })
  await view.findByText('Copied Child service.')
  expect(copyText).toHaveBeenCalledExactlyOnceWith('complete retained worker')
  labels.mockReturnValue(new Promise(() => {}))
  await fireEvent.click(button)
  view.unmount()
  expect(labels.mock.calls[1][1]?.aborted).toBe(true)
})

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

it.each([
  { order: 'page-first', outcome: 'pending' },
  { order: 'page-first', outcome: 'failed' },
  { order: 'replacement-first', outcome: 'pending' },
  { order: 'replacement-first', outcome: 'failed' },
])(
  'pages retained evidence in its own scope: $order with $outcome replacement',
  async ({ order, outcome }) => {
    const { view, requests, props } = mountEvidence()
    requests[0].resolve({
      items: [evidence('worker evidence')],
      truncated: true,
    })
    await view.findByRole('link', { name: /worker evidence/ })
    if (order === 'page-first')
      await fireEvent.click(view.getByRole('button', { name: 'Next' }))
    navigate('/?start=3000&end=4000&q=B')
    await view.rerender({
      componentProps: {
        ...props,
        connection: edge(9, store),
        options: {
          startTime: '3000000000',
          endTime: '4000000000',
          queries: homeQueries(parseQuery("search('B')", [])),
        },
        revision: 1,
      },
    })
    const pendingReplacement = requests.at(-1)!
    expect(pendingReplacement.options).toMatchObject({
      targetIdentity: 'store',
      offset: 0,
    })
    if (outcome === 'failed') {
      pendingReplacement.reject(new Error('Store unavailable'))
      await view.findByRole('alert')
    }
    const next = view.getByRole('button', { name: 'Next' })
    if (order === 'replacement-first') {
      expect(next).toBeEnabled()
      await fireEvent.click(next)
    }
    const retainedPage = requests[order === 'page-first' ? 1 : 2]
    expect(next).toBeDisabled()
    expect(retainedPage.options).toEqual({ ...requests[0].options, offset: 25 })
    expect(retainedPage.signal?.aborted).toBe(false)
    expect(pendingReplacement.signal?.aborted).toBe(false)
    retainedPage.resolve({
      items: [evidence('worker page two', '3')],
      truncated: false,
    })
    const retainedLink = await view.findByRole('link', {
      name: /worker page two/,
    })
    expect(retainedLink).toHaveAttribute(
      'href',
      expect.stringContaining('start=1000')
    )
    expect(retainedLink).toHaveAttribute(
      'href',
      expect.stringContaining('end=2000')
    )
    expect(
      new URL(
        retainedLink.getAttribute('href')!,
        'http://local'
      ).searchParams.has('q')
    ).toBe(false)
    expect(
      view.container.querySelector(
        '[data-field="Child service"] .detail-property__body'
      )
    ).toHaveTextContent('worker')
    if (outcome === 'failed')
      await fireEvent.click(view.getByRole('button', { name: 'Retry' }))
    const replacement = outcome === 'failed' ? requests[3] : pendingReplacement
    expect(replacement.options).toEqual(pendingReplacement.options)
    replacement.resolve({
      items: [evidence('store evidence', '4')],
      truncated: false,
    })
    const replacementLink = await view.findByRole('link', {
      name: /store evidence/,
    })
    expect(replacementLink).toHaveAttribute(
      'href',
      expect.stringContaining('start=3000')
    )
    expect(replacementLink).toHaveAttribute(
      'href',
      expect.stringContaining('end=4000')
    )
    expect(
      new URL(
        replacementLink.getAttribute('href')!,
        'http://local'
      ).searchParams.has('q')
    ).toBe(false)
    expect(view.getByText('Page 1')).toBeInTheDocument()
    expect(
      view.container.querySelector(
        '[data-field="Parent-child connections"] .detail-property__value'
      )
    ).toHaveTextContent('9')
    expect(view.queryByRole('link', { name: /worker page two/ })).toBeNull()
  }
)

it.each([
  { order: 'page-first', outcome: 'success' },
  { order: 'page-first', outcome: 'failure' },
  { order: 'replacement-first', outcome: 'success' },
  { order: 'replacement-first', outcome: 'failure' },
])(
  'ignores late page $outcome after replacement or unmount: $order',
  async ({ order, outcome }) => {
    const { view, requests, props } = mountEvidence()
    requests[0].resolve({
      items: [evidence('worker evidence')],
      truncated: true,
    })
    await view.findByRole('link', { name: /worker evidence/ })
    if (order === 'page-first')
      await fireEvent.click(view.getByRole('button', { name: 'Next' }))
    await view.rerender({
      componentProps: { ...props, connection: edge(9, store), revision: 1 },
    })
    const replacement = requests.at(-1)!
    if (order === 'replacement-first')
      await fireEvent.click(view.getByRole('button', { name: 'Next' }))
    const retainedPage = requests[order === 'page-first' ? 1 : 2]
    expect(requests).toHaveLength(3)
    expect(retainedPage.signal?.aborted).toBe(false)
    replacement.resolve({
      items: [evidence('store evidence')],
      truncated: true,
    })
    await view.findByRole('link', { name: /store evidence/ })
    expect(retainedPage.signal?.aborted).toBe(true)
    if (outcome === 'success')
      retainedPage.resolve({
        items: [evidence('stale page')],
        truncated: false,
      })
    else retainedPage.reject(new Error('Stale page error'))
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
