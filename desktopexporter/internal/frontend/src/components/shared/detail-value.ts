import type { AttributeValue } from '@/types/api-types'

export type ValueToken = {
  text: string
  kind: 'key' | 'string' | 'number' | 'literal' | 'punctuation'
  type?: AttributeValue['kind']
}

/** Colour bounded display summaries without parsing or changing their text. */
export function compactValueTokens(text: string): ValueToken[] {
  const parts = text.matchAll(
    /"(?:[^"\\]|\\.)*"|[^{}[\],:"\s][^{}[\],:"]*(?=: )|[{}[\],:]|\s+|[^{}[\],:"\s]+|"/g
  )
  return Array.from(parts, part => {
    const lexeme = part[0]
    const key = /^\s*:/.test(text.slice(part.index + lexeme.length))
    return {
      text: lexeme,
      kind: key
        ? 'key'
        : lexeme.startsWith('"')
          ? 'string'
          : /^-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?$/.test(lexeme)
            ? 'number'
            : /^(true|false|null)$/.test(lexeme)
              ? 'literal'
              : 'punctuation',
    }
  })
}

export type ValueContainer = {
  kind: 'container'
  id: number
  path: string
  opening: ValueToken
  closing: ValueToken
  children: ValueNode[]
  count: number
}
export type ValueNode = ValueToken | ValueContainer

/** Group already-formatted tokens for folding, retaining every source lexeme. */
export function valueTokenTree(
  tokens: ValueToken[],
  path: string
): ValueNode[] {
  let cursor = 0
  function read(path: string, map: boolean, root = false): ValueNode[] {
    const nodes: ValueNode[] = []
    let key = ''
    let index = 0
    while (cursor < tokens.length) {
      const token = tokens[cursor]
      if (token.text === '}' || token.text === ']') break
      const id = cursor++
      if (token.text === '{' || token.text === '[') {
        const childPath = root
          ? path
          : map
            ? `${path}.${key}`
            : `${path}[${index}]`
        const children = read(childPath, token.text === '{')
        const closing = tokens[cursor++]
        nodes.push({
          kind: 'container',
          id,
          path: childPath,
          opening: token,
          closing,
          children,
          count: children.filter(child =>
            token.text === '{'
              ? child.kind === 'key'
              : child.kind !== 'key' && child.kind !== 'punctuation'
          ).length,
        })
        index++
      } else {
        nodes.push(token)
        if (token.kind === 'key') key = token.text.slice(1, -1)
        else if (token.kind !== 'punctuation') index++
      }
    }
    return nodes
  }
  // The outer token list is a single value, not an array entry.
  return read(path, false, true)
}

/** Recognize only complete JSON containers; format lexemes, never parsed numbers. */
export function jsonContainerTokens(text: string): ValueToken[] | null {
  if (!/^\s*[[{]/.test(text)) return null
  try {
    JSON.parse(text)
  } catch {
    return null
  }
  // Validation above means every non-whitespace character belongs to a token.
  // Keeping source lexemes preserves duplicate keys, large integers and -0.
  const parts = text.match(/"(?:[^"\\]|\\.)*"|[{}[\],:]|[^\s{}[\],:]+/g)!
  const tokens: ValueToken[] = []
  let depth = 0
  const punctuation = (text: string) =>
    tokens.push({ text, kind: 'punctuation' })
  const newline = () => punctuation('\n' + '  '.repeat(depth))
  parts.forEach((part, index) => {
    if (part === '{' || part === '[') {
      punctuation(part)
      depth++
      if (parts[index + 1] !== '}' && parts[index + 1] !== ']') newline()
    } else if (part === '}' || part === ']') {
      depth--
      if (parts[index - 1] !== '{' && parts[index - 1] !== '[') newline()
      punctuation(part)
    } else if (part === ',') {
      punctuation(part)
      newline()
    } else if (part === ':') {
      punctuation(': ')
    } else {
      tokens.push({
        text: part,
        kind: part.startsWith('"')
          ? parts[index + 1] === ':'
            ? 'key'
            : 'string'
          : /^(true|false|null)$/.test(part)
            ? 'literal'
            : 'number',
      })
    }
  })
  return tokens
}

/** Names and prose use text styling; recognizable IDs and numeric strings do not. */
export function isReadableText(text: string, path?: string): boolean {
  return (
    text.trim().length > 0 &&
    !/(?:^|[._])(?:id|.*Id|.*ID)$/.test(path ?? '') &&
    !/^(?:[+-]?(?:\d+(?:\.\d*)?|\.\d+)(?:[eE][+-]?\d+)?|[\da-f]{16,}|[\da-f]{8}(?:-[\da-f]{4}){3}-[\da-f]{12})$/i.test(
      text.trim()
    )
  )
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
