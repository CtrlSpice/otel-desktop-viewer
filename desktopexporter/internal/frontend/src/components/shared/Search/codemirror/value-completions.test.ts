import { describe, it, expect, vi } from 'vitest'
import { EditorState } from '@codemirror/state'
import { CompletionContext } from '@codemirror/autocomplete'
import {
  createValueDiscoverySource,
  matchToQuery,
} from './value-completions'
import { queryLanguageSupport } from './query-language'
import type { JsonAttributeMatch } from '@/types/wire-types'

const matches: JsonAttributeMatch[] = [
  {
    name: 'service.name',
    attributeScope: 'resource',
    type: 'string',
    matchCount: 1,
    sampleValues: ['checkout-api'],
  },
  {
    name: 'http.route',
    attributeScope: 'span',
    type: 'string',
    matchCount: 6,
    sampleValues: ['/checkout', '/checkout/confirm'],
  },
]

// Drives the source the way CodeMirror does: build a document, put the cursor
// at the end, and ask for completions there.
async function complete(doc: string, search = vi.fn().mockResolvedValue(matches)) {
  const state = EditorState.create({
    doc,
    extensions: [queryLanguageSupport()],
    selection: { anchor: doc.length },
  })
  const context = new CompletionContext(state, doc.length, true)
  const result = await createValueDiscoverySource(search)(context)
  return { result, search }
}

describe('matchToQuery', () => {
  it('renders a match as the query that would find it', () => {
    expect(matchToQuery(matches[0], 'checkout-api')).toBe(
      'service.name = "checkout-api"'
    )
  })

  // Values are always quoted, so a value containing a space cannot produce the
  // unquoted multi-word form the parser rejects.
  it('quotes values containing spaces', () => {
    expect(matchToQuery(matches[0], 'Red Bull Racing')).toBe(
      'service.name = "Red Bull Racing"'
    )
  })

  it('escapes quotes and backslashes in the value', () => {
    expect(matchToQuery(matches[0], 'say "hi"')).toBe(
      'service.name = "say \\"hi\\""'
    )
    expect(matchToQuery(matches[0], 'a\\b')).toBe('service.name = "a\\\\b"')
  })
})

describe('value discovery completions', () => {
  it('offers one option per sample value, not per key', async () => {
    const { result } = await complete('checkout')
    expect(result).not.toBeNull()
    expect(result!.options.map(o => o.label)).toEqual([
      'service.name = "checkout-api"',
      'http.route = "/checkout"',
      'http.route = "/checkout/confirm"',
    ])
  })

  it('replaces the typed term rather than appending to it', async () => {
    const { result } = await complete('checkout')
    expect(result!.from).toBe(0)
  })

  // A term matching one value of a key is a stronger signal than one matching
  // fifty, so it sorts first.
  it('boosts keys where the term is specific', async () => {
    const { result } = await complete('checkout')
    const specific = result!.options.find(o => o.label.startsWith('service.name'))
    const broad = result!.options.find(o => o.label.startsWith('http.route'))
    expect(specific!.boost).toBeGreaterThan(broad!.boost ?? 0)
  })

  it('shows the value count when more match than are shown', async () => {
    const { result } = await complete('checkout')
    expect(result!.options.find(o => o.label.includes('/checkout"'))!.detail)
      .toBe('span · 6 values')
    // Fully sampled keys show just the scope -- no misleading count.
    expect(result!.options[0].detail).toBe('resource')
  })

  it('does not query for terms too short to discriminate', async () => {
    const { result, search } = await complete('c')
    expect(result).toBeNull()
    expect(search).not.toHaveBeenCalled()
  })

  // Inside a comparison the user has already named a field, so key-first
  // completion is correct and this must stay out of the way.
  it('stays silent once a field has been named', async () => {
    const { result, search } = await complete('http.method = GET')
    expect(result).toBeNull()
    expect(search).not.toHaveBeenCalled()
  })

  it('fires after a logical operator', async () => {
    const { result } = await complete('http.method = "GET" AND checkout')
    expect(result).not.toBeNull()
  })

  it('returns nothing when the dictionary has no match', async () => {
    const { result } = await complete('zzz', vi.fn().mockResolvedValue([]))
    expect(result).toBeNull()
  })

  // Discovery is a convenience; a failing store must not break typing.
  it('degrades silently when the lookup fails', async () => {
    const { result } = await complete(
      'checkout',
      vi.fn().mockRejectedValue(new Error('offline'))
    )
    expect(result).toBeNull()
  })
})
