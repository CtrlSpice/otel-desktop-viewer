import type { AttributeValue } from '@/types/api-types'

export type DiscoverySignal = 'traces' | 'logs' | 'metrics'
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
  kind: 'services' | 'keys' | 'values'
  signal?: DiscoverySignal
  ownerType?: DiscoveryOwner
  serviceName?: string
  namespace?: string
  key?: string
  term?: string
  startTime?: string | null
  endTime?: string | null
  offset?: number
  limit?: number
  sortBy?: 'name' | 'frequency'
  sortDirection?: 'asc' | 'desc'
}
export type ServiceSummary = {
  serviceName: string
  serviceNamespace: string
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
export type AttributeFrequency = {
  value: AttributeValue
  foundOn: AttributeKey['foundOn']
  count: number
  denominator: number
  relativeFrequency: number
}
export type DiscoveryPage<T> = { items: T[]; truncated: boolean }

export function serviceIdentity(service: ServiceSummary): string {
  return JSON.stringify([service.serviceNamespace, service.serviceName])
}

export function serviceSignalHref(
  service: ServiceSummary,
  signal: DiscoverySignal,
  range: Pick<DiscoveryOptions, 'startTime' | 'endTime'> = {}
): string {
  const match = (key: string, value: string) => {
    const field = `attr(resource, ${JSON.stringify(key)}, string)`
    return value === ''
      ? `(${field} = "" OR ${field} = NULL)`
      : `${field} = ${JSON.stringify(value)}`
  }
  const q = `${match('service.name', service.serviceName)} AND ${match('service.namespace', service.serviceNamespace)}`
  const params = new URLSearchParams({ q })
  if (range.startTime && range.endTime) {
    params.set('start', String(BigInt(range.startTime) / 1_000_000n))
    params.set('end', String(BigInt(range.endTime) / 1_000_000n))
  } else params.set('time', 'all')
  return `/${signal}?${params}`
}
