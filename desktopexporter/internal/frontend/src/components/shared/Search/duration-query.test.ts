import { describe, expect, it } from 'vitest'
import { OPERATORS, type Operator } from '@/constants/operators'
import type { QueryNode } from './queryTree'
import { normalizeDurationValues } from './duration-query'

function durationQuery(
  value: string,
  operator: Operator = OPERATORS.EQUALS
): Extract<QueryNode, { type: 'condition' }> {
  return {
    id: 'duration',
    type: 'condition',
    query: {
      field: {
        name: 'duration',
        type: 'int64',
        searchScope: 'field',
        description: 'Span duration in nanoseconds',
        operators: [OPERATORS.EQUALS, OPERATORS.IN, OPERATORS.NOT_IN],
      },
      operator,
      value,
    },
  }
}

describe('normalizeDurationValues', () => {
  it.each([
    ['1.5h', '5400000000000'],
    ['0.5ns', '1'],
    ['0.499999999999999999ns', '0'],
    ['0s', '0'],
  ])('normalizes %s exactly', (input, expected) => {
    const node = durationQuery(input)

    expect(normalizeDurationValues(node)).toBeNull()
    expect(node.query.value).toBe(expected)
  })

  it.each(['-1ms', '9223372036854775808ns'])('rejects %s', input => {
    expect(normalizeDurationValues(durationQuery(input))).toMatch(
      /^Invalid duration:/
    )
  })

  it('normalizes and transports a JSON string list byte-exactly', () => {
    const node = durationQuery('["1s","0.5ns","0s"]', OPERATORS.IN)

    expect(normalizeDurationValues(node)).toBeNull()
    expect(node.query.value).toBe('["1000000000","1","0"]')
  })

  it.each([
    ['malformed JSON', '["1s"', 'Invalid duration list'],
    ['empty', '[]', 'IN and NOT IN require a nonempty list'],
    ['nested', '[["1s"]]', 'Invalid duration at list element 1'],
    ['null', '[null]', 'Invalid duration at list element 1'],
    ['non-string', '[1]', 'Invalid duration at list element 1'],
    ['invalid member', '["1s","bad"]', 'Invalid duration at list element 2'],
  ])('rejects a %s list', (_, input, expected) => {
    expect(
      normalizeDurationValues(durationQuery(input, OPERATORS.NOT_IN))
    ).toBe(expected)
  })

  it('leaves an attribute named duration as a text operand', () => {
    const node = durationQuery('not a duration')
    node.query.field = {
      name: 'duration',
      type: 'string',
      searchScope: 'attribute',
      attributeScope: 'span',
      description: 'An attribute named duration',
      operators: [OPERATORS.EQUALS],
    }

    expect(normalizeDurationValues(node)).toBeNull()
    expect(node.query.value).toBe('not a duration')
  })
})
