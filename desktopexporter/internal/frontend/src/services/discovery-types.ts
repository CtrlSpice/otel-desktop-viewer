import type { AttributeValue } from '@/types/api-types'
import type { HomeQueries } from '@/components/home/home-search'

export const SIGNAL_ORDER = ['traces', 'metrics', 'logs'] as const
export type DiscoverySignal = (typeof SIGNAL_ORDER)[number]
export type DiscoveryOwner =
  | 'resource'
  | 'scope'
  | 'span'
  | 'event'
  | 'link'
  | 'log'
  | 'datapoint'
  | 'exemplar'
  | 'metadata'
export type DiscoveryOptions = {
  kind: 'services' | 'keys' | 'values' | 'connections' | 'connection-spans'
  signal?: DiscoverySignal
  ownerType?: DiscoveryOwner
  serviceName?: string
  namespace?: string
  serviceIdentity?: string
  targetIdentity?: string
  key?: string
  term?: string
  startTime?: string | null
  endTime?: string | null
  offset?: number
  limit?: number
  sortBy?: 'name' | 'frequency'
  sortDirection?: 'asc' | 'desc'
  queries?: HomeQueries
}
export type ServiceSummary = {
  serviceName: string
  serviceNamespace: string
  serviceIdentity?: string
  serviceNameKind?: string
  serviceNamespaceKind?: string
  traceCount: number
  spanCount: number
  errorSpanCount: number
  logCount: number
  errorLogCount: number
  logSeverityCounts: Record<
    'trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal' | 'other',
    number
  >
  metricCount: number
  dataPointCount: number
  exemplarCount: number
  lastSeen: string | null
}
export type AttributeKey = {
  key: string
  kind: string
  foundOn: { signal: DiscoverySignal; ownerType: DiscoveryOwner }[]
}

export type ServiceConnection = {
  source: ServiceSummary
  target: ServiceSummary
  spanCount: number
  traceCount: number
  errorSpanCount: number
}

export type ServiceConnectionSpan = {
  traceID: string
  spanID: string
  name: string
  startTime: string
  endTime: string
  statusCode: number
}
export type AttributeFrequency = {
  identity: string
  value: AttributeValue
  foundOn: AttributeKey['foundOn']
  count: number
  /** Distinct records carrying this key and received kind, before value pagination. */
  kindCount: number
  /** Distinct records carrying this key across all received kinds. */
  denominator: number
  relativeFrequency: number
}
export type DiscoveryTotals = {
  home: number
  traces: number
  metrics: number
  logs: number
}
export type DiscoveryPage<T> = {
  items: T[]
  truncated: boolean
  totals?: DiscoveryTotals
}

export function serviceIdentity(service: ServiceSummary): string {
  return (
    service.serviceIdentity ??
    JSON.stringify([service.serviceNamespace, service.serviceName])
  )
}

export function serviceSignalHref(
  service: ServiceSummary,
  signal: DiscoverySignal,
  range: Pick<DiscoveryOptions, 'startTime' | 'endTime'> = {},
  query = ''
): string {
  return discoverySignalHref(signal, range, serviceQueryText(service, query))
}

export function serviceQueryText(service: ServiceSummary, query = ''): string {
  const match = (key: string, value: string) => {
    const field = `resource.attributes[${JSON.stringify(key)}]::VARCHAR`
    return value === ''
      ? `(${field} = "" OR ${field} IS NULL)`
      : `${field} = ${JSON.stringify(value)}`
  }
  const serviceQuery =
    service.serviceIdentity !== undefined
      ? `serviceIdentity = ${JSON.stringify(service.serviceIdentity)}`
      : `${match('service.name', service.serviceName)} AND ${match('service.namespace', service.serviceNamespace)}`
  return query ? `(${query}) AND ${serviceQuery}` : serviceQuery
}

export function discoverySignalHref(
  signal: DiscoverySignal,
  range: Pick<DiscoveryOptions, 'startTime' | 'endTime'>,
  q: string
): string {
  const params = new URLSearchParams({ q })
  if (range.startTime && range.endTime) {
    params.set('start', String(BigInt(range.startTime) / 1_000_000n))
    params.set('end', String(BigInt(range.endTime) / 1_000_000n))
  } else params.set('time', 'all')
  return `/${signal}?${params}`
}
