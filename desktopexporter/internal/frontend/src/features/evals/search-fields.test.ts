import { describe, expect, it } from 'vitest'
import {
  parseSearchRequest,
  validateQuery,
} from '@/components/shared/Search/queryParser'
import { convertQueryTreeForBackend } from '@/services/telemetry-service'
import { evalSearchDefinition } from './search-fields'

const fields = evalSearchDefinition.fields

describe('Evals shared search grammar', () => {
  it('retains Boolean groups, phrases, typed operands and the result limit', () => {
    const request = parseSearchRequest(
      '(result.score >= 0.5 or result.success = false) and search("exact phrase") limit 25',
      fields
    )
    expect(request?.limit).toBe(25)
    expect(
      request?.predicate && convertQueryTreeForBackend(request.predicate)
    ).toMatchObject({
      type: 'group',
      group: {
        logicalOperator: 'AND',
        children: [
          {
            type: 'group',
            group: {
              logicalOperator: 'OR',
              children: [
                {
                  query: {
                    field: {
                      owner: 'result',
                      path: ['score'],
                      type: 'float64',
                    },
                    fieldOperator: '>=',
                    value: '0.5',
                  },
                },
                {
                  query: {
                    field: {
                      owner: 'result',
                      path: ['success'],
                      type: 'boolean',
                    },
                    fieldOperator: '=',
                    value: 'false',
                  },
                },
              ],
            },
          },
          {
            type: 'text',
            query: {
              fieldOperator: 'CONTAINS',
              value: 'exact phrase',
            },
          },
        ],
      },
    })
  })

  it('keeps exact integer operands as text through transport', () => {
    const request = parseSearchRequest(
      'result.ordinal = 9007199254740993',
      fields
    )
    expect(
      request?.predicate && convertQueryTreeForBackend(request.predicate)
    ).toMatchObject({ query: { value: '9007199254740993' } })
  })

  it.each(['result.score contains "x"', 'unknown = 1', 'name ='])(
    'reports shared validation errors for %s',
    text => {
      expect(() => parseSearchRequest(text, fields)).toThrow()
      expect(validateQuery(text, fields).length).toBeGreaterThan(0)
    }
  )

  it.each(evalSearchDefinition.examples)(
    'accepts help example: $label',
    ({ query }) => {
      expect(parseSearchRequest(query, fields)?.predicate).toBeTruthy()
      expect(validateQuery(query, fields)).toEqual([])
    }
  )
})
