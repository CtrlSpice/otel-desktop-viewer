import { vi } from 'vitest'
import { fireEvent } from '@testing-library/svelte'
import HomePage from '@/pages/HomePage.svelte'
import { telemetryAPI } from '@/services/telemetry-service'
import { renderWithContexts, setTestUrl } from './render-helpers'
import type {
  DiscoveryOptions,
  DiscoveryPage,
  ServiceSummary,
  ServiceConnection,
  ServiceConnectionSpan,
} from '@/services/discovery-types'

export const service = (serviceName: string): ServiceSummary => ({
  serviceIdentity: serviceName,
  serviceName,
  serviceNamespace: 'test',
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
export const api = service('api')
export const worker = service('worker')
export const store = service('store')
export const edge = (spanCount = 1, target = worker): ServiceConnection => ({
  source: api,
  target,
  spanCount,
  traceCount: 1,
  errorSpanCount: 0,
})
export const evidence = (name: string, digit = '2'): ServiceConnectionSpan => ({
  traceID: '1'.repeat(32),
  spanID: digit.repeat(16),
  name,
  startTime: '1000000000',
  endTime: '1000000100',
  statusCode: 0,
})

export function discoveryRequests<T>() {
  const requests: Array<
    ReturnType<typeof Promise.withResolvers<DiscoveryPage<T>>> & {
      options: Omit<DiscoveryOptions, 'kind'>
      signal?: AbortSignal
    }
  > = []
  const read = (
    options: Omit<DiscoveryOptions, 'kind'> = {},
    signal?: AbortSignal
  ) => {
    const request = {
      ...Promise.withResolvers<DiscoveryPage<T>>(),
      options,
      signal,
    }
    requests.push(request)
    return request.promise
  }
  return { requests, read }
}

export function mountServiceMap() {
  vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(900)
  vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(800)
  for (const key of [
    'getTraceAttributeDefinitions',
    'getMetricAttributeDefinitions',
    'getLogAttributeDefinitions',
    'searchTraceSummaries',
    'searchMetricSummaries',
    'searchLogSummaries',
  ] as const)
    vi.spyOn(telemetryAPI, key).mockResolvedValue([])
  vi.spyOn(telemetryAPI, 'getStats').mockImplementation(
    () => new Promise(() => {})
  )
  vi.spyOn(telemetryAPI, 'getImportConfig').mockResolvedValue({
    otlpHttpPort: 4318,
  })
  const list = discoveryRequests<ServiceSummary>()
  const connections = discoveryRequests<ServiceConnection>()
  const spans = discoveryRequests<ServiceConnectionSpan>()
  vi.spyOn(telemetryAPI, 'discoverServices').mockImplementation(list.read)
  vi.spyOn(telemetryAPI, 'discoverServiceConnections').mockImplementation(
    connections.read
  )
  vi.spyOn(telemetryAPI, 'discoverServiceConnectionSpans').mockImplementation(
    spans.read
  )
  setTestUrl('/?start=1000&end=2000')
  const view = renderWithContexts(HomePage)
  const refresh = () =>
    fireEvent.submit(
      view.getByRole('searchbox', { name: 'Search telemetry' }).closest('form')!
    )
  return {
    view,
    list: list.requests,
    connections: connections.requests,
    spans: spans.requests,
    refresh,
  }
}
