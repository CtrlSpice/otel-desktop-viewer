import { describe, expect, it } from 'vitest'
import { EditorState } from '@codemirror/state'
import { CompletionContext } from '@codemirror/autocomplete'
import { createQueryCompletionSource } from './completions'
import { queryLanguage } from './query-language'
import { getFieldsBySignal } from '@/constants/fields'

// The completion source decides everything from the Lezer tree plus a little
// position logic, and until now had no tests -- every regression in it was
// found by driving the running app. These construct real editor states, so
// what is pinned is what the editor actually asks.

const fields = getFieldsBySignal('traces')
const source = createQueryCompletionSource(() => fields)

function complete(doc: string, pos = doc.length, explicit = false) {
  const state = EditorState.create({ doc, extensions: [queryLanguage] })
  return source(new CompletionContext(state, pos, explicit)) as {
    from: number
    options: { label: string }[]
    validFor?: RegExp
  } | null
}

function labels(doc: string, pos?: number): string[] | null {
  const r = complete(doc, pos)
  return r ? r.options.map(o => o.label) : null
}

describe('field positions', () => {
  it('offers fields at the start of a word', () => {
    expect(labels('dur')).toContain('duration')
  })

  it('offers fields after AND, either case', () => {
    // Static fields only here: service.name is a discovered attribute the
    // live editor merges in, invisible to this unit's field list.
    expect(labels('name = x AND sta')).toContain('statusCode')
    expect(labels('name = x and sta')).toContain('statusCode')
  })

  it('offers fields inside an open group', () => {
    expect(labels('(ki')).toContain('kind')
  })
})

describe('operator positions', () => {
  it("offers a field's own operators while typing one after its name", () => {
    const r = complete('name C')
    expect(r).not.toBeNull()
    expect(r!.options.map(o => o.label)).toContain('CONTAINS')
    // Anchored at the typed prefix, so accepting replaces "C" rather than
    // appending after it.
    expect(r!.from).toBe('name '.length)
  })

  it('offers symbol operators from a symbol prefix', () => {
    const r = complete('duration >')
    expect(r).not.toBeNull()
    expect(r!.options.map(o => o.label)).toContain('>=')
    expect(r!.from).toBe('duration '.length)
  })

  it('marks further typing as continuation, so the list filters not closes', () => {
    const r = complete('name C')
    expect(r!.validFor).toBeInstanceOf(RegExp)
    expect((r!.validFor as RegExp).test('CONT')).toBe(true)
    expect((r!.validFor as RegExp).test('!=')).toBe(true)
  })

  it('never offers the derived wire operators', () => {
    const r = complete('name C')
    const syms = r!.options.map(o => o.label)
    expect(syms).not.toContain('IS NULL')
    expect(syms).not.toContain('IS NOT NULL')
    expect(syms).not.toContain('NOT REGEXP')
  })
})

describe('after a complete expression', () => {
  it('offers exactly AND and OR', () => {
    expect(labels('kind = Server ')).toEqual(['AND', 'OR'])
  })

  it('filters to AND while typing it', () => {
    const r = complete('kind = Server A')
    expect(r!.options.map(o => o.label)).toEqual(['AND', 'OR'])
    expect(r!.from).toBe('kind = Server '.length)
  })

  it('works inside an unclosed group', () => {
    expect(labels('(kind = Server ')).toEqual(['AND', 'OR'])
  })

  it('but an empty open group is not a complete expression', () => {
    expect(labels('(ki')).not.toEqual(['AND', 'OR'])
  })
})

describe('id shapes', () => {
  it('builds unfiltered field = hex conditions for 16-char hex', () => {
    const r = complete('69b92b2b89e59215')
    expect(r).not.toBeNull()
    expect(r!.options.map(o => o.label)).toEqual([
      'spanID = 69b92b2b89e59215',
      'link.spanID = 69b92b2b89e59215',
    ])
  })

  it('offers trace fields for 32-char hex', () => {
    const r = complete('474ba3f3d156b778e927b2811aede70a')
    expect(r!.options.map(o => o.label)).toEqual([
      'traceID = 474ba3f3d156b778e927b2811aede70a',
      'link.traceID = 474ba3f3d156b778e927b2811aede70a',
    ])
  })
})

describe('review findings', () => {
  it('array item completions anchor at the item, not the bracket', () => {
    const first = complete('statusCode IN [O')
    expect(first).not.toBeNull()
    expect(first!.from).toBe('statusCode IN ['.length)
    const later = complete('statusCode IN [Ok, E')
    expect(later).not.toBeNull()
    expect(later!.from).toBe('statusCode IN [Ok, '.length)
  })

  it('offers fields immediately after AND, before any typing', () => {
    const r = complete('kind = Server AND ')
    expect(r).not.toBeNull()
    expect(r!.options.map(o => o.label)).toContain('kind')
  })
})
