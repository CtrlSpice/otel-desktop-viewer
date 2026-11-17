export const rawJsonFormatLimits = {
  characters: 2 * 1024 * 1024,
  depth: 64,
} as const

/** Change whitespace only. Numeric and string tokens are never decoded or re-encoded.
 * If indentation exceeds the preview budget, return the intact source instead. */
export function formatRawJSON(raw: string): string {
  if (raw.length > rawJsonFormatLimits.characters) return raw
  const parts: string[] = []
  let characters = 0
  let depth = 0
  let quoted = false
  let escaped = false
  const append = (text: string) => {
    characters += text.length
    parts.push(text)
  }
  const newline = () => append('\n' + '  '.repeat(depth))
  for (let i = 0; i < raw.length; i++) {
    if (characters > rawJsonFormatLimits.characters) return raw
    const char = raw[i]!
    if (quoted) {
      append(char)
      if (escaped) escaped = false
      else if (char === '\\') escaped = true
      else if (char === '"') quoted = false
      continue
    }
    if (/\s/.test(char)) continue
    if (char === '"') {
      quoted = true
      append(char)
    } else if (char === '{' || char === '[') {
      append(char)
      depth++
      if (depth > rawJsonFormatLimits.depth) return raw
      newline()
    } else if (char === '}' || char === ']') {
      depth = Math.max(0, depth - 1)
      newline()
      append(char)
    } else if (char === ',') {
      append(char)
      newline()
    } else if (char === ':') append(': ')
    else append(char)
  }
  return characters > rawJsonFormatLimits.characters ? raw : parts.join('')
}
