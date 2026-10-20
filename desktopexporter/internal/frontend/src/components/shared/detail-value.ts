import type { AttributeValue } from '@/types/api-types'

export type ValueToken = {
  text: string
  kind: 'key' | 'string' | 'number' | 'literal' | 'punctuation'
  type?: AttributeValue['kind']
}

/** Colour bounded previews; optionally decode embedded container strings for display. */
export function compactValueTokens(
  text: string,
  expandStrings = false
): ValueToken[] {
  const parts = text.matchAll(
    /"(?:[^"\\]|\\.)*(?:"|$)|[^{}[\],:"\s][^{}[\],:"]*(?=: )|[{}[\],:]|\s+|[^{}[\],:"\s]+|"/g
  )
  return Array.from(parts).flatMap((part): ValueToken[] => {
    const lexeme = part[0]
    const key = /^\s*:/.test(text.slice(part.index + lexeme.length))
    if (expandStrings && !key && lexeme.startsWith('"')) {
      try {
        const decoded = JSON.parse(lexeme) as string
        // Server previews can end inside a container. Tokenize that prefix as
        // a preview, without inventing missing closing quotes or brackets.
        if (/^\s*[[{]/.test(decoded)) return compactValueTokens(decoded, true)
      } catch {
        // An incomplete quoted token stays as received.
      }
    }
    return [
      {
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
      },
    ]
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
export function jsonContainerTokens(
  text: string,
  expandStrings = false
): ValueToken[] | null {
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
      // Decode only string values containing complete containers. Keep keys,
      // scalar strings and invalid JSON as received. This is display-only;
      // copy/export callers use the default, lossless mode.
      if (expandStrings && part.startsWith('"') && parts[index + 1] !== ':') {
        const nested = jsonContainerTokens(JSON.parse(part) as string, true)
        if (nested) {
          for (const token of indentTokens(nested, depth)) tokens.push(token)
          return
        }
      }
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

function indentTokens(tokens: ValueToken[], depth: number): ValueToken[] {
  const indent = '  '.repeat(depth)
  return tokens.map(token =>
    token.kind === 'punctuation'
      ? { ...token, text: token.text.replaceAll('\n', `\n${indent}`) }
      : token
  )
}

/** Format received values directly: do not round int64s or merge duplicate keys. */
export function attributeValueTokens(
  value: AttributeValue,
  expandStrings = false
): ValueToken[] {
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
        if (expandStrings) {
          const nested = jsonContainerTokens(value.value, true)
          if (nested) {
            for (const token of indentTokens(nested, depth)) tokens.push(token)
            break
          }
        }
        tokens.push({
          text: JSON.stringify(value.value),
          kind: 'string',
          type: value.kind,
        })
        break
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
