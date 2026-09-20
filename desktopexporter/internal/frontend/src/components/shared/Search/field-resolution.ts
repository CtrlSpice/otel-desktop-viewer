import type { FieldDefinition } from '@/constants/fields'
import {
  attributeFieldIdentity,
  parseAttributeFieldReference,
  type SearchSignal,
} from './attribute-field-reference'

export type NamedField = Exclude<FieldDefinition, { searchScope: 'global' }>

/** Resolve query text without changing the identity of received attribute keys. */
export function resolveField(
  name: string,
  availableFields: FieldDefinition[],
  signal?: SearchSignal
): NamedField | undefined {
  const explicit = parseAttributeFieldReference(name, signal)
  if (explicit) return explicit

  const foldedName = name.toLowerCase()
  const nativeField = availableFields.find(
    (field): field is Extract<NamedField, { searchScope: 'field' }> =>
      field.searchScope === 'field' && field.name.toLowerCase() === foldedName
  )
  if (nativeField) return nativeField

  const attributes = availableFields.filter(
    (field): field is Extract<NamedField, { searchScope: 'attribute' }> =>
      field.searchScope === 'attribute' && field.name === name
  )
  const unique = new Map(
    attributes.map(field => [attributeFieldIdentity(field), field])
  )
  if (unique.size === 1) return unique.values().next().value
  return undefined
}

export function fieldResolutionIsAmbiguous(
  name: string,
  availableFields: FieldDefinition[]
): boolean {
  const identities = new Set<string>()
  for (const field of availableFields) {
    if (field.searchScope === 'attribute' && field.name === name) {
      identities.add(attributeFieldIdentity(field))
    }
  }
  return identities.size > 1
}
