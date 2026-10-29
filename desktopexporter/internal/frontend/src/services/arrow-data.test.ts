import { describe, expect, it } from 'vitest'
import {
  DataType,
  Decimal,
  DenseUnion,
  Field,
  Float64,
  Int64,
  Map_,
  Struct,
  Table,
  Utf8,
  makeData,
  makeVector,
  tableToIPC,
  vectorFromArray,
} from 'apache-arrow'
import { ArrowCell, ArrowFrame, decodeArrow } from './arrow-data'

function values() {
  const type = new DenseUnion(
    [2, 3],
    [new Field('int64', new Int64()), new Field('float64', new Float64())]
  )
  const floats = new Float64Array([1, -0, NaN])
  new DataView(floats.buffer).setBigUint64(16, 0x7ff8000000000042n, true)
  return makeData({
    type,
    length: 5,
    typeIds: new Int8Array([2, 3, 2, 3, 3]),
    valueOffsets: new Int32Array([0, 0, 1, 1, 2]),
    children: [
      vectorFromArray([1n, 9007199254740993n], new Int64()).data[0]!,
      makeData({ type: new Float64(), data: floats }),
    ],
  })
}

describe('Arrow-backed access', () => {
  it('keeps signed 128-bit arithmetic exact', () => {
    const vector = makeVector(
      makeData({
        type: new Decimal(0, 38, 128),
        length: 2,
        data: new Uint32Array([
          0, 0, 1, 0, 0xffffffff, 0xffffffff, 0xffffffff, 0xffffffff,
        ]),
      })
    )
    expect(new ArrowCell(vector, 0).integer()).toBe(1n << 64n)
    expect(new ArrowCell(vector, 1).integer()).toBe(-1n)
  })

  it('resolves union types across batches and sliced vectors without numeric coercion', () => {
    const data = values()
    const vector = makeVector([data, data])
    expect(new ArrowCell(vector, 5).integer()).toBe(1n)
    const integralDouble = new ArrowCell(vector, 6)
    expect(DataType.isFloat(integralDouble.type)).toBe(true)
    expect(integralDouble.number()).toBe(1)
    expect(new ArrowCell(vector.slice(5), 2).integer()).toBe(9007199254740993n)
    expect(() => new ArrowCell(vector, 2).number()).toThrow('exact JavaScript')
    expect(Object.is(new ArrowCell(vector, 8).number(), -0)).toBe(true)
    const nan = new ArrowCell(vector, 9)
    expect(Number.isNaN(nan.number())).toBe(true)
    const stored: Float64Array = nan.vector.data[0]!.values
    expect(
      new DataView(stored.buffer, stored.byteOffset).getBigUint64(
        nan.row * 8,
        true
      )
    ).toBe(0x7ff8000000000042n)
  })

  it('reads literal map keys through entry columns after IPC round-trip', () => {
    const entries = new Struct([
      new Field('key', new Utf8(), false),
      new Field('value', values().type, true),
    ])
    const map = makeVector(
      makeData({
        type: new Map_(new Field('entries', entries, false), false),
        length: 2,
        valueOffsets: new Int32Array([0, 2, 5]),
        child: makeData({
          type: entries,
          length: 5,
          children: [
            vectorFromArray(
              ['__proto__', 'constructor', 'large', 'zero', 'nan'],
              new Utf8()
            ).data[0]!,
            values(),
          ],
        }),
      })
    )
    const table = new Table({ attributes: map })
    const bytes = tableToIPC(table, 'stream')
    const frame = decodeArrow(bytes, 2n)
    const first = frame.cell('attributes', 0).map()
    expect(first.length).toBe(2)
    expect(first.cell('key', 0).text()).toBe('__proto__')
    expect(first.cell('value', 0).integer()).toBe(1n)
    expect(first.cell('key', 1).text()).toBe('constructor')
    const second = frame.cell('attributes', 1).map()
    expect(second.cell('value', 0).integer()).toBe(9007199254740993n)
    expect(Object.is(second.cell('value', 1).number(), -0)).toBe(true)
    expect(() => decodeArrow(bytes.subarray(0, bytes.length - 8), 2n)).toThrow(
      'Incomplete'
    )
    expect(() => decodeArrow(bytes, 3n)).toThrow('row count')
  })

  it('keeps nullable fields distinct from absent columns', () => {
    const frame = new ArrowFrame(
      new Table({ name: vectorFromArray([null, '雪'], new Utf8()) })
    )
    expect(frame.cell('name', 0).isNull).toBe(true)
    expect(frame.accessor('name')(1).text()).toBe('雪')
    expect(() => frame.column('missing')).toThrow('missing column')
  })
})
