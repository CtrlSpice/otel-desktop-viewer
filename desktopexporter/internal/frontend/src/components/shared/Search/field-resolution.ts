import {
  usesFieldToken,
  type FieldDefinition,
  type SearchSignal,
} from '@/search/model'
import {
  attributeFieldIdentity,
  parseAttributeFieldReference,
} from './attribute-field-reference'

export type NamedField = FieldDefinition

/** Resolve query text without changing the identity of received attribute keys. */
export function resolveField(
  name: string,
  availableFields: FieldDefinition[],
  signal?: SearchSignal,
  inferredKind?: string
): NamedField | undefined {
  const explicit = parseAttributeFieldReference(name, signal, inferredKind)
  if (explicit) return explicit
  if (/^(?:(?:\w+)\.)?(?:attributes|body|metadata)\s*\[/.test(name))
    return undefined

  const foldedName = name.toLowerCase()
  const nativeField = availableFields.find(
    field => !usesFieldToken(field) && field.name.toLowerCase() === foldedName
  )
  if (nativeField) return nativeField

  const attributes = availableFields.filter(
    field => usesFieldToken(field) && field.name === name
  )
  const unique = new Map(
    attributes.map(field => [
      attributeFieldIdentity({ ...field, owner: '' }),
      field,
    ])
  )
  const candidate = unique.values().next().value
  if (unique.size === 1 && candidate) return { ...candidate, owner: '' }
  return undefined
}

export function fieldResolutionIsAmbiguous(
  name: string,
  availableFields: FieldDefinition[]
): boolean {
  const identities = new Set<string>()
  for (const field of availableFields) {
    if (usesFieldToken(field) && field.name === name) {
      identities.add(attributeFieldIdentity({ ...field, owner: '' }))
    }
  }
  return identities.size > 1
}
