import type { TraceSummary } from "@/types/api-types"

export type TraceListStats = {
  traces: number
  spans: number
  services: number
  errors: number
  exceptions: number
}

/** Fold the current result set into summary counters */
export function traceListStats(summaries: TraceSummary[]): TraceListStats {
  const acc = summaries.reduce(
    (a, t) => {
      a.spans += t.spanCount
      a.errors += t.errorCount
      a.exceptions += t.exceptionCount
      const name = t.rootSpan?.serviceName?.trim()
      if (name) a.serviceNames.add(name)
      return a
    },
    {
      spans: 0,
      errors: 0,
      exceptions: 0,
      serviceNames: new Set<string>(),
    }
  )
  return {
    traces: summaries.length,
    spans: acc.spans,
    services: acc.serviceNames.size,
    errors: acc.errors,
    exceptions: acc.exceptions,
  }
}
