import {
  startCompletion,
  type CompletionContext,
  type CompletionResult,
  type Completion,
} from '@codemirror/autocomplete'
import { syntaxTree } from '@codemirror/language'
import type { SyntaxNode } from '@lezer/common'
import { FieldName as FieldTerm } from './query.parser.terms'
import type { FieldDefinition } from '@/constants/fields'
import type { FieldValueCache } from './field-value-cache'

/**
 * Value completion for plain-column fields: `name = ` offers the names the
 * store actually holds (#412), and any field marked `discoverableValues`
 * works the same way. Also completes items inside an `IN [...]` array, where
 * each item completes independently of the ones already listed.
 *
 * @remarks
 * These are columns, not attributes, so the dictionary-backed value
 * discovery cannot see them; a dedicated RPC serves them instead. The server
 * allowlists the same fields the definitions mark; the two change together.
 *
 * One fetch, then local filtering. The store query scans every span row per
 * call (a contains-match cannot use an index, and frequency ordering must
 * count everything before it can rank), measured at ~11ms per call on a 245k
 * span store -- fine once, not fine per keystroke. So the source fetches the
 * top names by frequency a single time and hands CodeMirror a `validFor`
 * regex, which filters the same list client-side as the user types. Fuzzy
 * matching locally is also better than ILIKE remotely: `chkpay` finds
 * `checkout/pay`.
 *
 * The limit is the one trade: a name outside the top `FETCH_LIMIT` by
 * frequency is not in the dropdown. It can still be typed.
 */

/** Options handed to CodeMirror per open. It renders the list eagerly, so
 *  this caps DOM size while leaving local filtering plenty to match into. */
const DISPLAY_CAP = 64

function findAncestor(node: SyntaxNode, name: string): SyntaxNode | null {
  for (let cur: SyntaxNode | null = node; cur; cur = cur.parent) {
    if (cur.name === name) return cur
  }
  return null
}

export function createFieldValueSource(
  cache: FieldValueCache,
  getFields: () => FieldDefinition[]
) {
  return async function fieldValueSource(
    context: CompletionContext
  ): Promise<CompletionResult | null> {
    const tree = syntaxTree(context.state)
    const node = tree.resolveInner(context.pos, -1)
    const comparison = findAncestor(node, 'Comparison')
    if (!comparison) return null

    const field = comparison.getChild(FieldTerm)
    if (!field) return null
    const fieldText = context.state.sliceDoc(field.from, field.to)
    const def = getFields().find(
      (f): f is Extract<FieldDefinition, { searchScope: 'field' }> =>
        f.searchScope === 'field' &&
        f.discoverableValues === true &&
        f.name.toLowerCase() === fieldText.toLowerCase()
    )
    if (!def) return null

    const op =
      comparison.getChild('Operator') ?? comparison.getChild('KeywordOperator')
    if (!op || context.pos <= op.to) return null

    // The operator has to be one this field accepts, or completion helps
    // write an expression the linter immediately flags -- `name IN [` was
    // offering span names while being underlined as invalid, since name
    // takes no array operator.
    const opText = context.state.sliceDoc(op.from, op.to).toUpperCase()
    const accepted = def.operators.some(o => o.symbol.toUpperCase() === opText)
    if (!accepted) return null

    // An array operator needs its brackets. Offering bare values after
    // `unit IN ` completes into `unit IN {errors}`, which does not parse, so
    // the bracket is offered instead -- one keystroke to the values.
    const isArrayOp = opText === 'IN' || opText === 'NOT IN'

    // The value region: everything after the operator. When the user has
    // opened a quote, the anchor sits AFTER it -- CodeMirror filters options
    // against the text from the anchor to the cursor, and a label never
    // starts with a quote, so anchoring on the quote filters everything out.
    const afterOp = context.state.sliceDoc(op.to, context.pos)
    const lead = afterOp.match(/^\s*/)?.[0].length ?? 0
    const valueFrom = op.to + lead
    const region = context.state.sliceDoc(valueFrom, context.pos)

    // An IN array holds several values, so the item being typed -- not the
    // whole region -- is what completes. Anchor after the last bracket or
    // comma; everything before it is settled.
    const inArray = region.startsWith('[')
    if (isArrayOp && !inArray) {
      // Nothing typed yet, or something that is not an array: offer the
      // opening bracket, which reopens completion on the values inside.
      if (region.trim() !== '') return null
      return {
        from: valueFrom,
        options: [
          {
            label: '[',
            type: 'text',
            detail: `list of ${def.name} values`,
            // Reopens completion on the values inside: accepting a bracket
            // and then facing a closed dropdown makes the suggestion look
            // like it led nowhere.
            apply: (view, _completion, applyFrom, applyTo) => {
              view.dispatch({
                changes: { from: applyFrom, to: applyTo, insert: '[' },
                selection: { anchor: applyFrom + 1 },
              })
              startCompletion(view)
            },
          },
        ],
        validFor: /^$/,
      }
    }
    if (inArray && region.includes(']')) return null
    let itemFrom = valueFrom
    if (inArray) {
      const lastSep = Math.max(region.lastIndexOf('['), region.lastIndexOf(','))
      const afterSep = region.slice(lastSep + 1)
      itemFrom =
        valueFrom + lastSep + 1 + (afterSep.match(/^\s*/)?.[0].length ?? 0)
    }

    let from = itemFrom
    let typed = context.state.sliceDoc(itemFrom, context.pos)
    let openQuote: '"' | "'" | null = null
    if (typed.startsWith('"') || typed.startsWith("'")) {
      openQuote = typed[0] as '"' | "'"
      from = itemFrom + 1
      typed = typed.slice(1)
    }
    // A quote left in the typed text means this item is already closed and
    // the cursor sits past it, where only a comma or a bracket is valid.
    if (/["']/.test(typed)) return null

    // closeBrackets() pairs the opening quote the moment it is typed, so the
    // closing quote may already sit just past the cursor. Appending another
    // would double it.
    const autoClosed =
      openQuote !== null &&
      context.state.sliceDoc(context.pos, context.pos + 1) === openQuote

    let names: string[]
    try {
      names = await cache.values(def.name)
    } catch {
      // Completion is a convenience; typing still works without it. The
      // cache evicts the failed fetch itself, so the next trigger retries.
      return null
    }
    if (context.aborted || names.length === 0) return null

    const options: Completion[] = names.map(name => {
      // Quoted uniformly: a name with spaces is a parse error unquoted, and
      // never varying the inserted shape beats varying it by content. The
      // user's own opening quote is kept; ours is added only when absent.
      const q = openQuote ?? '"'
      const escaped = name.replace(new RegExp(`([${q}\\\\])`, 'g'), '\\$1')
      const apply = openQuote
        ? escaped + (autoClosed ? '' : q)
        : q + escaped + q
      return { label: name, type: 'text', detail: def.description, apply }
    })

    return {
      from,
      options: options.slice(0, DISPLAY_CAP),
      // Filter the fetched list locally while the value is being typed
      // rather than re-querying: the fetch is the expensive part. Inside a
      // quote, spaces are part of the value; outside, one ends the token.
      validFor: openQuote ? /^[^"']*$/ : /^[^"'\s]*$/,
    }
  }
}
