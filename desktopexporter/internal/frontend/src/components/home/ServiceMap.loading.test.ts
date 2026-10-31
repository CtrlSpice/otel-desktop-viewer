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

afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.restoreAllMocks()
})

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
