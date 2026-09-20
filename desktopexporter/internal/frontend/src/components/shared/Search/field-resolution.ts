import type { FieldDefinition } from '@/constants/fields'

export type NamedField = Exclude<FieldDefinition, { searchScope: 'global' }>

/** Resolve query text without changing the identity of received attribute keys. */
export function resolveField(
  name: string,
  availableFields: FieldDefinition[]
): NamedField | undefined {
  const foldedName = name.toLowerCase()
  const nativeField = availableFields.find(
    (field): field is Extract<NamedField, { searchScope: 'field' }> =>
      field.searchScope === 'field' && field.name.toLowerCase() === foldedName
  )
  if (nativeField) return nativeField

  return availableFields.find(
    (field): field is Extract<NamedField, { searchScope: 'attribute' }> =>
      field.searchScope === 'attribute' && field.name === name
  )
}
