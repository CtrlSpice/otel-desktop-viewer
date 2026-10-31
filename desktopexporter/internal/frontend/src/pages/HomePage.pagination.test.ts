// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { fireEvent, waitFor } from '@testing-library/svelte'
import { tick } from 'svelte'
import HomePage from './HomePage.svelte'
import { telemetryAPI } from '@/services/telemetry-service'
import * as navigationCounts from '@/contexts/navigation-counts.svelte'
import { navigate } from '@/route'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import {
  serviceIdentity,
  type ServiceSummary,
} from '@/services/discovery-types'

afterEach(() => vi.restoreAllMocks())

const service = (serviceName: string): ServiceSummary => ({
  serviceName,
  serviceNamespace: 'pagination',
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
})

function mountDiscovery() {
  vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(900)
  vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(800)
  vi.spyOn(telemetryAPI, 'getTraceAttributeDefinitions').mockResolvedValue([])
  vi.spyOn(telemetryAPI, 'getMetricAttributeDefinitions').mockResolvedValue([])
  vi.spyOn(telemetryAPI, 'getLogAttributeDefinitions').mockResolvedValue([])
  vi.spyOn(telemetryAPI, 'searchTraceSummaries').mockResolvedValue([])
  vi.spyOn(telemetryAPI, 'searchMetricSummaries').mockResolvedValue([])
  vi.spyOn(telemetryAPI, 'searchLogSummaries').mockResolvedValue([])
  vi.spyOn(telemetryAPI, 'getStats').mockImplementation(
    () => new Promise(() => {})
  )
  vi.spyOn(telemetryAPI, 'getImportConfig').mockResolvedValue({
    otlpHttpPort: 4318,
  })
  vi.spyOn(telemetryAPI, 'discoverServiceConnections').mockResolvedValue({
    items: [],
    truncated: false,
  })
  const publish = vi.fn()
  vi.spyOn(navigationCounts, 'getNavigationCounts').mockReturnValue({
    counts: {},
    matching: false,
    links: {},
    setHomeCounts: publish,
    refresh: vi.fn().mockResolvedValue(undefined),
  })
  type Page = Awaited<ReturnType<typeof telemetryAPI.discoverServices>>
  const requests: ReturnType<typeof Promise.withResolvers<Page>>[] = []
  const discovery = vi
    .spyOn(telemetryAPI, 'discoverServices')
    .mockImplementation(() => {
      const request = Promise.withResolvers<Page>()
      requests.push(request)
      return request.promise
    })
  setTestUrl('/?start=1000&end=2000')
  const view = renderWithContexts(HomePage)
  const identities = () =>
    [...view.container.querySelectorAll('.service-map__node')]
      .map(node => node.getAttribute('data-service-identity'))
      .sort()
  async function receive(index: number, names: string[], truncated = true) {
    requests[index].resolve({ items: names.map(service), truncated })
    await waitFor(() =>
      expect(view.getByRole('button', { name: 'Refresh' })).toBeEnabled()
    )
    await tick()
  }
  async function search(text: string) {
    const input = view.getByRole('searchbox', { name: 'Search telemetry' })
    await fireEvent.input(input, { target: { value: text } })
    await fireEvent.submit(input.closest('form')!)
  }
  const more = () =>
    fireEvent.click(view.getByRole('button', { name: 'More services' }))
  const expectNodes = (names: string[]) =>
    expect(identities()).toEqual(
      names.map(name => serviceIdentity(service(name))).sort()
    )
  return {
    view,
    discovery,
    requests,
    publish,
    receive,
    search,
    more,
    expectNodes,
  }
}

it.each(['query', 'time range'])(
  'keeps pagination and its navigation targets with the displayed list after a failed %s replacement',
  async change => {
    const {
      view,
      discovery,
      requests,
      publish,
      receive,
      search,
      more,
      expectNodes,
    } = mountDiscovery()
    await receive(0, ['A-first'])
    await more()
    await receive(1, ['A-second'])
    expectNodes(['A-first', 'A-second'])
    const initial = discovery.mock.calls[0][0]
    const links = publish.mock.lastCall![1]
    const canvas = view.container.querySelector('.service-map__canvas')

    if (change === 'query') await search('search("B")')
    else navigate('/?start=3000&end=4000')
    await waitFor(() => expect(discovery).toHaveBeenCalledTimes(3))
    const replacement = discovery.mock.calls[2][0]!
    expect(replacement.offset).toBe(0)
    if (change === 'query')
      expect(JSON.stringify(replacement.queries)).toContain('B')
    else
      expect(replacement).toMatchObject({
        startTime: '3000000000',
        endTime: '4000000000',
      })
    expectNodes(['A-first', 'A-second'])
    expect(publish.mock.lastCall![1]).toEqual(links)
    requests[2].reject(new Error('Replacement failed'))
    await view.findByText('Error: Replacement failed')

    await more()
    expect(discovery.mock.calls[3][0]).toEqual({ ...initial, offset: 200 })
    requests[3].reject(new Error('Page failed'))
    await view.findByText('Error: Page failed')
    expectNodes(['A-first', 'A-second'])
    await more()
    expect(discovery.mock.calls[4][0]).toEqual({ ...initial, offset: 200 })
    await receive(4, ['A-third'])
    expectNodes(['A-first', 'A-second', 'A-third'])
    expect(publish.mock.lastCall![1]).toEqual(links)
    expect(publish.mock.lastCall![2]).toBe(false)
    expect(view.container.querySelector('.service-map__canvas')).toBe(canvas)

    await fireEvent.click(view.getByRole('button', { name: 'Refresh' }))
    await waitFor(() => expect(discovery).toHaveBeenCalledTimes(6))
    // Search parsing can allocate new query IDs; the request keeps B's values and bounds.
    expect(discovery.mock.calls[5][0]).toMatchObject({
      startTime: replacement.startTime,
      endTime: replacement.endTime,
      offset: 0,
    })
    if (change === 'query')
      expect(JSON.stringify(discovery.mock.calls[5][0]!.queries)).toContain('B')
    await receive(5, ['B-first'])
    expectNodes(['B-first'])
    expect(publish.mock.lastCall![1]).not.toEqual(links)
    expect(publish.mock.lastCall![2]).toBe(change === 'query')
    await more()
    expect(discovery.mock.calls[6][0]).toEqual({
      ...discovery.mock.calls[5][0],
      offset: 100,
    })
    await receive(6, ['B-second'], false)
    expectNodes(['B-first', 'B-second'])
    expect(view.queryByRole('button', { name: 'More services' })).toBeNull()
  }
)

it.each(['success', 'failure'])(
  'cancels superseded pagination through A/B/A and ignores late %s after supersession or unmount',
  async outcome => {
    const {
      view,
      discovery,
      requests,
      publish,
      receive,
      search,
      more,
      expectNodes,
    } = mountDiscovery()
    await receive(0, ['A-first'])
    await more()
    expect(discovery.mock.calls[1][0]!.offset).toBe(100)
    await search('search("B")')
    await waitFor(() => expect(discovery).toHaveBeenCalledTimes(3))
    expect(discovery.mock.calls[1][1]?.aborted).toBe(true)
    await search('')
    await waitFor(() => expect(discovery).toHaveBeenCalledTimes(4))
    expect(discovery.mock.calls[2][1]?.aborted).toBe(true)
    const publications = publish.mock.calls.length
    for (const index of [1, 2]) {
      if (outcome === 'success')
        requests[index].resolve({ items: [service('stale')], truncated: false })
      else requests[index].reject(new Error('Stale failure'))
    }
    await tick()
    expectNodes(['A-first'])
    expect(view.queryByText('Error: Stale failure')).toBeNull()
    expect(view.getByRole('button', { name: 'More services' })).toBeDisabled()
    expect(discovery.mock.calls[3][1]?.aborted).toBe(false)
    expect(publish).toHaveBeenCalledTimes(publications)

    await receive(3, ['A-replaced'])
    expectNodes(['A-replaced'])
    await more()
    expect(discovery.mock.calls[4][0]).toEqual({
      ...discovery.mock.calls[0][0],
      offset: 100,
    })
    view.unmount()
    expect(discovery.mock.calls[4][1]?.aborted).toBe(true)
    const afterUnmount = publish.mock.calls.length
    if (outcome === 'success')
      requests[4].resolve({ items: [service('unmounted')], truncated: false })
    else requests[4].reject(new Error('Unmounted failure'))
    await tick()
    expect(publish).toHaveBeenCalledTimes(afterUnmount)
  }
)
