import { DataType } from 'apache-arrow'
import type { Attribute, AttributeValue } from '@/types/api-types'
import { ArrowCell, type ArrowFrame } from './arrow-data'

/** Allocate row references only; getters continue reading the retained vectors. */
export function rowViews<T>(
  frame: ArrowFrame,
  view: (frame: ArrowFrame, row: number) => T
): T[] {
  return Array.from({ length: frame.length }, (_, row) => view(frame, row))
}

export function valueKind(cell: ArrowCell): AttributeValue['kind'] {
  if (cell.isNull) return 'empty'
  if (DataType.isUtf8(cell.type)) return 'string'
  if (DataType.isInt(cell.type)) return 'int64'
  if (DataType.isFloat(cell.type)) return 'double'
  if (DataType.isBool(cell.type)) return 'bool'
  if (DataType.isBinary(cell.type)) return 'bytes'
  if (DataType.isMap(cell.type)) return 'map'
  if (DataType.isList(cell.type)) return 'array'
  throw new Error(`Unsupported telemetry value type: ${cell.type}`)
}

function base64(bytes: Uint8Array): string {
  const parts: string[] = []
  for (let i = 0; i < bytes.length; i += 8192)
    parts.push(String.fromCharCode(...bytes.subarray(i, i + 8192)))
  return btoa(parts.join(''))
}

/** Presentation adapter: containers open lazily, scalars retain their Arrow type. */
export function attributeValue(cell: ArrowCell): AttributeValue {
  switch (valueKind(cell)) {
    case 'empty':
      return { kind: 'empty', value: null }
    case 'string':
      return {
        kind: 'string',
        get value() {
          return cell.text()
        },
      }
    case 'int64':
      return {
        kind: 'int64',
        get value() {
          return cell.integer()
        },
      }
    case 'double':
      return {
        kind: 'double',
        get value() {
          return cell.number()
        },
      }
    case 'bool':
      return {
        kind: 'bool',
        get value() {
          return cell.boolean()
        },
      }
    case 'bytes':
      return {
        kind: 'bytes',
        get value() {
          return base64(cell.bytes())
        },
      }
    case 'map':
      return {
        kind: 'map',
        get value() {
          return attributes(cell)
        },
      }
    case 'array':
      return {
        kind: 'array',
        get value() {
          const values = cell.list()
          return Array.from({ length: values.length }, (_, row) =>
            attributeValue(new ArrowCell(values, row))
          )
        },
      }
  }
}

export function attributes(cell: ArrowCell): Attribute[] {
  if (cell.isNull) return []
  return rowViews(cell.map(), (frame, row) => ({
    get key() {
      return frame.cell('key', row).text()
    },
    get value() {
      return attributeValue(frame.cell('value', row))
    },
  }))
}

function shorten(
  text: string,
  length: number,
  ellipsisWithinLimit = false
): string {
  const chars: string[] = []
  for (const char of text) {
    if (chars.length === length) {
      if (ellipsisWithinLimit) chars.pop()
      return chars.join('') + '…'
    }
    chars.push(char)
  }
  return chars.join('')
}

function previewItem(cell: ArrowCell): string {
  switch (valueKind(cell)) {
    case 'empty':
      return 'empty'
    case 'string':
      return JSON.stringify(
        shorten(cell.text().replace(/[\t\n\f\r ]+/g, ' '), 48)
      )
    case 'bytes': {
      const bytes = cell.bytes()
      return `bytes (base64): ${base64(bytes.subarray(0, 36))}${bytes.length > 36 ? '…' : ''}`
    }
    case 'int64':
      return cell.integer().toString()
    case 'bool':
      return String(cell.boolean())
    case 'double': {
      const value = cell.number()
      if (Number.isFinite(value) && !Object.is(value, -0)) return String(value)
      const bits = new DataView(new ArrayBuffer(8))
      bits.setFloat64(0, value)
      return (
        '0x' +
        (Number.isNaN(value) ? 0x7ff8000000000000n : bits.getBigUint64(0))
          .toString(16)
          .padStart(16, '0')
      )
    }
    case 'map':
      return `object (${cell.map().length} fields)`
    case 'array':
      return `array (${cell.list().length} items)`
  }
}

/** Bounded summary: visit at most three children, never serialize the body. */
export function bodyPreview(cell: ArrowCell): string {
  switch (valueKind(cell)) {
    case 'string':
      return shorten(cell.text().replace(/[\t\n\f\r ]+/g, ' '), 300, true)
    case 'map': {
      const entries = cell.map()
      const items: string[] = []
      for (let row = 0; row < Math.min(3, entries.length); row++)
        items.push(
          `${shorten(entries.cell('key', row).text(), 32)}: ${previewItem(entries.cell('value', row))}`
        )
      return (
        '{' +
        items.join(', ') +
        (entries.length > 3 ? `, … (+${entries.length - 3} fields)` : '') +
        '}'
      )
    }
    case 'array': {
      const values = cell.list()
      const items: string[] = []
      for (let row = 0; row < Math.min(3, values.length); row++)
        items.push(previewItem(new ArrowCell(values, row)))
      return (
        '[' +
        items.join(', ') +
        (values.length > 3 ? `, … (+${values.length - 3} items)` : '') +
        ']'
      )
    }
    default:
      return previewItem(cell)
  }
}
