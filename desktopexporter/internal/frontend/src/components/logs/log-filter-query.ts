import type { SyntaxNode } from '@lezer/common'
import type { FieldDefinition, QueryNode } from '@/search/model'
import { parser } from '@/components/shared/Search/codemirror/query.parser'
import { parseSearchRequest } from '@/components/shared/Search/queryParser'

export const LOG_SEVERITIES = [
  { label: 'Unspecified', min: 0, max: 0 },
  { label: 'Trace', min: 1, max: 4 },
  { label: 'Debug', min: 5, max: 8 },
  { label: 'Info', min: 9, max: 12 },
  { label: 'Warn', min: 13, max: 16 },
  { label: 'Error', min: 17, max: 20 },
  { label: 'Fatal', min: 21, max: 24 },
] as const

export type LogFilter = 'service' | 'severity'
type Facet = { kind: LogFilter; values: string[] }
type Term = { text: string; facet: Facet | null; node: QueryNode | null }

function isField(node: QueryNode, kind: LogFilter): boolean {
  if (node.type !== 'condition') return false
  const field = node.query.field
  return kind === 'service'
    ? field.searchScope === 'attribute' &&
        field.attributeScope === 'resource' &&
        field.name === 'service.name' &&
        field.type === 'string'
    : field.searchScope === 'field' && field.name === 'severityNumber'
}

function mentions(node: QueryNode | null, kind: LogFilter): boolean {
  if (!node) return false
  return node.type === 'group'
    ? node.group.children.some(child => mentions(child, kind))
    : isField(node, kind)
}

function isServiceList(value: unknown): value is string[] {
  return (
    Array.isArray(value) &&
    value.every((item): item is string => typeof item === 'string')
  )
}

function facet(node: QueryNode | null): Facet | null {
  if (!node) return null
  if (node.type === 'condition') {
    if (isField(node, 'service')) {
      if (node.query.operator.symbol === '=')
        return { kind: 'service', values: [node.query.value] }
      if (node.query.operator.symbol === 'IN') {
        const values: unknown = JSON.parse(node.query.value)
        if (isServiceList(values)) return { kind: 'service', values }
      }
    }
    if (
      isField(node, 'severity') &&
      node.query.operator.symbol === '=' &&
      node.query.value === '0'
    )
      return { kind: 'severity', values: ['Unspecified'] }
    return null
  }
  if (node.group.operator === 'OR') {
    const children = node.group.children.map(facet)
    const first = children[0]
    if (first && children.every(child => child?.kind === first.kind))
      return {
        kind: first.kind,
        values: [...new Set(children.flatMap(child => child?.values ?? []))],
      }
  }
  if (node.group.operator === 'AND' && node.group.children.length === 2) {
    const band = LOG_SEVERITIES.find(
      level =>
        level.min !== level.max &&
        node.group.children.every(
          child =>
            child.type === 'condition' &&
            isField(child, 'severity') &&
            ((child.query.operator.symbol === '>=' &&
              child.query.value === String(level.min)) ||
              (child.query.operator.symbol === '<=' &&
                child.query.value === String(level.max)))
        ) &&
        node.group.children.some(
          child =>
            child.type === 'condition' && child.query.operator.symbol === '>='
        ) &&
        node.group.children.some(
          child =>
            child.type === 'condition' && child.query.operator.symbol === '<='
        )
    )
    if (band) return { kind: 'severity', values: [band.label] }
  }
  return null
}

/** Read only top-level conjuncts. Mixed OR groups, negation and drafts stay intact. */
export function readLogFilters(text: string, fields: FieldDefinition[]) {
  const terms: Term[] = []
  let suffix = ''
  let predicateText = text.trim()
  let editable = true
  try {
    if (!text.trim())
      return {
        terms,
        suffix,
        predicateText,
        editable,
        service: { values: [], custom: false },
        severity: { values: [], custom: false },
      }
    const parsed = parseSearchRequest(text, fields, 'logs')
    if (
      parsed?.predicate?.type === 'condition' &&
      parsed.predicate.query.field.searchScope === 'global'
    )
      editable = false
    const tree = parser.parse(text)
    tree.iterate({
      enter: node => {
        if (node.type.isError) editable = false
      },
    })
    const request = tree.topNode.getChild('SearchRequest')
    const limit = request?.getChild('LimitClause')
    if (limit) {
      suffix = text.slice(limit.from).trim()
      predicateText = text.slice(0, limit.from).trim()
    }
    function walk(syntax: SyntaxNode) {
      const raw = text.slice(syntax.from, syntax.to)
      const node = parseSearchRequest(raw, fields, 'logs')?.predicate ?? null
      const recognized = facet(node)
      if (recognized) {
        terms.push({ text: raw, node, facet: recognized })
        return
      }
      if (syntax.name === 'Group' && syntax.firstChild) {
        walk(syntax.firstChild)
        return
      }
      if (syntax.name === 'AndExpression') {
        for (let child = syntax.firstChild; child; child = child.nextSibling)
          if (child.name !== 'And') walk(child)
      } else terms.push({ text: raw, node, facet: null })
    }
    const predicate = request?.firstChild
    if (predicate && predicate.name !== 'LimitClause' && editable)
      walk(predicate)
  } catch {
    editable = false
  }
  function group(kind: LogFilter) {
    const matches = terms.filter(term => term.facet?.kind === kind)
    const custom =
      !editable ||
      matches.length > 1 ||
      terms.some(term => !term.facet && mentions(term.node, kind))
    return { values: custom ? [] : (matches[0]?.facet?.values ?? []), custom }
  }
  return {
    terms,
    suffix,
    predicateText,
    editable,
    service: group('service'),
    severity: group('severity'),
  }
}

export function setLogFilter(
  text: string,
  fields: FieldDefinition[],
  kind: LogFilter,
  values: string[]
): string | null {
  const state = readLogFilters(text, fields)
  if (!state.editable || state[kind].custom) return null
  const kept = state.terms.some(term => term.facet?.kind === kind)
    ? state.terms.flatMap(term =>
        term.facet?.kind !== kind ? [`(${term.text})`] : []
      )
    : state.predicateText
      ? [`(${state.predicateText})`]
      : []
  if (values.length) {
    if (kind === 'service') {
      const quoted = values.map(
        value =>
          `"${value.replace(/\\/g, '\\\\').replace(/"/g, '\\"').replace(/\n/g, '\\n').replace(/\r/g, '\\r').replace(/\t/g, '\\t')}"`
      )
      kept.push(
        `attr(resource, "service.name", string) IN [${quoted.join(', ')}]`
      )
    } else {
      const conditions = LOG_SEVERITIES.flatMap(level =>
        !values.includes(level.label)
          ? []
          : [
              level.min === level.max
                ? `severityNumber = ${level.min}`
                : `(severityNumber >= ${level.min} AND severityNumber <= ${level.max})`,
            ]
      )
      if (conditions.length) kept.push(`(${conditions.join(' OR ')})`)
    }
  }
  const next = [kept.join(' AND '), state.suffix].filter(Boolean).join(' ')
  try {
    parseSearchRequest(next, fields, 'logs')
  } catch {
    return null
  }
  return next
}
