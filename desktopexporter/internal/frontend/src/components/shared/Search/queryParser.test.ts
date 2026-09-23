import { describe, it, expect } from 'vitest'
import { parseQuery, parseSearchRequest, validateQuery } from './queryParser'
import { OPERATORS } from '../../../constants/operators'
import type { FieldDefinition } from '../../../constants/fields'
import type { QueryNode } from './queryTree'
import {
  formatAttributeFieldReference,
  parseAttributeFieldReference,
} from './attribute-field-reference'

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

const integerListFields: FieldDefinition[] = [
  {
    name: 'severityNumber',
    type: 'int64',
    searchScope: 'field',
    description: 'log severity number',
    operators: [OPERATORS.IN, OPERATORS.NOT_IN],
  },
]

const durationFields: FieldDefinition[] = [
  {
    name: 'duration',
    type: 'int64',
    searchScope: 'field',
    description: 'span duration',
    operators: [
      OPERATORS.EQUALS,
      OPERATORS.NOT_EQUALS,
      OPERATORS.GREATER_THAN,
      OPERATORS.GREATER_THAN_OR_EQUAL,
      OPERATORS.LESS_THAN,
      OPERATORS.LESS_THAN_OR_EQUAL,
      OPERATORS.IN,
      OPERATORS.NOT_IN,
    ],
  },
]

const durationAttributeFields: FieldDefinition[] = [
  {
    name: 'duration',
    type: 'string',
    searchScope: 'attribute',
    attributeScope: 'span',
    operators: [OPERATORS.EQUALS, OPERATORS.IN, OPERATORS.NOT_IN],
  },
]

const timestampListFields: FieldDefinition[] = [
  {
    name: 'timestamp',
    type: 'int64',
    searchScope: 'field',
    description: 'log timestamp',
    operators: [OPERATORS.IN, OPERATORS.NOT_IN],
  },
]

const numericEnumListFields: FieldDefinition[] = [
  {
    name: 'kindCode',
    type: 'int64',
    searchScope: 'field',
    description: 'received span kind number',
    operators: [OPERATORS.IN, OPERATORS.NOT_IN],
  },
  {
    name: 'statusCodeValue',
    type: 'int64',
    searchScope: 'field',
    description: 'received span status code number',
    operators: [OPERATORS.IN, OPERATORS.NOT_IN],
  },
]

const collidingIntegerFields: FieldDefinition[] = [
  ...integerListFields,
  {
    name: 'severityNumber',
    type: 'int64',
    searchScope: 'attribute',
    attributeScope: 'log',
    operators: [OPERATORS.IN, OPERATORS.NOT_IN],
  },
]

const mixedKindFields: FieldDefinition[] = [
  {
    name: 'same.key',
    type: 'int64',
    searchScope: 'attribute',
    attributeScope: 'span',
    operators: [OPERATORS.EQUALS, OPERATORS.GREATER_THAN],
  },
  {
    name: 'same.key',
    type: 'string',
    searchScope: 'attribute',
    attributeScope: 'span',
    operators: [OPERATORS.EQUALS, OPERATORS.CONTAINS],
  },
  {
    name: 'same.key',
    type: 'int64',
    searchScope: 'attribute',
    attributeScope: 'resource',
    operators: [OPERATORS.EQUALS],
  },
]

function expectCondition(
  node: QueryNode | null | undefined
): Extract<QueryNode, { type: 'condition' }> {
  if (!node || node.type !== 'condition') {
    throw new Error('Expected a condition query')
  }
  return node
}

function expectGroup(
  node: QueryNode | null | undefined
): Extract<QueryNode, { type: 'group' }> {
  if (!node || node.type !== 'group') {
    throw new Error('Expected a group query')
  }
  return node
}

function valueOf(input: string): string {
  return expectCondition(parseQuery(input, fields)).query.value
}

const caseDistinctAttributeFields: FieldDefinition[] = [
  {
    name: 'env',
    type: 'string',
    searchScope: 'attribute',
    attributeScope: 'resource',
    operators: [OPERATORS.EQUALS, OPERATORS.CONTAINS],
  },
  {
    name: 'Env',
    type: 'boolean',
    searchScope: 'attribute',
    attributeScope: 'span',
    operators: [OPERATORS.EQUALS],
  },
  {
    name: 'ENV',
    type: 'int64',
    searchScope: 'attribute',
    attributeScope: 'event',
    operators: [OPERATORS.EQUALS],
  },
]

describe('attribute key identity', () => {
  it.each([
    ['env', 'string', 'resource'],
    ['Env', 'boolean', 'span'],
    ['ENV', 'int64', 'event'],
  ] as const)('resolves %s with exact received casing', (name, type, scope) => {
    const condition = expectCondition(
      parseQuery(`${name} = value`, caseDistinctAttributeFields)
    )

    expect(condition.query.field).toMatchObject({
      name,
      type,
      searchScope: 'attribute',
      attributeScope: scope,
    })
  })

  it('does not depend on case-variant discovery order', () => {
    const reversed = [...caseDistinctAttributeFields].reverse()

    for (const availableFields of [caseDistinctAttributeFields, reversed]) {
      const condition = expectCondition(
        parseQuery('Env = true', availableFields)
      )
      expect(condition.query.field).toMatchObject({
        name: 'Env',
        type: 'boolean',
        attributeScope: 'span',
      })
    }
  })

  it('rejects a casing that has no exact attribute key', () => {
    expect(() =>
      parseQuery('eNv = value', caseDistinctAttributeFields)
    ).toThrow(/Unknown field: eNv/)
  })

  it('preserves exact casing for dotted attribute keys', () => {
    const condition = expectCondition(
      parseQuery('service.Env = prod', [
        {
          name: 'service.Env',
          type: 'string',
          searchScope: 'attribute',
          attributeScope: 'resource',
          operators: [OPERATORS.EQUALS],
        },
        {
          name: 'service.env',
          type: 'string',
          searchScope: 'attribute',
          attributeScope: 'resource',
          operators: [OPERATORS.EQUALS],
        },
      ])
    )

    expect(condition.query.field).toMatchObject({ name: 'service.Env' })
  })

  it('keeps built-in fields case-insensitive and ahead of collisions', () => {
    const native: FieldDefinition = {
      name: 'name',
      type: 'string',
      searchScope: 'field',
      description: 'span name',
      operators: [OPERATORS.EQUALS],
    }
    const collidingAttribute: FieldDefinition = {
      name: 'Name',
      type: 'boolean',
      searchScope: 'attribute',
      attributeScope: 'span',
      operators: [OPERATORS.EQUALS],
    }

    for (const availableFields of [
      [native, collidingAttribute],
      [collidingAttribute, native],
    ]) {
      expect(
        expectCondition(parseQuery('NAME = checkout', availableFields)).query
          .field
      ).toBe(native)
    }
  })
})

describe('explicit attribute identity', () => {
  it.each([
    ['attr(span, "same.key", int64) = 42', 'int64', 'span'],
    ['attr(span, "same.key", string) = "42"', 'string', 'span'],
    ['attr(resource, "same.key", int64) = 42', 'int64', 'resource'],
  ] as const)('preserves the selected tuple in %s', (input, type, scope) => {
    const condition = expectCondition(
      parseQuery(input, mixedKindFields, 'traces')
    )
    expect(condition.query.field).toMatchObject({
      name: 'same.key',
      type,
      searchScope: 'attribute',
      attributeScope: scope,
    })
  })

  it('does not depend on discovery availability, order, or duplicates', () => {
    const input = 'attr(span, "same.key", string) = "42"'
    const expected = expectCondition(parseQuery(input, [], 'traces')).query
      .field
    for (const available of [
      mixedKindFields,
      [...mixedKindFields].reverse(),
      [...mixedKindFields, mixedKindFields[1]],
    ]) {
      expect(
        expectCondition(parseQuery(input, available, 'traces')).query.field
      ).toEqual(expected)
    }
  })

  it('rejects a scope that the active signal cannot own', () => {
    expect(() =>
      parseQuery('attr(log, "same.key", string) = "42"', [], 'traces')
    ).toThrow(/Unknown field/)
  })

  it.each(['attr(log, "x", string)', 'attr(span, "x", decimal)'])(
    'does not reinterpret invalid explicit syntax as the bare key %s',
    reference => {
      const collidingAttribute: FieldDefinition = {
        name: reference,
        type: 'string',
        searchScope: 'attribute',
        attributeScope: 'span',
        operators: [OPERATORS.EQUALS],
      }
      expect(() =>
        parseQuery(`${reference} = "value"`, [collidingAttribute], 'traces')
      ).toThrow(/Unknown field/)
      expect(() => parseQuery(reference, [], 'traces')).toThrow(
        /Incomplete expression/
      )
      expect(validateQuery(reference, [], 'traces')).toEqual([
        expect.objectContaining({ message: 'Incomplete expression' }),
      ])
    }
  )

  it('deduplicates identical tuples before checking bare-name ambiguity', () => {
    const duplicate = [mixedKindFields[0], mixedKindFields[0]]
    expect(
      expectCondition(parseQuery('same.key = 42', duplicate, 'traces')).query
        .field
    ).toMatchObject({ type: 'int64', attributeScope: 'span' })
  })

  it('requires explicit text for a bare name with multiple tuples', () => {
    expect(() =>
      parseQuery('same.key = 42', mixedKindFields, 'traces')
    ).toThrow(
      'Ambiguous field: same.key. Select an explicit attribute scope and kind.'
    )
  })

  it.each([
    'quote"key',
    'back\\slash',
    'comma,key',
    'dot.key',
    'colon:key',
    'open(key',
    'close)key',
    'space key',
    'control\u0001key',
    '雪',
  ])('round-trips the exact quoted key %j', key => {
    const field = parseAttributeFieldReference(
      `attr(span, ${JSON.stringify(key)}, string)`,
      'traces'
    )
    if (!field) throw new Error('Expected an attribute field')
    const text = `${formatAttributeFieldReference(field)} = "value"`
    expect(
      expectCondition(parseQuery(text, [], 'traces')).query.field
    ).toMatchObject({ name: key, type: 'string', attributeScope: 'span' })
  })

  it.each([
    ['string', 'string'],
    ['int64', 'int64'],
    ['double', 'float64'],
    ['bool', 'boolean'],
    ['bytes', 'bytes'],
    ['empty', 'empty'],
    ['array', 'array'],
    ['map', 'map'],
  ] as const)('maps stored kind %s to frontend type %s', (kind, type) => {
    expect(
      parseAttributeFieldReference(`attr(span, "key", ${kind})`, 'traces')
    ).toMatchObject({ type })
  })
})

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
    const tree = expectCondition(parseQuery('Red Bull Racing', fields))
    expect(tree.query.field.searchScope).toBe('global')
    expect(tree.query.value).toBe('Red Bull Racing')
  })

  it('structured queries with logical operators still parse whole', () => {
    const tree = expectGroup(
      parseQuery(
        'http.method = GET AND service.name = "Red Bull Racing"',
        fields
      )
    )
    expect(tree.type).toBe('group')
  })
})

describe('LIMIT modifier syntax', () => {
  it('keeps a structured predicate and limit as separate request fields', () => {
    const request = parseSearchRequest('http.method = GET | LIMIT 25', fields)

    expect(request?.predicate?.type).toBe('condition')
    expect(request?.limit).toBe(25)
  })

  it('keeps multi-word free text intact before the modifier', () => {
    const request = parseSearchRequest('rate limit reached | LIMIT 50', fields)
    const predicate = expectCondition(request?.predicate)

    expect(predicate.query.field.searchScope).toBe('global')
    expect(predicate.query.value).toBe('rate limit reached')
    expect(request?.limit).toBe(50)
  })

  it('accepts a limit without a predicate', () => {
    expect(parseSearchRequest('| LIMIT 10', fields)).toEqual({
      predicate: null,
      limit: 10,
    })
  })

  it('accepts the lowercase keyword', () => {
    expect(parseSearchRequest('checkout | limit 7', fields)?.limit).toBe(7)
  })

  it('does not split a pipe inside an unquoted value', () => {
    const request = parseSearchRequest(
      'http.method = GET|POST | LIMIT 5',
      fields
    )
    const predicate = expectCondition(request?.predicate)

    expect(predicate.query.value).toBe('GET|POST')
    expect(request?.limit).toBe(5)
  })

  it.each([
    ['http.method = GET | LIMIT', /form \| LIMIT 100/],
    ['http.method = GET | LIM 10', /form \| LIMIT 100/],
    ['http.method = GET | LIMIT 0', /positive whole number/],
    ['http.method = GET | LIMIT -1', /positive whole number/],
    ['http.method = GET | LIMIT 1.5', /positive whole number/],
    ['http.method = GET | LIMIT 9007199254740992', /positive whole number/],
  ])('rejects invalid modifier %s', (input, message) => {
    expect(() => parseSearchRequest(input, fields)).toThrow(message)
    expect(
      validateQuery(input, fields).some(e => message.test(e.message))
    ).toBe(true)
  })

  it('rejects repeated modifiers', () => {
    const input = 'http.method = GET | LIMIT 10 | LIMIT 5'
    expect(() => parseSearchRequest(input, fields)).toThrow(/Only one LIMIT/)
    expect(validateQuery(input, fields)).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          message: 'Only one LIMIT modifier is allowed',
        }),
      ])
    )
  })

  it('does not silently drop a limit for predicate-only callers', () => {
    expect(() => parseQuery('http.method = GET | LIMIT 10', fields)).toThrow(
      /not available/
    )
  })
})

// The contract the Lezer unification changed, pinned. Each of these was
// either impossible or silently wrong under the hand-written parser.
describe('unified grammar contract', () => {
  it('preserves exact native integer list spellings and rejects fractions', () => {
    const query = expectCondition(
      parseQuery(
        'severityNumber IN [42, "42.0", 4.2e1, "9007199254740993"]',
        integerListFields
      )
    )
    expect(JSON.parse(query.query.value)).toEqual([
      '42',
      '42.0',
      '4.2e1',
      '9007199254740993',
    ])
    expect(() =>
      parseQuery('severityNumber IN ["42.5"]', integerListFields)
    ).toThrow(/exact signed 64-bit integer/)
  })

  it('accepts long exponent spellings by value without expanding them', () => {
    const query = expectCondition(
      parseQuery(
        'severityNumber IN [42e000000, 0e999999999999999999999]',
        integerListFields
      )
    )
    expect(JSON.parse(query.query.value)).toEqual([
      '42e000000',
      '0e999999999999999999999',
    ])
    expect(() =>
      parseQuery('severityNumber IN [1e999999]', integerListFields)
    ).toThrow(/exact signed 64-bit integer/)
  })

  it('accepts exact unsigned timestamp lists through uint64 max', () => {
    const query = expectCondition(
      parseQuery(
        'timestamp IN [9223372036854775808, 18446744073709551615]',
        timestampListFields
      )
    )
    expect(JSON.parse(query.query.value)).toEqual([
      '9223372036854775808',
      '18446744073709551615',
    ])
    for (const invalid of ['-1', '18446744073709551616', '1.5']) {
      expect(() =>
        parseQuery(`timestamp IN [${invalid}]`, timestampListFields)
      ).toThrow(/exact unsigned 64-bit integer/)
    }
  })

  it.each(['kindCode', 'statusCodeValue'])(
    'requires exact signed integer lists for %s',
    field => {
      expect(() =>
        parseQuery(`${field} IN [2.5]`, numericEnumListFields)
      ).toThrow(/exact signed 64-bit integer/)
    }
  )

  it('uses static provenance rather than a colliding attribute name', () => {
    expect(() =>
      parseQuery('severityNumber IN [42.5]', collidingIntegerFields)
    ).toThrow(/exact signed 64-bit integer/)

    const query = expectCondition(
      parseQuery('severityNumber IN [42.5]', collidingIntegerFields.slice(1))
    )
    expect(query.query.field).toMatchObject({
      searchScope: 'attribute',
      attributeScope: 'log',
    })
  })

  it('AND binds tighter than OR', () => {
    const group = expectGroup(
      parseQuery('body = a OR body = b AND body = c', contractFields)
    )
    expect(group.group.operator).toBe('OR')
    const [left, right] = group.group.children
    expectCondition(left)
    expect(expectGroup(right).group.operator).toBe('AND')
  })

  it('a bare NULL is the null check, carried as an explicit operator', () => {
    const q = expectCondition(parseQuery('body = NULL', contractFields))
    expect(q.query.operator.symbol).toBe('IS NULL')
    const q2 = expectCondition(parseQuery('body != nil', contractFields))
    expect(q2.query.operator.symbol).toBe('IS NOT NULL')
  })

  it('a quoted "NULL" is the literal string, not the null check', () => {
    const q = expectCondition(parseQuery('body = "NULL"', contractFields))
    expect(q.query.operator.symbol).toBe('=')
    expect(q.query.value).toBe('NULL')
  })

  it('array values travel as JSON, so quoted commas survive', () => {
    const q = expectCondition(
      parseQuery('statusCode IN ["a,b", "c"]', contractFields)
    )
    expect(JSON.parse(q.query.value)).toEqual(['a,b', 'c'])
  })

  it('=~ and !~ are the PromQL spellings of the regex operators', () => {
    const q = expectCondition(parseQuery('body =~ foo.*', contractFields))
    expect(q.query.operator.symbol).toBe('REGEXP')
    const q2 = expectCondition(parseQuery('body !~ foo.*', contractFields))
    expect(q2.query.operator.symbol).toBe('NOT REGEXP')
  })

  it('keyword operators are case-insensitive in lowercase form', () => {
    const q = expectCondition(parseQuery('body contains foo', contractFields))
    expect(q.query.operator.symbol).toBe('CONTAINS')
    const q2 = expectCondition(
      parseQuery('statusCode not in [a, b]', contractFields)
    )
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
    const q = expectCondition(
      parseQuery('body = http://example.com/x', contractFields)
    )
    expect(q.query.value).toBe('http://example.com/x')
    const q2 = expectCondition(
      parseQuery('body = "http://example.com/x?y=1"', contractFields)
    )
    expect(q2.query.value).toBe('http://example.com/x?y=1')
  })

  it('plain words are still a global text search', () => {
    const q = expectCondition(parseQuery('checkout latency', contractFields))
    expect(q.query.field.searchScope).toBe('global')
    expect(q.query.value).toBe('checkout latency')
  })
})

describe('native duration operands', () => {
  it.each([
    ['duration = 1.5h', '5400000000000'],
    ['DURATION = 1s', '1000000000'],
    ['duration >= "0.5ns"', '1'],
    ['duration < 0.499999999999999999ns', '0'],
    ['duration != 9007199254740993ns', '9007199254740993'],
  ])('serializes %s as exact nanoseconds', (input, expected) => {
    expect(expectCondition(parseQuery(input, durationFields)).query.value).toBe(
      expected
    )
  })

  it.each(['IN', 'NOT IN'])('%s preserves list order on the wire', operator => {
    const query = expectCondition(
      parseQuery(
        `duration ${operator} [1s, "0.5ns", 9007199254740993ns, 0s]`,
        durationFields
      )
    )

    expect(query.query.value).toBe('["1000000000","1","9007199254740993","0"]')
  })

  it('normalizes parseSearchRequest before producing its final wire value', () => {
    const request = parseSearchRequest(
      'duration IN [2m, 500ms] | LIMIT 5',
      durationFields
    )

    expect(expectCondition(request?.predicate).query.value).toBe(
      '["120000000000","500000000"]'
    )
    expect(request?.limit).toBe(5)
  })

  it.each([
    ['duration = -1ms', /Invalid duration:/],
    ['duration = [1s]', /Invalid duration:/],
    ['duration = 9223372036854775808ns', /Invalid duration:/],
    [
      'duration IN [1s, 9223372036854775808ns]',
      /Invalid duration at list element 2/,
    ],
    ['duration IN [1s, bad]', /Invalid duration at list element 2/],
  ])('rejects invalid or overflowing input in %s', (input, message) => {
    expect(() => parseQuery(input, durationFields)).toThrow(message)
    expect(validateQuery(input, durationFields)).toEqual([
      expect.objectContaining({ message: expect.stringMatching(message) }),
    ])
  })

  it('points validation at the invalid list element', () => {
    const input = 'duration IN [1s, bad, 2s]'
    expect(validateQuery(input, durationFields)).toEqual([
      {
        from: input.indexOf('bad'),
        to: input.indexOf('bad') + 3,
        message: 'Invalid duration at list element 2',
      },
    ])
  })

  it.each([
    ['duration IN [1s', /Invalid duration/],
    ['duration IN []', /nonempty list/],
    ['duration IN [NULL]', /NULL is not allowed/],
    ['duration IN [[1s], 2s]', /nested/],
  ])(
    'rejects malformed list syntax at the parser boundary: %s',
    (input, message) => {
      expect(() => parseQuery(input, durationFields)).toThrow(message)
      expect(validateQuery(input, durationFields).length).toBeGreaterThan(0)
    }
  )

  it.each([
    ['duration = NULL', 'IS NULL'],
    ['duration != nil', 'IS NOT NULL'],
  ])('keeps the operand-free null check %s', (input, operator) => {
    const query = expectCondition(parseQuery(input, durationFields))
    expect(query.query.operator.symbol).toBe(operator)
    expect(query.query.value).toBe('')
  })

  it('leaves an attribute named duration as text', () => {
    const scalar = expectCondition(
      parseQuery('duration = "not a duration"', durationAttributeFields)
    )
    const list = expectCondition(
      parseQuery('duration IN [1s, "not a duration"]', durationAttributeFields)
    )

    expect(scalar.query.value).toBe('not a duration')
    expect(list.query.value).toBe('["1s","not a duration"]')
  })

  it('leaves an explicitly selected duration attribute as text', () => {
    const query = expectCondition(
      parseQuery(
        'attr(span, "duration", string) = "not a duration"',
        durationFields,
        'traces'
      )
    )

    expect(query.query.field).toMatchObject({
      name: 'duration',
      type: 'string',
      searchScope: 'attribute',
      attributeScope: 'span',
    })
    expect(query.query.value).toBe('not a duration')
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
    const q = expectCondition(parseQuery(input, contractFields))
    expect(q.query.value).toBe(want)
  })

  it('free text starting with a keyword prefix stays free text', () => {
    const q = expectCondition(parseQuery('orderly android', contractFields))
    expect(q.query.field.searchScope).toBe('global')
    expect(q.query.value).toBe('orderly android')
  })

  it('mixed precedence with keyword-prefixed values', () => {
    const group = expectGroup(
      parseQuery(
        'body = Server OR body = Client AND body contains orders',
        contractFields
      )
    )
    expect(group.group.operator).toBe('OR')
    expect(expectGroup(group.group.children[1]).group.operator).toBe('AND')
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
      const q = expectCondition(parseQuery(input, contractFields))
      expect(q.query.field.searchScope, input).toBe('global')
      expect(q.query.value).toBe(input)
      expect(validateQuery(input, contractFields)).toEqual([])
    }
  })

  it('grouped conditions are still structured', () => {
    expectGroup(parseQuery('(body = a OR body = b)', contractFields))
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

  it.each(['IN', 'NOT IN'])('%s requires a nonempty list', operator => {
    for (const value of ['one', '"one"', '[]']) {
      const input = `statusCode ${operator} ${value}`
      expect(() => parseQuery(input, contractFields)).toThrow(/require.*list/i)
      expect(validateQuery(input, contractFields).length).toBeGreaterThan(0)
    }
  })

  it('a bare word inside a structured query gets an operator hint, not a quoting hint', () => {
    expect(() => parseQuery('body = 1 AND foo', contractFields)).toThrow(
      /field operator value/
    )
  })
})
