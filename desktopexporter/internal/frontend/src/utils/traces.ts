import {
  compareByOptionalBigintField,
  compareByStringField,
  compareByTimestampField,
} from '@/utils/compare'
import { traceSummaryDurationNs } from '@/utils/time'
import type { TraceSummary } from '@/types/api-types'

// --- Sort ---

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

// --- Table state (localStorage persistence) ---

const STORAGE_KEY = 'otel-desktop-viewer:trace-list-table-state-v1'

export interface TraceListTableState {
  sortColumn: TraceSummarySortColumn
  sortDirection: TraceSummarySortDirection
  rowsPerPage: number
}

const DEFAULTS: TraceListTableState = {
  sortColumn: 'startTime',
  sortDirection: 'desc',
  rowsPerPage: 25,
}

const VALID_SORT_COLUMNS: ReadonlySet<string> = new Set<TraceSummarySortColumn>(
  [
    'serviceName',
    'rootSpanName',
    'startTime',
    'duration',
    'spanCount',
    'errorCount',
    'exceptionCount',
  ]
)

const VALID_ROWS_PER_PAGE: ReadonlySet<number> = new Set([10, 25, 50, 100])

export function loadTraceListTableState(): TraceListTableState {
  if (typeof localStorage === 'undefined') return { ...DEFAULTS }
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return { ...DEFAULTS }
    const o = JSON.parse(raw) as Partial<TraceListTableState>
    return {
      sortColumn: VALID_SORT_COLUMNS.has(o.sortColumn ?? '')
        ? (o.sortColumn as TraceSummarySortColumn)
        : DEFAULTS.sortColumn,
      sortDirection:
        o.sortDirection === 'asc' || o.sortDirection === 'desc'
          ? o.sortDirection
          : DEFAULTS.sortDirection,
      rowsPerPage: VALID_ROWS_PER_PAGE.has(o.rowsPerPage ?? -1)
        ? o.rowsPerPage!
        : DEFAULTS.rowsPerPage,
    }
  } catch {
    return { ...DEFAULTS }
  }
}

export function saveTraceListTableState(state: TraceListTableState): void {
  if (typeof localStorage === 'undefined') return
  localStorage.setItem(STORAGE_KEY, JSON.stringify(state))
}
