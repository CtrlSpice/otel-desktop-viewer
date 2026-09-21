import type { EventData, SpanNode, TraceLogSummary } from '@/types/api-types'
import { severityBand } from '@/components/logs/log-severity'

export const WATERFALL_BAR_HEIGHT_REM = 0.875
export const MARKER_GLYPH_REM = WATERFALL_BAR_HEIGHT_REM
export const MARKER_HIT_AREA_REM = 1.5
export const MARKER_CLUSTER_DISTANCE_REM = 1
export const TIMELINE_LEFT_INSET_REM = 1.25
export const DURATION_GUTTER_REM = 5
export const OUTSIDE_RECORD_SLOT_REM = 2
export const BAR_LABEL_GAP_REM = 0.625

export type TimelineRecord =
  | {
      kind: 'event'
      id: string
      timestamp: bigint
      eventIndex: number
      event: EventData
    }
  | {
      kind: 'log'
      id: string
      timestamp: bigint
      log: TraceLogSummary
    }

export type MarkerPosition =
  | { placement: 'inside'; pixel: number; percent: number }
  | { placement: 'before' | 'after' | 'outside' }

export type TimelineMarker = {
  id: string
  members: TimelineRecord[]
  position: MarkerPosition
}

export type PartitionedTraceLogs = {
  bySpanID: Map<string, TraceLogSummary[]>
  unmatched: TraceLogSummary[]
}

export function partitionTraceLogs(
  logs: readonly TraceLogSummary[],
  spans: readonly SpanNode[]
): PartitionedTraceLogs {
  const spanIDs = new Set(spans.map(node => node.spanData.spanID))
  const bySpanID = new Map<string, TraceLogSummary[]>()
  const unmatched: TraceLogSummary[] = []
  for (const log of logs) {
    if (log.spanID === null || !spanIDs.has(log.spanID)) {
      unmatched.push(log)
      continue
    }
    const owned = bySpanID.get(log.spanID) ?? []
    owned.push(log)
    bySpanID.set(log.spanID, owned)
  }
  return { bySpanID, unmatched }
}

export function recordsForSpan(
  span: SpanNode,
  logs: readonly TraceLogSummary[]
): TimelineRecord[] {
  const spanID = span.spanData.spanID
  const events: TimelineRecord[] = span.spanData.events.map(
    (event, eventIndex) => ({
      kind: 'event',
      id: `event:${spanID}:${eventIndex}`,
      timestamp: event.timestamp,
      eventIndex,
      event,
    })
  )
  const logRecords: TimelineRecord[] = logs.map(log => ({
    kind: 'log',
    id: `log:${log.id}`,
    timestamp: log.timestamp,
    log,
  }))
  return [...events, ...logRecords].sort(compareTimelineRecords)
}

export function compareTimelineRecords(
  left: TimelineRecord,
  right: TimelineRecord
): number {
  if (left.timestamp !== right.timestamp)
    return left.timestamp < right.timestamp ? -1 : 1
  if (left.kind !== right.kind) return left.kind === 'event' ? -1 : 1
  if (left.kind === 'event' && right.kind === 'event')
    return left.eventIndex - right.eventIndex
  return left.id.localeCompare(right.id)
}

export function projectTimestamp(
  timestamp: bigint,
  start: bigint,
  end: bigint,
  widthPx: number
): MarkerPosition {
  if (timestamp < start) return { placement: 'before' }
  if (timestamp > end) return { placement: 'after' }
  const duration = end - start
  if (duration <= 0n || widthPx <= 0)
    return { placement: 'inside', pixel: 0, percent: 0 }
  const width = BigInt(Math.max(0, Math.floor(widthPx)))
  const offset = timestamp - start
  const pixel = Number((offset * width) / duration)
  return {
    placement: 'inside',
    pixel,
    percent: Number((offset * 1_000_000n) / duration) / 10_000,
  }
}

export function clusterTimelineRecords(
  records: readonly TimelineRecord[],
  start: bigint,
  end: bigint,
  widthPx: number,
  thresholdPx: number
): TimelineMarker[] {
  const projected = [...records].sort(compareTimelineRecords).map(record => ({
    record,
    position: projectTimestamp(record.timestamp, start, end, widthPx),
  }))
  const markers: TimelineMarker[] = []
  const outside: TimelineRecord[] = []
  for (const item of projected) {
    if (item.position.placement !== 'inside') {
      outside.push(item.record)
      continue
    }
    const previous = markers.at(-1)
    const nearInside =
      previous?.position.placement === 'inside' &&
      item.position.pixel - previous.position.pixel <= thresholdPx
    if (previous && nearInside) {
      previous.members.push(item.record)
      previous.id = previous.members.map(member => member.id).join('|')
    } else {
      markers.push({
        id: item.record.id,
        members: [item.record],
        position: item.position,
      })
    }
  }
  if (outside.length > 0) {
    markers.push({
      id: outside.map(record => record.id).join('|'),
      members: outside,
      position: { placement: 'outside' },
    })
  }
  return markers
}

export function markerComposition(
  marker: TimelineMarker
): 'event' | 'log' | 'mixed' {
  const events = marker.members.some(member => member.kind === 'event')
  const logs = marker.members.some(member => member.kind === 'log')
  return events && logs ? 'mixed' : events ? 'event' : 'log'
}

export function markerColor(marker: TimelineMarker, spanColor: string): string {
  const logs = marker.members.filter(
    (member): member is Extract<TimelineRecord, { kind: 'log' }> =>
      member.kind === 'log'
  )
  if (logs.length > 0) {
    const highest = logs.reduce((current, member) =>
      member.log.severityNumber > current.log.severityNumber ? member : current
    )
    const colors = {
      trace: 'var(--color-base-content)',
      debug: 'var(--color-success)',
      info: 'var(--color-info)',
      warn: 'var(--color-warning)',
      error: 'var(--color-error)',
      fatal: 'var(--color-error)',
    } as const
    return colors[severityBand(highest.log.severityNumber)]
  }
  return marker.members.some(
    member => member.kind === 'event' && member.event.name === 'exception'
  )
    ? 'var(--color-error)'
    : spanColor
}

export function markerLabel(marker: TimelineMarker): string {
  const eventCount = marker.members.filter(
    member => member.kind === 'event'
  ).length
  const logCount = marker.members.length - eventCount
  const parts = [
    eventCount > 0 ? `${eventCount} event${eventCount === 1 ? '' : 's'}` : '',
    logCount > 0 ? `${logCount} log${logCount === 1 ? '' : 's'}` : '',
  ].filter(Boolean)
  return marker.members.length === 1
    ? `Select ${parts[0]}`
    : `Open ${parts.join(' and ')}`
}
