import {
  attributeMatchesSQL,
  type AttributeMatchOptions,
  type AttributeRecordMatch,
} from './attribute-matches'
import { attributeMatch, type AttributeMatch } from './arrow/arrow-search'
import { rowViews } from './arrow/arrow-values'
import { named, callRPC, callArrow } from './telemetry-request'

export const attributeSearchAPI = {
  getAttributeRecordMatches: async (
    options: AttributeMatchOptions,
    signal?: AbortSignal
  ): Promise<AttributeRecordMatch[]> => {
    if (options.valueIdentity?.startsWith('home:')) {
      const resolved = await callRPC<{ identity: string }>(
        'discoverTelemetry',
        named({
          options: { kind: 'home-value', valueRef: options.valueIdentity },
        }),
        signal
      )
      options = { ...options, valueIdentity: resolved.identity }
    }
    const matches: AttributeRecordMatch[] = []
    let more = true
    while (more) {
      signal?.throwIfAborted()
      const result = await callRPC<{
        rows: [string, string[], number][]
        truncated: boolean
      }>(
        'query',
        named({
          sql: attributeMatchesSQL(options, matches.length),
          limit: 500,
        }),
        signal
      )
      matches.push(
        ...result.rows.map(([itemRef, spanIDs, count]) => ({
          itemRef,
          spanIDs,
          count,
        }))
      )
      more = result.truncated
    }
    return matches
  },

  // Distinct values of one completable column, most frequent first. Fetched
  // once per field per completion session with a generous limit; the editor
  // filters the list client-side per keystroke, so this does not round-trip
  // while typing. The server allowlists which fields answer.
  getFieldValueCompletions: async (
    signal: string,
    field: string,
    term: string,
    limit: number,
    abort?: AbortSignal
  ): Promise<string[]> => {
    const rawData = await callArrow(
      'getFieldValueCompletions',
      named({ signal, field, term, limit }),
      abort
    )
    return rowViews(rawData, (frame, row) => frame.cell('value', row).text())
  },

  searchAttributeMatches: async (
    term: string,
    options: {
      signal?: string
      owner?: string
      path?: readonly (string | number)[]
      type?: string
    } = {},
    signal?: AbortSignal
  ): Promise<AttributeMatch[]> => {
    if (!term.trim() && !options.path) return []
    const rawData = await callArrow(
      'searchAttributeMatches',
      named({ term, ...options }),
      signal
    )
    return rowViews(rawData, attributeMatch)
  },
}
