import { describe, expect, it } from 'vitest'
import { parseBigInt, parseNullableBigInt } from './bigint'

describe('parseBigInt', () => {
  it('returns a bigint input unchanged', () => {
    expect(parseBigInt(123n)).toBe(123n)
  })

  it('converts a numeric string to a bigint', () => {
    expect(parseBigInt('123')).toBe(123n)
  })

  it('converts a number to a bigint', () => {
    expect(parseBigInt(123)).toBe(123n)
  })

  it('throws for an invalid string', () => {
    expect(() => parseBigInt('not-a-number')).toThrow()
  })

  it('throws for null', () => {
    expect(() => parseBigInt(null)).toThrow('Invalid bigint value: null')
  })

  it('throws for undefined', () => {
    expect(() => parseBigInt(undefined)).toThrow(
      'Invalid bigint value: undefined'
    )
  })

  it('throws for a boolean', () => {
    expect(() => parseBigInt(true)).toThrow('Invalid bigint value: true')
  })
})

describe('parseNullableBigInt', () => {
  it('returns null for null', () => {
    expect(parseNullableBigInt(null)).toBeNull()
  })

  it('returns null for undefined', () => {
    expect(parseNullableBigInt(undefined)).toBeNull()
  })

  it('converts a numeric string to a bigint', () => {
    expect(parseNullableBigInt('123')).toBe(123n)
  })

  it('converts a number to a bigint', () => {
    expect(parseNullableBigInt(123)).toBe(123n)
  })

  it('returns a bigint input unchanged', () => {
    expect(parseNullableBigInt(123n)).toBe(123n)
  })

  it('throws for an invalid value', () => {
    expect(() => parseNullableBigInt('not-a-number')).toThrow()
  })
})
