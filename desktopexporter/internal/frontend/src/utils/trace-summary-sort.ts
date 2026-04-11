import {
  compareByOptionalBigintField,
  compareByStringField,
  compareByTimestampField,
} from '@/utils/compare'
import { traceSummaryDurationNs } from '@/utils/duration'
import type { TraceSummary } from '@/types/api-types'

export type TraceSummarySortColumn =
  | 'serviceName'
  | 'rootSpanName'
  | 'startTime'
  | 'duration'
  | 'spanCount'
  | 'errorCount'
  | 'exceptionCount'

export type TraceSummarySortDirection = 'asc' | 'desc'

/** Primary key by column + direction; tie-break on trace ID. */
export function compareTraceSummaries(
  a: TraceSummary,
  b: TraceSummary,
  col: TraceSummarySortColumn,
  dir: TraceSummarySortDirection
): number {
  const cmp =
    col === 'serviceName'
      ? compareByStringField(a, b, t => t.rootSpan?.serviceName)
      : col === 'rootSpanName'
        ? compareByStringField(a, b, t => t.rootSpan?.name)
        : col === 'startTime'
          ? compareByTimestampField(a, b, t => t.rootSpan?.startTime)
          : col === 'duration'
            ? compareByOptionalBigintField(a, b, traceSummaryDurationNs)
            : col === 'spanCount'
              ? a.spanCount - b.spanCount
              : col === 'errorCount'
                ? a.errorCount - b.errorCount
                : a.exceptionCount - b.exceptionCount

  return cmp !== 0
    ? dir === 'asc'
      ? cmp
      : -cmp
    : a.traceID.localeCompare(b.traceID)
}

export function sortTraceSummaries(
  rows: TraceSummary[],
  col: TraceSummarySortColumn,
  dir: TraceSummarySortDirection
): TraceSummary[] {
  const out = [...rows]
  out.sort((a, b) => compareTraceSummaries(a, b, col, dir))
  return out
}
