import { type FieldDefinition } from '@/constants/fields'
import { OPERATORS, type Operator } from '@/constants/operators'
import {
  type ParsedSearchRequest,
  type QueryNode,
  generateID,
} from './queryTree'
import { parser } from './codemirror/query.parser'
import type { SyntaxNode } from '@lezer/common'

// One grammar, one parse.
//
// This file used to hold a second, hand-written lexer and parser for the
// query language, alongside the Lezer grammar the editor highlights with.
// Two implementations of one language drifted exactly the way two
// implementations drift: the editor accepted `=~` the parser rejected,
// the parser accepted backticks the editor underlined, a NOT typo the
// editor flagged was silently submitted as free text. Now the Lezer tree
// -- the same one CodeMirror builds incrementally on every keystroke --
// is the only syntax authority, and this file only walks it: syntax
// errors are error nodes in the tree, semantics (field names, operator
// compatibility) are checked against the same definitions the
// completions use.

// Validation error with position info for the linter
export interface ValidationError {
  from: number
  to: number
  message: string
}

// Shared by the parser and the validator so the editor's underline and the
// error a submitted query produces say the same thing. Names the fix rather
// than just the symptom: the overwhelmingly common cause is an unquoted value
// containing a space.
function unexpectedTokenMessage(value: string): string {
  if (!value.trim()) return 'Incomplete expression'
  return (
    `Unexpected "${value}". Values containing spaces must be quoted, ` +
    `for example: service.name = "Red Bull Racing"`
  )
}

// Unescape a QuotedString token's text: strip the quotes, resolve the
// escapes the language supports. Anything else after a backslash is kept
// as itself, so "\d" survives into regex values.
function unquote(text: string): string {
  const quote = text[0]
  const body = text.slice(1, text.endsWith(quote) ? -1 : undefined)
  let out = ''
  for (let i = 0; i < body.length; i++) {
    const c = body[i]
    if (c !== '\\') {
      out += c
      continue
    }
    const next = body[++i]
    if (next === undefined) break
    if (next === 'n') out += '\n'
    else if (next === 't') out += '\t'
    else if (next === 'r') out += '\r'
    else out += next
  }
  return out
}

type NamedField = Exclude<FieldDefinition, { searchScope: 'global' }>

function findField(
  name: string,
  availableFields: FieldDefinition[]
): NamedField | undefined {
  return availableFields.find(
    (f): f is NamedField =>
      f.searchScope !== 'global' && f.name.toLowerCase() === name.toLowerCase()
  )
}

function findOperator(symbol: string): Operator | undefined {
  for (const op of Object.values(OPERATORS)) {
    if (op.symbol === symbol) return op
  }
  return undefined
}

// The symbol an operator node resolves to. Symbol operators mostly map to
// themselves; the regex sigils are spelled the PromQL way in query text and
// the SQL way on the wire.
const SIGIL_ALIASES: Record<string, string> = {
  '=~': 'REGEXP',
  '!~': 'NOT REGEXP',
}

const KEYWORD_SYMBOLS: Record<string, string> = {
  Contains: 'CONTAINS',
  Regexp: 'REGEXP',
  In: 'IN',
  NotContains: 'NOT CONTAINS',
  NotIn: 'NOT IN',
}

// Which operator a field must allow for a derived operator to be legal.
// `field = NULL` has always been legal wherever `=` is, so IS NULL rides on
// =; the negations ride on what they negate.
const COMPAT_ALIASES: Record<string, string> = {
  'IS NULL': '=',
  'IS NOT NULL': '!=',
  'NOT REGEXP': 'REGEXP',
}

interface WalkContext {
  input: string
  availableFields: FieldDefinition[]
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

// A structured query is one that uses the language's *operators*:
// comparisons, keyword operators, AND/OR. Anything else -- however mangled
// -- is free text, searched globally. An erroneous *structured* query is an
// error, never free text: `duration NOT 5` used to be submitted as a global
// search for the literal string "duration NOT 5", underlined red in the
// editor and silently wrong on the wire.
//
// A Group node deliberately does NOT count. Parentheses are ordinary
// characters in log bodies and error text -- "(error)", "(500) internal
// error" -- and the grammar eagerly parses a leading paren as a Group, so
// counting Groups turned every parenthetical remark into a hard parse
// error. The old lexer's heuristic was "has an operator or logical token",
// and this is that rule expressed over the tree.
function surveyTree(
  tree: ReturnType<typeof parser.parse>,
  predicateEnd = Number.POSITIVE_INFINITY
): {
  structured: boolean
  firstError: { from: number; to: number } | null
} {
  let structured = false
  let firstError: { from: number; to: number } | null = null
  tree.iterate({
    enter(n) {
      if (n.from >= predicateEnd) return false
      if (
        n.name === 'Comparison' ||
        n.name === 'AndExpression' ||
        n.name === 'OrExpression' ||
        n.name === 'Operator' ||
        n.name === 'KeywordOperator'
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
    namedChildren(request).find(child => child.name !== 'LimitClause') ?? null
  )
}

function findLimitClauses(tree: ReturnType<typeof parser.parse>): SyntaxNode[] {
  const clauses: SyntaxNode[] = []
  tree.iterate({
    enter(node) {
      if (node.name === 'LimitClause') clauses.push(node.node)
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
      'Result limits use the form | LIMIT 100'
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
      // Reached only when the query is structured elsewhere -- a bare word
      // sitting inside AND/OR or a group, like `x = 1 AND foo`. The quoting
      // hint would be wrong here; the word is not a value missing its
      // quotes, it is a condition missing its operator.
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

  const fieldName = text(ctx, fieldNode)
  const field = findField(fieldName, ctx.availableFields)
  if (!field) {
    fail(ctx, fieldNode.from, fieldNode.to, `Unknown field: ${fieldName}`)
  }

  let symbol: string
  if (opNode.name === 'Operator') {
    const raw = text(ctx, opNode)
    symbol = SIGIL_ALIASES[raw] ?? raw
  } else {
    const kw = opNode.firstChild
    symbol = kw ? (KEYWORD_SYMBOLS[kw.name] ?? '') : ''
  }

  if (!valueNode) {
    fail(ctx, opNode.from, opNode.to, 'Expected value')
    return null
  }

  let value: string
  if (valueNode.name === 'Null') {
    // A bare NULL/NIL keyword is the null check; a quoted "NULL" stays the
    // literal string, which the old parser could not distinguish.
    if (symbol === '=') symbol = 'IS NULL'
    else if (symbol === '!=') symbol = 'IS NOT NULL'
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
        // The grammar recurses, so [[a],b] parses cleanly -- but the wire
        // format is a flat list, and the old parser rejected nesting too.
        // Without this the nested array's raw source text, brackets and
        // all, would travel as one string element.
        fail(ctx, item.from, item.to, 'Arrays cannot be nested')
        return null
      }
      items.push(
        item.name === 'QuotedString'
          ? unquote(text(ctx, item))
          : text(ctx, item)
      )
    }
    // JSON, not a comma-join: a quoted value may itself contain commas,
    // which the old "[a,b,c]" serialization corrupted on the way through
    // the backend's comma split.
    value = JSON.stringify(items)
  } else {
    value = text(ctx, valueNode)
  }

  const operator = findOperator(symbol)
  if (!operator) {
    fail(ctx, opNode.from, opNode.to, `Unknown operator: ${text(ctx, opNode)}`)
    return null
  }

  if (field) {
    const required = COMPAT_ALIASES[symbol] ?? symbol
    if (!field.operators.some(op => op.symbol === required)) {
      fail(
        ctx,
        opNode.from,
        opNode.to,
        `Operator '${symbol}' is not valid for field '${field.name}'`
      )
    }
  }

  if (!field) return null

  return {
    id: generateID(),
    type: 'condition',
    query: { field, operator, value },
  }
}

// Create Global Text Search Query
function createGlobalTextSearch(input: string): QueryNode {
  return {
    id: generateID(),
    type: 'condition',
    query: {
      field: { searchScope: 'global' },
      operator: OPERATORS.CONTAINS,
      value: input.trim(),
    },
  }
}

// Parse the complete search language. Result controls are siblings of the
// predicate rather than fake boolean nodes, so adding sorting or pagination
// later does not change what QueryNode means.
export function parseSearchRequest(
  input: string,
  availableFields: FieldDefinition[]
): ParsedSearchRequest | null {
  if (!input.trim()) return null

  const tree = parser.parse(input)
  const request = searchRequestNode(tree)
  const limitClauses = findLimitClauses(tree)
  if (limitClauses.length > 1) {
    throw new Error('Only one LIMIT modifier is allowed')
  }

  const limitClause = limitClauses[0] ?? null
  const predicateEnd = limitClause?.from ?? input.length
  const predicateText = input.slice(0, predicateEnd).trim()
  const { structured, firstError } = surveyTree(tree, predicateEnd)

  const ctx: WalkContext = { input, availableFields, errors: null }
  let predicate: QueryNode | null = null

  if (predicateText) {
    if (!structured) {
      predicate = createGlobalTextSearch(predicateText)
    } else {
      if (firstError) {
        throw new Error(
          unexpectedTokenMessage(input.slice(firstError.from, firstError.to))
        )
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

// Existing callers still ask for a predicate alone. Refuse a result modifier
// rather than silently dropping it; SearchEditor moves to parseSearchRequest
// when the transport slice starts carrying limits.
export function parseQuery(
  input: string,
  availableFields: FieldDefinition[]
): QueryNode | null {
  const request = parseSearchRequest(input, availableFields)
  if (request?.limit !== null && request?.limit !== undefined) {
    throw new Error('LIMIT is not available for this search yet')
  }
  return request?.predicate ?? null
}

// Lightweight validator: same tree, same walk, but collects every problem
// with its position instead of throwing on the first.
export function validateQuery(
  input: string,
  availableFields: FieldDefinition[]
): ValidationError[] {
  if (!input.trim()) return []

  const tree = parser.parse(input)
  const request = searchRequestNode(tree)
  const limitClauses = findLimitClauses(tree)
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
      message: unexpectedTokenMessage(
        input.slice(firstError.from, firstError.to)
      ),
    })
  }

  const ctx: WalkContext = { input, availableFields, errors }
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
