import { getStaticFieldsForSearch } from '@/constants/fields'
import {
  usesFieldToken,
  type FieldDefinition,
  type QueryNode,
  type SearchSignal,
} from '@/search/model'
import {
  createPathFieldReference,
  formatAttributeFieldReference,
  storedKindForField,
} from '@/components/shared/Search/attribute-field-reference'
import { quoteQueryString } from '@/components/shared/Search/query-literals'

export type HomeQueries = Record<SearchSignal, QueryNode | false | null>

/** An unavailable signal-specific condition cannot match that signal. */
function forSignal(node: QueryNode, signal: SearchSignal): QueryNode | false {
  if (node.type === 'text') return node
  if (node.type === 'condition') {
    const field = node.query.field
    if (usesFieldToken(field))
      return createPathFieldReference(
        field.path[0] === 'metadata' ? 'metadata' : field.owner,
        field.path,
        storedKindForField(field),
        signal
      )
        ? node
        : false
    const target = getStaticFieldsForSearch(signal).find(
      candidate => !usesFieldToken(candidate) && candidate.name === field.name
    )
    return target ? { ...node, query: { ...node.query, field: target } } : false
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
      const key = !usesFieldToken(field)
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
  if (node.type === 'text')
    return `search(${quoteQueryString(node.query.value)})`
  const { field, operator, value } = node.query
  const name = usesFieldToken(field)
    ? formatAttributeFieldReference(field)
    : field.name
  if (operator.symbol === 'IS NULL' || operator.symbol === 'IS NOT NULL')
    return `${name} ${operator.symbol}`
  // SAFETY: The query parser encodes IN operands as JSON string arrays, including numeric lexemes.
  const operand =
    operator.symbol === 'IN' || operator.symbol === 'NOT IN'
      ? `(${(JSON.parse(value) as string[]).map(quoteQueryString).join(', ')})`
      : quoteQueryString(value)
  return `${name} ${operator.symbol} ${operand}`
}
