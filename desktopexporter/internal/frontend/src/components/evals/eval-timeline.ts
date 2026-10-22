import type { LogData, SpanNode, TraceLogSummary } from '@/types/api-types'
import {
  recordsForSpan,
  compareTimelineRecords,
  type TimelineRecord,
} from '@/components/traces/Waterfall/timeline-markers'
import {
  recordedToolCallID,
  uniqueToolCallID,
  toolLogOwners,
} from './eval-span-evidence'
import { referenceKey } from './eval-traces'
import type { EvalOutlineRow } from './eval-outline'

/** Exact-span log ownership and unique recorded call-ID correlations only. */
export function evalTimelineRecords(
  node: SpanNode,
  nodes: Map<string, SpanNode>,
  logs: TraceLogSummary[],
  correlationLogs: LogData[],
  row?: EvalOutlineRow
): TimelineRecord[] {
  const span = node.spanData
  const records = recordsForSpan(
    node,
    logs.filter(log => log.spanID?.toLowerCase() === span.spanID.toLowerCase())
  )
  const callID = uniqueToolCallID(span, nodes)
  if (callID) {
    const owners = new Set(
      toolLogOwners([span], nodes, callID).map(referenceKey)
    )
    const seen = new Set<string>()
    for (const log of correlationLogs) {
      if (
        !log.traceID ||
        !log.spanID ||
        log.traceID.toLowerCase() !== span.traceID.toLowerCase() ||
        log.spanID.toLowerCase() === span.spanID.toLowerCase() ||
        !owners.has(
          referenceKey({ traceID: log.traceID, spanID: log.spanID })
        ) ||
        recordedToolCallID(log.attributes) !== callID ||
        seen.has(log.logRef)
      )
        continue
      seen.add(log.logRef)
      const timestamp =
        log.timestamp !== 0n ? log.timestamp : log.observedTimestamp
      if (timestamp === 0n) continue
      records.push({
        kind: 'annotation',
        id: `correlation:${log.logRef}`,
        timestamp,
        name:
          log.eventName ||
          (log.body.kind === 'string' ? log.body.value : 'Correlated log'),
        description: `Call-ID-linked log · ${log.timestamp !== 0n ? 'recorded log time' : 'observed log time'}`,
        href: `/traces/${log.traceID}?${new URLSearchParams({ span: log.spanID, log: log.logRef })}`,
      })
    }
  }
  const assessment = row?.result?.assessments.find(
    item => item.id === row.assessmentID
  )
  if (
    assessment?.telemetry &&
    referenceKey(assessment.telemetry) === referenceKey(span) &&
    span.endTime >= span.startTime
  ) {
    records.push({
      kind: 'annotation',
      id: `assessment:${assessment.id}`,
      timestamp: span.endTime,
      name: assessment.sourceName || assessment.id,
      description: 'Correlated assessment · recorded span end',
    })
  }
  return records.sort(compareTimelineRecords)
}
