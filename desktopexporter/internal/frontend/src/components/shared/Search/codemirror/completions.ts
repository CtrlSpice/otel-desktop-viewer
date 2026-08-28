import {
  type CompletionContext,
  type CompletionResult,
  type Completion,
} from '@codemirror/autocomplete'
import { syntaxTree } from '@codemirror/language'
import type { SyntaxNode } from '@lezer/common'
import type { FieldDefinition } from '@/constants/fields'
import { OPERATORS } from '@/constants/operators'
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

/**
 * Whether the text is a complete, error-free structured expression -- the
 * state after which AND and OR are the only tokens the grammar accepts.
 *
 * Unclosed groups are balanced before parsing, so a condition typed inside
 * parentheses still counts as complete: `(a = 1` continues with AND/OR just
 * as `a = 1` does, the closer simply hasn't been typed yet.
 */
function expressionIsComplete(text: string): boolean {
  let trimmed = text.trim()
  if (!trimmed || trimmed.endsWith('(')) return false
  let depth = 0
  for (const c of trimmed) {
    if (c === '(') depth++
    else if (c === ')') depth--
  }
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
    // The options are whole conditions built around the typed hex, so they
    // must not be re-filtered against it: CodeMirror's fuzzy matcher scores
    // "spanID = <hex>" so poorly against the bare hex that the suggestions
    // never rendered at all.
    filter: false,
  }
}

export function createQueryCompletionSource(
  getFields: () => FieldDefinition[]
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

      // A bare field name is not a Comparison yet -- `name` parses as
      // FreeText, so the operator branch below is unreachable until an
      // operator has already been typed, which is too late to suggest one.
      // Once the name is complete and followed by a space, the only thing
      // that can come next is an operator, so offer them here too.
      const opHit = topLevelOperatorCompletions(context, getFields())
      if (opHit) return opHit
    }

    if (comparison) {
      const field = comparison.getChild(FieldTerm)
      const opNode = getOperatorNode(comparison)
      const valueNode = getValueNode(comparison)
      const pos = context.pos

      // Still in or at end of field name → complete field names, not operators.
      if (field && pos <= field.to) {
        // Replace through the end of the word: accepting mid-name used to
        // keep the tail, so `na|me` accepted as name became "name me".
        return fieldCompletions(context, getFields(), field.from, field.to)
      }

      // After field: whitespace before operator → operators (not another field).
      if (field && pos > field.to) {
        const between = context.state.sliceDoc(field.to, pos)
        if (/^\s*$/.test(between)) {
          if (!opNode || pos < opNode.from) {
            return operatorCompletions(
              context,
              context.state.sliceDoc(field.from, field.to),
              getFields()
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
              node.from
            )
          }
          return valueCompletions(context, fieldText, getFields())
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
              const r = valueCompletions(context, fieldText, getFields(), from)
              if (!r) return null
              return {
                ...r,
                options: r.options.map(o => ({ ...o, apply: o.label + prev })),
              }
            }
          }
          return valueCompletions(context, fieldText, getFields(), from)
        }
      }
    }

    // A finished expression continues with AND or OR and nothing else --
    // this is the branch that makes the logical operators actually appear.
    // The comparison-scoped branch above only fires while the cursor still
    // resolves inside the Comparison node, which one space past the value it
    // no longer does; the fallthrough used to offer field names there, which
    // the grammar cannot accept after a complete condition.
    {
      const partial = context.matchBefore(/[A-Za-z]*/)
      const before = context.state.sliceDoc(
        0,
        partial ? partial.from : context.pos
      )
      if (expressionIsComplete(before)) {
        return {
          from: partial?.from ?? context.pos,
          options: LOGICAL_COMPLETIONS,
          validFor: /^(AND|OR)?$/i,
        }
      }
    }

    // After logical op: fields.
    if (node.name === 'And' || node.name === 'Or') {
      return fieldCompletions(context, getFields())
    }

    if (node.name === 'Query') {
      return fieldCompletions(context, getFields())
    }

    if (node.name === 'Group' && context.pos < node.to) {
      return fieldCompletions(context, getFields())
    }

    const parentNode = node.parent

    if (
      (node.name === 'FieldName' ||
        (node.name === 'Word' && parentNode?.name === 'FieldName')) &&
      context.pos > node.to
    ) {
      const fieldText = context.state.sliceDoc(node.from, node.to)
      return operatorCompletions(context, fieldText, getFields())
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
        const only = t.firstChild
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
            opPartial.from
          )
        }
      }
    }

    // Typing the first word of a new condition: what precedes the word is
    // empty, an open paren, or a logical operator. This used to be decided
    // by a block of regexes over the raw text -- including a second spelling
    // of the whole operator list, which is the same split-brain the grammar
    // unification removed -- and every other context that block served is
    // now answered from the tree above. AND/OR are matched as whole words;
    // "operand" does not end with the operator AND.
    const word = context.matchBefore(/[\w.]+/)
    if (word) {
      const before = context.state.sliceDoc(0, word.from).trim()
      if (
        before === '' ||
        before.endsWith('(') ||
        /(?:^|[\s(])(?:AND|OR)$/i.test(before)
      ) {
        return fieldCompletions(context, getFields(), word.from)
      }
    }

    // Right after "AND " or "OR ", before any letters are typed: the
    // cursor resolves into the AndExpression/OrExpression parent, not the
    // And/Or token, so the node-name branch above never sees it. What
    // precedes the cursor tells the truth directly.
    if (!word) {
      // Trailing open parens are transparent: `AND (` is the same "a
      // condition starts here" position as `AND `.
      const before = context.state
        .sliceDoc(0, context.pos)
        .replace(/[(\s]+$/, '')
      if (/(?:^|[\s(])(?:AND|OR)$/i.test(before)) {
        return fieldCompletions(context, getFields())
      }
    }

    if (context.explicit) {
      return fieldCompletions(context, getFields())
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
  fields: FieldDefinition[]
): CompletionResult | null {
  // An unterminated string swallows the guard this function relies on: the
  // error-recovered Comparison node ends before the trailing space, so the
  // cursor resolves outside it and a field name inside the value -- typing
  // `x = "error: name ` -- looks exactly like a field awaiting an operator.
  // A string is unterminated for as long as it is being typed, so this is
  // the common case, not a corner.
  if (inOpenString(context.state.sliceDoc(0, context.pos))) return null

  const before = context.matchBefore(/[\w.]+\s+/)
  if (!before) return null
  const word = before.text.trim()
  const known = fields.some(
    f =>
      f.searchScope !== 'global' && f.name.toLowerCase() === word.toLowerCase()
  )
  if (!known) return null
  return operatorCompletions(context, word, fields, context.pos)
}

function fieldCompletions(
  context: CompletionContext,
  fields: FieldDefinition[],
  from?: number,
  to?: number
): CompletionResult | null {
  const options: Completion[] = fields
    .filter(
      (f): f is Exclude<FieldDefinition, { searchScope: 'global' }> =>
        f.searchScope !== 'global'
    )
    .map(f => ({
      label: f.name,
      type: 'property',
      detail: f.type,
      info: 'description' in f ? f.description : undefined,
      boost: f.searchScope === 'field' ? 1 : 0,
      // Accepting a field inserts the trailing space that ends it, which is
      // also what makes the operator list fire: picking `name` should leave
      // the cursor somewhere the next suggestion is waiting, not somewhere
      // the user has to guess that a space is expected.
      apply: f.name + ' ',
    }))

  if (options.length === 0) return null

  return {
    from: from ?? context.pos,
    ...(to !== undefined ? { to } : {}),
    options,
    validFor: /^[\w.]*$/,
  }
}

function operatorCompletions(
  context: CompletionContext,
  fieldName: string,
  fields: FieldDefinition[],
  from?: number
): CompletionResult | null {
  const field = fields.find(
    f =>
      f.searchScope !== 'global' &&
      f.name.toLowerCase() === fieldName.toLowerCase()
  )

  // The derived operators are wire spellings, not query syntax: the null
  // check is typed `= NULL` and negated regex is typed `!~`, so offering
  // "IS NULL" or "NOT REGEXP" here would complete into text the grammar
  // cannot parse.
  const derived = new Set(['IS NULL', 'IS NOT NULL', 'NOT REGEXP'])
  const ops = (
    field && field.searchScope !== 'global'
      ? field.operators
      : Object.values(OPERATORS)
  ).filter(op => !derived.has(op.symbol))

  const options: Completion[] = ops.map(op => ({
    label: op.symbol,
    type: 'operator',
    detail: op.label,
  }))

  return {
    from: from ?? context.pos,
    options,
    // Keep the list open and filtering while an operator is being typed.
    // Without this the dropdown closed on the first keystroke: the result
    // was anchored at the cursor with nothing marking further typing as a
    // continuation, so "C" on the way to CONTAINS dismissed the list that
    // had just offered it.
    validFor: /^[\w=!<>~^$]*$/,
  }
}

function valueCompletions(
  context: CompletionContext,
  fieldName: string,
  fields: FieldDefinition[],
  from?: number
): CompletionResult | null {
  const field = fields.find(
    f =>
      f.searchScope !== 'global' &&
      f.name.toLowerCase() === fieldName.toLowerCase()
  )
  if (!field || field.searchScope === 'global') return null

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
