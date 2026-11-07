import { describe, expect, it } from 'vitest'
import { EditorState } from '@codemirror/state'
import { CompletionContext } from '@codemirror/autocomplete'
import { convertQueryTreeForBackend } from '@/services/telemetry-service'
import { fieldIdentity } from '@/search/model'
import { parseSearchRequest } from './queryParser'
import {
  createPathFieldReference,
  formatAttributeFieldReference,
  parseAttributeFieldReference,
} from './attribute-field-reference'
import { createQueryCompletionSource } from './codemirror/completions'
import { queryLanguage } from './codemirror/query-language'
import { queryText } from '@/components/home/home-search'

describe('nested selected addresses', () => {
  it.each([
    ['user', 'id'],
    ['user.id'],
    [''],
    ['', ''],
    ['items', 0, '0'],
    ['items', '0', 0],
    ['a.b', '[0]', 'quote"key', 'back\\slash', '雪'],
  ])(
    'retains exact path %j through completion, parsing, wire and history',
    (...path) => {
      const field = createPathFieldReference(
        'span',
        ['attributes', ...path],
        'int64',
        'traces'
      )
      if (!field) throw new Error('Expected a field')
      const state = EditorState.create({ doc: '', extensions: [queryLanguage] })
      const completion = createQueryCompletionSource(
        () => [field],
        'traces'
      )(new CompletionContext(state, 0, true))
      const insert = completion?.options.find(
        option => option.label === (field.name || '""')
      )?.apply
      expect(insert).toBe(`${formatAttributeFieldReference(field)} `)
      const parsed = parseSearchRequest(
        `${insert}= 9007199254740993 LIMIT 7`,
        [],
        'traces'
      )
      const node = parsed?.predicate
      if (!node || node.type !== 'condition')
        throw new Error('Expected a condition')
      expect(fieldIdentity(node.query.field)).toBe(fieldIdentity(field))
      expect(convertQueryTreeForBackend(node)).toMatchObject({
        type: 'condition',
        query: {
          field: {
            owner: 'span',
            path: ['attributes', ...path],
            type: 'int64',
          },
          value: '9007199254740993',
        },
      })
      expect(
        parseSearchRequest(queryText(node), [], 'traces')?.predicate
      ).toMatchObject({ query: { field } })
      expect(parsed?.limit).toBe(7)
    }
  )

  it('formats an exact typed single-key address', () => {
    const field = parseAttributeFieldReference(
      "resource.attributes['service.name']::VARCHAR"
    )
    if (!field) throw new Error('Expected a field')
    expect(field.path).toEqual(['attributes', 'service.name'])
    expect(formatAttributeFieldReference(field)).toBe(
      'resource.attributes["service.name"]::VARCHAR'
    )
  })

  it.each([
    'attributes',
    'attributes[0]',
    'attributes["a"][-1]',
    'attributes["a"][1.5]',
    'attributes["a"][null]',
    'attributes["a"][{}]',
    'attributes["a"][[0]]',
    'attributes["a"][4294967295]',
  ])('rejects invalid selected path %s', path => {
    expect(parseAttributeFieldReference(`span.${path}::VARCHAR`)).toBeNull()
  })

  it('uses the same token and wire address for structured log bodies', () => {
    const text = 'log.body[0]["0"]::BIGINT = 9007199254740993'
    const node = parseSearchRequest(text, [], 'logs')?.predicate
    if (!node || node.type !== 'condition')
      throw new Error('Expected a condition')
    expect(node.query.field.path).toEqual(['body', 0, '0'])
    expect(queryText(node)).toBe(
      `log.body[0]["0"]::BIGINT = '9007199254740993'`
    )
    expect(convertQueryTreeForBackend(node)).toMatchObject({
      query: { field: { owner: 'log', path: ['body', 0, '0'], type: 'int64' } },
    })
  })
})
