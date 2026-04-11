import type { TraceData } from '@/types/api-types'

export type TraceDetailStats = {
  spanCount: number
  serviceCount: number
  errorCount: number
  exceptionCount: number
}

/** Fold a loaded trace into summary counters (aligned with trace list row semantics). */
export function traceDetailStats(data: TraceData): TraceDetailStats {
  const serviceNames = new Set<string>()
  let errorCount = 0
  let exceptionCount = 0
  for (const { spanData: s } of data.spans) {
    const name = s.resource.attributes
      .find(a => a.key === 'service.name')
      ?.value?.trim()
    if (name) serviceNames.add(name)
    if (s.statusCode === 'Error') errorCount++
    if (s.events.some(e => e.name === 'exception')) exceptionCount++
  }
  return {
    spanCount: data.spans.length,
    serviceCount: serviceNames.size,
    errorCount,
    exceptionCount,
  }
}
