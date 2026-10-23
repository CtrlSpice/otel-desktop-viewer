import { getStaticFieldsForSearch } from '@/constants/fields'
import type { FieldDefinition, QueryNode, SearchSignal } from '@/search/model'
import {
  createAttributeFieldReference,
  formatAttributeFieldReference,
  storedKindForField,
} from '@/components/shared/Search/attribute-field-reference'

export type HomeQueries = Record<SearchSignal, QueryNode | false | null>

/** An unavailable signal-specific condition cannot match that signal. */
function forSignal(node: QueryNode, signal: SearchSignal): QueryNode | false {
  if (node.type === 'condition') {
    const field = node.query.field
    if (field.searchScope === 'global') return node
    if (field.searchScope === 'attribute')
      return createAttributeFieldReference(
        field.attributeScope,
        field.name,
        storedKindForField(field),
        signal
      )
        ? node
        : false
    return getStaticFieldsForSearch(signal).some(
      candidate =>
        candidate.searchScope === 'field' && candidate.name === field.name
    )
      ? node
      : false
  }
  const children: QueryNode[] = []
  for (const child of node.group.children) {
    const projected = forSignal(child, signal)
    if (projected === false) {
      if (node.group.operator === 'AND') return false
    } else children.push(projected)
  }
  if (!children.length) return false
  return children.length === 1
    ? children[0]
    : { ...node, group: { ...node.group, children } }
}

export function homeQueries(node: QueryNode | null): HomeQueries {
  return {
    traces: node && forSignal(node, 'traces'),
    metrics: node && forSignal(node, 'metrics'),
    logs: node && forSignal(node, 'logs'),
  }
}

export function homeFields(
  groups: readonly FieldDefinition[][]
): FieldDefinition[] {
  const fields = new Map<string, FieldDefinition>()
  for (const group of groups)
    for (const field of group) {
      if (field.searchScope === 'global') continue
      const key =
        field.searchScope === 'field'
          ? `field:${field.name}`
          : formatAttributeFieldReference(field)
      fields.set(key, field)
    }
  return [...fields.values()]
}

/** Emit editable query syntax, retaining exact strings and numeric lexemes. */
export function queryText(node: QueryNode | false | null): string {
  if (!node) return ''
  if (node.type === 'group')
    return `(${node.group.children.map(queryText).join(` ${node.group.operator} `)})`
  const { field, operator, value } = node.query
  if (field.searchScope === 'global') return `search(${JSON.stringify(value)})`
  const name =
    field.searchScope === 'attribute'
      ? formatAttributeFieldReference(field)
      : field.name
  if (operator.symbol === 'IS NULL') return `${name} = NULL`
  if (operator.symbol === 'IS NOT NULL') return `${name} != NULL`
  const operand =
    operator.symbol === 'IN' ||
    operator.symbol === 'NOT IN' ||
    (field.searchScope === 'attribute' && field.type.endsWith('[]')) ||
    field.type === 'array'
      ? value
      : JSON.stringify(value)
  return `${name} ${operator.symbol} ${operand}`
}
