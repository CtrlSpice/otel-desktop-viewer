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
import { attributeValue, bodyPreview, rowViews } from './arrow-values'
import { logSummary } from './arrow-summaries'
import { attributeValueCopy } from '@/components/shared/detail-value'
import { logSummaryTable } from '@/test/arrow-fixtures'

describe('Arrow value presentation', () => {
  it('creates summary references without reading or copying response fields', () => {
    const frame = new ArrowFrame(logSummaryTable())
    const read = vi.spyOn(frame, 'cell')
    const rows = rowViews(frame, logSummary)
    expect(read).not.toHaveBeenCalled()
    expect(rows[0].timestamp).toBe(9223372036854775807n)
    expect(read).toHaveBeenCalledWith('timestamp', 0)
    expect(rows[0].bodyPreview).toBe('received')
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
    expect(bodyPreview(new ArrowCell(vector, 0))).toBe(
      '{__proto__: 1, constructor: 1, large: 9007199254740993, … (+2 fields)}'
    )
  })

  it('bounds Unicode previews and opens nested lists only on demand', () => {
    const text = vectorFromArray(['雪'.repeat(301)], new Utf8())
    expect([...bodyPreview(new ArrowCell(text, 0))]).toHaveLength(300)
    const nested = vectorFromArray(
      [[['雪']]],
      new List(new Field('item', new List(new Field('item', new Utf8()))))
    )
    const cell = new ArrowCell(nested, 0)
    const list = vi.spyOn(cell, 'list')
    const value = attributeValue(cell)
    expect(list).not.toHaveBeenCalled()
    expect(bodyPreview(cell)).toBe('[array (1 items)]')
    expect(attributeValueCopy(value)).toContain('雪')
  })
})
