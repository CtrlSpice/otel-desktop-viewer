import type { DetailFieldTarget } from '@/components/shared/utils/reveal-detail-field'
import type { LogColumn, LogTableRow } from '@/types/api-types'
import type { FieldDefinition } from '@/search/model'
import { attributeValueTokens } from '@/components/shared/detail-value'

export type LogSortColumn =
  'timestamp' | 'severity' | 'service' | 'body' | 'name'
export type LogSortDirection = 'asc' | 'desc'
export const LOG_SEVERITY_MIN_WIDTH = 160

export type LogColumnOption = {
  id: string
  key: string
  source: string
  min: number
  target: DetailFieldTarget
  sort?: LogSortColumn
  projection?: LogColumn
  identifier?: boolean
}

export const LOG_COLUMNS: LogColumnOption[] = [
  {
    id: 'severity',
    sort: 'severity',
    key: 'severity',
    source: 'Log',
    min: LOG_SEVERITY_MIN_WIDTH,
    target: { source: 'log', key: 'severityNumber', kind: 'field' },
  },
  {
    id: 'timestamp',
    sort: 'timestamp',
    key: 'timestamp',
    source: 'Log',
    min: 240,
    target: { source: 'log', key: 'timestamp', kind: 'field' },
  },
  {
    id: 'service',
    sort: 'service',
    key: 'service.name',
    source: 'Resource',
    min: 160,
    target: { source: 'resource', key: 'service.name', kind: 'attribute' },
  },
  {
    id: 'name',
    sort: 'name',
    key: 'eventName',
    source: 'Log',
    min: 160,
    target: { source: 'log', key: 'eventName', kind: 'field' },
  },
  {
    id: 'body',
    sort: 'body',
    key: 'body',
    source: 'Log',
    min: 240,
    target: { source: 'log', key: 'body', kind: 'field' },
  },
]

export const LOG_COLUMN_SOURCES = ['log', 'resource', 'scope'] as const
const FIELD_KEYS: Record<LogColumn['source'], readonly string[]> = {
  log: [
    'timeUnixNano',
    'observedTimeUnixNano',
    'severityText',
    'severityNumber',
    'traceId',
    'spanId',
    'eventName',
    'body',
    'flags',
    'droppedAttributesCount',
  ],
  resource: ['schemaUrl', 'droppedAttributesCount'],
  scope: ['name', 'version', 'schemaUrl', 'droppedAttributesCount'],
}

export function logColumnID(column: LogColumn): string {
  return JSON.stringify([column.kind, column.source, column.key])
}

function optionFor(column: LogColumn): LogColumnOption {
  const existing = LOG_COLUMNS.find(
    option =>
      option.target.source === column.source &&
      option.target.kind === column.kind &&
      option.target.key === column.key
  )
  if (existing) return existing
  const identifier =
    column.kind === 'field' &&
    column.source === 'log' &&
    ['traceId', 'spanId'].includes(column.key)
  return {
    id: logColumnID(column),
    key: column.key,
    source: column.source[0].toUpperCase() + column.source.slice(1),
    min:
      column.key === 'timeUnixNano' || column.key === 'observedTimeUnixNano'
        ? 240
        : identifier
          ? 180
          : 160,
    target: column,
    projection: column,
    identifier,
  }
}

function columnFromID(id: string): LogColumnOption | null {
  const fixed = LOG_COLUMNS.find(column => column.id === id)
  if (fixed) return fixed
  try {
    const parts: unknown = JSON.parse(id)
    if (!Array.isArray(parts) || parts.length !== 3) return null
    const [kind, source, key]: unknown[] = parts
    if (
      (kind !== 'field' && kind !== 'attribute') ||
      (source !== 'log' && source !== 'resource' && source !== 'scope') ||
      // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Validates a persisted column tuple before accepting its key as a field or literal attribute name.
      typeof key !== 'string' ||
      (kind === 'field' && !FIELD_KEYS[source].includes(key))
    )
      return null
    return optionFor({ kind, source, key })
  } catch {
    return null
  }
}

export function logColumnOptions(
  fields: FieldDefinition[],
  selected: string[]
): LogColumnOption[] {
  const options = new Map(LOG_COLUMNS.map(column => [column.id, column]))
  const add = (column: LogColumnOption) => options.set(column.id, column)
  for (const source of LOG_COLUMN_SOURCES) {
    for (const key of FIELD_KEYS[source])
      add(optionFor({ source, kind: 'field', key }))
  }
  for (const field of fields) {
    if (
      field.searchScope !== 'attribute' ||
      (field.attributeScope !== 'log' &&
        field.attributeScope !== 'resource' &&
        field.attributeScope !== 'scope')
    )
      continue
    add(
      optionFor({
        source: field.attributeScope,
        kind: 'attribute',
        key: field.name,
      })
    )
  }
  for (const id of selected) {
    const column = columnFromID(id)
    if (column) add(column)
  }
  // Keep Body on the trailing pinned edge as more columns are selected.
  return [...options.values()]
    .filter(column => column.id !== 'body')
    .concat(LOG_COLUMNS.filter(column => column.id === 'body'))
}

export function logColumnLabel(column: LogColumnOption): string {
  return `${column.source}${column.projection ? ` ${column.target.kind}` : ''}: ${column.key || '(empty key)'}`
}

export function logColumnRequests(selected: string[]): LogColumn[] {
  return selected.flatMap(id => {
    const column = columnFromID(id)
    return column?.projection ? [column.projection] : []
  })
}

export function logColumnCell(log: LogTableRow, column: LogColumnOption) {
  const result = log.columnValues?.find(
    value =>
      value.source === column.target.source &&
      value.kind === column.target.kind &&
      value.key === column.key
  )
  if (!result) return null
  return {
    text: result.values.length
      ? result.values
          .map(value => {
            if (value.kind === 'string' && value.value !== '')
              return value.value
            return attributeValueTokens(value)
              .map(token => token.text)
              .join('')
          })
          .join(' | ')
      : '—',
    numeric:
      result.values.length > 0 &&
      result.values.every(
        value => value.kind === 'int64' || value.kind === 'double'
      ),
    present: result.values.length > 0,
    string: result.values.length === 1 && result.values[0].kind === 'string',
  }
}

export function loadLogColumns(): string[] {
  try {
    const stored: unknown = JSON.parse(
      localStorage.getItem('logs-table-columns') ?? 'null'
    )
    if (Array.isArray(stored)) {
      const selected = [
        ...new Set(
          stored.flatMap(id => {
            // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Rejects non-string localStorage entries before parsing the persisted column identifier.
            if (typeof id !== 'string') return []
            const column = columnFromID(id)
            return column ? [column.id] : []
          })
        ),
      ]
      if (selected.length) return selected
    }
  } catch {
    /* Defaults remain usable when storage is unavailable. */
  }
  return LOG_COLUMNS.flatMap(column =>
    column.id === 'name' ? [] : [column.id]
  )
}
