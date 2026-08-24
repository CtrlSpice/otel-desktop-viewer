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

const contractFields: FieldDefinition[] = [
  ...fields,
  {
    name: 'statusCode',
    type: 'string',
    searchScope: 'field',
    description: 'span status code',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.IN,
      OPERATORS.NOT_IN,
    ],
  },
  {
    name: 'body',
    type: 'string',
    searchScope: 'field',
    description: 'log body',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.CONTAINS,
      OPERATORS.NOT_CONTAINS,
      OPERATORS.REGEX,
    ],
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
    'http.method = GET ',
  ])('%s yields GET', input => {
    expect(valueOf(input)).toBe('GET')
  })

  // Backticks are not a quote style. The old hand-written lexer accepted
  // them; the grammar never did, so the editor underlined what the parser
  // accepted. The grammar is the language now, and it has two quote styles.
  it('backticks are rejected, not treated as quotes', () => {
    expect(() => parseQuery('http.method = `GET`', fields)).toThrow()
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

// The contract the Lezer unification changed, pinned. Each of these was
// either impossible or silently wrong under the hand-written parser.
describe('unified grammar contract', () => {
  it('AND binds tighter than OR', () => {
    const n: any = parseQuery(
      'body = a OR body = b AND body = c',
      contractFields
    )
    expect(n.type).toBe('group')
    expect(n.group.operator).toBe('OR')
    const [left, right] = n.group.children
    expect(left.type).toBe('condition')
    expect(right.type).toBe('group')
    expect(right.group.operator).toBe('AND')
  })

  it('a bare NULL is the null check, carried as an explicit operator', () => {
    const q: any = parseQuery('body = NULL', contractFields)
    expect(q.query.operator.symbol).toBe('IS NULL')
    const q2: any = parseQuery('body != nil', contractFields)
    expect(q2.query.operator.symbol).toBe('IS NOT NULL')
  })

  it('a quoted "NULL" is the literal string, not the null check', () => {
    const q: any = parseQuery('body = "NULL"', contractFields)
    expect(q.query.operator.symbol).toBe('=')
    expect(q.query.value).toBe('NULL')
  })

  it('array values travel as JSON, so quoted commas survive', () => {
    const q: any = parseQuery('statusCode IN ["a,b", "c"]', contractFields)
    expect(JSON.parse(q.query.value)).toEqual(['a,b', 'c'])
  })

  it('=~ and !~ are the PromQL spellings of the regex operators', () => {
    const q: any = parseQuery('body =~ foo.*', contractFields)
    expect(q.query.operator.symbol).toBe('REGEXP')
    const q2: any = parseQuery('body !~ foo.*', contractFields)
    expect(q2.query.operator.symbol).toBe('NOT REGEXP')
  })

  it('keyword operators are case-insensitive in lowercase form', () => {
    const q: any = parseQuery('body contains foo', contractFields)
    expect(q.query.operator.symbol).toBe('CONTAINS')
    const q2: any = parseQuery('statusCode not in [a, b]', contractFields)
    expect(q2.query.operator.symbol).toBe('NOT IN')
  })

  it('a NOT typo is an error, never a silent free-text search', () => {
    // The old parser submitted this as a global text search for the literal
    // string, while the editor underlined it as an error. The two now agree
    // that a query using the language and failing to parse is an error.
    expect(() => parseQuery('body NOT 5', contractFields)).toThrow()
    expect(validateQuery('body NOT 5', contractFields).length).toBeGreaterThan(
      0
    )
  })

  it('URLs work as unquoted values, up to an equals sign', () => {
    // ':' '/' '?' '&' are all value characters now (the old lexer stopped at
    // ':'). '=' cannot be: with no-space comparisons like http.method=GET in
    // the language, an '=' inside an unquoted value would be indistinguishable
    // from the operator. A URL with query parameters needs quotes.
    const q: any = parseQuery('body = http://example.com/x', contractFields)
    expect(q.query.value).toBe('http://example.com/x')
    const q2: any = parseQuery(
      'body = "http://example.com/x?y=1"',
      contractFields
    )
    expect(q2.query.value).toBe('http://example.com/x?y=1')
  })

  it('plain words are still a global text search', () => {
    const q: any = parseQuery('checkout latency', contractFields)
    expect(q.query.field.searchScope).toBe('global')
    expect(q.query.value).toBe('checkout latency')
  })
})

// Words that merely start with a keyword must stay words. AND/OR are
// specialized from Word -- exact-text match -- rather than standalone tokens,
// because a token above Word in the precedence list overrides longest-match
// in Lezer: "orders" lexed as Or + "ders", and every value, field, or free
// text beginning with "or"/"and"/"in"/"not" shattered. Found live, not by
// unit tests, because no fixture value happened to start with a keyword.
describe('keyword-prefixed words', () => {
  it.each([
    ['body = orders', 'orders'],
    ['body = android', 'android'],
    ['body = information', 'information'],
    ['body = nothing', 'nothing'],
    ['body = ANDES', 'ANDES'],
  ])('%s keeps the value intact', (input, want) => {
    const q: any = parseQuery(input, contractFields)
    expect(q.query.value).toBe(want)
  })

  it('free text starting with a keyword prefix stays free text', () => {
    const q: any = parseQuery('orderly android', contractFields)
    expect(q.query.field.searchScope).toBe('global')
    expect(q.query.value).toBe('orderly android')
  })

  it('mixed precedence with keyword-prefixed values', () => {
    const n: any = parseQuery(
      'body = Server OR body = Client AND body contains orders',
      contractFields
    )
    expect(n.group.operator).toBe('OR')
    expect(n.group.children[1].group.operator).toBe('AND')
  })
})

// Review findings on the unification, pinned. Each of these was a regression
// or a silent misbehavior the reviewer caught before merge.
describe('review findings', () => {
  it('parenthetical free text is a global search, not an error', () => {
    // The grammar eagerly parses a leading "(" as a Group; counting Groups
    // as structure turned every parenthetical remark into a parse error.
    for (const input of [
      '(error)',
      '(retrying) connection failed',
      '(500) internal error',
    ]) {
      const q: any = parseQuery(input, contractFields)
      expect(q.query.field.searchScope, input).toBe('global')
      expect(q.query.value).toBe(input)
      expect(validateQuery(input, contractFields)).toEqual([])
    }
  })

  it('grouped conditions are still structured', () => {
    const n: any = parseQuery('(body = a OR body = b)', contractFields)
    expect(n.type).toBe('group')
  })

  it('a lone operator is still an error, not free text', () => {
    expect(() => parseQuery('= 5', contractFields)).toThrow()
    expect(validateQuery('= 5', contractFields).length).toBeGreaterThan(0)
  })

  it('nested arrays are rejected, not silently stringified', () => {
    expect(() => parseQuery('statusCode IN [[a],b]', contractFields)).toThrow(
      /nested/i
    )
    expect(
      validateQuery('statusCode IN [[a],b]', contractFields).length
    ).toBeGreaterThan(0)
  })

  it('a bare word inside a structured query gets an operator hint, not a quoting hint', () => {
    expect(() => parseQuery('body = 1 AND foo', contractFields)).toThrow(
      /field operator value/
    )
  })
})
