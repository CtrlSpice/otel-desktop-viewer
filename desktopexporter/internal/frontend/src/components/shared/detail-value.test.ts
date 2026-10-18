import { describe, expect, it } from 'vitest'
import {
  attributeValueCopy,
  attributeValueTokens,
  hasConflictingKeys,
} from './detail-value'
import type { AttributeValue } from '@/types/api-types'

describe('detail value formatting', () => {
  it('copies raw strings and bytes without presentation quotes or JSON coercion', () => {
    for (const kind of ['string', 'bytes'] as const) {
      expect(
        attributeValueCopy({ kind, value: '{"value":9007199254740993}' })
      ).toBe('{"value":9007199254740993}')
    }
    expect(
      attributeValueTokens({ kind: 'string', value: '{"a":1}' })
        .map(token => token.text)
        .join('')
    ).toBe('"{\\"a\\":1}"')
  })
  it('preserves int64 bounds without a Number conversion', () => {
    for (const value of [
      -9_223_372_036_854_775_808n,
      9_223_372_036_854_775_807n,
    ]) {
      expect(attributeValueCopy({ kind: 'int64', value })).toBe(String(value))
    }
  })
  it('outputs valid indented JSON for arrays, maps, booleans, bytes and null', () => {
    const value: AttributeValue = {
      kind: 'map',
      value: [
        {
          key: 'quoted"key',
          value: {
            kind: 'array',
            value: [
              { kind: 'bool', value: false },
              { kind: 'empty', value: null },
              { kind: 'bytes', value: 'AQI=' },
            ],
          },
        },
      ],
    }
    expect(attributeValueCopy(value)).toBe(
      JSON.stringify({ 'quoted"key': [false, null, 'AQI='] }, null, 2)
    )
    expect(JSON.parse(attributeValueCopy(value))).toEqual({
      'quoted"key': [false, null, 'AQI='],
    })
  })
  it('preserves empty containers, signed zero and non-finite double spellings', () => {
    expect(attributeValueCopy({ kind: 'map', value: [] })).toBe('{}')
    expect(attributeValueCopy({ kind: 'array', value: [] })).toBe('[]')
    expect(attributeValueCopy({ kind: 'double', value: -0 })).toBe('-0')
    for (const value of [NaN, Infinity, -Infinity]) {
      expect(attributeValueCopy({ kind: 'double', value })).toBe(
        JSON.stringify(String(value))
      )
    }
  })
  it('detects conflicts through nested arrays without merging the duplicate entries', () => {
    const value: AttributeValue = {
      kind: 'array',
      value: [
        {
          kind: 'map',
          conflictingKeys: ['same'],
          value: [
            { key: 'same', value: { kind: 'int64', value: 1n } },
            { key: 'same', value: { kind: 'string', value: '1' } },
          ],
        },
      ],
    }
    expect(hasConflictingKeys(value)).toBe(true)
    expect(attributeValueCopy(value)).toContain('"same": 1,\n    "same": "1"')
    expect(hasConflictingKeys({ kind: 'map', value: [] })).toBe(false)
  })
})
