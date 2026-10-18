import type { AttributeValue } from '@/types/api-types'

export type ValueToken = {
  text: string
  kind: 'key' | 'string' | 'number' | 'literal' | 'punctuation'
  type?: AttributeValue['kind']
}

/** Format received values directly: do not round int64s or merge duplicate keys. */
export function attributeValueTokens(value: AttributeValue): ValueToken[] {
  const tokens: ValueToken[] = []
  const punctuation = (text: string) =>
    tokens.push({ text, kind: 'punctuation' })
  function visit(value: AttributeValue, depth: number): void {
    switch (value.kind) {
      case 'array':
      case 'map': {
        const map = value.kind === 'map'
        const children = value.value
        punctuation(map ? '{' : '[')
        if (children.length > 0) {
          punctuation('\n')
          if (value.kind === 'map') {
            value.value.forEach((entry, index) => {
              punctuation('  '.repeat(depth + 1))
              tokens.push({ text: JSON.stringify(entry.key), kind: 'key' })
              punctuation(': ')
              visit(entry.value, depth + 1)
              punctuation(index < children.length - 1 ? ',\n' : '\n')
            })
          } else {
            value.value.forEach((child, index) => {
              punctuation('  '.repeat(depth + 1))
              visit(child, depth + 1)
              punctuation(index < children.length - 1 ? ',\n' : '\n')
            })
          }
          punctuation('  '.repeat(depth))
        }
        punctuation(map ? '}' : ']')
        break
      }
      case 'string':
      case 'bytes':
        tokens.push({
          text: JSON.stringify(value.value),
          kind: 'string',
          type: value.kind,
        })
        break
      case 'empty':
        tokens.push({ text: 'null', kind: 'literal', type: value.kind })
        break
      case 'bool':
        tokens.push({
          text: String(value.value),
          kind: 'literal',
          type: value.kind,
        })
        break
      case 'int64':
        tokens.push({
          text: value.value.toString(),
          kind: 'number',
          type: value.kind,
        })
        break
      case 'double':
        tokens.push({
          // JSON has no non-finite number literals. Keep their OTLP spelling
          // as a string instead of silently replacing the received value with null.
          text: Number.isFinite(value.value)
            ? Object.is(value.value, -0)
              ? '-0'
              : String(value.value)
            : JSON.stringify(String(value.value)),
          kind: Number.isFinite(value.value) ? 'number' : 'string',
          type: value.kind,
        })
    }
  }
  visit(value, 0)
  return tokens
}

export function attributeValueCopy(value: AttributeValue): string {
  if (value.kind === 'string' || value.kind === 'bytes') return value.value
  return attributeValueTokens(value)
    .map(token => token.text)
    .join('')
}

export function hasConflictingKeys(value: AttributeValue): boolean {
  if (value.kind === 'map') {
    return (
      Boolean(value.conflictingKeys?.length) ||
      value.value.some(entry => hasConflictingKeys(entry.value))
    )
  }
  return value.kind === 'array' && value.value.some(hasConflictingKeys)
}
