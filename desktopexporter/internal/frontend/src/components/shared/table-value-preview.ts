import type { AttributeValue } from '@/types/api-types'

/** Display-only limits shared by nested table cells. Exact copy uses the value. */
function shorten(text: string, limit: number, withinLimit = false): string {
  const chars: string[] = []
  for (const char of text) {
    if (chars.length === limit) {
      if (withinLimit) chars.pop()
      return chars.join('') + '…'
    }
    chars.push(char)
  }
  return chars.join('')
}

export function tableValuePreview(
  value: AttributeValue,
  child = false
): string {
  switch (value.kind) {
    case 'empty':
      return 'null'
    case 'string': {
      const text = value.value
      return child
        ? JSON.stringify(shorten(text.replace(/[\t\n\f\r ]+/g, ' '), 48))
        : text === ''
          ? '""'
          : shorten(text, 300, true)
    }
    case 'bytes':
      return JSON.stringify(shorten(value.value, 48))
    case 'int64':
      return value.value.toString()
    case 'bool':
      return String(value.value)
    case 'double': {
      const number = value.value
      return Object.is(number, -0)
        ? '-0'
        : Number.isFinite(number)
          ? String(number)
          : JSON.stringify(String(number))
    }
    case 'map': {
      const count = value.childCount ?? value.value.length
      if (child) return `object (${count} fields)`
      const entries = value.previewChildren ?? value.value.slice(0, 3)
      const parts = entries.map(
        entry =>
          `${shorten(entry.key, 32)}: ${tableValuePreview(entry.value, true)}`
      )
      if (count > 3) parts.push(`… (+${count - 3} fields)`)
      return `{${parts.join(', ')}}`
    }
    case 'array': {
      const count = value.childCount ?? value.value.length
      if (child) return `array (${count} items)`
      const items = value.previewChildren ?? value.value.slice(0, 3)
      const parts = items.map(item => tableValuePreview(item, true))
      if (count > 3) parts.push(`… (+${count - 3} items)`)
      return `[${parts.join(', ')}]`
    }
  }
}
