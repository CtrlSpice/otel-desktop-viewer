import {
  type AttributeScope,
  type FieldDefinition,
  type FieldType,
} from '@/constants/fields'
import { getOperatorsForFieldType } from '@/constants/operators'

export type SearchSignal = 'traces' | 'logs' | 'metrics'
export type AttributeField = Extract<
  FieldDefinition,
  { searchScope: 'attribute' }
>

const SCOPES_BY_SIGNAL: Record<SearchSignal, ReadonlySet<AttributeScope>> = {
  traces: new Set(['resource', 'scope', 'span', 'event', 'link']),
  logs: new Set(['resource', 'scope', 'log']),
  metrics: new Set(['resource', 'scope', 'datapoint', 'exemplar', 'metadata']),
}

const FIELD_TYPES_BY_STORED_KIND = new Map<string, FieldType>([
  ['string', 'string'],
  ['int64', 'int64'],
  ['double', 'float64'],
  ['bool', 'boolean'],
  ['bytes', 'bytes'],
  ['empty', 'empty'],
  ['array', 'array'],
  ['map', 'map'],
])

const STORED_KINDS_BY_FIELD_TYPE = new Map<FieldType, string>([
  ['string', 'string'],
  ['int64', 'int64'],
  ['float64', 'double'],
  ['boolean', 'bool'],
  ['bytes', 'bytes'],
  ['empty', 'empty'],
  ['array', 'array'],
  ['map', 'map'],
])

export function storedKindForField(field: AttributeField): string {
  const kind = STORED_KINDS_BY_FIELD_TYPE.get(field.type)
  if (!kind) {
    throw new Error(`Attribute field type has no stored kind: ${field.type}`)
  }
  return kind
}

export function attributeFieldIdentity(field: AttributeField): string {
  return `${field.attributeScope}\u0000${field.name}\u0000${storedKindForField(field)}`
}

export function formatAttributeFieldReference(field: AttributeField): string {
  return `attr(${field.attributeScope}, ${JSON.stringify(field.name)}, ${storedKindForField(field)})`
}

export function createAttributeFieldReference(
  scope: string,
  name: string,
  kind: string,
  signal?: SearchSignal
): AttributeField | null {
  const type = FIELD_TYPES_BY_STORED_KIND.get(kind)
  if (!type) return null

  // SAFETY: The membership checks below reject unknown scopes and scopes that
  // do not belong to the selected signal before a search field is constructed.
  const attributeScope = scope as AttributeScope
  if (signal && !SCOPES_BY_SIGNAL[signal].has(attributeScope)) {
    return null
  }
  if (
    !Object.values(SCOPES_BY_SIGNAL).some(scopes => scopes.has(attributeScope))
  ) {
    return null
  }

  return {
    name,
    type,
    searchScope: 'attribute',
    attributeScope,
    operators: getOperatorsForFieldType(type),
  }
}

export function parseAttributeFieldReference(
  text: string,
  signal?: SearchSignal
): AttributeField | null {
  const match =
    /^attr\(\s*([a-z]+)\s*,\s*("(?:\\.|[^"\\])*")\s*,\s*([a-z0-9]+)\s*\)$/.exec(
      text
    )
  if (!match) return null

  let name: string
  try {
    // SAFETY: The regex captures a quoted JSON string. Parsing returns a
    // string or throws; the catch rejects invalid JSON.
    name = JSON.parse(match[2]) as string
  } catch {
    return null
  }
  return createAttributeFieldReference(match[1], name, match[3], signal)
}
