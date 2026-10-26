import type {
  AttributeValue,
  Attributes,
  SpanData,
  SpanNode,
} from '@/types/api-types'
import { attributeValueTokens } from '@/components/shared/detail-value'
import { formatDuration } from '@/utils/time'

export const COLUMN_SOURCES = ['span', 'resource', 'scope'] as const
export type ColumnSource = (typeof COLUMN_SOURCES)[number]
export type WaterfallColumn = {
  source: ColumnSource
  key: string
  field?: true
}
export type SizedWaterfallColumn = WaterfallColumn & {
  id: string
  width: number
}
export type WaterfallColumnOption = WaterfallColumn & {
  id: string
  count?: number
}

export const WATERFALL_COLUMNS_KEY = 'waterfall-attribute-columns'
export const DEFAULT_COLUMNS: readonly WaterfallColumn[] = [
  { source: 'resource', key: 'service.name' },
]

/** Owner and received key are separate identity fields, even when a key contains punctuation. */
export function columnID(column: WaterfallColumn): string {
  return JSON.stringify(
    column.field
      ? ['field', column.source, column.key]
      : [column.source, column.key]
  )
}

export function columnLabel(column: WaterfallColumn): string {
  return `${column.source}${column.field ? ' field' : ''}: ${column.key}`
}

/** Include cell padding and both visible ends of the shared Identifier. */
export function columnMinimumWidth(column: WaterfallColumn): number {
  if (column.field && column.source === 'span') {
    if (column.key === 'traceId') return 180
    if (column.key === 'spanId' || column.key === 'parentSpanId') return 120
  }
  return 100
}

type ColumnCell = {
  text: string
  title: string
  missing: boolean
  numeric: boolean
}

function scalarCell(
  value: string | number | bigint | null,
  numeric = false
): ColumnCell {
  return {
    text: value === null ? '—' : value === '' ? '""' : String(value),
    title: value === null ? 'Not present on this span' : String(value),
    missing: value === null,
    numeric,
  }
}

function enumCell(code: number, label: string): ColumnCell {
  const text = `${code} (${label})`
  return { text, title: text, missing: false, numeric: true }
}

/** Scalar fields already exposed by span details; the fixed Name column is omitted. */
const FIELD_CELLS: Record<
  ColumnSource,
  Record<string, (span: SpanData) => ColumnCell>
> = {
  span: {
    kind: span => enumCell(span.kindCode, span.kind),
    startTimeUnixNano: span => scalarCell(span.startTime, true),
    endTimeUnixNano: span => scalarCell(span.endTime, true),
    duration: span => ({
      text: formatDuration(span.endTime - span.startTime),
      title: `endTimeUnixNano − startTimeUnixNano: ${span.endTime - span.startTime} ns`,
      missing: false,
      numeric: true,
    }),
    'status.code': span => enumCell(span.statusCodeValue, span.statusCode),
    'status.message': span => scalarCell(span.statusMessage),
    traceId: span => scalarCell(span.traceID),
    traceState: span => scalarCell(span.traceState),
    spanId: span => scalarCell(span.spanID),
    parentSpanId: span => scalarCell(span.parentSpanID),
    flags: span => scalarCell(span.flags, true),
    droppedAttributesCount: span =>
      scalarCell(span.droppedAttributesCount, true),
    droppedEventsCount: span => scalarCell(span.droppedEventsCount, true),
    droppedLinksCount: span => scalarCell(span.droppedLinksCount, true),
  },
  resource: {
    droppedAttributesCount: span =>
      scalarCell(span.resource.droppedAttributesCount, true),
  },
  scope: {
    name: span => scalarCell(span.scope.name),
    version: span => scalarCell(span.scope.version),
    droppedAttributesCount: span =>
      scalarCell(span.scope.droppedAttributesCount, true),
  },
}

function attributesFor(span: SpanData, source: ColumnSource): Attributes {
  return source === 'span' ? span.attributes : span[source].attributes
}

/** Count spans carrying the key, not occurrences or distinct values. */
export function columnOptions(
  spans: readonly SpanNode[],
  selected: readonly WaterfallColumn[]
): WaterfallColumnOption[] {
  const options = new Map<string, WaterfallColumnOption>()
  for (const column of selected) {
    const id = columnID(column)
    options.set(id, { ...column, id, count: 0 })
  }
  for (const source of COLUMN_SOURCES) {
    for (const key of Object.keys(FIELD_CELLS[source])) {
      const column: WaterfallColumn = { source, key, field: true }
      const id = columnID(column)
      options.set(id, { ...column, id })
    }
  }
  for (const { spanData } of spans) {
    for (const source of COLUMN_SOURCES) {
      const seen = new Set<string>()
      for (const { key } of attributesFor(spanData, source)) {
        if (seen.has(key)) continue
        seen.add(key)
        const id = columnID({ source, key })
        const option = options.get(id)
        if (option) option.count = (option.count ?? 0) + 1
        else options.set(id, { source, key, id, count: 1 })
      }
    }
  }
  return [...options.values()].sort(
    (a, b) =>
      COLUMN_SOURCES.indexOf(a.source) - COLUMN_SOURCES.indexOf(b.source) ||
      (a.key < b.key ? -1 : a.key > b.key ? 1 : 0)
  )
}

function compactValue(value: AttributeValue): string {
  if (value.kind === 'string' && value.value !== '') return value.value
  return attributeValueTokens(value)
    .map(token =>
      token.kind === 'punctuation'
        ? token.text.replace(/\s+/g, ' ')
        : token.text
    )
    .join('')
}

/** Missing is not null, false, zero or an empty string; every received occurrence remains visible. */
export function columnCell(
  span: SpanData,
  column: WaterfallColumn
): ColumnCell {
  if (column.field) return FIELD_CELLS[column.source][column.key](span)
  const values = attributesFor(span, column.source).filter(
    attr => attr.key === column.key
  )
  return {
    text: values.length
      ? values.map(attr => compactValue(attr.value)).join(' | ')
      : '—',
    title: values.length
      ? values
          .map(attr => `${attr.value.kind}: ${compactValue(attr.value)}`)
          .join('\n')
      : 'Not present on this span',
    missing: values.length === 0,
    numeric:
      values.length > 0 &&
      values.every(
        ({ value }) => value.kind === 'int64' || value.kind === 'double'
      ),
  }
}

function isColumn(value: unknown): value is WaterfallColumn {
  return (
    value !== null &&
    typeof value === 'object' &&
    'source' in value &&
    (value.source === 'span' ||
      value.source === 'resource' ||
      value.source === 'scope') &&
    'key' in value &&
    typeof value.key === 'string' &&
    (!('field' in value) ||
      (value.field === true &&
        Object.hasOwn(FIELD_CELLS[value.source], value.key)))
  )
}

export function loadColumns(): WaterfallColumn[] {
  try {
    const saved: unknown = JSON.parse(
      localStorage.getItem(WATERFALL_COLUMNS_KEY) ?? 'null'
    )
    if (!Array.isArray(saved)) return [...DEFAULT_COLUMNS]
    return [
      ...new Map(
        saved.filter(isColumn).map(({ source, key, field }) => {
          const column: WaterfallColumn = field
            ? { source, key, field }
            : { source, key }
          return [columnID(column), column] as const
        })
      ).values(),
    ]
  } catch {
    return [...DEFAULT_COLUMNS]
  }
}

export function saveColumns(columns: readonly WaterfallColumn[]): void {
  try {
    localStorage.setItem(WATERFALL_COLUMNS_KEY, JSON.stringify(columns))
  } catch {
    // Column selection still works when preference storage is unavailable.
  }
}
