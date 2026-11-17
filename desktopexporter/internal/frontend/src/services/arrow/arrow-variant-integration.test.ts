import { describe, expect, it } from 'vitest'
import {
  Binary,
  DenseUnion,
  Dictionary,
  Field,
  Int8,
  Int64,
  List,
  RecordBatch,
  Schema,
  Struct,
  Table,
  Utf8,
  makeData,
  makeVector,
  tableToIPC,
  vectorFromArray,
} from 'apache-arrow'
import { ArrowFrame, decodeArrow } from './arrow-data'
import { listValues } from './arrow-record'
import { attributeValue, attributes, readArrowVariant } from './arrow-values'
import { attributeValueCopy } from '@/components/shared/Detail/detail-value'
import { tableValuePreview } from '@/components/shared/Tables/table-value-preview'
import {
  encodeVariant,
  variantColumn,
  variantField,
  variantStorageType,
  variantTable,
} from '@/test/variant-fixtures'
import type { AttributeFixture } from '@/test/telemetry-fixture-types'

function partsFrame(
  rows: ({ metadata: Uint8Array | null; value: Uint8Array | null } | null)[],
  extension: string | undefined = 'arrow.parquet.variant',
  type = variantStorageType
) {
  const field = new Field(
    'payload',
    type,
    true,
    new Map(
      extension === undefined ? [] : [['ARROW:extension:name', extension]]
    )
  )
  const data = makeData({
    type: new Struct([field]),
    length: rows.length,
    children: [vectorFromArray(rows, type).data[0]!],
  })
  return decodeArrow(
    tableToIPC(new Table(new RecordBatch(new Schema([field]), data))),
    BigInt(rows.length)
  )
}

describe('native Arrow Variant integration', () => {
  it('retains extension metadata through dictionary encoding, including null rows', () => {
    const type = new Dictionary(variantStorageType, new Int8(), 42)
    const field = new Field(
      'payload',
      type,
      true,
      variantField('payload').metadata
    )
    const data = makeData({
      type: new Struct([field]),
      length: 3,
      children: [
        makeData({
          type,
          length: 3,
          data: Int8Array.of(1, 0, 0),
          nullCount: 1,
          nullBitmap: Uint8Array.of(0b101),
          dictionary: makeVector(
            variantColumn([
              { kind: 'int64', value: 9007199254740993n },
              { kind: 'string', value: 'dictionary' },
            ])
          ),
        }),
      ],
    })
    const frame = decodeArrow(
      tableToIPC(new Table(new RecordBatch(new Schema([field]), data))),
      3n
    )
    expect(attributeValue(frame.cell('payload', 0))).toEqual({
      kind: 'string',
      value: 'dictionary',
    })
    expect(attributeValue(frame.cell('payload', 1))).toEqual({
      kind: 'empty',
      value: null,
    })
    expect(attributeValue(frame.cell('payload', 2))).toEqual({
      kind: 'int64',
      value: 9007199254740993n,
    })
  })

  it('retains row-specific dictionaries across batches, accessors and slices', () => {
    const first = variantTable([
      null,
      {
        kind: 'map',
        value: [
          {
            key: '__proto__',
            value: { kind: 'int64', value: '9007199254740993' },
          },
        ],
      },
    ])
    const second = variantTable([
      {
        kind: 'map',
        value: [
          { key: '雪', value: { kind: 'string', value: '\ufeffliteral' } },
        ],
      },
      { kind: 'double', value: -0 },
    ])
    const frame = decodeArrow(
      tableToIPC(new Table([...first.batches, ...second.batches])),
      4n
    )
    expect(attributeValue(frame.cell('value', 0))).toEqual({
      kind: 'empty',
      value: null,
    })
    expect(attributes(frame.cell('value', 1))[0]).toEqual({
      key: '__proto__',
      value: { kind: 'int64', value: 9007199254740993n },
    })
    expect(attributes(frame.accessor('value')(2))[0]).toEqual({
      key: '雪',
      value: { kind: 'string', value: '\ufeffliteral' },
    })
    const sliced = new ArrowFrame(frame.columns.slice(2))
    expect(attributes(sliced.cell('value', 0))[0]!.key).toBe('雪')
    expect(attributeValue(sliced.cell('value', 1))).toEqual({
      kind: 'double',
      value: -0,
    })
  })

  it('keeps extension metadata through nested structs, list items and union children', () => {
    const parts = encodeVariant({ kind: 'int64', value: '9007199254740993' })
    const records = new List(
      new Field('item', new Struct([variantField('payload')]))
    )
    const frame = decodeArrow(
      tableToIPC(
        new Table({
          records: vectorFromArray(
            [[{ payload: parts }, { payload: parts }]],
            records
          ),
          samples: vectorFromArray([[parts]], new List(variantField('item'))),
        })
      ),
      1n
    )
    const nested = new ArrowFrame(frame.cell('records', 0).list())
    expect(attributeValue(nested.cell('payload', 1))).toEqual({
      kind: 'int64',
      value: 9007199254740993n,
    })
    expect(listValues(frame.cell('samples', 0), attributeValue)).toEqual([
      { kind: 'int64', value: 9007199254740993n },
    ])

    const union = new DenseUnion(
      [4, 9],
      [new Field('integer', new Int64()), variantField('dynamic')]
    )
    const unions = makeVector(
      makeData({
        type: union,
        length: 2,
        typeIds: Int8Array.of(4, 9),
        valueOffsets: Int32Array.of(0, 0),
        children: [
          vectorFromArray([2n], new Int64()).data[0]!,
          variantColumn([{ kind: 'string', value: 'native' }]),
        ],
      })
    )
    const unionFrame = decodeArrow(
      tableToIPC(new Table({ values: unions })),
      2n
    )
    expect(attributeValue(unionFrame.cell('values', 0))).toEqual({
      kind: 'int64',
      value: 2n,
    })
    expect(attributeValue(unionFrame.cell('values', 1))).toEqual({
      kind: 'string',
      value: 'native',
    })
  })

  it('bounds previews and leaves unrequested children unread', () => {
    const frame = partsFrame([
      {
        metadata: Uint8Array.of(1, 0, 0),
        // [0, 1, 2, <unsupported>]
        value: Uint8Array.of(
          3,
          4,
          0,
          2,
          4,
          6,
          7,
          0x0c,
          0,
          0x0c,
          1,
          0x0c,
          2,
          0xfc
        ),
      },
    ])
    const value = attributeValue(frame.cell('payload', 0))
    if (value.kind !== 'array') throw new Error('Expected an array')
    expect(tableValuePreview(value, true)).toBe('array (4 items)')
    expect(tableValuePreview(value)).toBe('[0, 1, 2, … (+1 items)]')
    expect(value.previewChildren).toBe(value.previewChildren)
    expect(value.childAt!(0)).toBe(value.previewChildren![0])
    expect(Object.isFrozen(value.previewChildren)).toBe(true)
    expect(() => value.childAt!(3)).toThrow(
      'Unsupported Variant primitive type 63'
    )
    expect(() => value.value).toThrow('Unsupported Variant primitive type 63')
  })

  it('limits owner attributes and caches map entries independently of their values', () => {
    const frame = partsFrame([
      {
        metadata: Uint8Array.of(0x11, 2, 0, 1, 2, 0x61, 0x62),
        value: Uint8Array.of(2, 2, 0, 1, 0, 1, 2, 4, 0xfc),
      },
    ])
    const cell = frame.cell('payload', 0)
    expect(attributes(cell, 1)).toEqual([
      { key: 'a', value: { kind: 'bool', value: true } },
    ])
    const value = attributeValue(cell)
    if (value.kind !== 'map') throw new Error('Expected a map')
    const entry = value.childAt!(1)
    expect(entry.key).toBe('b')
    expect(value.childAt!(1)).toBe(entry)
    expect(value.value).toBe(value.value)
    expect(Object.isFrozen(value.value)).toBe(true)
    expect(() => entry.value).toThrow('Unsupported Variant primitive type 63')
  })

  it('preserves scalar types, binary data and double bits through presentation and Copy', () => {
    const nanBytes = new DataView(new ArrayBuffer(8))
    nanBytes.setBigUint64(0, 0x7ff8000000000042n, true)
    const frame = new ArrowFrame(
      variantTable([
        {
          kind: 'array',
          value: [
            { kind: 'int64', value: '9007199254740993' },
            { kind: 'int64', value: 1 },
            { kind: 'double', value: 1 },
            { kind: 'double', value: -0 },
            { kind: 'double', value: nanBytes.getFloat64(0, true) },
            { kind: 'bytes', value: 'AP8=' },
            { kind: 'bytes', value: '' },
            { kind: 'string', value: '' },
            { kind: 'empty', value: null },
            { kind: 'map', value: [] },
            { kind: 'array', value: [] },
          ],
        },
      ])
    )
    const value = attributeValue(frame.cell('value', 0))
    if (value.kind !== 'array') throw new Error('Expected an array')
    expect(value.value[0]).toEqual({ kind: 'int64', value: 9007199254740993n })
    expect(value.value[1]).toEqual({ kind: 'int64', value: 1n })
    expect(value.value[2]).toEqual({ kind: 'double', value: 1 })
    expect(value.value[3]).toEqual({ kind: 'double', value: -0 })
    const nan = value.value[4]!
    if (nan.kind !== 'double') throw new Error('Expected a double')
    nanBytes.setFloat64(0, nan.value, true)
    expect(nanBytes.getBigUint64(0, true)).toBe(0x7ff8000000000042n)
    expect(value.value.slice(5, 9)).toEqual([
      { kind: 'bytes', value: 'AP8=' },
      { kind: 'bytes', value: '' },
      { kind: 'string', value: '' },
      { kind: 'empty', value: null },
    ])
    expect(attributeValueCopy(value)).toContain('9007199254740993')
    expect(attributeValueCopy(value)).toContain('AP8=')
    expect(value.value).toBe(value.value)
    expect(value.childAt!(10)).toBe(value.value[10])
  })

  it('formats and copies 200 nested levels without changing the leaf', () => {
    let input: AttributeFixture = { kind: 'int64', value: '9007199254740993' }
    for (let level = 0; level < 200; level++)
      input =
        level % 2
          ? { kind: 'map', value: [{ key: 'child', value: input }] }
          : { kind: 'array', value: [input] }
    const frame = decodeArrow(tableToIPC(variantTable([input])), 1n)
    const value = attributeValue(frame.cell('value', 0))
    const text = attributeValueCopy(value)
    expect(text).toContain('9007199254740993')
    expect(text.match(/"child"/g)).toHaveLength(100)
    expect(tableValuePreview(value)).toContain('child')
  })

  it('distinguishes SQL null from malformed non-null storage', () => {
    const frame = partsFrame([
      null,
      { metadata: null, value: Uint8Array.of(0) },
      { metadata: Uint8Array.of(1, 0, 0), value: null },
    ])
    expect(attributeValue(frame.cell('payload', 0))).toEqual({
      kind: 'empty',
      value: null,
    })
    expect(attributes(frame.cell('payload', 0))).toEqual([])
    expect(() => readArrowVariant(frame.cell('payload', 1))).toThrow(
      'Expected Arrow Binary'
    )
    expect(() => readArrowVariant(frame.cell('payload', 2))).toThrow(
      'Expected Arrow Binary'
    )
  })

  it.each(['other.extension', ''])(
    'rejects extension %s instead of guessing its contents',
    extension => {
      const frame = partsFrame(
        [encodeVariant({ kind: 'bool', value: true })],
        extension
      )
      expect(() => attributeValue(frame.cell('payload', 0))).toThrow(
        'Expected Arrow Variant extension'
      )
    }
  )

  it('requires the extension annotation instead of treating any two binary fields as Variant', () => {
    const frame = new ArrowFrame(
      new Table({
        payload: vectorFromArray(
          [encodeVariant({ kind: 'bool', value: true })],
          variantStorageType
        ),
      })
    )
    expect(() => attributeValue(frame.cell('payload', 0))).toThrow(
      'Unsupported telemetry value type'
    )
  })

  it.each([
    new Struct([new Field('metadata', new Binary())]),
    new Struct([
      new Field('value', new Binary()),
      new Field('metadata', new Binary()),
    ]),
    new Struct([
      new Field('metadata', new Utf8()),
      new Field('value', new Binary()),
    ]),
  ])('rejects malformed extension storage %s', type => {
    const frame = partsFrame([null], 'arrow.parquet.variant', type)
    expect(() => attributeValue(frame.cell('payload', 0))).toThrow(
      'Invalid Arrow Variant storage'
    )
  })

  it('rejects unsupported versions and non-map owner attributes', () => {
    const frame = partsFrame([
      { metadata: Uint8Array.of(2, 0, 0), value: Uint8Array.of(0) },
    ])
    expect(() => attributeValue(frame.cell('payload', 0))).toThrow(
      'Unsupported Variant version 2'
    )
    const scalar = new ArrowFrame(variantTable([{ kind: 'bool', value: true }]))
    expect(() => attributes(scalar.cell('value', 0))).toThrow(
      'Expected Variant attributes map'
    )
  })
})
