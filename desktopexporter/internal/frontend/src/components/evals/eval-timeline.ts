import type { SpanNode, TraceLogSummary } from '@/types/api-types'
import { recordsForSpan } from '@/components/traces/Waterfall/timeline-markers'

/** Timeline markers retain their received signal and recorded span ownership. */
export function evalTimelineRecords(node: SpanNode, logs: TraceLogSummary[]) {
  return recordsForSpan(
    node,
    logs.filter(
      log => log.spanID?.toLowerCase() === node.spanData.spanID.toLowerCase()
    )
  )
}
