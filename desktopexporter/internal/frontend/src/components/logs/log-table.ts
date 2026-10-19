import type { DetailFieldTarget } from '@/components/shared/utils/reveal-detail-field'

export type LogSortColumn =
  'timestamp' | 'severity' | 'service' | 'body' | 'name'
export type LogSortDirection = 'asc' | 'desc'
export const LOG_SEVERITY_MIN_WIDTH = 160

export const LOG_COLUMNS: {
  id: LogSortColumn
  key: string
  source: string
  min: number
  target: DetailFieldTarget
}[] = [
  {
    id: 'severity',
    key: 'severity',
    source: 'Log',
    min: LOG_SEVERITY_MIN_WIDTH,
    target: { source: 'log', key: 'severityNumber', kind: 'field' },
  },
  {
    id: 'timestamp',
    key: 'timestamp',
    source: 'Log',
    min: 240,
    target: { source: 'log', key: 'timestamp', kind: 'field' },
  },
  {
    id: 'service',
    key: 'service.name',
    source: 'Resource',
    min: 160,
    target: { source: 'resource', key: 'service.name', kind: 'attribute' },
  },
  {
    id: 'name',
    key: 'eventName',
    source: 'Log',
    min: 160,
    target: { source: 'log', key: 'eventName', kind: 'field' },
  },
  {
    id: 'body',
    key: 'body',
    source: 'Log',
    min: 240,
    target: { source: 'log', key: 'body', kind: 'field' },
  },
]

export function loadLogColumns(): LogSortColumn[] {
  try {
    const stored: unknown = JSON.parse(
      localStorage.getItem('logs-table-columns') ?? 'null'
    )
    if (Array.isArray(stored)) {
      const selected = LOG_COLUMNS.flatMap(column =>
        stored.includes(column.id) ? [column.id] : []
      )
      if (selected.length) return selected
    }
  } catch {
    /* Defaults remain usable when storage is unavailable. */
  }
  return LOG_COLUMNS.flatMap(column =>
    column.id === 'name' ? [] : [column.id]
  )
}
