import { getStaticFieldsForSearch } from '@/constants/fields'
import { parseQuery } from '@/components/shared/Search/queryParser'
import {
  serviceQueryText,
  type DiscoveryOptions,
  type DiscoverySignal,
  type ServiceSummary,
} from '@/services/discovery-types'
import { telemetryAPI } from '@/services/telemetry-service'
import type { LogSummary, MetricSummary, TraceSummary } from '@/types/api-types'
import type { QueryNode } from '@/search/model'
import type { DiscoveredAttribute } from './attribute-coverage'

export type AttributeTelemetryCard =
  | { signal: 'traces'; trace: TraceSummary; count: number }
  | { signal: 'logs'; log: LogSummary }
  | { signal: 'metrics'; metric: MetricSummary; count: number }

export async function loadAttributeTelemetry(
  source: DiscoverySignal,
  attribute: DiscoveredAttribute,
  valueIdentity: string | undefined,
  service: ServiceSummary,
  options: Pick<DiscoveryOptions, 'queries' | 'startTime' | 'endTime'>,
  signal: AbortSignal
): Promise<AttributeTelemetryCard[]> {
  const submitted = options.queries?.[source]
  if (submitted === false) return []
  const serviceQuery = parseQuery(
    serviceQueryText(service),
    getStaticFieldsForSearch(source)
  )!
  const query: QueryNode = submitted
    ? {
        id: 'attribute-telemetry',
        type: 'group',
        group: { operator: 'AND', children: [submitted, serviceQuery] },
      }
    : serviceQuery
  const start = options.startTime == null ? null : BigInt(options.startTime)
  const end = options.endTime == null ? null : BigInt(options.endTime)
  const matches = await telemetryAPI.getAttributeRecordMatches(
    {
      ...options,
      signal: source,
      ownerType: attribute.ownerType,
      key: attribute.key,
      kind: attribute.kind,
      valueIdentity,
    },
    signal
  )
  signal.throwIfAborted()
  if (!matches.length) return []
  const byID = new Map(matches.map(match => [match.itemRef, match]))
  if (source === 'traces') {
    const traces = await telemetryAPI.searchTraceSummaries(
      start,
      end,
      query,
      undefined,
      undefined,
      signal
    )
    return traces.flatMap(trace => {
      const match = byID.get(trace.traceID)
      if (!match) return []
      const spanIDs = new Set(match.spanIDs)
      const matchedSpans =
        trace.matchedSpans?.filter(span => spanIDs.has(span.spanID)) ?? []
      return matchedSpans.length
        ? [
            {
              signal: 'traces',
              trace: { ...trace, matchedSpans },
              count: matchedSpans.length,
            },
          ]
        : []
    })
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
    return logs
      .filter(log => byID.has(log.logRef))
      .map(log => ({ signal: 'logs', log }))
  }
  const metrics = await telemetryAPI.searchMetricSummaries(
    start,
    end,
    query,
    undefined,
    undefined,
    signal
  )
  return metrics.flatMap(metric => {
    const match = byID.get(metric.metricRef)
    return match ? [{ signal: 'metrics', metric, count: match.count }] : []
  })
}
