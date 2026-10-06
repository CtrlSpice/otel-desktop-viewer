import {
  type CompletionContext,
  type CompletionResult,
  type Completion,
} from '@codemirror/autocomplete'
import { syntaxTree } from '@codemirror/language'
import type { SyntaxNode } from '@lezer/common'
import {
  OPERATORS,
  type FieldDefinition,
  type SearchSignal,
} from '@/search/model'
import {
  Array as ArrayTerm,
  FieldName as FieldTerm,
  KeywordOperator,
  Null,
  Operator as OperatorTerm,
  QuotedString,
  Word as ValueTerm,
} from './query.parser.terms'
import { parser } from './query.parser'
import { resolveField } from '../field-resolution'
import {
  attributeFieldIdentity,
  formatAttributeFieldReference,
  parseAttributeFieldReference,
  storedKindForField,
  type AttributeField,
} from '../attribute-field-reference'

const LOGICAL_COMPLETIONS: Completion[] = [
  {
    label: 'AND',
    type: 'keyword',
  },
  {
    label: 'OR',
    type: 'keyword',
  },
]

const RESULT_LIMIT_COMPLETION: Completion = {
  label: '| LIMIT',
  type: 'keyword',
  apply: '| LIMIT ',
}

function unclosedGroupDepth(text: string): number {
  let depth = 0
  let quote: '"' | "'" | null = null
  for (let i = 0; i < text.length; i++) {
    const char = text[i]
    if (quote) {
      if (char === '\\') i++
      else if (char === quote) quote = null
      continue
    }
    if (char === '"' || char === "'") quote = char
    else if (char === '(') depth++
    else if (char === ')') depth = Math.max(0, depth - 1)
  }
  return depth
}

/**
 * Whether the text is a complete, error-free structured expression -- the
 * state after which boolean continuations or a result limit are accepted.
 *
 * Unclosed groups are balanced before parsing, so a condition typed inside
 * parentheses still counts as complete: `(a = 1` continues with AND/OR just
 * as `a = 1` does, the closer simply hasn't been typed yet.
 */
function expressionIsComplete(text: string): boolean {
  let trimmed = text.trim()
  if (!trimmed || trimmed.endsWith('(')) return false
  const depth = unclosedGroupDepth(trimmed)
  if (depth > 0) trimmed += ')'.repeat(depth)
  let structured = false
  let hasError = false
  parser.parse(trimmed).iterate({
    enter(n) {
      if (
        n.name === 'Comparison' ||
        n.name === 'Group' ||
        n.name === 'AndExpression' ||
        n.name === 'OrExpression'
      ) {
        structured = true
      }
      if (n.type.isError) hasError = true
    },
  })
  return structured && !hasError
}

function findAncestor(node: SyntaxNode, name: string): SyntaxNode | null {
  let n: SyntaxNode | null = node
  while (n) {
    if (n.name === name) return n
    n = n.parent
  }
  return null
}

function getValueNode(comparison: SyntaxNode): SyntaxNode | null {
  return (
    comparison.getChild(ValueTerm) ??
    comparison.getChild(QuotedString) ??
    comparison.getChild(ArrayTerm) ??
    comparison.getChild(Null)
  )
}

function getOperatorNode(comparison: SyntaxNode): SyntaxNode | null {
  return (
    comparison.getChild(OperatorTerm) ?? comparison.getChild(KeywordOperator)
  )
}

/** Heuristic: raw hex that looks like a trace or span id → suggest field = value conditions. */
function idPatternCompletions(
  context: CompletionContext
): CompletionResult | null {
  const word = context.matchBefore(/[a-fA-F0-9]+/)
  if (!word || word.from === word.to) return null
  const hex = word.text
  if (hex.length !== 16 && hex.length !== 32) return null
  if (!/^[a-fA-F0-9]+$/.test(hex)) return null

  const fields =
    hex.length === 32 ? ['traceID', 'link.traceID'] : ['spanID', 'link.spanID']

  const options: Completion[] = fields.map(f => ({
    label: `${f} = ${hex}`,
    type: 'text',
    apply: `${f} = ${hex}`,
  }))

  return {
    from: word.from,
    to: word.to,
    options,
    // Whole-condition options must not be filtered against the bare hex.
    filter: false,
  }
}

export function createQueryCompletionSource(
  getFields: () => FieldDefinition[],
  signal?: SearchSignal
) {
  return function queryCompletionSource(
    context: CompletionContext
  ): CompletionResult | null {
    const tree = syntaxTree(context.state)
    const node = tree.resolveInner(context.pos, -1)

    // Only offer id-shape completions at the top level (not inside a Comparison).
    const comparison = findAncestor(node, 'Comparison')

    if (!comparison) {
      const idHit = idPatternCompletions(context)
      if (idHit) return idHit

      // A completed top-level field remains FreeText until an operator is typed.
      const opHit = topLevelOperatorCompletions(context, getFields(), signal)
      if (opHit) return opHit
    }

    if (comparison) {
      const field = comparison.getChild(FieldTerm)
      const opNode = getOperatorNode(comparison)
      const valueNode = getValueNode(comparison)
      const pos = context.pos

      // Still in or at end of field name → complete field names, not operators.
      if (field && pos <= field.to) {
        // Replace the full field token when accepting mid-name.
        return fieldCompletions(
          context,
          getFields(),
          signal,
          field.from,
          field.to
        )
      }

      // After field: whitespace before operator → operators (not another field).
      if (field && pos > field.to) {
        const between = context.state.sliceDoc(field.to, pos)
        if (/^\s*$/.test(between)) {
          if (!opNode || pos < opNode.from) {
            return operatorCompletions(
              context,
              context.state.sliceDoc(field.from, field.to),
              getFields(),
              signal
            )
          }
        }
      }

      if (
        (node.name === 'Operator' || node.name === 'KeywordOperator') &&
        findAncestor(node, 'Comparison') === comparison
      ) {
        const fieldNode = comparison.getChild(FieldTerm)
        if (fieldNode) {
          const fieldText = context.state.sliceDoc(fieldNode.from, fieldNode.to)
          // Cursor still touching a symbol operator: it may be mid-typing --
          // `>` on the way to `>=` -- so offer the operators anchored at the
          // symbol's start. Past the operator, the value position begins.
          if (node.name === 'Operator' && pos <= node.to) {
            return operatorCompletions(
              context,
              fieldText,
              getFields(),
              signal,
              node.from
            )
          }
          return valueCompletions(context, fieldText, getFields(), signal)
        }
      }

      // Cursor in value position (inside or at end of value token)
      if (valueNode && pos >= valueNode.from && pos <= valueNode.to) {
        const fieldNode = comparison.getChild(FieldTerm)
        if (fieldNode) {
          const fieldText = context.state.sliceDoc(fieldNode.from, fieldNode.to)
          // For a scalar the value node is the token itself. For an Array,
          // valueNode.from is the opening bracket -- anchoring there made
          // accepting a suggestion replace "[Ok, E" with a single bare
          // value, destroying the array. Anchor at the item being typed.
          let from = valueNode.from
          if (valueNode.name === 'Array') {
            const item = context.matchBefore(/[\w.]*/)
            from = item && item.from < pos ? item.from : pos
            // A quote immediately before the anchor is one of two states,
            // told apart by counting quotes since the bracket: an odd count
            // means the quote OPENED the item being typed -- `["O|` -- so
            // accepting must close it; an even count means the quote CLOSED
            // the previous item -- `["Ok"|` -- where the only valid next
            // characters are a comma or the bracket, so nothing is offered.
            const prev = context.state.sliceDoc(Math.max(0, from - 1), from)
            if (prev === '"' || prev === "'") {
              const sinceBracket = context.state.sliceDoc(valueNode.from, from)
              const quotes = (sinceBracket.match(/["']/g) ?? []).length
              if (quotes % 2 === 0) return null
              const r = valueCompletions(
                context,
                fieldText,
                getFields(),
                signal,
                from
              )
              if (!r) return null
              return {
                ...r,
                options: r.options.map(o => ({ ...o, apply: o.label + prev })),
              }
            }
          }
          return valueCompletions(context, fieldText, getFields(), signal, from)
        }
      }
    }

    // A finished expression continues with AND, OR, or a result limit.
    {
      const partial = context.matchBefore(/[A-Za-z]*/)
      const before = context.state.sliceDoc(
        0,
        partial ? partial.from : context.pos
      )
      if (expressionIsComplete(before)) {
        const unclosedGroups = unclosedGroupDepth(before)
        return {
          from: partial?.from ?? context.pos,
          options:
            unclosedGroups > 0
              ? LOGICAL_COMPLETIONS
              : [...LOGICAL_COMPLETIONS, RESULT_LIMIT_COMPLETION],
          validFor: /^(?:(?:AND|OR)|(?:\|\s*(?:LIMIT)?))?$/i,
        }
      }
    }

    // After logical op: fields.
    if (node.name === 'And' || node.name === 'Or') {
      return fieldCompletions(context, getFields(), signal)
    }

    if (node.name === 'Query' || node.name === 'SearchRequest') {
      return fieldCompletions(context, getFields(), signal)
    }

    if (node.name === 'Group' && context.pos < node.to) {
      return fieldCompletions(context, getFields(), signal)
    }

    const parentNode = node.parent

    if (
      (node.name === 'FieldName' ||
        (node.name === 'Word' && parentNode?.name === 'FieldName')) &&
      context.pos > node.to
    ) {
      const fieldText = context.state.sliceDoc(node.from, node.to)
      return operatorCompletions(context, fieldText, getFields(), signal)
    }

    // Typing an operator after a bare field name -- `name C` on the way to
    // CONTAINS, or `duration >` on the way to >=. The text before the
    // partial must parse to exactly one free-standing word: that is a field
    // name awaiting its operator. Symbol prefixes are matched as a separate
    // character class since they are not word characters.
    const opPartial = context.matchBefore(/[A-Za-z]+|[=!<>~^$]+/)
    if (opPartial && opPartial.from > 0) {
      const before = context.state.sliceDoc(0, opPartial.from).trim()
      if (before !== '') {
        const t = parser.parse(before).topNode
        const request = t.getChild('SearchRequest')
        const only = request?.firstChild ?? t.firstChild
        if (
          only &&
          only.name === 'FreeText' &&
          !only.nextSibling &&
          only.from === 0 &&
          only.to === before.length
        ) {
          return operatorCompletions(
            context,
            before,
            getFields(),
            signal,
            opPartial.from
          )
        }
      }
    }

    // A new condition starts at input start, an open group, or a logical operator.
    const word = context.matchBefore(/[\w.]+/)
    if (word) {
      const before = context.state.sliceDoc(0, word.from).trim()
      if (
        before === '' ||
        before.endsWith('(') ||
        /(?:^|[\s(])(?:AND|OR)$/i.test(before)
      ) {
        return fieldCompletions(context, getFields(), signal, word.from)
      }
    }

    // After AND/OR whitespace, the cursor resolves to the parent expression.
    if (!word) {
      // Trailing open groups still begin a condition.
      const before = context.state
        .sliceDoc(0, context.pos)
        .replace(/[(\s]+$/, '')
      if (/(?:^|[\s(])(?:AND|OR)$/i.test(before)) {
        return fieldCompletions(context, getFields(), signal)
      }
    }

    if (context.explicit) {
      return fieldCompletions(context, getFields(), signal)
    }

    return null
  }
}

/**
 * Whether text ends inside a quoted string that has not been closed.
 *
 * Escape-aware, unlike a bare quote count: generated queries contain \"
 * inside values, and counting those as delimiters would flip the answer.
 */
function inOpenString(text: string): boolean {
  let open: '"' | "'" | null = null
  for (let i = 0; i < text.length; i++) {
    const ch = text[i]
    if (open) {
      if (ch === '\\') i++
      else if (ch === open) open = null
    } else if (ch === '"' || ch === "'") {
      open = ch
    }
  }
  return open !== null
}

/**
 * Operators for a complete field name typed outside a Comparison.
 *
 * The trailing space is what marks the name as finished: while the cursor
 * still touches the word the user may be extending it, and field names stay
 * the useful suggestion.
 */
function topLevelOperatorCompletions(
  context: CompletionContext,
  fields: FieldDefinition[],
  signal?: SearchSignal
): CompletionResult | null {
  // Error recovery can place the cursor outside an unterminated string's comparison.
  if (inOpenString(context.state.sliceDoc(0, context.pos))) return null

  const source = context.state.sliceDoc(0, context.pos)
  if (/\s$/.test(source)) {
    const explicit = parseAttributeFieldReference(source.trim(), signal)
    if (explicit) {
      return operatorCompletions(context, source.trim(), fields, signal)
    }
  }

  const before = context.matchBefore(/[\w.]+\s+/)
  if (!before) return null
  const word = before.text.trim()
  const known = resolveField(word, fields, signal) !== undefined
  if (!known) return null
  return operatorCompletions(context, word, fields, signal, context.pos)
}

function compareText(left: string, right: string): number {
  return left < right ? -1 : left > right ? 1 : 0
}

function compareAttributeFields(left: AttributeField, right: AttributeField) {
  return (
    compareText(left.attributeScope, right.attributeScope) ||
    compareText(left.name, right.name) ||
    compareText(storedKindForField(left), storedKindForField(right))
  )
}

function fieldCompletions(
  context: CompletionContext,
  fields: FieldDefinition[],
  signal?: SearchSignal,
  from?: number,
  to?: number
): CompletionResult | null {
  const options: Completion[] = []
  const nativeFields = fields.filter(
    (field): field is Extract<FieldDefinition, { searchScope: 'field' }> =>
      field.searchScope === 'field'
  )
  const attributes = new Map<string, AttributeField>()
  for (const field of fields) {
    if (field.searchScope === 'attribute') {
      attributes.set(attributeFieldIdentity(field), field)
    }
  }

  for (const field of [
    ...nativeFields,
    ...[...attributes.values()].sort(compareAttributeFields),
  ]) {
    const attribute = field.searchScope === 'attribute' ? field : null
    options.push({
      label: field.name,
      type: 'property',
      detail: attribute
        ? `${attribute.attributeScope} · ${storedKindForField(attribute)}`
        : field.type,
      info: 'description' in field ? field.description : undefined,
      boost: field.searchScope === 'field' ? 1 : 0,
      section: attribute
        ? `${attribute.attributeScope} / ${attribute.name}`
        : 'Fields',
      // The trailing space opens operator completion.
      apply: `${attribute ? formatAttributeFieldReference(attribute) : field.name} `,
    })
  }

  if (options.length === 0) return null

  const result: CompletionResult = {
    from: from ?? context.pos,
    options,
    validFor: /^[\w.]*$/,
  }
  if (to !== undefined) result.to = to
  return result
}

function operatorCompletions(
  context: CompletionContext,
  fieldName: string,
  fields: FieldDefinition[],
  signal?: SearchSignal,
  from?: number
): CompletionResult | null {
  const field = resolveField(fieldName, fields, signal)

  // The derived operators are wire spellings, not query syntax: the null
  // check is typed `= NULL` and negated regex is typed `!~`, so offering
  // "IS NULL" or "NOT REGEXP" here would complete into text the grammar
  // cannot parse.
  const derived = new Set(['IS NULL', 'IS NOT NULL', 'NOT REGEXP'])
  const ops = (field ? field.operators : Object.values(OPERATORS)).filter(
    op => !derived.has(op.symbol)
  )

  const options: Completion[] = ops.map(op => ({
    label: op.symbol,
    type: 'operator',
    detail: op.label,
  }))

  return {
    from: from ?? context.pos,
    options,
    // Keep filtering while an operator is typed.
    validFor: /^[\w=!<>~^$]*$/,
  }
}

function valueCompletions(
  context: CompletionContext,
  fieldName: string,
  fields: FieldDefinition[],
  signal?: SearchSignal,
  from?: number
): CompletionResult | null {
  const field = resolveField(fieldName, fields, signal)
  if (!field) return null

  const knownValues =
    'enumValues' in field && field.enumValues && field.enumValues.length > 0
      ? field.enumValues
      : null
  if (!knownValues) return null

  const options: Completion[] = [...knownValues].map(v => ({
    label: v,
    type: 'enum',
  }))

  return {
    from: from ?? context.pos,
    options,
    validFor: /^[\w]*$/,
  }
}
