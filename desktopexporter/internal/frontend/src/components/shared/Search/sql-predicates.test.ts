import { describe, expect, it } from 'vitest'
import { getStaticFieldsForSearch } from '@/constants/fields'
import { queryText } from '@/components/home/home-search'
import { parseSearchRequest, validateQuery } from './queryParser'
import { quoteQueryString } from './query-literals'

const fields = getStaticFieldsForSearch('traces')

describe('SQL-shaped telemetry predicates', () => {
  it('lowers inclusive duration ranges into the existing Boolean tree', () => {
    const parsed = parseSearchRequest(
      "WHERE duration BETWEEN 500ms AND 2s OR name LIKE 'GET %' LIMIT 7",
      fields,
      'traces'
    )
    expect(parsed?.limit).toBe(7)
    expect(parsed?.predicate).toMatchObject({
      type: 'group',
      group: {
        operator: 'OR',
        children: [
          {
            type: 'group',
            group: {
              operator: 'AND',
              children: [
                { query: { operator: { symbol: '>=' }, value: '500000000' } },
                { query: { operator: { symbol: '<=' }, value: '2000000000' } },
              ],
            },
          },
          { query: { operator: { symbol: 'LIKE' }, value: 'GET %' } },
        ],
      },
    })
  })

  it.each([
    ['attributes["x"] = 9007199254740993', '', 'int64', '9007199254740993'],
    ['attributes["x"] = "42"', '', 'string', '42'],
    ['event.attributes["x"] = true', 'event', 'boolean', 'true'],
    ['resource.attributes["x"]::DOUBLE = 42', 'resource', 'float64', '42'],
  ])('preserves scope and received type in %s', (text, owner, type, value) => {
    expect(parseSearchRequest(text, fields, 'traces')?.predicate).toMatchObject(
      {
        type: 'condition',
        query: { field: { owner, type, path: ['attributes', 'x'] }, value },
      }
    )
  })

  it('keeps one numeric type for both unscoped BETWEEN bounds', () => {
    const node = parseSearchRequest(
      'attributes["latency"] BETWEEN 1 AND 1.5',
      fields
    )?.predicate
    if (node?.type !== 'group') throw new Error('Expected a range')
    for (const child of node.group.children)
      expect(child).toMatchObject({
        query: { field: { type: 'float64', owner: '' } },
      })
  })

  it.each(["O'Reilly", 'a\\"b', "a\\'b", 'first\nsecond', '雪', ''])(
    'round-trips SQL quoted strings %j',
    value => {
      const node = parseSearchRequest(
        `name = ${quoteQueryString(value)}`,
        fields
      )?.predicate
      expect(node).toMatchObject({ query: { value } })
      expect(
        parseSearchRequest(queryText(node ?? null), fields)?.predicate
      ).toMatchObject({ query: { value } })
    }
  )

  it('keeps complete plain text even when it contains the word limit', () => {
    expect(
      parseSearchRequest('rate limit reached', fields)?.predicate
    ).toMatchObject({ type: 'text', query: { value: 'rate limit reached' } })
  })

  it.each([
    'name = (a, b)',
    'WHERE name IS 42',
    'WHERE duration BETWEEN 1s AND bad',
    "WHERE name = 'x'; DROP TABLE spans",
  ])('rejects invalid predicates: %s', text => {
    expect(() => parseSearchRequest(text, fields)).toThrow()
    expect(validateQuery(text, fields).length).toBeGreaterThan(0)
  })
})
