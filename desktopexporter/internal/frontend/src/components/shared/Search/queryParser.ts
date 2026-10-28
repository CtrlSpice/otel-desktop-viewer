import {
  OPERATORS,
  usesFieldToken,
  type FieldDefinition,
  type Operator,
  type ParsedSearchRequest,
  type QueryNode,
  type SearchSignal,
} from '@/search/model'
import { parser } from './codemirror/query.parser'
import type { SyntaxNode } from '@lezer/common'
import { fieldResolutionIsAmbiguous, resolveField } from './field-resolution'
import { normalizeDuration, normalizeDurationList } from './duration-query'
import { unquoteQueryString as unquote } from './query-literals'

let nextQueryID = 0
function generateID(): string {
  return `query-${++nextQueryID}`
}

// Walk the same Lezer syntax tree used by CodeMirror.
export interface ValidationError {
  from: number
  to: number
  message: string
}

// Shared by parsing and validation so submitted and inline errors agree.
function unexpectedTokenMessage(value: string): string {
  if (!value.trim()) return 'Incomplete expression'
  return (
    `Unexpected "${value}". Values containing spaces must be quoted, ` +
    `for example: service.name = "Red Bull Racing"`
  )
}

function syntaxErrorMessage(
  input: string,
  error: { from: number; to: number }
): string {
  const token =
    input.slice(error.from, error.to) ||
    input.slice(error.from).trimStart().match(/^\S+/)?.[0] ||
    ''
  return unexpectedTokenMessage(token)
}

function findOperator(symbol: string): Operator | undefined {
  for (const op of Object.values(OPERATORS)) {
    if (op.symbol === symbol) return op
  }
  return undefined
}

const SIGIL_ALIASES = new Map<string, string>([['<>', '!=']])

const KEYWORD_SYMBOLS = new Map<string, string>([
  ['Contains', 'CONTAINS'],
  ['Regexp', 'REGEXP'],
  ['In', 'IN'],
  ['NotContains', 'NOT CONTAINS'],
  ['NotIn', 'NOT IN'],
  ['Like', 'LIKE'],
  ['NotLike', 'NOT LIKE'],
  ['NotRegexp', 'NOT REGEXP'],
  ['Is', 'IS'],
  ['IsNot', 'IS NOT'],
])

// Derived null and regexp operators inherit permission from their base operator.
const COMPAT_ALIASES = new Map<string, string>([
  ['IS NULL', '='],
  ['IS NOT NULL', '!='],
  ['NOT REGEXP', 'REGEXP'],
])

const SIGNED_INTEGER_FIELDS = new Set([
  'severitynumber',
  'droppedattributescount',
  'flags',
  'droppedeventscount',
  'droppedlinkscount',
  'event.droppedattributescount',
  'link.flags',
  'link.droppedattributescount',
  'resource.droppedattributescount',
  'scope.droppedattributescount',
  'kindcode',
  'statuscodevalue',
])

const TIMESTAMP_FIELDS = new Set([
  'timestamp',
  'observedtimestamp',
  'starttime',
  'endtime',
  'event.timestamp',
])

const INT64_MIN = -(1n << 63n)
const INT64_MAX = (1n << 63n) - 1n
const UINT64_MAX = (1n << 64n) - 1n

function parseExactInteger(value: string): bigint | null {
  const match =
    /^([+-]?)(?:(\d+)(?:\.(\d*))?|\.(\d+))(?:[eE]([+-]?\d+))?$/.exec(value)
  if (!match || value.length > 256) return null

  const sign = match[1] === '-' ? -1n : 1n
  const whole = match[2] ?? ''
  const fraction = match[3] ?? match[4] ?? ''
  const digits = (whole + fraction).replace(/^0+/, '')
  if (!digits) return 0n

  const scale = BigInt(match[5] ?? '0') - BigInt(fraction.length)
  let integerText: string
  if (scale >= 0n) {
    if (BigInt(digits.length) + scale > 20n) return null
    integerText = digits + '0'.repeat(Number(scale))
  } else {
    const shift = -scale
    const trailingZeros = digits.length - digits.replace(/0+$/, '').length
    if (shift > BigInt(trailingZeros)) return null
    integerText = digits.slice(0, digits.length - Number(shift))
  }

  return sign * BigInt(integerText || '0')
}

function isExactInt64(value: string): boolean {
  const integer = parseExactInteger(value)
  if (integer === null) return false
  return integer >= INT64_MIN && integer <= INT64_MAX
}

function isExactUint64(value: string): boolean {
  const integer = parseExactInteger(value)
  if (integer === null) return false
  return integer >= 0n && integer <= UINT64_MAX
}

interface WalkContext {
  input: string
  availableFields: FieldDefinition[]
  signal?: SearchSignal
  // parse mode throws on the first problem; validate mode collects them all
  errors: ValidationError[] | null
}

function fail(ctx: WalkContext, from: number, to: number, message: string) {
  if (ctx.errors) {
    ctx.errors.push({ from, to, message })
    return
  }
  throw new Error(message)
}

function text(ctx: WalkContext, node: SyntaxNode): string {
  return ctx.input.slice(node.from, node.to)
}

function namedChildren(node: SyntaxNode): SyntaxNode[] {
  const out: SyntaxNode[] = []
  for (let c = node.firstChild; c; c = c.nextSibling) out.push(c)
  return out
}

// Operators make a query structured. Parentheses alone remain searchable text.
type TreeSurvey = {
  structured: boolean
  firstError: { from: number; to: number } | null
}

function surveyTree(
  tree: ReturnType<typeof parser.parse>,
  predicateEnd = Number.POSITIVE_INFINITY
): TreeSurvey {
  let structured = false
  let firstError: { from: number; to: number } | null = null
  tree.iterate({
    enter(n) {
      if (n.from > predicateEnd) return false
      if (
        n.name === 'Comparison' ||
        n.name === 'GlobalText' ||
        n.name === 'AndExpression' ||
        n.name === 'OrExpression' ||
        n.name === 'Operator' ||
        n.name === 'KeywordOperator' ||
        n.name === 'AttributeReference' ||
        n.name === 'Between' ||
        n.name === 'Where'
      ) {
        structured = true
      }
      if (n.type.isError && !firstError) {
        firstError = { from: n.from, to: n.to }
      }
    },
  })
  return { structured, firstError }
}

function searchRequestNode(tree: ReturnType<typeof parser.parse>): SyntaxNode {
  return tree.topNode.getChild('SearchRequest') ?? tree.topNode
}

function findPredicateNode(request: SyntaxNode): SyntaxNode | null {
  return (
    namedChildren(request).find(
      child => child.name !== 'LimitClause' && child.name !== 'Where'
    ) ?? null
  )
}

function findLimitClauses(
  tree: ReturnType<typeof parser.parse>,
  input: string
): SyntaxNode[] {
  const clauses: SyntaxNode[] = []
  const structured = surveyTree(tree).structured
  tree.iterate({
    enter(node) {
      const keyword = node.node.getChild('Limit')
      if (
        node.name !== 'LimitClause' ||
        !keyword ||
        keyword.to === keyword.from
      )
        return
      const value = node.node.getChild('LimitValue')
      // "rate limit reached" is ordinary text. A numeric limit, a standalone
      // LIMIT clause or a clause after a structured predicate is a modifier.
      if (
        !structured &&
        keyword.from > 0 &&
        value &&
        !/^[+-]?\d/.test(input.slice(value.from, value.to))
      )
        return
      clauses.push(node.node)
    },
  })
  return clauses
}

function firstErrorFrom(
  tree: ReturnType<typeof parser.parse>,
  from: number
): { from: number; to: number } | null {
  let first: { from: number; to: number } | null = null
  tree.iterate({
    enter(node) {
      if (node.from < from) return
      if (node.type.isError && !first) {
        first = { from: node.from, to: node.to }
      }
    },
  })
  return first
}

function walkLimit(ctx: WalkContext, clause: SyntaxNode): number | null {
  const keyword = clause.getChild('Limit')
  const value = clause.getChild('LimitValue')
  if (!keyword || !value || !/^(?:LIMIT|limit)$/.test(text(ctx, keyword))) {
    fail(
      ctx,
      clause.from,
      Math.max(clause.to, clause.from + 1),
      'Result limits use the form LIMIT 100'
    )
    return null
  }

  const raw = text(ctx, value)
  if (!/^\d+$/.test(raw)) {
    fail(ctx, value.from, value.to, 'LIMIT must be a positive whole number')
    return null
  }

  const limit = Number(raw)
  if (!Number.isSafeInteger(limit) || limit < 1) {
    fail(ctx, value.from, value.to, 'LIMIT must be a positive whole number')
    return null
  }
  return limit
}

function walkExpression(ctx: WalkContext, node: SyntaxNode): QueryNode | null {
  switch (node.name) {
    case 'GlobalText': {
      const name = node.getChild('Word')
      const value = node.getChild('QuotedString')
      if (!name || text(ctx, name).toLowerCase() !== 'search' || !value) {
        fail(ctx, node.from, node.to, 'Global text uses search("text")')
        return null
      }
      return createGlobalTextSearch(unquote(text(ctx, value)))
    }
    case 'AndExpression':
    case 'OrExpression': {
      const operator = node.name === 'AndExpression' ? 'AND' : 'OR'
      const parts = namedChildren(node).filter(
        c => c.name !== 'And' && c.name !== 'Or'
      )
      const children: QueryNode[] = []
      for (const part of parts) {
        const child = walkExpression(ctx, part)
        if (child) children.push(child)
      }
      if (children.length < 2) return children[0] ?? null
      return {
        id: generateID(),
        type: 'group',
        group: { operator, children },
      }
    }
    case 'Group': {
      const inner = node.firstChild
      return inner ? walkExpression(ctx, inner) : null
    }
    case 'Comparison':
      return walkComparison(ctx, node)
    case 'FreeText':
      // A bare word inside a structured query is missing an operator.
      fail(
        ctx,
        node.from,
        node.to,
        `Unexpected "${text(ctx, node)}" — conditions take the form field operator value`
      )
      return null
    default:
      return null
  }
}

function walkComparison(ctx: WalkContext, node: SyntaxNode): QueryNode | null {
  const fieldNode = node.getChild('FieldName')
  if (fieldNode && node.getChild('Between')) {
    const bounds = namedChildren(node).filter(child =>
      ['Word', 'QuotedString', 'Null', 'Array'].includes(child.name)
    )
    if (bounds.length !== 2) {
      fail(ctx, node.from, node.to, 'BETWEEN requires lower AND upper bounds')
      return null
    }
    const kinds = bounds.map(bound => inferValueKind(ctx, bound))
    const kind = kinds.includes('double') ? 'double' : kinds[0]
    const lower = walkComparisonValue(
      ctx,
      fieldNode,
      node,
      bounds[0],
      '>=',
      kind
    )
    const upper = walkComparisonValue(
      ctx,
      fieldNode,
      node,
      bounds[1],
      '<=',
      kind
    )
    if (!lower || !upper) return null
    return {
      id: generateID(),
      type: 'group',
      group: { operator: 'AND', children: [lower, upper] },
    }
  }
  const opNode = node.getChild('Operator') ?? node.getChild('KeywordOperator')
  const valueNode =
    node.getChild('QuotedString') ??
    node.getChild('Array') ??
    node.getChild('Word') ??
    node.getChild('Null')

  if (!fieldNode || !opNode) {
    fail(ctx, node.from, node.to, 'Incomplete expression')
    return null
  }
  return walkComparisonValue(ctx, fieldNode, opNode, valueNode)
}

function inferValueKind(
  ctx: WalkContext,
  valueNode: SyntaxNode | null
): string {
  const firstListValue =
    valueNode?.name === 'Array' ? valueNode.firstChild : valueNode
  const inferredText = firstListValue ? text(ctx, firstListValue) : ''
  return firstListValue?.name === 'QuotedString'
    ? 'string'
    : /^(true|false)$/i.test(inferredText)
      ? 'bool'
      : /^[+-]?\d+$/.test(inferredText)
        ? 'int64'
        : /^[+-]?(?:\d+\.?\d*|\.\d+)(?:e[+-]?\d+)?$/i.test(inferredText)
          ? 'double'
          : 'string'
}

function walkComparisonValue(
  ctx: WalkContext,
  fieldNode: SyntaxNode,
  opNode: SyntaxNode,
  valueNode: SyntaxNode | null,
  operatorSymbol?: string,
  inferredKind = inferValueKind(ctx, valueNode)
): QueryNode | null {
  const fieldName = text(ctx, fieldNode)
  const field = resolveField(
    fieldName,
    ctx.availableFields,
    ctx.signal,
    inferredKind
  )
  if (!field) {
    fail(
      ctx,
      fieldNode.from,
      fieldNode.to,
      fieldResolutionIsAmbiguous(fieldName, ctx.availableFields)
        ? `Ambiguous field: ${fieldName}. Choose a suggestion to select its scope and type.`
        : `Unknown field: ${fieldName}`
    )
  }

  let symbol: string
  if (operatorSymbol) symbol = operatorSymbol
  else if (opNode.name === 'Operator') {
    const raw = text(ctx, opNode)
    symbol = SIGIL_ALIASES.get(raw) ?? raw
  } else {
    const kw = opNode.firstChild
    symbol = kw ? (KEYWORD_SYMBOLS.get(kw.name) ?? '') : ''
  }

  if (!valueNode) {
    fail(ctx, opNode.from, opNode.to, 'Expected value')
    return null
  }

  let value = ''
  let listItems: string[] | null = null
  let listItemNodes: SyntaxNode[] = []
  if (valueNode.name === 'Null') {
    if (symbol === 'IS') symbol = 'IS NULL'
    else if (symbol === 'IS NOT') symbol = 'IS NOT NULL'
    else {
      fail(
        ctx,
        valueNode.from,
        valueNode.to,
        `Operator '${symbol}' cannot be used with NULL`
      )
      return null
    }
    value = ''
  } else if (symbol === 'IS' || symbol === 'IS NOT') {
    fail(ctx, valueNode.from, valueNode.to, 'IS and IS NOT require NULL')
    return null
  } else if (valueNode.name === 'QuotedString') {
    value = unquote(text(ctx, valueNode))
  } else if (valueNode.name === 'Array') {
    const items: string[] = []
    for (const item of namedChildren(valueNode)) {
      if (item.name === 'Null') {
        fail(ctx, item.from, item.to, 'NULL is not allowed inside an array')
        return null
      }
      if (item.name === 'Array') {
        // The wire format accepts only flat lists.
        fail(ctx, item.from, item.to, 'Arrays cannot be nested')
        return null
      }
      items.push(
        item.name === 'QuotedString'
          ? unquote(text(ctx, item))
          : text(ctx, item)
      )
      listItemNodes.push(item)
    }
    if ((symbol === 'IN' || symbol === 'NOT IN') && items.length === 0) {
      fail(
        ctx,
        valueNode.from,
        valueNode.to,
        'IN and NOT IN require a nonempty list'
      )
      return null
    }
    listItems = items
  } else {
    value = text(ctx, valueNode)
  }

  if ((symbol === 'IN' || symbol === 'NOT IN') && valueNode.name !== 'Array') {
    fail(ctx, valueNode.from, valueNode.to, 'IN and NOT IN require a list')
    return null
  }
  if (listItems && symbol !== 'IN' && symbol !== 'NOT IN') {
    fail(ctx, valueNode.from, valueNode.to, 'Lists require IN or NOT IN')
    return null
  }

  const operator = findOperator(symbol)
  if (!operator) {
    fail(ctx, opNode.from, opNode.to, `Unknown operator: ${text(ctx, opNode)}`)
    return null
  }

  if (field) {
    const required = COMPAT_ALIASES.get(symbol) ?? symbol
    const operatorIsValid = field.operators.some(op => op.symbol === required)
    if (!operatorIsValid) {
      fail(
        ctx,
        opNode.from,
        opNode.to,
        `Operator '${symbol}' is not valid for field '${field.name}'`
      )
    }

    if (
      !usesFieldToken(field) &&
      (symbol === 'IN' || symbol === 'NOT IN') &&
      (SIGNED_INTEGER_FIELDS.has(field.name.toLowerCase()) ||
        TIMESTAMP_FIELDS.has(field.name.toLowerCase())) &&
      valueNode.name === 'Array'
    ) {
      const timestamp = TIMESTAMP_FIELDS.has(field.name.toLowerCase())
      const invalid = listItems?.find(item =>
        timestamp ? !isExactUint64(item) : !isExactInt64(item)
      )
      if (invalid !== undefined) {
        fail(
          ctx,
          valueNode.from,
          valueNode.to,
          `Integer list value '${invalid}' must be an exact ${timestamp ? 'unsigned' : 'signed'} 64-bit integer`
        )
      }
    }

    if (
      !usesFieldToken(field) &&
      operatorIsValid &&
      valueNode.name !== 'Null' &&
      !listItems &&
      (SIGNED_INTEGER_FIELDS.has(field.name.toLowerCase()) ||
        TIMESTAMP_FIELDS.has(field.name.toLowerCase()))
    ) {
      const timestamp = TIMESTAMP_FIELDS.has(field.name.toLowerCase())
      const integer = parseExactInteger(value)
      if (
        integer === null ||
        (timestamp
          ? integer < 0n || integer > UINT64_MAX
          : integer < INT64_MIN || integer > INT64_MAX)
      ) {
        fail(
          ctx,
          valueNode.from,
          valueNode.to,
          `Integer value '${value}' must be an exact ${timestamp ? 'unsigned' : 'signed'} 64-bit integer`
        )
        return null
      }
      value = integer.toString()
    }

    if (
      !usesFieldToken(field) &&
      field.name === 'duration' &&
      operatorIsValid &&
      valueNode.name !== 'Null'
    ) {
      if (symbol === 'IN' || symbol === 'NOT IN') {
        if (!listItems) return null
        const result = normalizeDurationList(listItems)
        if (!result.ok) {
          const errorNode =
            result.index === undefined
              ? valueNode
              : (listItemNodes[result.index] ?? valueNode)
          fail(ctx, errorNode.from, errorNode.to, result.error)
          return null
        }
        listItems = result.value
      } else {
        if (listItems) {
          fail(
            ctx,
            valueNode.from,
            valueNode.to,
            `Invalid duration: "${text(ctx, valueNode)}". Try "1s", "500ms", "2m", etc.`
          )
          return null
        }
        const result = normalizeDuration(value)
        if (!result.ok) {
          fail(ctx, valueNode.from, valueNode.to, result.error)
          return null
        }
        value = result.value
      }
    }
  }

  if (!field) return null

  if (listItems) {
    // JSON preserves commas inside quoted list values.
    value = JSON.stringify(listItems)
  }

  return {
    id: generateID(),
    type: 'condition',
    query: { field, operator, value },
  }
}

function createGlobalTextSearch(input: string): QueryNode {
  return {
    id: generateID(),
    type: 'text',
    query: {
      operator: OPERATORS.CONTAINS,
      value: input,
    },
  }
}

// Parse the complete search language. Result controls are siblings of the
// predicate rather than fake boolean nodes, so adding sorting or pagination
// later does not change what QueryNode means.
export function parseSearchRequest(
  input: string,
  availableFields: FieldDefinition[],
  signal?: SearchSignal
): ParsedSearchRequest | null {
  if (!input.trim()) return null

  const tree = parser.parse(input)
  const request = searchRequestNode(tree)
  const limitClauses = findLimitClauses(tree, input)
  if (limitClauses.length > 1) {
    throw new Error('Only one LIMIT modifier is allowed')
  }

  const limitClause = limitClauses[0] ?? null
  const predicateEnd = limitClause?.from ?? input.length
  const predicateText = input.slice(0, predicateEnd).trim()
  const { structured, firstError } = surveyTree(tree, predicateEnd)

  const ctx: WalkContext = { input, availableFields, signal, errors: null }
  let predicate: QueryNode | null = null

  if (predicateText) {
    if (!structured) {
      predicate = createGlobalTextSearch(predicateText)
    } else {
      if (firstError) {
        throw new Error(syntaxErrorMessage(input, firstError))
      }
      const predicateNode = findPredicateNode(request)
      predicate = predicateNode ? walkExpression(ctx, predicateNode) : null
    }
  }

  let limit: number | null = null
  if (limitClause) {
    limit = walkLimit(ctx, limitClause)
    const trailingError = firstErrorFrom(tree, limitClause.to)
    if (trailingError) {
      throw new Error('LIMIT must be the final search modifier')
    }
  }

  return { predicate, limit }
}

// The predicate-only API rejects result modifiers rather than dropping them.
export function parseQuery(
  input: string,
  availableFields: FieldDefinition[],
  signal?: SearchSignal
): QueryNode | null {
  const request = parseSearchRequest(input, availableFields, signal)
  if (request?.limit !== null && request?.limit !== undefined) {
    throw new Error('LIMIT is not available for this search yet')
  }
  return request?.predicate ?? null
}

// Lightweight validator: same tree, same walk, but collects every problem
// with its position instead of throwing on the first.
export function validateQuery(
  input: string,
  availableFields: FieldDefinition[],
  signal?: SearchSignal
): ValidationError[] {
  if (!input.trim()) return []

  const tree = parser.parse(input)
  const request = searchRequestNode(tree)
  const limitClauses = findLimitClauses(tree, input)
  const limitClause = limitClauses[0] ?? null
  const predicateEnd = limitClause?.from ?? input.length
  const predicateText = input.slice(0, predicateEnd).trim()
  const { structured, firstError } = surveyTree(tree, predicateEnd)

  if (!structured && !limitClause) return []

  const errors: ValidationError[] = []
  if (structured && firstError) {
    errors.push({
      from: firstError.from,
      to: Math.max(firstError.to, firstError.from + 1),
      message: syntaxErrorMessage(input, firstError),
    })
  }

  const ctx: WalkContext = { input, availableFields, signal, errors }
  if (structured && predicateText) {
    const predicateNode = findPredicateNode(request)
    if (predicateNode) walkExpression(ctx, predicateNode)
  }

  if (limitClauses.length > 1) {
    const repeated = limitClauses[1]
    errors.push({
      from: repeated.from,
      to: repeated.to,
      message: 'Only one LIMIT modifier is allowed',
    })
  }
  if (limitClause) {
    walkLimit(ctx, limitClause)
    const trailingError = firstErrorFrom(tree, limitClause.to)
    if (trailingError) {
      errors.push({
        from: trailingError.from,
        to: Math.max(trailingError.to, trailingError.from + 1),
        message: 'LIMIT must be the final search modifier',
      })
    }
  }
  return errors
}
