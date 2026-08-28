import {
  type CompletionContext,
  type CompletionResult,
  type Completion,
} from '@codemirror/autocomplete'
import type { JsonAttributeMatch } from '@/types/wire-types'
import type { FieldDefinition } from '@/constants/fields'
import type { FieldValueCache } from './field-value-cache'

/**
 * Value-first completion: the user types text they can see in the UI, and the
 * editor offers the field expressions that would find it.
 *
 * @remarks
 * The existing completion source is key-first — it knows the field names and
 * offers them, so you can only search for something whose key you already know.
 * That is the wrong way round for the commonest question: *I can see `checkout`
 * in this trace, which field is that?*
 *
 * This answers it by asking the attribute dictionary, which holds one row per
 * distinct (key, value, type, scope) for the whole store and is therefore small
 * enough to grep on a keystroke. Typing `checkout` offers
 * `service.name = "checkout-api"` and `http.route = "/checkout"`, across traces,
 * logs and metrics at once.
 *
 * Deliberately additive: it only fires where the key-first source has nothing
 * useful to say, so it can never displace a field-name completion.
 *
 * It is also filtered to the fields this editor actually accepts. The
 * dictionary is shared by every signal, so a raw lookup for "Mercedes" returns
 * `f1.team` (a datapoint label) alongside `service.name` (a resource
 * attribute) — and offering the first in the traces box produces an expression
 * the linter immediately flags as `Unknown field`, because spans cannot be
 * filtered by a metric label. Suggesting something and then underlining it is
 * worse than not suggesting it.
 */

/** How many suggestions to show. Enough to choose from, few enough to scan. */
const MAX_SUGGESTIONS = 8

/** Below this, the term is too short to be discriminating and every keystroke
 *  would round-trip for a list nobody wants. */
const MIN_TERM_LENGTH = 2

/**
 * Renders a match as the query text that would find it.
 *
 * @remarks
 * Always quotes the value. It is not optional for anything containing a space
 * — an unquoted multi-word value is a parse error — and quoting uniformly means
 * the inserted text never depends on what the value happens to contain.
 */
export function matchToQuery(match: JsonAttributeMatch, value: string): string {
  return `${match.name} = "${value.replace(/(["\\])/g, '\\$1')}"`
}

/**
 * Builds the completion list for one dictionary match.
 *
 * @remarks
 * One option per sample value rather than one per key: the whole point is to
 * skip the step where you pick a key and then have to guess its exact value.
 * `matchCount` is surfaced when it exceeds the samples shown, so a term that
 * hits a high-cardinality key reads as "6 values" rather than silently looking
 * like there are only three.
 */
function optionsForMatch(match: JsonAttributeMatch): Completion[] {
  return match.sampleValues.map(value => ({
    label: matchToQuery(match, value),
    type: 'text',
    detail:
      match.matchCount > match.sampleValues.length
        ? `${match.attributeScope} · ${match.matchCount} values`
        : match.attributeScope,
    // Sort by how specific the key is: a term matching one value of a key is a
    // stronger signal than one matching fifty.
    boost: match.matchCount === 1 ? 1 : 0,
  }))
}

/**
 * Creates an async completion source backed by value-first discovery.
 *
 * @param search - dictionary lookup, normally telemetryAPI.searchAttributes
 * @returns a CodeMirror completion source
 */
export function createValueDiscoverySource(
  search: (term: string) => Promise<JsonAttributeMatch[]>,
  getFields: () => FieldDefinition[],
  // Column values come from the shared per-session cache; the dictionary
  // lookup above stays per-keystroke because it is cheap and term-dependent.
  fieldValues?: FieldValueCache
) {
  return async function valueDiscoverySource(
    context: CompletionContext
  ): Promise<CompletionResult | null> {
    const word = context.matchBefore(/[\w.\-/]+/)
    if (!word || word.text.length < MIN_TERM_LENGTH) return null

    // The word must start a fresh expression: at the beginning of the input,
    // after AND/OR, or after an opening bracket. Anything else means the user
    // is mid-comparison and has already named a field, where key-first
    // completion is correct and this must stay out of the way.
    //
    // This single check is the whole gate. An earlier version also bailed when
    // the syntax tree put the cursor inside a Comparison node, which read as
    // the safety net -- but mutation testing showed removing it changed no
    // behaviour, because reaching a Comparison always means something precedes
    // the word. The redundant guard is gone rather than left to look
    // load-bearing to whoever next touches this.
    const before = context.state.sliceDoc(0, word.from).trim()
    if (
      before !== '' &&
      !/\b(AND|OR)$/i.test(before) &&
      !before.endsWith('(')
    ) {
      return null
    }

    let matches: JsonAttributeMatch[]
    try {
      matches = await search(word.text)
    } catch {
      // Discovery is a convenience. If the store is unreachable or the query
      // fails, the key-first completions still work and the user can still
      // type; surfacing an error here would be worse than showing nothing.
      return null
    }
    if (context.aborted) return null

    // Only suggest what this editor can actually search. Matching on scope as
    // well as name matters: the same key can exist under several scopes, and
    // only some of them are valid here.
    const fields = getFields()
    const searchable = matches.filter(match =>
      fields.some(
        field =>
          field.searchScope === 'attribute' &&
          field.name === match.name &&
          field.attributeScope === match.attributeScope
      )
    )

    const term = word.text.toLowerCase()
    const fieldDefs = getFields().filter(
      (f): f is Extract<FieldDefinition, { searchScope: 'field' }> =>
        f.searchScope === 'field'
    )

    // Enum fields carry their values in the definition, so matching them
    // costs nothing: `Serv` offers `kind = Server`.
    const enumOptions: Completion[] = fieldDefs
      .filter(f => f.enumValues)
      .flatMap(f =>
        (f.enumValues ?? [])
          .filter(v => v.toLowerCase().includes(term))
          .map(v => ({
            label: `${f.name} = ${v}`,
            type: 'text' as const,
            detail: f.name,
          }))
      )

    // Discoverable columns: the cached top values, filtered locally.
    const columnOptions: Completion[] = (
      await Promise.all(
        fieldDefs
          .filter(f => f.discoverableValues)
          .map(async f => {
            if (!fieldValues) return []
            let values: string[]
            try {
              values = await fieldValues.values(f.name)
            } catch {
              // The cache evicts the failed fetch; nothing to offer here.
              return []
            }
            return values
              .filter(v => v.toLowerCase().includes(term))
              .map(v => ({
                label: `${f.name} = "${v.replace(/(["\\])/g, '\\$1')}"`,
                type: 'text' as const,
                detail: f.name,
              }))
          })
      )
    ).flat()
    if (context.aborted) return null

    // Fair-share the cap: attributes are usually plentiful and would starve
    // the other two categories out of the list entirely -- `Serv` should
    // offer `kind = Server` even when eight service.* attributes match.
    // Each category gets a quota, then leftovers backfill spare capacity.
    const attributeOptions = searchable.flatMap(optionsForMatch)
    const quotas: [Completion[], number][] = [
      [attributeOptions, 4],
      [enumOptions, 2],
      [columnOptions, 2],
    ]
    const options = quotas.flatMap(([group, quota]) => group.slice(0, quota))
    for (const [group, quota] of quotas) {
      if (options.length >= MAX_SUGGESTIONS) break
      options.push(
        ...group.slice(quota, quota + MAX_SUGGESTIONS - options.length)
      )
    }
    options.length = Math.min(options.length, MAX_SUGGESTIONS)
    if (options.length === 0) return null

    return {
      from: word.from,
      options,
      // Results depend on the term, so CodeMirror must re-query rather than
      // filter the previous list as the user keeps typing.
      validFor: undefined,
    }
  }
}
