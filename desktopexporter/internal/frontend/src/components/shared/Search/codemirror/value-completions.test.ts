import { describe, it, expect, vi } from 'vitest'
import { EditorState } from '@codemirror/state'
import { CompletionContext } from '@codemirror/autocomplete'
import { createValueDiscoverySource, matchToQuery } from './value-completions'
import { queryLanguageSupport } from './query-language'
import type { JsonAttributeMatch } from '@/types/wire-types'
import { OPERATORS } from '@/constants/operators'
import type { AttributeScope, FieldDefinition } from '@/constants/fields'
import { getFieldsBySignal } from '@/constants/fields'

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

// The fields this editor accepts. Suggestions are filtered against these, so
// anything offered is guaranteed to pass the linter -- see the
// cross-signal test below.
const attrField = (
  name: string,
  attributeScope: AttributeScope
): FieldDefinition => ({
  name,
  type: 'string',
  searchScope: 'attribute',
  attributeScope,
  operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS, OPERATORS.CONTAINS],
})

const traceFields: FieldDefinition[] = [
  attrField('service.name', 'resource'),
  attrField('http.route', 'span'),
]

// Drives the source the way CodeMirror does: build a document, put the cursor
// at the end, and ask for completions there.
async function complete(
  doc: string,
  search = vi.fn().mockResolvedValue(matches),
  fields: FieldDefinition[] = traceFields
) {
  const state = EditorState.create({
    doc,
    extensions: [queryLanguageSupport()],
    selection: { anchor: doc.length },
  })
  const context = new CompletionContext(state, doc.length, true)
  const result = await createValueDiscoverySource(search, () => fields)(context)
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
    const specific = result!.options.find(o =>
      o.label.startsWith('service.name')
    )
    const broad = result!.options.find(o => o.label.startsWith('http.route'))
    expect(specific!.boost).toBeGreaterThan(broad!.boost ?? 0)
  })

  it('shows the value count when more match than are shown', async () => {
    const { result } = await complete('checkout')
    expect(
      result!.options.find(o => o.label.includes('/checkout"'))!.detail
    ).toBe('span · 6 values')
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

describe('only suggests fields this editor can search', () => {
  // Found by using it: the dictionary is shared across signals, so looking up
  // "Mercedes" in the *traces* box returned f1.team -- a datapoint label from
  // metrics. Accepting it produced `f1.team = "Mercedes"`, which the linter
  // immediately underlined as `Unknown field`, because a span cannot be
  // filtered by a metric label. Suggesting something and then rejecting it is
  // worse than staying quiet.
  const crossSignal: JsonAttributeMatch[] = [
    {
      name: 'f1.team',
      attributeScope: 'datapoint',
      type: 'string',
      matchCount: 1,
      sampleValues: ['Mercedes'],
    },
    {
      name: 'service.name',
      attributeScope: 'resource',
      type: 'string',
      matchCount: 1,
      sampleValues: ['Mercedes'],
    },
  ]

  it('drops matches whose scope this signal cannot search', async () => {
    const { result } = await complete(
      'Merce',
      vi.fn().mockResolvedValue(crossSignal)
    )
    expect(result!.options.map(o => o.label)).toEqual([
      'service.name = "Mercedes"',
    ])
  })

  it('offers nothing rather than something unusable', async () => {
    const { result } = await complete(
      'Merce',
      vi.fn().mockResolvedValue([crossSignal[0]])
    )
    expect(result).toBeNull()
  })

  // The same key can exist under several scopes, and only some are valid here,
  // so the scope has to match too -- not just the name.
  it('matches on scope as well as name', async () => {
    const sameNameWrongScope: JsonAttributeMatch[] = [
      {
        name: 'service.name',
        attributeScope: 'datapoint',
        type: 'string',
        matchCount: 1,
        sampleValues: ['Mercedes'],
      },
    ]
    const { result } = await complete(
      'Merce',
      vi.fn().mockResolvedValue(sameNameWrongScope)
    )
    expect(result).toBeNull()
  })
})

describe('bare-text discovery of enums and columns', () => {
  const fields = getFieldsBySignal('traces')

  function discoverySource(
    fetch = async (): Promise<string[]> => ['checkout/pay', 'checkout/verify']
  ) {
    return createValueDiscoverySource(
      async () => [],
      () => fields,
      fetch,
      'traces'
    )
  }

  async function discover(doc: string, source = discoverySource()) {
    const state = EditorState.create({
      doc,
      extensions: [queryLanguageSupport()],
    })
    return source(new CompletionContext(state, doc.length, false))
  }

  it('offers an enum comparison from bare text', async () => {
    const r = await discover('Serv')
    expect(r).not.toBeNull()
    expect(r!.options.map(o => o.label)).toContain('kind = Server')
  })

  it('does not let plentiful attributes starve enums out of the list', async () => {
    // Eight attribute matches would fill the cap on their own; the quota
    // keeps room so `kind = Server` still surfaces.
    const manyAttrs = async (): Promise<JsonAttributeMatch[]> =>
      Array.from({ length: 8 }, (_, i) => ({
        name: 'service.name',
        attributeScope: 'resource' as AttributeScope,
        type: 'string',
        matchCount: 1,
        sampleValues: [`service-${i}`],
      }))
    const source = createValueDiscoverySource(
      manyAttrs,
      () => fields,
      async () => [],
      'traces'
    )
    const r = await discover('Serv', source)
    expect(r).not.toBeNull()
    expect(r!.options.map(o => o.label)).toContain('kind = Server')
  })

  it('offers a column comparison from bare text, quoted', async () => {
    const r = await discover('checkout')
    expect(r).not.toBeNull()
    expect(r!.options.map(o => o.label)).toContain('name = "checkout/pay"')
  })

  it('fetches column values once across keystrokes', async () => {
    const fetch = vi.fn(async () => ['checkout/pay'])
    const source = discoverySource(fetch)
    await discover('chec', source)
    await discover('check', source)
    expect(fetch).toHaveBeenCalledTimes(1)
  })

  it('stays quiet mid-comparison, as before', async () => {
    expect(await discover('name = check')).toBeNull()
  })
})
