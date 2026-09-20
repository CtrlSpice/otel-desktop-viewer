import { describe, expect, it } from 'vitest'
import { parseQuery } from '@/components/shared/Search/queryParser'
import { OPERATORS, getOperatorsForFieldType } from './operators'
import { getStaticFieldsForSearch, type FieldDefinition } from './fields'

const staticFieldNames = {
  traces: [
    'resource.droppedAttributesCount',
    'scope.name',
    'scope.version',
    'scope.droppedAttributesCount',
    'traceID',
    'traceState',
    'spanID',
    'parentSpanID',
    'name',
    'kind',
    'kindCode',
    'startTime',
    'endTime',
    'duration',
    'droppedAttributesCount',
    'droppedEventsCount',
    'droppedLinksCount',
    'statusCode',
    'statusCodeValue',
    'statusMessage',
    'event.name',
    'event.timestamp',
    'event.droppedAttributesCount',
    'flags',
    'link.flags',
    'link.traceID',
    'link.spanID',
    'link.traceState',
    'link.droppedAttributesCount',
  ],
  logs: [
    'resource.droppedAttributesCount',
    'scope.name',
    'scope.version',
    'scope.droppedAttributesCount',
    'timestamp',
    'observedTimestamp',
    'traceID',
    'spanID',
    'severityText',
    'severityNumber',
    'body',
    'droppedAttributesCount',
    'flags',
    'eventName',
  ],
  metrics: [
    'resource.droppedAttributesCount',
    'scope.name',
    'scope.version',
    'scope.droppedAttributesCount',
    'name',
    'description',
    'unit',
    'type',
  ],
} as const

describe('static scalar membership contract', () => {
  for (const signal of ['traces', 'logs', 'metrics'] as const) {
    it(`${signal} exposes and parses IN and NOT IN for every static field`, () => {
      const fields = getStaticFieldsForSearch(signal)
      const namedFields = fields.filter(
        (field): field is Extract<FieldDefinition, { searchScope: 'field' }> =>
          field.searchScope === 'field'
      )
      expect(namedFields.map(field => field.name)).toEqual(
        staticFieldNames[signal]
      )

      for (const field of namedFields) {
        const symbols = field.operators.map(operator => operator.symbol)
        expect(
          symbols.filter(symbol => symbol === 'IN'),
          field.name
        ).toHaveLength(1)
        expect(
          symbols.filter(symbol => symbol === 'NOT IN'),
          field.name
        ).toHaveLength(1)

        for (const operator of ['IN', 'NOT IN']) {
          const node = parseQuery(`${field.name} ${operator} ["0"]`, fields)
          expect(node?.type, `${field.name} ${operator}`).toBe('condition')
        }
      }
    })
  }

  it('does not advertise the unsupported metric received field', () => {
    expect(
      getStaticFieldsForSearch('metrics').some(
        field => field.searchScope === 'field' && field.name === 'received'
      )
    ).toBe(false)
  })

  it('keeps received OTel arrays on contains operators only', () => {
    expect(getOperatorsForFieldType('array')).toEqual([
      OPERATORS.CONTAINS,
      OPERATORS.NOT_CONTAINS,
    ])
  })
})

describe('membership operand shape', () => {
  const categories = [
    ['text', 'statusCode', getStaticFieldsForSearch('traces')],
    ['native integer', 'severityNumber', getStaticFieldsForSearch('logs')],
    ['duration', 'duration', getStaticFieldsForSearch('traces')],
    ['wire ID', 'traceID', getStaticFieldsForSearch('traces')],
  ] as const

  it.each(categories)(
    'rejects malformed %s lists',
    (_category, name, fields) => {
      for (const operator of ['IN', 'NOT IN']) {
        for (const operand of ['value', '[]', '[[value]]', '[value, NULL]']) {
          expect(() =>
            parseQuery(`${name} ${operator} ${operand}`, fields)
          ).toThrow()
        }
      }
    }
  )

  it('rejects scalar membership for dynamic scalar attributes', () => {
    const fields: FieldDefinition[] = [
      {
        name: 'custom.value',
        type: 'string',
        searchScope: 'attribute',
        attributeScope: 'span',
        operators: getOperatorsForFieldType('string'),
      },
    ]

    expect(() => parseQuery('custom.value IN value', fields)).toThrow(
      /require a list/
    )
  })

  it('rejects scalar membership operators for received arrays', () => {
    const fields: FieldDefinition[] = [
      {
        name: 'custom.items',
        type: 'array',
        searchScope: 'attribute',
        attributeScope: 'span',
        operators: getOperatorsForFieldType('array'),
      },
    ]

    for (const operator of ['IN', 'NOT IN']) {
      expect(() =>
        parseQuery(`custom.items ${operator} [value]`, fields)
      ).toThrow(/not valid/)
    }
  })
})
