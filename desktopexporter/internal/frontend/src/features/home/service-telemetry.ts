import { getStaticFieldsForSearch } from '@/constants/fields'
import { parseQuery } from '@/components/shared/Search/queryParser'
import {
  serviceQueryText,
  type DiscoveryOptions,
  type DiscoverySignal,
  type ServiceSummary,
} from '@/services/discovery/discovery-types'
import { telemetryAPI } from '@/services/telemetry-service'
import type { LogSummary, MetricSummary, TraceSummary } from '@/types/api-types'
import type { QueryNode } from '@/search/model'

export type ServiceTelemetryCard =
  | { signal: 'traces'; trace: TraceSummary }
  | { signal: 'logs'; log: LogSummary }
  | { signal: 'metrics'; metric: MetricSummary }

export async function loadServiceTelemetry(
  source: DiscoverySignal,
  service: ServiceSummary,
  options: Pick<DiscoveryOptions, 'queries' | 'startTime' | 'endTime'>,
  signal: AbortSignal
): Promise<ServiceTelemetryCard[]> {
  const submitted = options.queries?.[source]
  if (submitted === false) return []
  const serviceQuery = parseQuery(
    serviceQueryText(service),
    getStaticFieldsForSearch(source)
  )!
  const query: QueryNode = submitted
    ? {
        id: 'service-telemetry',
        type: 'group',
        group: { operator: 'AND', children: [submitted, serviceQuery] },
      }
    : serviceQuery
  const start = options.startTime == null ? null : BigInt(options.startTime)
  const end = options.endTime == null ? null : BigInt(options.endTime)
  if (source === 'traces') {
    const traces = await telemetryAPI.searchTraceSummaries(
      start,
      end,
      query,
      undefined,
      undefined,
      signal
    )
    signal.throwIfAborted()
    return traces.map(trace => ({ signal: 'traces', trace }))
  }
  if (source === 'logs') {
    const logs = await telemetryAPI.searchLogSummaries(
      start,
      end,
      query,
      undefined,
      undefined,
      signal
    )
    signal.throwIfAborted()
    return logs.map(log => ({ signal: 'logs', log }))
  }
  const metrics = await telemetryAPI.searchMetricSummaries(
    start,
    end,
    query,
    undefined,
    undefined,
    signal
  )
  signal.throwIfAborted()
  return metrics.map(metric => ({ signal: 'metrics', metric }))
}
