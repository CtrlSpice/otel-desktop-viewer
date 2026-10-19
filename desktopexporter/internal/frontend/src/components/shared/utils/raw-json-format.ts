/** Change whitespace only. Numeric and string tokens are never decoded or re-encoded. */
export function formatRawJSON(raw: string): string {
  const parts: string[] = []
  let depth = 0
  let quoted = false
  let escaped = false
  const newline = () => parts.push('\n', '  '.repeat(depth))
  for (let i = 0; i < raw.length; i++) {
    const char = raw[i]!
    if (quoted) {
      parts.push(char)
      if (escaped) escaped = false
      else if (char === '\\') escaped = true
      else if (char === '"') quoted = false
      continue
    }
    if (/\s/.test(char)) continue
    if (char === '"') {
      quoted = true
      parts.push(char)
    } else if (char === '{' || char === '[') {
      parts.push(char)
      depth++
      newline()
    } else if (char === '}' || char === ']') {
      depth = Math.max(0, depth - 1)
      newline()
      parts.push(char)
    } else if (char === ',') {
      parts.push(char)
      newline()
    } else if (char === ':') parts.push(': ')
    else parts.push(char)
  }
  return parts.join('')
}
