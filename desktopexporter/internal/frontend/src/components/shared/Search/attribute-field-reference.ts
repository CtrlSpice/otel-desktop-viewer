import {
  getOperatorsForFieldType,
  fieldIdentity,
  type AttributeScope,
  type FieldDefinition,
  type FieldType,
  type SearchSignal,
  type ValuePath,
} from '@/search/model'
import type { JsonAttributeDefinition } from '@/types/wire-types'
import { unquoteQueryString } from './query-literals'

export type AttributeField = FieldDefinition

export function attributeMetadata(field: AttributeField): string {
  const scope =
    field.owner === ''
      ? 'Any owner'
      : field.owner === 'scope'
        ? 'Instrumentation scope'
        : field.owner[0].toUpperCase() + field.owner.slice(1)
  const type =
    field.type === 'int64'
      ? 'Integer'
      : field.type === 'float64'
        ? 'Float'
        : field.type[0].toUpperCase() + field.type.slice(1)
  return `${scope} · ${type}${field.path.length > 2 || field.path[0] === 'body' ? ' · Nested' : ''}`
}

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
  return fieldIdentity(field)
}

const SQL_TYPES_BY_KIND = new Map([
  ['string', 'VARCHAR'],
  ['int64', 'BIGINT'],
  ['double', 'DOUBLE'],
  ['bool', 'BOOLEAN'],
  ['bytes', 'BLOB'],
  ['empty', 'EMPTY'],
  ['map', 'MAP'],
  ['array', 'ARRAY'],
])
const KINDS_BY_SQL_TYPE = new Map(
  [...SQL_TYPES_BY_KIND].map(([kind, type]) => [type.toLowerCase(), kind])
)

export function formatAttributeFieldReference(field: AttributeField): string {
  const prefix = field.owner ? `${field.owner}.` : ''
  const path = field.path
    .slice(1)
    .map(segment => `[${JSON.stringify(segment)}]`)
    .join('')
  const type = SQL_TYPES_BY_KIND.get(storedKindForField(field))
  if (!type)
    throw new Error(`Attribute field type has no SQL spelling: ${field.type}`)
  return `${prefix}${field.path[0]}${path}::${type}`
}

export function createAttributeFieldReference(
  scope: string,
  name: string,
  kind: string,
  signal?: SearchSignal
): AttributeField | null {
  return createPathFieldReference(
    scope,
    [scope === 'metadata' ? 'metadata' : 'attributes', name],
    kind,
    signal
  )
}

export function createPathFieldReference(
  scope: string,
  path: ValuePath,
  kind: string,
  signal?: SearchSignal
): AttributeField | null {
  const type = FIELD_TYPES_BY_STORED_KIND.get(kind)
  if (!type) return null

  // SAFETY: The membership checks below reject unknown scopes and scopes that
  // do not belong to the selected signal before a search field is constructed.
  const attributeScope = scope as AttributeScope
  if (scope && signal && !SCOPES_BY_SIGNAL[signal].has(attributeScope)) {
    return null
  }
  if (
    scope &&
    !Object.values(SCOPES_BY_SIGNAL).some(scopes => scopes.has(attributeScope))
  ) {
    return null
  }

  if (path[0] === 'body' && attributeScope !== 'log') return null
  if (path[0] === 'metadata' && attributeScope !== 'metadata') return null
  if (attributeScope === 'metadata' && path[0] !== 'metadata') return null
  const displayPath = path[0] === 'body' ? path : path.slice(1)
  return {
    name:
      displayPath.length === 1
        ? String(displayPath[0])
        : JSON.stringify(displayPath),
    type,
    owner:
      scope === ''
        ? ''
        : attributeScope === 'metadata'
          ? 'metric'
          : attributeScope,
    path,
    operators: getOperatorsForFieldType(type),
  }
}

export function fieldFromDiscovery(
  attribute: JsonAttributeDefinition
): AttributeField | null {
  return attribute.path
    ? createPathFieldReference(
        attribute.path[0] === 'metadata'
          ? 'metadata'
          : (attribute.owner ?? attribute.attributeScope),
        attribute.path,
        attribute.type
      )
    : createAttributeFieldReference(
        attribute.attributeScope,
        attribute.name,
        attribute.type
      )
}

export function parseAttributeFieldReference(
  text: string,
  signal?: SearchSignal,
  inferredKind = 'string'
): AttributeField | null {
  const match = /^(?:(\w+)\.)?(attributes|body|metadata)(.*)$/.exec(text.trim())
  if (!match) return null
  let rest = match[3]
  const path: (string | number)[] = [match[2]]
  while (rest.trimStart().startsWith('[')) {
    const part =
      /^\s*\[\s*('(?:''|\\.|[^'\\])*'|"(?:""|\\.|[^"\\])*"|\d+)\s*\]/.exec(rest)
    if (!part) return null
    if (/^\d+$/.test(part[1])) {
      const index = Number(part[1])
      if (index >= 4294967295 || (path.length === 1 && match[2] !== 'body'))
        return null
      path.push(index)
    } else path.push(unquoteQueryString(part[1]))
    rest = rest.slice(part[0].length)
  }
  const annotation = /^\s*::\s*([a-zA-Z0-9]+)\s*$/.exec(rest)
  if (rest.trim() && !annotation) return null
  const kind = annotation
    ? KINDS_BY_SQL_TYPE.get(annotation[1].toLowerCase())
    : inferredKind
  if (!kind || path.length < 2) return null
  const owner =
    match[1] ??
    (match[2] === 'body' ? 'log' : match[2] === 'metadata' ? 'metric' : '')
  const scope =
    owner === 'metric' && match[2] === 'metadata' ? 'metadata' : owner
  return createPathFieldReference(scope, path, kind, signal)
}
