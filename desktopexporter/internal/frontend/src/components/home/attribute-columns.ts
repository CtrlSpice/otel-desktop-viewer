import type { ResizableTableColumn } from '@/components/shared/ResizableTable.svelte'
import type { TableColumnOption } from '@/components/shared/TableColumnsPicker.svelte'
import type { RecordCounts } from './attribute-coverage'

export type AttributeColumnID =
  | 'name'
  | 'type'
  | 'source'
  | 'signals'
  | 'values'
  | 'count'
  | 'traces'
  | 'metrics'
  | 'logs'
  | 'frequency'
  | 'copy'
export type AttributeSortColumn = Exclude<AttributeColumnID, 'copy'>
export type AttributeSummary = {
  count: number
  values: number
  counts: RecordCounts
}
export type AttributeColumn = ResizableTableColumn &
  TableColumnOption & { id: AttributeColumnID; description: string }

export const ATTRIBUTE_COLUMNS: AttributeColumn[] = [
  {
    id: 'name',
    label: 'Name',
    group: 'Attribute',
    min: 240,
    width: 400,
    disabled: true,
    description: 'Received attribute key',
  },
  {
    id: 'type',
    label: 'Type',
    group: 'Attribute',
    min: 96,
    width: 112,
    description: 'Received value type; values are not coerced',
  },
  {
    id: 'source',
    label: 'Source',
    group: 'Attribute',
    min: 104,
    width: 128,
    description: 'Attribute owner, such as resource, span, event or datapoint',
  },
  {
    id: 'signals',
    label: 'Signals',
    group: 'Attribute',
    min: 160,
    width: 200,
    description: 'Signals containing this attribute or exact value',
  },
  {
    id: 'values',
    label: 'Distinct values',
    group: 'Attribute',
    min: 144,
    width: 152,
    description: 'Distinct typed values in this service, search and time range',
  },
  {
    id: 'count',
    label: 'Records',
    group: 'Records',
    min: 112,
    width: 112,
    description:
      'Distinct records containing this attribute and type; exact-value rows count records containing that value',
  },
  {
    id: 'traces',
    label: 'Spans',
    group: 'Records',
    min: 104,
    width: 112,
    description:
      'Distinct spans containing this attribute and type, or this exact value',
  },
  {
    id: 'metrics',
    label: 'Datapoints',
    group: 'Records',
    min: 128,
    width: 136,
    description:
      'Distinct datapoints containing this attribute and type, or this exact value',
  },
  {
    id: 'logs',
    label: 'Logs',
    group: 'Records',
    min: 96,
    width: 104,
    description:
      'Distinct logs containing this attribute and type, or this exact value',
  },
  {
    id: 'frequency',
    label: '%',
    group: 'Records',
    min: 80,
    width: 88,
    fixed: true,
    description:
      'Exact-value frequency among records carrying this attribute, across all received types',
  },
  {
    id: 'copy',
    label: 'Copy',
    group: 'Actions',
    min: 64,
    width: 64,
    fixed: true,
    pinnable: false,
    description: 'Copy the complete received value',
  },
]

const DEFAULT_COLUMNS: AttributeColumnID[] = [
  'name',
  'values',
  'count',
  'frequency',
  'copy',
]
const COLUMNS_KEY = 'home-attribute-columns'
const PINNED_KEY = 'home-attribute-pinned-columns'

function readColumns(
  key: string,
  fallback: AttributeColumnID[],
  pinnable = false
): AttributeColumnID[] {
  try {
    const stored: unknown = JSON.parse(localStorage.getItem(key) ?? 'null')
    if (!Array.isArray(stored)) return [...fallback]
    // Establish the domain at the storage boundary: only catalogue IDs are columns.
    const recognized = ATTRIBUTE_COLUMNS.filter(column =>
      stored.includes(column.id)
    )
    if (stored.length > 0 && recognized.length === 0) return [...fallback]
    return recognized.flatMap(column =>
      !pinnable || column.pinnable !== false ? [column.id] : []
    )
  } catch {
    return [...fallback]
  }
}

export function loadAttributeColumns(): AttributeColumnID[] {
  return [
    ...new Set<AttributeColumnID>([
      'name',
      ...readColumns(COLUMNS_KEY, DEFAULT_COLUMNS),
    ]),
  ]
}

export function loadPinnedAttributeColumns(): AttributeColumnID[] {
  return readColumns(PINNED_KEY, ['name'], true)
}

export function saveAttributeColumns(
  selected: readonly string[],
  pinned: readonly string[]
): void {
  try {
    localStorage.setItem(COLUMNS_KEY, JSON.stringify(selected))
    localStorage.setItem(PINNED_KEY, JSON.stringify(pinned))
  } catch {
    // Column choices still work for the current view when storage is blocked.
  }
}

export function visibleAttributeColumns(
  selected: readonly string[],
  pinned: readonly string[]
): AttributeColumn[] {
  const visible = ATTRIBUTE_COLUMNS.filter(
    column => selected.includes(column.id) || column.id === 'name'
  )
  return [
    ...visible.filter(
      column => column.pinnable !== false && pinned.includes(column.id)
    ),
    ...visible.filter(
      column => column.pinnable === false || !pinned.includes(column.id)
    ),
  ]
}
