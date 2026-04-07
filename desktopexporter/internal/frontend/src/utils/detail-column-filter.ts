import type { AttributeScope, FieldDefinition } from '@/constants/fields';

/** When empty, show all detail rows; otherwise only matching search fields / attributes. */
export function detailSearchFieldVisible(
  selected: FieldDefinition[],
  searchFieldName: string
): boolean {
  if (selected.length === 0) return true;
  return selected.some(
    f =>
      f.searchScope === 'field' &&
      'name' in f &&
      f.name === searchFieldName
  );
}

export function detailAttributeVisible(
  selected: FieldDefinition[],
  key: string,
  attributeScope: AttributeScope
): boolean {
  if (selected.length === 0) return true;
  return selected.some(
    f =>
      f.searchScope === 'attribute' &&
      'name' in f &&
      'attributeScope' in f &&
      f.name === key &&
      f.attributeScope === attributeScope
  );
}

/** Duration is not a search field; tie visibility to start/end time columns. */
export function detailDurationVisible(selected: FieldDefinition[]): boolean {
  if (selected.length === 0) return true;
  return (
    detailSearchFieldVisible(selected, 'startTime') ||
    detailSearchFieldVisible(selected, 'endTime')
  );
}
