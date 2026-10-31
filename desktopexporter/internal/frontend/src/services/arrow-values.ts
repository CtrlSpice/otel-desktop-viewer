import { DataType } from 'apache-arrow'
import type { Attribute, AttributeValue } from '@/types/api-types'
import { ArrowCell, type ArrowFrame } from './arrow-data'
import { cachedView, memoize } from './arrow-record'

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
  return cachedView(readAttributeValue(cell))
}

function attributeEntry(entries: ArrowFrame, row: number): Attribute {
  return cachedView({
    get key() {
      return entries.cell('key', row).text()
    },
    get value() {
      return attributeValue(entries.cell('value', row))
    },
  })
}

function readAttributeValue(cell: ArrowCell): AttributeValue {
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
    case 'map': {
      const entries = memoize(() => cell.map())
      const children = new Map<number, Attribute>()
      const childAt = (index: number): Attribute => {
        let child = children.get(index)
        if (!child) {
          child = attributeEntry(entries(), index)
          children.set(index, child)
        }
        return child
      }
      return {
        kind: 'map',
        childAt,
        get value() {
          return Array.from({ length: entries().length }, (_, i) => childAt(i))
        },
        get childCount() {
          return entries().length
        },
        get previewChildren() {
          return Array.from({ length: Math.min(entries().length, 3) }, (_, i) =>
            childAt(i)
          )
        },
      }
    }
    case 'array': {
      const values = memoize(() => cell.list())
      const children = new Map<number, AttributeValue>()
      const childAt = (index: number): AttributeValue => {
        let child = children.get(index)
        if (!child) {
          child = attributeValue(new ArrowCell(values(), index))
          children.set(index, child)
        }
        return child
      }
      return {
        kind: 'array',
        childAt,
        get value() {
          return Array.from({ length: values().length }, (_, i) => childAt(i))
        },
        get childCount() {
          return values().length
        },
        get previewChildren() {
          return Array.from({ length: Math.min(values().length, 3) }, (_, i) =>
            childAt(i)
          )
        },
      }
    }
  }
}

export function attributes(cell: ArrowCell, limit = Infinity): Attribute[] {
  if (cell.isNull) return []
  const entries = cell.map()
  return Array.from({ length: Math.min(entries.length, limit) }, (_, row) =>
    attributeEntry(entries, row)
  )
}
