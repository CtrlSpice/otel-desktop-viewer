import type { AttributeValue } from '@/types/api-types'
import type { FieldDefinition } from '@/search/model'
import { createPathFieldReference } from '@/components/shared/Search/attribute-field-reference'
import { ArrowCell, ArrowFrame } from './arrow-data'
import { columnView, listValues, number, text } from './arrow-record'
import { attributeValue, rowViews } from './arrow-values'

export type AttributeMatch = {
  name: string
  attributeScope: string
  owner: string
  path: readonly (string | number)[]
  type: string
  /** Distinct matching values, not the number of records. */
  matchCount: number
  sampleValues: AttributeValue[]
}

function path(cell: ArrowCell): (string | number)[] {
  return listValues(cell, part => {
    const fields = part.struct()
    const key = fields.cell('key', 0)
    return key.isNull ? fields.cell('index', 0).number() : key.text()
  })
}

export function attributeMatch(frame: ArrowFrame, row: number): AttributeMatch {
  return columnView<AttributeMatch>(frame, row, {
    name: text,
    attributeScope: text,
    owner: text,
    path,
    type: text,
    matchCount: number,
    sampleValues: cell => listValues(cell, attributeValue),
  })
}

export function attributeDefinitions(frame: ArrowFrame): FieldDefinition[] {
  return rowViews(frame, (frame, row) => {
    const segments = path(frame.cell('path', row))
    return createPathFieldReference(
      segments[0] === 'metadata' ? 'metadata' : frame.cell('owner', row).text(),
      segments,
      frame.cell('type', row).text()
    )
  }).filter((field): field is FieldDefinition => field !== null)
}

/** Preserve the existing completion spelling; native samples keep their bits. */
export function attributeSampleText(value: AttributeValue): string[] {
  if (value.kind === 'map' || value.kind === 'array') return []
  if (value.kind === 'empty') return ['null']
  if (value.kind === 'double') {
    const number = value.value
    if (!Number.isFinite(number) || Object.is(number, -0)) {
      const data = new DataView(new ArrayBuffer(8))
      data.setFloat64(0, number)
      const bits = Number.isNaN(number)
        ? 0x7ff8000000000000n
        : data.getBigUint64(0)
      return [`0x${bits.toString(16).padStart(16, '0')}`]
    }
  }
  return [String(value.value)]
}
