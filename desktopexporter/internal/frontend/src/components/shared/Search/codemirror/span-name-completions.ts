import {
  type CompletionContext,
  type CompletionResult,
  type Completion,
} from '@codemirror/autocomplete'
import { syntaxTree } from '@codemirror/language'
import type { SyntaxNode } from '@lezer/common'
import { FieldName as FieldTerm } from './query.parser.terms'

/**
 * Value completion for span names: `name = ` offers the names the store
 * actually holds (#412).
 *
 * @remarks
 * Span names are a plain column, not attributes, so the dictionary-backed
 * value discovery cannot see them; this asks a dedicated RPC instead.
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

/** Names fetched per session. ~15KB of payload at the extreme, once. */
const FETCH_LIMIT = 500

/** Options handed to CodeMirror per open. It renders the list eagerly, so
 *  this caps DOM size while leaving local filtering plenty to match into. */
const DISPLAY_CAP = 64

function findAncestor(node: SyntaxNode, name: string): SyntaxNode | null {
  for (let cur: SyntaxNode | null = node; cur; cur = cur.parent) {
    if (cur.name === name) return cur
  }
  return null
}

export function createSpanNameValueSource(
  fetchNames: (term: string, limit: number) => Promise<string[]>,
  signal: string
) {
  return async function spanNameValueSource(
    context: CompletionContext
  ): Promise<CompletionResult | null> {
    // Span names are a traces concept; the metrics editor's `name` field is
    // a metric name and gets nothing from this source.
    if (signal !== 'traces') return null

    const tree = syntaxTree(context.state)
    const node = tree.resolveInner(context.pos, -1)
    const comparison = findAncestor(node, 'Comparison')
    if (!comparison) return null

    const field = comparison.getChild(FieldTerm)
    if (!field) return null
    const fieldText = context.state.sliceDoc(field.from, field.to)
    if (fieldText.toLowerCase() !== 'name') return null

    const op =
      comparison.getChild('Operator') ?? comparison.getChild('KeywordOperator')
    if (!op || context.pos <= op.to) return null

    // The value region: everything after the operator. An opening quote is
    // part of what gets replaced, so accepting a suggestion produces one
    // consistently quoted value whether or not the user had started quoting.
    const afterOp = context.state.sliceDoc(op.to, context.pos)
    const lead = afterOp.match(/^\s*/)?.[0].length ?? 0
    const valueFrom = op.to + lead
    let typed = context.state.sliceDoc(valueFrom, context.pos)
    if (typed.startsWith('"') || typed.startsWith("'")) {
      typed = typed.slice(1)
    }
    // A quote later in the typed text means the value is already closed and
    // the cursor is beyond it; nothing useful to offer there.
    if (/["']/.test(typed)) return null

    let names: string[]
    try {
      // Empty term: the server returns the top names by frequency, which is
      // exactly what the empty value position should offer.
      names = await fetchNames('', FETCH_LIMIT)
    } catch {
      // Completion is a convenience; typing still works without it.
      return null
    }
    if (context.aborted || names.length === 0) return null

    const options: Completion[] = names.map(name => ({
      label: name,
      type: 'text',
      detail: 'span name',
      // Quoted uniformly: a name with spaces is a parse error unquoted, and
      // never varying the inserted shape beats varying it by content.
      apply: `"${name.replace(/(["\\])/g, '\\$1')}"`,
    }))

    return {
      from: valueFrom,
      options: options.slice(0, DISPLAY_CAP),
      // Filter the fetched list locally while the value is being typed
      // rather than re-querying: the fetch is the expensive part.
      validFor: /^["']?[^"']*$/,
    }
  }
}
