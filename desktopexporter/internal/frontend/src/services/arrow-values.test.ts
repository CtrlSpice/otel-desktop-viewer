import { describe, expect, it, vi } from 'vitest'
import {
  DenseUnion,
  Field,
  Float64,
  Int64,
  List,
  Map_,
  Struct,
  Utf8,
  makeData,
  makeVector,
  vectorFromArray,
} from 'apache-arrow'
import { ArrowCell, ArrowFrame } from './arrow-data'
import { attributeValue, rowViews } from './arrow-values'
import { logSummary } from './arrow-summaries'
import { attributeValueCopy } from '@/components/shared/detail-value'
import { logSummaryTable } from '@/test/arrow-fixtures'
import { tableValuePreview } from '@/components/shared/table-value-preview'

describe('Arrow value presentation', () => {
  it('counts folded containers and opens only the three preview children', () => {
    const vector = vectorFromArray(
      [Array.from({ length: 5000 }, (_, i) => BigInt(i))],
      new List(new Field('item', new Int64()))
    )
    const value = attributeValue(new ArrowCell(vector, 0))
    if (value.kind !== 'array') throw new Error('expected array')
    const reads = vi.spyOn(ArrowCell.prototype, 'isNull', 'get')
    expect(tableValuePreview(value, true)).toBe('array (5000 items)')
    expect(reads).toHaveBeenCalledTimes(1) // The list's validity, not its children.
    expect(tableValuePreview(value)).toBe('[0, 1, 2, … (+4997 items)]')
    expect(reads.mock.calls.length).toBeLessThan(30)
    reads.mockRestore()
    expect(value.value.at(-1)?.value).toBe(4999n)
  })
  it('creates summary references without reading or copying response fields', () => {
    const frame = new ArrowFrame(logSummaryTable())
    const read = vi.spyOn(frame, 'cell')
    const rows = rowViews(frame, logSummary)
    expect(read).not.toHaveBeenCalled()
    expect(rows[0].timestamp).toBe(9223372036854775807n)
    expect(read).toHaveBeenCalledWith('timestamp', 0)
    expect(rows[0].bodyPreview).toBe('received')
    expect(rows[0].bodyType).toBe('string')
    expect(read).not.toHaveBeenCalledWith('body', 0)
    expect(
      Object.getOwnPropertyDescriptor(rows[0], 'bodyPreview')?.get
    ).toBeDefined()
  })

  it('retains numeric kinds and reserved map keys through exact Copy', () => {
    const union = new DenseUnion(
      [2, 3],
      [new Field('int64', new Int64()), new Field('float64', new Float64())]
    )
    const floats = new Float64Array([1, -0, NaN])
    new DataView(floats.buffer).setBigUint64(16, 0x7ff8000000000042n, true)
    const values = makeData({
      type: union,
      length: 5,
      typeIds: new Int8Array([2, 3, 2, 3, 3]),
      valueOffsets: new Int32Array([0, 0, 1, 1, 2]),
      children: [
        vectorFromArray([1n, 9007199254740993n], new Int64()).data[0]!,
        makeData({ type: new Float64(), data: floats }),
      ],
    })
    const entries = new Struct([
      new Field('key', new Utf8(), false),
      new Field('value', union, true),
    ])
    const vector = makeVector(
      makeData({
        type: new Map_(new Field('entries', entries, false)),
        length: 1,
        valueOffsets: new Int32Array([0, 5]),
        child: makeData({
          type: entries,
          length: 5,
          children: [
            vectorFromArray(
              ['__proto__', 'constructor', 'large', 'zero', 'nan'],
              new Utf8()
            ).data[0]!,
            values,
          ],
        }),
      })
    )
    const value = attributeValue(new ArrowCell(vector, 0))
    expect(value.kind).toBe('map')
    if (value.kind !== 'map') throw new Error('expected map')
    expect(value.value[0].value).toEqual({ kind: 'int64', value: 1n })
    expect(value.value[1].value).toEqual({ kind: 'double', value: 1 })
    expect(attributeValueCopy(value)).toContain('9007199254740993')
    expect(attributeValueCopy(value)).toContain('"__proto__"')
    expect(Object.is(value.value[3].value.value, -0)).toBe(true)
    const nan = value.value[4].value.value
    expect(Number.isNaN(nan)).toBe(true)
    const out = new DataView(new ArrayBuffer(8))
    out.setFloat64(0, Number(nan))
    expect(out.getBigUint64(0)).toBe(0x7ff8000000000042n)
  })

  it('opens nested lists only on demand', () => {
    const nested = vectorFromArray(
      [[['雪']]],
      new List(new Field('item', new List(new Field('item', new Utf8()))))
    )
    const cell = new ArrowCell(nested, 0)
    const list = vi.spyOn(cell, 'list')
    const value = attributeValue(cell)
    expect(list).not.toHaveBeenCalled()
    expect(attributeValueCopy(value)).toContain('雪')
  })
})
