// @vitest-environment jsdom
import { cleanup, fireEvent, render, waitFor } from '@testing-library/svelte'
import { afterEach, expect, it, vi } from 'vitest'
import { tick } from 'svelte'
import ServiceMap from './ServiceMap.svelte'
import { telemetryAPI } from '@/services/telemetry-service'
import {
  serviceIdentity,
  type ServiceSummary,
  type ServiceConnection,
} from '@/services/discovery-types'
import { renderWithContexts } from '@/test/render-helpers'
import type { MapSelection } from './service-map'

afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.restoreAllMocks()
})

it.each([
  ['page-first', 'fresh-first'],
  ['page-first', 'old-first'],
  ['refresh-first', 'fresh-first'],
  ['refresh-first', 'old-first'],
])(
  'keeps shared-edge generation with %s start and %s settlement',
  async (start, settlement) => {
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(900)
    vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(800)
    const a: ServiceSummary = {
      serviceName: 'A',
      serviceNamespace: '',
      traceCount: 1,
      spanCount: 1,
      errorSpanCount: 0,
      logCount: 0,
      errorLogCount: 0,
      logSeverityCounts: {
        trace: 0,
        debug: 0,
        info: 0,
        warn: 0,
        error: 0,
        fatal: 0,
        other: 0,
      },
      metricCount: 0,
      dataPointCount: 0,
      exemplarCount: 0,
      lastSeen: null,
    }
    const b = { ...a, serviceName: 'B' }
    const c = { ...a, serviceName: 'C' }
    const edge: ServiceConnection = {
      source: a,
      target: b,
      spanCount: 1,
      traceCount: 1,
      errorSpanCount: 0,
    }
    const fresh = { ...edge, spanCount: 9 }
    const sibling = { ...edge, source: b, target: c }
    type Page = Awaited<
      ReturnType<typeof telemetryAPI.discoverServiceConnections>
    >
    const work: ReturnType<typeof Promise.withResolvers<Page>>[] = []
    const read = vi
      .spyOn(telemetryAPI, 'discoverServiceConnections')
      .mockImplementation(() => {
        const request = Promise.withResolvers<Page>()
        work.push(request)
        return request.promise
      })
    const oldOptions = { startTime: '100', endTime: '200' }
    const newOptions = { startTime: '300', endTime: '400' }
    const props = {
      service: b,
      services: [a, b],
      servicesLoaded: true,
      options: oldOptions,
      revision: 0,
      selection: null as MapSelection | null,
      onSelect: vi.fn(),
      onUpdateSelection: vi.fn(),
      loadingServices: false,
      servicesError: '',
      moreServices: false,
      onMoreServices: vi.fn(),
    }
    const view = renderWithContexts(ServiceMap, props)
    await waitFor(() => expect(read).toHaveBeenCalledTimes(2))
    work[0].resolve({ items: [edge], truncated: false })
    work[1].resolve({ items: [edge], truncated: true })
    await waitFor(() =>
      expect(
        view.getByRole('button', { name: 'More connections' })
      ).toBeEnabled()
    )
    await tick()
    const canvas = view.container.querySelector('.service-map__canvas')
    const requestPage = () =>
      fireEvent.click(view.getByRole('button', { name: 'More connections' }))
    if (start === 'page-first') await requestPage()
    await view.rerender({
      componentProps: { ...props, options: newOptions, revision: 1 },
    })
    if (start === 'refresh-first') await requestPage()
    await waitFor(() => expect(read).toHaveBeenCalledTimes(5))
    const oldIndex = start === 'page-first' ? 2 : 4
    const aIndex = start === 'page-first' ? 3 : 2
    const bIndex = aIndex + 1
    expect(read.mock.calls[oldIndex][0]).toMatchObject({
      ...oldOptions,
      offset: 1,
      serviceIdentity: serviceIdentity(b),
    })
    expect(read.mock.calls[aIndex][0]).toMatchObject({
      ...newOptions,
      offset: 0,
      serviceIdentity: serviceIdentity(a),
    })
    const publishOld = async () => {
      work[oldIndex].resolve({ items: [edge, sibling], truncated: false })
      await view.findByRole('button', {
        name: 'B → C: 1 parent-child connections in 1 traces',
      })
      expect(read.mock.calls[oldIndex][1]?.aborted).toBe(false)
    }
    if (settlement === 'old-first') await publishOld()
    work[aIndex].resolve({ items: [fresh], truncated: false })
    const freshButton = await view.findByRole('button', {
      name: 'A → B: 9 parent-child connections in 1 traces',
    })
    const selected: MapSelection = {
      kind: 'connection',
      connection: fresh,
      options: newOptions,
      revision: 1,
    }
    await view.rerender({
      componentProps: {
        ...props,
        options: newOptions,
        revision: 1,
        selection: selected,
      },
    })
    props.onUpdateSelection.mockClear()
    if (settlement === 'fresh-first') await publishOld()
    expect(freshButton).toHaveAttribute(
      'aria-label',
      'A → B: 9 parent-child connections in 1 traces'
    )
    expect(props.onUpdateSelection).not.toHaveBeenCalled()
    await fireEvent.click(freshButton)
    expect(props.onSelect).toHaveBeenLastCalledWith(selected)
    expect(view.container.querySelector('.service-map__canvas')).toBe(canvas)
    expect(view.queryByRole('button', { name: 'More connections' })).toBeNull()
    // A failed replacement cannot remove B's independently published membership.
    work[bIndex].reject(new Error('B unavailable'))
    await view.findByRole('alert')
    expect(
      view.getByRole('button', {
        name: 'B → C: 1 parent-child connections in 1 traces',
      })
    ).toBeInTheDocument()
    await fireEvent.click(view.getByRole('button', { name: 'Retry' }))
    await waitFor(() => expect(read).toHaveBeenCalledTimes(6))
    work[5].resolve({ items: [], truncated: false })
    await waitFor(() =>
      expect(
        view.queryByRole('button', {
          name: 'B → C: 1 parent-child connections in 1 traces',
        })
      ).toBeNull()
    )
    expect(freshButton).toHaveAttribute(
      'aria-label',
      'A → B: 9 parent-child connections in 1 traces'
    )
  }
)

it.each(['failure', 'pagination'])(
  'keeps a fresh edge when a sibling retains an older copy: %s',
  async outcome => {
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(900)
    vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(800)
    const a: ServiceSummary = {
      serviceName: 'A',
      serviceNamespace: '',
      traceCount: 1,
      spanCount: 1,
      errorSpanCount: 0,
      logCount: 0,
      errorLogCount: 0,
      logSeverityCounts: {
        trace: 0,
        debug: 0,
        info: 0,
        warn: 0,
        error: 0,
        fatal: 0,
        other: 0,
      },
      metricCount: 0,
      dataPointCount: 0,
      exemplarCount: 0,
      lastSeen: null,
    }
    const b = { ...a, serviceName: 'B' }
    const edge: ServiceConnection = {
      source: a,
      target: b,
      spanCount: 1,
      traceCount: 1,
      errorSpanCount: 0,
    }
    type Page = Awaited<
      ReturnType<typeof telemetryAPI.discoverServiceConnections>
    >
    const pending = new Map<
      string,
      ReturnType<typeof Promise.withResolvers<Page>>
    >()
    const read = vi
      .spyOn(telemetryAPI, 'discoverServiceConnections')
      .mockImplementation(options => {
        const request = Promise.withResolvers<Page>()
        pending.set(options.serviceIdentity!, request)
        return request.promise
      })
    const props = {
      service: b,
      services: [a, b],
      servicesLoaded: true,
      options: {},
      revision: 0,
      selection: null,
      onSelect: vi.fn(),
      loadingServices: false,
      servicesError: '',
      moreServices: false,
      onMoreServices: vi.fn(),
    }
    const view = renderWithContexts(ServiceMap, props)
    await waitFor(() => expect(read).toHaveBeenCalledTimes(2))
    pending
      .get(serviceIdentity(a))!
      .resolve({ items: [edge], truncated: false })
    await view.findByRole('button', {
      name: 'A → B: 1 parent-child connections in 1 traces',
    })
    pending.get(serviceIdentity(b))!.resolve({ items: [edge], truncated: true })
    await waitFor(() =>
      expect(view.container.querySelector('.service-map')).toHaveAttribute(
        'aria-busy',
        'false'
      )
    )
    if (outcome === 'failure') {
      await view.rerender({ componentProps: { ...props, revision: 1 } })
      await waitFor(() => expect(read).toHaveBeenCalledTimes(4))
    } else {
      // Refresh A while B's earlier page remains available for pagination.
      await view.rerender({
        componentProps: { ...props, services: [a, b], revision: 1 },
      })
      await waitFor(() => expect(read).toHaveBeenCalledTimes(4))
      pending
        .get(serviceIdentity(b))!
        .resolve({ items: [edge], truncated: true })
      await tick()
    }
    pending
      .get(serviceIdentity(a))!
      .resolve({ items: [{ ...edge, spanCount: 9 }], truncated: false })
    const fresh = await view.findByRole('button', {
      name: 'A → B: 9 parent-child connections in 1 traces',
    })
    if (outcome === 'failure') {
      pending.get(serviceIdentity(b))!.reject(new Error('B unavailable'))
      await view.findByRole('alert')
    } else {
      await fireEvent.click(
        view.getByRole('button', { name: 'More connections' })
      )
      pending.get(serviceIdentity(b))!.resolve({ items: [], truncated: false })
      await waitFor(() =>
        expect(
          view.queryByRole('button', { name: 'More connections' })
        ).toBeNull()
      )
    }
    expect(fresh).toHaveAttribute(
      'aria-label',
      'A → B: 9 parent-child connections in 1 traces'
    )
    await fireEvent.click(fresh)
    expect(props.onSelect).toHaveBeenLastCalledWith({
      kind: 'connection',
      connection: { ...edge, spanCount: 9 },
      options: {},
      revision: 1,
    })
  }
)

it('shows ready services immediately and retains the canvas and connections during refresh', async () => {
  vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(900)
  vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(800)
  const service: ServiceSummary = {
    serviceName: 'ready',
    serviceNamespace: '',
    traceCount: 1,
    spanCount: 1,
    errorSpanCount: 0,
    logCount: 0,
    errorLogCount: 0,
    logSeverityCounts: {
      trace: 0,
      debug: 0,
      info: 0,
      warn: 0,
      error: 0,
      fatal: 0,
      other: 0,
    },
    metricCount: 0,
    dataPointCount: 0,
    exemplarCount: 0,
    lastSeen: null,
  }
  let finish!: (page: {
    items: ServiceConnection[]
    truncated: boolean
  }) => void
  const connections = vi
    .spyOn(telemetryAPI, 'discoverServiceConnections')
    .mockImplementation(
      () =>
        new Promise(resolve => {
          finish = resolve
        })
    )
  const props = {
    service,
    services: [service],
    servicesLoaded: true,
    options: {},
    revision: 0,
    selection: { kind: 'service' as const, service },
    onSelect: vi.fn(),
    loadingServices: false,
    servicesError: '',
    moreServices: false,
    onMoreServices: vi.fn(),
  }
  const view = renderWithContexts(ServiceMap, props)
  await waitFor(() => expect(connections).toHaveBeenCalledTimes(1))
  await view.findByRole('button', { name: /^ready/ })
  // LayerChart installs its lazy transform context once on initial mount.
  await waitFor(() =>
    expect(
      view.container.querySelector('.lc-layout-svg-g-transform')
    ).toHaveAttribute('transform')
  )
  const canvas = view.container.querySelector('.service-map__canvas svg')
  expect(canvas).not.toBeNull()
  await view.findByText('Loading service map…')
  finish({ items: [], truncated: false })
  await waitFor(() =>
    expect(view.queryByText('Loading service map…')).toBeNull()
  )
  expect(view.container.querySelector('.service-map__canvas svg')).toBe(canvas)
  await view.rerender({ componentProps: { ...props, revision: 1 } })
  await waitFor(() => expect(connections).toHaveBeenCalledTimes(2))
  expect(view.container.querySelector('.service-map__canvas svg')).toBe(canvas)
  expect(
    view.getByRole('button', { name: 'Zoom in service map' })
  ).toBeEnabled()
  await view.findByText('Loading service map…')
  view.unmount()
  expect(connections.mock.calls[1]![1]?.aborted).toBe(true)
})

it('restarts the service-map feedback grace period when service discovery is replaced', async () => {
  vi.useFakeTimers()
  const view = render(ServiceMap, {
    service: null,
    options: {},
    revision: 0,
    selection: null,
    onSelect: vi.fn(),
    services: [],
    servicesLoaded: false,
    loadingServices: true,
    servicesRequestKey: new AbortController().signal,
    servicesError: '',
    moreServices: false,
    onMoreServices: vi.fn(),
  })
  await tick()
  await vi.advanceTimersByTimeAsync(200)
  expect(view.getByText('Loading service map…')).toBeInTheDocument()
  await view.rerender({ servicesRequestKey: new AbortController().signal })
  expect(view.queryByText('Loading service map…')).toBeNull()
  await vi.advanceTimersByTimeAsync(199)
  expect(view.queryByText('Loading service map…')).toBeNull()
  await vi.advanceTimersByTimeAsync(1)
  expect(view.getByText('Loading service map…')).toBeInTheDocument()
  await view.rerender({ loadingServices: false })
  expect(view.queryByText('Loading service map…')).toBeNull()
  view.unmount()
  await vi.advanceTimersByTimeAsync(200)
  expect(view.queryByRole('status')).toBeNull()
})

it('keeps initial counts unknown until discovery succeeds and retains confirmed zeros on refresh', async () => {
  vi.useFakeTimers()
  const view = render(ServiceMap, {
    service: null,
    services: [],
    servicesLoaded: false,
    options: {},
    revision: 0,
    selection: null,
    onSelect: vi.fn(),
    loadingServices: false,
    servicesError: '',
    moreServices: false,
    onMoreServices: vi.fn(),
  })
  const header = view.getByRole('region', { name: 'Service map' })
  expect(header).not.toHaveTextContent(/\d+ (services|connections)/)
  expect(
    view.queryByText('No services match this search and time range.')
  ).toBeNull()
  await view.rerender({ loadingServices: true })
  await vi.advanceTimersByTimeAsync(199)
  expect(view.queryByText('Loading service map…')).toBeNull()
  expect(header).not.toHaveTextContent(/\d+ (services|connections)/)
  await vi.advanceTimersByTimeAsync(1)
  expect(view.getByText('Loading service map…')).toBeVisible()
  await view.rerender({
    loadingServices: false,
    servicesError: 'Discovery failed',
  })
  expect(header).not.toHaveTextContent(/\d+ (services|connections)/)
  expect(view.getByRole('alert')).toHaveTextContent('Discovery failed')
  expect(
    view.queryByText('No services match this search and time range.')
  ).toBeNull()
  await view.rerender({ servicesLoaded: true, servicesError: '' })
  expect(header).toHaveTextContent('0 services · 0 connections')
  expect(
    view.getByText('No services match this search and time range.')
  ).toBeVisible()
  await view.rerender({ loadingServices: true, revision: 1 })
  expect(header).toHaveTextContent('0 services · 0 connections')
  await view.rerender({
    loadingServices: false,
    servicesError: 'Refresh failed',
  })
  expect(header).toHaveTextContent('0 services · 0 connections')
  expect(view.getByRole('region', { name: 'Service map' })).toBe(header)
})

it.each(['empty', 'connection'])(
  'publishes a successful %s sibling without turning unfinished connection discovery into zero',
  async outcome => {
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(900)
    vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(800)
    const a: ServiceSummary = {
      serviceName: 'A',
      serviceNamespace: '',
      traceCount: 1,
      spanCount: 1,
      errorSpanCount: 0,
      logCount: 0,
      errorLogCount: 0,
      logSeverityCounts: {
        trace: 0,
        debug: 0,
        info: 0,
        warn: 0,
        error: 0,
        fatal: 0,
        other: 0,
      },
      metricCount: 0,
      dataPointCount: 0,
      exemplarCount: 0,
      lastSeen: null,
    }
    const b = { ...a, serviceName: 'B' }
    const edge: ServiceConnection = {
      source: a,
      target: b,
      spanCount: 1,
      traceCount: 1,
      errorSpanCount: 0,
    }
    type Page = Awaited<
      ReturnType<typeof telemetryAPI.discoverServiceConnections>
    >
    const pending = new Map<
      string,
      ReturnType<typeof Promise.withResolvers<Page>>
    >()
    const lookup = vi
      .spyOn(telemetryAPI, 'discoverServiceConnections')
      .mockImplementation(options => {
        const request = Promise.withResolvers<Page>()
        pending.set(options.serviceIdentity!, request)
        return request.promise
      })
    const props = {
      service: null,
      services: [a, b],
      servicesLoaded: true,
      options: {},
      revision: 0,
      selection: null,
      onSelect: vi.fn(),
      loadingServices: false,
      servicesError: '',
      moreServices: false,
      onMoreServices: vi.fn(),
    }
    const view = renderWithContexts(ServiceMap, props)
    const header = view.getByRole('region', {
      name: 'Service map',
    })
    await waitFor(() => expect(lookup).toHaveBeenCalledTimes(2))
    expect(header).toHaveTextContent('2 services')
    expect(header).not.toHaveTextContent(/\d+ connections?/)
    const aRequest = pending.get(serviceIdentity(a))!
    const bRequest = pending.get(serviceIdentity(b))!
    aRequest.resolve({
      items: outcome === 'connection' ? [edge] : [],
      truncated: false,
    })
    await tick()
    if (outcome === 'connection')
      await waitFor(() =>
        expect(header).toHaveTextContent('2 services · 1 connection')
      )
    else expect(header).not.toHaveTextContent(/\d+ connections?/)
    expect(lookup).toHaveBeenCalledTimes(2)
    expect(lookup.mock.calls[1][1]?.aborted).toBe(false)
    bRequest.reject(new Error('B unavailable'))
    await view.findByRole('alert')
    if (outcome === 'connection')
      expect(header).toHaveTextContent('2 services · 1 connection')
    else expect(header).not.toHaveTextContent(/\d+ connections?/)
    await fireEvent.click(view.getByRole('button', { name: 'Retry' }))
    pending.get(serviceIdentity(b))!.resolve({ items: [], truncated: false })
    const count = outcome === 'connection' ? '1 connection' : '0 connections'
    await waitFor(() =>
      expect(header).toHaveTextContent(`2 services · ${count}`)
    )
    const canvas = view.container.querySelector('.service-map__canvas')
    await view.rerender({
      componentProps: { ...props, revision: 1, loadingServices: true },
    })
    await waitFor(() => expect(lookup).toHaveBeenCalledTimes(5))
    expect(header).toHaveTextContent(`2 services · ${count}`)
    for (const request of pending.values())
      request.reject(new Error('Refresh unavailable'))
    await waitFor(() => expect(view.getAllByRole('alert')).toHaveLength(2))
    await view.rerender({
      componentProps: {
        ...props,
        revision: 1,
        servicesError: 'Services unavailable',
      },
    })
    expect(header).toHaveTextContent(`2 services · ${count}`)
    expect(view.container.querySelector('.service-map__canvas')).toBe(canvas)
  }
)
