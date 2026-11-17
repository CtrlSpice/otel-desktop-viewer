import { readFileSync } from 'node:fs'
import { expect, it } from 'vitest'
import { Table } from 'apache-arrow'
import { ArrowFrame, decodeArrow } from './arrow-data'
import { listValues } from './arrow-record'
import { attributeValue, attributes, readArrowVariant } from './arrow-values'
import { attributeValueCopy } from '@/components/shared/Detail/detail-value'
import type { AttributeValue } from '@/types/api-types'

const fixture = process.env.VARIANT_ARROW_FIXTURE

// Run through npm run test:variant, which creates fresh Go/DuckDB output first.
it.skipIf(!fixture)(
  'reads fresh Go → DuckDB → Arrow values across batches and nested fields',
  () => {
    if (!fixture) throw new Error('VARIANT_ARROW_FIXTURE is required')
    const scalar = new Map<string, AttributeValue>([
      ['empty', { kind: 'empty', value: null }],
      ['sql-null', { kind: 'empty', value: null }],
      ['true', { kind: 'bool', value: true }],
      ['false', { kind: 'bool', value: false }],
      ['text', { kind: 'string', value: '\ufeff雪\0literal' }],
      ['empty-string', { kind: 'string', value: '' }],
      ['double-one', { kind: 'double', value: 1 }],
      ['negative-zero', { kind: 'double', value: -0 }],
      ['nan', { kind: 'double', value: NaN }],
      ['positive-infinity', { kind: 'double', value: Infinity }],
      ['negative-infinity', { kind: 'double', value: -Infinity }],
      ['smallest-double', { kind: 'double', value: Number.MIN_VALUE }],
      ['largest-double', { kind: 'double', value: Number.MAX_VALUE }],
      ['bytes-0', { kind: 'bytes', value: '' }],
      ['bytes-3', { kind: 'bytes', value: 'AP+A' }],
    ])
    for (const integer of [
      0n,
      1n,
      -1n,
      -128n,
      127n,
      -129n,
      128n,
      -32768n,
      32767n,
      -32769n,
      32768n,
      -2147483648n,
      2147483647n,
      9007199254740993n,
      -9223372036854775808n,
      9223372036854775807n,
    ])
      scalar.set(`integer-${integer}`, { kind: 'int64', value: integer })
    const names = [
      ...scalar.keys(),
      'empty-map',
      'empty-array',
      'object',
      'wide-map',
      'deep-arrays',
      'deep-maps',
      'deep-mixed',
    ]
    const frame = decodeArrow(
      new Uint8Array(readFileSync(fixture)),
      BigInt(names.length + 9001)
    )
    if (!(frame.columns instanceof Table)) throw new Error('Expected a table')
    expect(frame.columns.batches.length).toBeGreaterThan(1)
    const rows = new Map<string, number>()
    for (let row = 0; row < names.length; row++)
      rows.set(frame.cell('name', row).text(), row)
    expect([...rows.keys()].sort()).toEqual([...names].sort())
    for (const [name, expected] of scalar) {
      const row = rows.get(name)!
      const cell = frame.cell('value', row)
      expect(attributeValue(cell), name).toEqual(expected)
      const nested = new ArrowFrame(frame.cell('nested', row).list())
      expect(nested.cell('timestamp', 0).integer()).toBe(18446744073709551615n)
      expect(attributeValue(nested.cell('payload', 0)), name).toEqual(expected)
      expect(
        listValues(frame.cell('samples', row), attributeValue),
        name
      ).toEqual([expected, { kind: 'empty', value: null }])
    }
    const nan = readArrowVariant(frame.cell('value', rows.get('nan')!))
    if (nan.kind !== 'double') throw new Error('Expected a double')
    expect(nan.bits).toBe(0x7ff8000000000042n)
    for (const kind of ['map', 'array'] as const) {
      const value = attributeValue(
        frame.cell('value', rows.get(`empty-${kind}`)!)
      )
      if (value.kind !== kind) throw new Error(`Expected ${kind}`)
      expect(value.childCount).toBe(0)
      expect(value.value).toEqual([])
    }
    const object = attributes(frame.cell('value', rows.get('object')!))
    const fields = new Map(object.map(entry => [entry.key, entry.value]))
    expect(fields.get('__proto__')).toEqual({
      kind: 'int64',
      value: 9007199254740993n,
    })
    expect(fields.get('constructor')).toEqual({ kind: 'double', value: 1 })
    expect(fields.get('')).toEqual({ kind: 'string', value: 'empty key' })
    expect(fields.get('a\0b')).toEqual({ kind: 'string', value: 'NUL key' })
    expect(fields.get('\ue000')).toEqual({ kind: 'string', value: 'BMP' })
    expect(fields.get('𐀀')).toEqual({ kind: 'string', value: 'non-BMP' })
    const items = fields.get('items')!
    if (items.kind !== 'array') throw new Error('Expected items array')
    expect(items.childCount).toBe(8)
    expect(items.value.slice(0, 5)).toEqual([
      { kind: 'int64', value: 1n },
      { kind: 'double', value: 1 },
      { kind: 'double', value: -0 },
      { kind: 'double', value: NaN },
      { kind: 'bytes', value: 'AP8=' },
    ])
    expect(items.value[5]).toMatchObject({ kind: 'map', value: [] })
    expect(items.value[6]).toMatchObject({ kind: 'array', value: [] })
    expect(items.value[7]).toEqual({ kind: 'empty', value: null })
    const wide = attributes(frame.cell('value', rows.get('wide-map')!))
    expect(wide).toHaveLength(300)
    wide.forEach((entry, index) => {
      expect(entry.key).toBe(`key-${String(index).padStart(3, '0')}`)
      expect(entry.value).toEqual({ kind: 'int64', value: BigInt(index) })
    })
    for (const nesting of ['arrays', 'maps', 'mixed']) {
      const root = attributeValue(
        frame.cell('value', rows.get(`deep-${nesting}`)!)
      )
      let child = root
      for (let level = 0; level < 200; level++) {
        if (nesting === 'maps' || (nesting === 'mixed' && level % 2 === 0)) {
          if (child.kind !== 'map') throw new Error('Expected a map')
          expect(child.childCount).toBe(2)
          expect(child.childAt!(1)).toEqual({
            key: 'flag',
            value: { kind: 'bool', value: false },
          })
          expect(child.childAt!(0).key).toBe('child')
          child = child.childAt!(0).value
        } else {
          if (child.kind !== 'array') throw new Error('Expected an array')
          expect(child.childCount).toBe(2)
          expect(child.childAt!(0)).toEqual({ kind: 'bool', value: false })
          child = child.childAt!(1)
        }
      }
      expect(child).toEqual({ kind: 'int64', value: 9007199254740993n })
      expect(attributeValueCopy(root)).toContain('9007199254740993')
    }
    for (let index = 0; index < 9001; index++) {
      const row = names.length + index
      expect(frame.cell('name', row).text()).toBe(`batch-${index}`)
      expect(attributeValue(frame.accessor('value')(row))).toEqual({
        kind: 'int64',
        value: BigInt(index),
      })
    }
  }
)
