import { describe, expect, it } from 'vitest'
import {
  compareByOptionalBigintField,
  compareByStringField,
  compareByTimestampField,
} from './compare'

type Row = { name?: string; time?: bigint }

describe('compareByStringField', () => {
  const rows: Row[] = [{ name: 'charlie' }, { name: 'alice' }, { name: 'bob' }]

  it('sorts ascending by the picked field', () => {
    const sorted = [...rows].sort((a, b) =>
      compareByStringField(a, b, r => r.name)
    )
    expect(sorted.map(r => r.name)).toEqual(['alice', 'bob', 'charlie'])
  })

  it('returns 0 for equal values', () => {
    expect(
      compareByStringField({ name: 'alice' }, { name: 'alice' }, r => r.name)
    ).toBe(0)
  })

  it('treats a missing field as an empty string, sorting it first', () => {
    const sorted = [{ name: 'alice' }, {}, { name: 'bob' }].sort((a, b) =>
      compareByStringField(a, b, r => r.name)
    )
    expect(sorted.map(r => r.name)).toEqual([undefined, 'alice', 'bob'])
  })
})

describe('compareByTimestampField', () => {
  const rows: Row[] = [{ time: 30n }, { time: 10n }, { time: 20n }]

  it('sorts ascending by the picked bigint field', () => {
    const sorted = [...rows].sort((a, b) =>
      compareByTimestampField(a, b, r => r.time)
    )
    expect(sorted.map(r => r.time)).toEqual([10n, 20n, 30n])
  })

  it('returns 0 for equal values', () => {
    expect(
      compareByTimestampField({ time: 10n }, { time: 10n }, r => r.time)
    ).toBe(0)
  })

  it('returns a negative number when a is smaller', () => {
    expect(
      compareByTimestampField({ time: 10n }, { time: 20n }, r => r.time)
    ).toBeLessThan(0)
  })

  it('returns a positive number when a is larger', () => {
    expect(
      compareByTimestampField({ time: 20n }, { time: 10n }, r => r.time)
    ).toBeGreaterThan(0)
  })

  it('treats a missing field as 0n', () => {
    expect(compareByTimestampField({}, { time: 10n }, r => r.time)).toBe(-1)
    expect(compareByTimestampField({ time: 10n }, {}, r => r.time)).toBe(1)
  })
})

describe('compareByOptionalBigintField', () => {
  it('sorts ascending by the picked field with defined rows first', () => {
    const rows: Row[] = [{ time: 30n }, {}, { time: 10n }]
    const sorted = [...rows].sort((a, b) =>
      compareByOptionalBigintField(a, b, r => r.time)
    )
    expect(sorted.map(r => r.time)).toEqual([10n, 30n, undefined])
  })

  it('returns 0 when both values are undefined', () => {
    expect(compareByOptionalBigintField<Row>({}, {}, r => r.time)).toBe(0)
  })

  it('returns 0 for equal defined values', () => {
    expect(
      compareByOptionalBigintField({ time: 10n }, { time: 10n }, r => r.time)
    ).toBe(0)
  })

  it('sorts a missing value after a defined value', () => {
    expect(compareByOptionalBigintField({ time: 10n }, {}, r => r.time)).toBe(
      -1
    )
    expect(compareByOptionalBigintField({}, { time: 10n }, r => r.time)).toBe(1)
  })

  it('returns a negative number when a is smaller', () => {
    expect(
      compareByOptionalBigintField({ time: 10n }, { time: 20n }, r => r.time)
    ).toBe(-1)
  })

  it('returns a positive number when a is larger', () => {
    expect(
      compareByOptionalBigintField({ time: 20n }, { time: 10n }, r => r.time)
    ).toBe(1)
  })
})
