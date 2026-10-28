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
    const reference = 'log.attributes["attempts"]::BIGINT'
    expect(tokens(`${reference} `)).toEqual([reference])
    expect(tokens(`${reference} >= 3`)).toEqual([reference])
    expect(tokens(`body = "checkout" and (${reference} >= 3)`)).toEqual([
      reference,
    ])
  })

  it('preserves arbitrary keys, spacing and escaped characters in the underlying document', () => {
    const reference = 'log.attributes[ "a \\"quoted\\" key" ] :: BIGINT'
    expect(tokens(`${reference} = 3`)).toEqual([reference])
  })

  it('does not disguise literal text, invalid references or another signal’s scope', () => {
    expect(tokens('attributes["attempts"] = 3')).toEqual([])
    expect(tokens('body = \'log.attributes["attempts"]::BIGINT\'')).toEqual([])
    expect(tokens('log.attributes["attempts"]::UNKNOWN = 3')).toEqual([])
    expect(tokens('span.attributes["attempts"]::BIGINT = 3')).toEqual([])
    expect(tokens('log.attributes["attempts')).toEqual([])
  })
})
