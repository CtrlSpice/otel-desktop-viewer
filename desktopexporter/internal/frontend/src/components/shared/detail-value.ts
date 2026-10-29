import type { AttributeValue } from '@/types/api-types'

export type ValueToken = {
  text: string
  kind: 'key' | 'string' | 'number' | 'literal' | 'punctuation'
  type?: AttributeValue['kind']
}

/** JSON-like runtime dumps can contain these bare values. Display them as code,
 * never coerce them into JSON values or evaluate the received text. */
export const isRuntimeLiteral = (text: string) =>
  /^(?:undefined|NaN|-?Infinity)$/.test(text)
const isLiteral = (text: string) =>
  /^(?:true|false|null)$/.test(text) || isRuntimeLiteral(text)

/** Display-only JSON; copy continues to use the original received value. */
export function structuredValueText(value: AttributeValue): string | undefined {
  const tokens =
    value.kind === 'string'
      ? jsonContainerTokens(value.value, true)
      : value.kind === 'array' || value.kind === 'map'
        ? attributeValueTokens(value, true)
        : null
  return tokens?.map(token => token.text).join('')
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
        const decoded: unknown = JSON.parse(lexeme)
        // Server previews can end inside a container. Tokenize that prefix as
        // a preview, without inventing missing closing quotes or brackets.
        // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Only decoded JSON strings can supply an embedded preview; the received token remains authoritative.
        if (typeof decoded === 'string' && /^\s*[[{]/.test(decoded))
          return compactValueTokens(decoded, true)
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
              : isLiteral(lexeme)
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

/** Recognize complete containers; formatted display also accepts runtime literals.
 * Validation uses a throwaway JSON-compatible copy; output always uses source
 * lexemes, retaining exact numbers, duplicate keys and non-JSON literal spelling. */
type FormattingBudget = { characters: number; tokens: number }
const formattingBudget = (): FormattingBudget => ({
  characters: 1_048_576,
  tokens: 65_536,
})

export function jsonContainerTokens(
  text: string,
  expandStrings = false,
  budget = formattingBudget(),
  baseDepth = 0
): ValueToken[] | null {
  // Embedded JSON is an optional display enhancement, not a reason to expand
  // a small received string into gigabytes of indentation on the UI thread.
  const maxCharacters = 1_048_576
  const maxDepth = 64
  const limit = Symbol('formatting limit')
  function format(text: string, baseDepth: number): ValueToken[] | null {
    if (
      budget.characters <= 0 ||
      budget.tokens <= 0 ||
      text.length > maxCharacters ||
      !/^\s*[[{]/.test(text)
    )
      return null
    try {
      JSON.parse(
        expandStrings
          ? text.replace(
              /"(?:[^"\\]|\\.)*"|\bundefined\b|\bNaN\b|-?\bInfinity\b/g,
              token => (isRuntimeLiteral(token) ? 'null' : token)
            )
          : text
      )
    } catch {
      return null
    }
    // Validation above means every non-whitespace character belongs to a token.
    // Keeping source lexemes preserves duplicate keys, large integers and -0.
    const parts = text.match(/"(?:[^"\\]|\\.)*"|[{}[\],:]|[^\s{}[\],:]+/g)!
    const tokens: ValueToken[] = []
    let depth = baseDepth
    const append = (token: ValueToken) => {
      budget.characters -= token.text.length
      if (budget.characters < 0 || --budget.tokens < 0) throw limit
      tokens.push(token)
    }
    const punctuation = (text: string) => append({ text, kind: 'punctuation' })
    const newline = () => punctuation('\n' + '  '.repeat(depth))
    parts.forEach((part, index) => {
      if (part === '{' || part === '[') {
        punctuation(part)
        depth++
        if (depth > maxDepth) throw limit
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
          const decoded: unknown = JSON.parse(part)
          const nested =
            // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Validate parsed string tokens before recursive display formatting; preserve source lexemes for all other values.
            typeof decoded === 'string' ? format(decoded, depth) : null
          if (nested) {
            // Nested formatting already includes the parent indentation and
            // consumes the same budget; do not copy/reindent it a second time.
            for (const token of nested) tokens.push(token)
            return
          }
        }
        append({
          text: part,
          kind: part.startsWith('"')
            ? parts[index + 1] === ':'
              ? 'key'
              : 'string'
            : isLiteral(part)
              ? 'literal'
              : 'number',
        })
      }
    })
    return tokens
  }
  try {
    return format(text, baseDepth)
  } catch (error) {
    if (error === limit) return null
    throw error
  }
}

/** Format received values directly: do not round int64s or merge duplicate keys. */
export function attributeValueTokens(
  value: AttributeValue,
  expandStrings = false
): ValueToken[] {
  const tokens: ValueToken[] = []
  const budget = formattingBudget()
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
          const nested = jsonContainerTokens(value.value, true, budget, depth)
          if (nested) {
            for (const token of nested) tokens.push(token)
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
