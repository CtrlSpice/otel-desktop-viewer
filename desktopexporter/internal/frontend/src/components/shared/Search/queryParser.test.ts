import { describe, it, expect } from 'vitest'
import { parseQuery, validateQuery } from './queryParser'
import { OPERATORS } from '../../../constants/operators'
import type { FieldDefinition } from '../../../constants/fields'

const fields: FieldDefinition[] = [
  {
    name: 'http.method',
    type: 'string',
    searchScope: 'attribute',
    attributeScope: 'span',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS, OPERATORS.CONTAINS],
  },
  {
    name: 'service.name',
    type: 'string',
    searchScope: 'attribute',
    attributeScope: 'resource',
    operators: [OPERATORS.EQUALS, OPERATORS.NOT_EQUALS, OPERATORS.CONTAINS],
  },
]

const valueOf = (input: string): unknown => {
  const tree = parseQuery(input, fields) as any
  return (tree?.query ?? tree)?.value
}

describe('queryParser value normalization', () => {
  // Spacing around the operator is syntax and carries no meaning, so every
  // arrangement has to reach the backend as the same value. These are the
  // forms people actually type.
  it.each([
    'http.method = "GET"',
    'http.method ="GET"',
    'http.method= GET',
    'http.method=GET',
    'http.method = GET',
    "http.method = 'GET'",
    'http.method = `GET`',
    'http.method = GET ',
  ])('%s yields GET', input => {
    expect(valueOf(input)).toBe('GET')
  })

  // Quoting is how a user says "the whitespace is part of the value", so it
  // must survive byte-exact. The search fast path hashes this string and
  // compares the result against what ingest stored, so trimming here would
  // silently match nothing.
  it.each([
    ['http.method = "GET "', 'GET '],
    ['http.method = " GET"', ' GET'],
    ['http.method =  "  GET  "  ', '  GET  '],
    ['http.method = "GE T"', 'GE T'],
  ])('%s preserves quoted whitespace', (input, expected) => {
    expect(valueOf(input)).toBe(expected)
  })
})

describe('unquoted multi-word values', () => {
  // The regression this file exists for. `service.name = Red Bull Racing`
  // used to parse as `Red` and silently drop the rest, returning confidently
  // wrong results.
  it('rejects rather than silently truncating', () => {
    expect(() => parseQuery('service.name = Red Bull Racing', fields)).toThrow(
      /Unexpected "Bull"/
    )
  })

  it('suggests quoting, since that is always the fix', () => {
    expect(() => parseQuery('service.name = Red Bull Racing', fields)).toThrow(
      /must be quoted/
    )
  })

  it('accepts the same value when quoted', () => {
    expect(valueOf('service.name = "Red Bull Racing"')).toBe('Red Bull Racing')
  })

  // The editor underlines it and a submitted query errors: the two agree.
  // They did not before -- validateQuery reported it, parseQuery did not.
  it('is reported identically by the validator and the parser', () => {
    const input = 'service.name = Red Bull Racing'
    const errors = validateQuery(input, fields)
    expect(errors.length).toBeGreaterThan(0)
    expect(errors.some(e => /Unexpected "Bull"/.test(e.message))).toBe(true)

    let thrown = ''
    try {
      parseQuery(input, fields)
    } catch (e) {
      thrown = e instanceof Error ? e.message : String(e)
    }
    expect(thrown).toBe(
      errors.find(e => /Unexpected "Bull"/.test(e.message))!.message
    )
  })
})

describe('plain text is still a global search', () => {
  // Free text has no operator, so it must not be dragged into the structured
  // path by the stricter parse -- multi-word global search is the common case.
  it('multi-word text becomes a global contains', () => {
    const tree = parseQuery('Red Bull Racing', fields) as any
    expect(tree.query.field.searchScope).toBe('global')
    expect(tree.query.value).toBe('Red Bull Racing')
  })

  it('structured queries with logical operators still parse whole', () => {
    const tree = parseQuery(
      'http.method = GET AND service.name = "Red Bull Racing"',
      fields
    ) as any
    expect(tree.type).toBe('group')
  })
})
