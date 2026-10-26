import { describe, expect, it } from 'vitest'
import { EditorState } from '@codemirror/state'
import { attributeTokens } from './attribute-tokens'

function tokens(doc: string) {
  const field = attributeTokens('logs')
  const state = EditorState.create({ doc, extensions: [field] })
  const result: string[] = []
  state.field(field).between(0, doc.length, (from, to) => {
    result.push(doc.slice(from, to))
  })
  return result
}

describe('attribute presentation', () => {
  it('decorates complete references before and after the comparator is entered', () => {
    const reference = 'attr(log, "attempts", int64)'
    expect(tokens(`${reference} `)).toEqual([reference])
    expect(tokens(`${reference} >= 3`)).toEqual([reference])
    expect(tokens(`body = "checkout" and (${reference} >= 3)`)).toEqual([
      reference,
    ])
  })

  it('preserves arbitrary keys, spacing and escaped characters in the underlying document', () => {
    const reference = 'attr( log, "a \\"quoted\\" key", int64 )'
    expect(tokens(`${reference} = 3`)).toEqual([reference])
  })

  it('does not disguise literal text, invalid references or another signal’s scope', () => {
    expect(tokens('body = \'attr(log, "attempts", int64)\'')).toEqual([])
    expect(tokens('attr(log, "attempts", unknown) = 3')).toEqual([])
    expect(tokens('attr(span, "attempts", int64) = 3')).toEqual([])
    expect(tokens('attr(log, "attempts", ')).toEqual([])
  })
})
