import type {
  DiscoveryOptions,
  DiscoveryPage,
  ServiceSummary,
  ServiceConnection,
  ServiceConnectionSpan,
  AttributeKey,
  HomeCoveragePage,
} from './discovery-types'
import type { QueryNode } from '@/search/model'
import { fetchArrow } from '../arrow/arrow-fetch'
import { attributeValue } from '../arrow/arrow-values'
import type { ArrowFrame } from '../arrow/arrow-data'
import {
  discoveryPage,
  serviceSummary,
  serviceConnection,
  serviceConnectionSpan,
  attributeKey,
} from '../arrow/arrow-discovery'
import {
  named,
  callRPC,
  callArrow,
  convertQueryTreeForBackend,
} from '../telemetry-request'

async function discover<T>(
  options: DiscoveryOptions,
  view: (frame: ArrowFrame, row: number) => T,
  signal?: AbortSignal
): Promise<DiscoveryPage<T>> {
  const convert = (query: QueryNode | false | null) =>
    query ? convertQueryTreeForBackend(query) : query
  const { queries, ...plainOptions } = options
  const wireOptions = queries
    ? {
        ...plainOptions,
        queries: {
          traces: convert(queries.traces),
          metrics: convert(queries.metrics),
          logs: convert(queries.logs),
        },
      }
    : plainOptions
  return discoveryPage(
    await fetchArrow(
      'discoverTelemetry',
      JSON.stringify({ options: wireOptions }),
      signal
    ),
    view
  )
}

export const discoveryAPI = {
  getServiceLabels: (
    serviceRef: string,
    signal?: AbortSignal
  ): Promise<{ serviceName: string; serviceNamespace: string }> =>
    callRPC('getServiceLabels', named({ serviceRef }), signal),

  discoverHomeAttributeValues: (
    options: Omit<DiscoveryOptions, 'kind' | 'signal'>,
    signal?: AbortSignal
  ): Promise<HomeCoveragePage> => {
    const convert = (query: QueryNode | false | null) =>
      query ? convertQueryTreeForBackend(query) : query
    return callRPC(
      'discoverTelemetry',
      named({
        options: {
          ...options,
          kind: 'home-values',
          queries: options.queries
            ? {
                traces: convert(options.queries.traces),
                metrics: convert(options.queries.metrics),
                logs: convert(options.queries.logs),
              }
            : undefined,
        },
      }),
      signal
    )
  },

  getHomeAttributeValue: async (valueRef: string, signal?: AbortSignal) => {
    const frame = await callArrow(
      'discoverTelemetry',
      named({
        options: {
          kind: 'home-value',
          valueRef,
        },
      }),
      signal
    )
    return attributeValue(frame.cell('value', 0))
  },

  discoverServiceConnections: (
    options: Omit<DiscoveryOptions, 'kind'>,
    signal?: AbortSignal
  ) =>
    discover<ServiceConnection>(
      { ...options, kind: 'connections' },
      serviceConnection,
      signal
    ),

  discoverServiceConnectionSpans: (
    options: Omit<DiscoveryOptions, 'kind'>,
    signal?: AbortSignal
  ) =>
    discover<ServiceConnectionSpan>(
      { ...options, kind: 'connection-spans' },
      serviceConnectionSpan,
      signal
    ),

  getServiceCount: (signal?: AbortSignal) =>
    callRPC<number>('getServiceCount', undefined, signal),

  discoverServices: (
    options: Omit<DiscoveryOptions, 'kind'> = {},
    signal?: AbortSignal
  ) =>
    discover<ServiceSummary>(
      { ...options, kind: 'services' },
      serviceSummary,
      signal
    ),

  discoverAttributeKeys: (
    options: Omit<DiscoveryOptions, 'kind'>,
    signal?: AbortSignal
  ) =>
    discover<AttributeKey>({ ...options, kind: 'keys' }, attributeKey, signal),
}
