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
  // A bare field name parses as FreeText, not a Comparison, so before this
  // the operator branch was unreachable until an operator had already been
  // typed -- by which point suggesting one is too late (#413).
  it('offers operators once a field name is complete and followed by a space', () => {
    expect(labels('name ')).toEqual(
      expect.arrayContaining(['=', '!=', 'CONTAINS'])
    )
  })

  it("offers the field's own operators, not every operator", () => {
    const ops = labels('duration ')
    expect(ops).toEqual(expect.arrayContaining(['>', '<', '>=']))
    expect(ops).not.toContain('CONTAINS')
  })

  it('offers operators after a field name inside a group', () => {
    expect(labels('(name ')).toEqual(expect.arrayContaining(['=']))
  })

  it('keeps offering fields while the name may still be extended', () => {
    // Cursor still touching the word: the user may be typing something
    // longer, so field names remain the useful suggestion.
    expect(labels('name')).toContain('traceID')
  })

  it('does not mistake a non-field word for a field name', () => {
    // Both of these end in a space after a word, which is the shape the
    // operator branch keys on -- neither is a field.
    expect(labels('notafield ')).toContain('traceID')
    expect(labels('name = x AND ')).toContain('statusCode')
  })

  it('accepting a field inserts the space that ends it', () => {
    // Which is also what makes the operator list fire next, so picking a
    // field leads to picking an operator without a guessed keystroke.
    const r = complete('nam')
    const name = r!.options.find(o => o.label === 'name') as
      { apply?: string } | undefined
    expect(name?.apply).toBe('name ')
  })

  it('stays quiet inside an unterminated quoted string', () => {
    // An open string swallows the Comparison guard: the error-recovered node
    // ends before the trailing space, so a field-shaped word inside the value
    // looks like a field awaiting an operator. A string is unterminated the
    // whole time it is being typed, so this is the common case.
    // The source falls through to its usual fallback here; the contract is
    // that operators are not offered, not that nothing is.
    expect(labels('x = "name ')).not.toContain('=')
    expect(labels('statusMessage = "error: name ')).not.toContain('CONTAINS')
  })

  it('is escape-aware about quotes when deciding a string is open', () => {
    // The \" belongs to the value; counting it as a delimiter would make the
    // string look closed and re-open the hole above.
    expect(labels('x = "a\\" name ')).not.toContain('=')
  })

  it('replaces the whole word when accepting a field mid-name', () => {
    // Anchored [from, to] across the word: accepting `name` at na|me used to
    // keep the tail and produce "nameme".
    const r = complete('name = x', 2)
    expect(r).not.toBeNull()
    expect(r!.from).toBe(0)
    expect((r as { to?: number }).to).toBe(4)
  })

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
  it('offers boolean continuations and a result limit', () => {
    expect(labels('kind = Server ')).toEqual(['AND', 'OR', '| LIMIT'])
  })

  it('filters to AND while typing it', () => {
    const r = complete('kind = Server A')
    expect(r!.options.map(o => o.label)).toEqual(['AND', 'OR', '| LIMIT'])
    expect(r!.from).toBe('kind = Server '.length)
  })

  it('works inside an unclosed group', () => {
    expect(labels('(kind = Server ')).toEqual(['AND', 'OR'])
  })

  it('but an empty open group is not a complete expression', () => {
    expect(labels('(ki')).not.toEqual(['AND', 'OR', '| LIMIT'])
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

// The three completion edges from the second #405 review (issue #406).
describe('array quote and group edges', () => {
  it('offers nothing right after a closed quoted item', () => {
    // `["Ok"|` -- the only valid next characters are `,` or `]`; anything
    // accepted here would glue onto the closed string.
    expect(complete('statusCode IN ["Ok"')).toBeNull()
  })

  it('closes a manually typed opening quote on accept', () => {
    const r = complete('statusCode IN ["O')
    expect(r).not.toBeNull()
    const ok = r!.options.find(o => o.label === 'Ok') as
      { label: string; apply?: string } | undefined
    expect(ok?.apply).toBe('Ok"')
  })

  it('still anchors per item after a closed item and comma', () => {
    const r = complete('statusCode IN ["Ok", E')
    expect(r).not.toBeNull()
    expect(r!.from).toBe('statusCode IN ["Ok", '.length)
  })

  it('offers fields after AND followed by an open paren', () => {
    const r = complete('kind = Server AND (')
    expect(r).not.toBeNull()
    expect(r!.options.map(o => o.label)).toContain('kind')
  })
})
