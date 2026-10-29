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
        get childCount() {
          return cell.map().length
        },
        get previewChildren() {
          return attributes(cell, 3)
        },
        get conflictingKeys() {
          return [...mapConflicts(cell.map())]
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
        get childCount() {
          return cell.list().length
        },
        get previewChildren() {
          const values = cell.list()
          return Array.from({ length: Math.min(values.length, 3) }, (_, row) =>
            attributeValue(new ArrowCell(values, row))
          )
        },
      }
  }
}

export function attributes(cell: ArrowCell, limit = Infinity): Attribute[] {
  if (cell.isNull) return []
  const entries = cell.map()
  let conflicts: Set<string> | undefined
  return Array.from({ length: Math.min(entries.length, limit) }, (_, row) => ({
    get key() {
      return entries.cell('key', row).text()
    },
    get value() {
      return attributeValue(entries.cell('value', row))
    },
    get hasConflict() {
      conflicts ??= mapConflicts(entries)
      return conflicts.has(entries.cell('key', row).text())
    },
  }))
}

function mapConflicts(frame: ArrowFrame): Set<string> {
  const first = new Map<string, number>()
  const conflicts = new Set<string>()
  for (let row = 0; row < frame.length; row++) {
    const key = frame.cell('key', row).text()
    const previous = first.get(key)
    if (previous === undefined) first.set(key, row)
    else if (
      !conflicts.has(key) &&
      !equalValue(frame.cell('value', previous), frame.cell('value', row))
    )
      conflicts.add(key)
  }
  return conflicts
}

/** Compare only duplicate-key values, preserving numeric kinds and NaN bits. */
function equalValue(left: ArrowCell, right: ArrowCell): boolean {
  const kind = valueKind(left)
  if (kind !== valueKind(right)) return false
  switch (kind) {
    case 'empty':
      return true
    case 'string':
      return left.text() === right.text()
    case 'int64':
      return left.integer() === right.integer()
    case 'bool':
      return left.boolean() === right.boolean()
    case 'double': {
      const bits = new DataView(new ArrayBuffer(16))
      bits.setFloat64(0, left.number())
      bits.setFloat64(8, right.number())
      return bits.getBigUint64(0) === bits.getBigUint64(8)
    }
    case 'bytes': {
      const a = left.bytes(),
        b = right.bytes()
      return a.length === b.length && a.every((byte, i) => byte === b[i])
    }
    case 'array': {
      const a = left.list(),
        b = right.list()
      if (a.length !== b.length) return false
      for (let i = 0; i < a.length; i++)
        if (!equalValue(new ArrowCell(a, i), new ArrowCell(b, i))) return false
      return true
    }
    case 'map': {
      const a = left.map(),
        b = right.map()
      if (a.length !== b.length) return false
      for (let i = 0; i < a.length; i++)
        if (
          a.cell('key', i).text() !== b.cell('key', i).text() ||
          !equalValue(a.cell('value', i), b.cell('value', i))
        )
          return false
      return true
    }
  }
}
