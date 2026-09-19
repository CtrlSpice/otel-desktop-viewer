import type { AttributeValue } from '@/types/api-types'

export function attributeValueLabel(value: AttributeValue): string {
  switch (value.kind) {
    case 'empty':
      return 'null'
    case 'string':
    case 'bytes':
    case 'bool':
    case 'int64':
    case 'double':
      return String(value.value)
    case 'array':
      return `[${value.value.map(attributeValueLabel).join(', ')}]`
    case 'map':
      return `{${value.value.map(entry => `${entry.key}: ${attributeValueLabel(entry.value)}`).join(', ')}}`
  }
}

export function attributeValueCanonical(value: AttributeValue): string {
  return `${value.kind}:${attributeValueLabel(value)}`
}
