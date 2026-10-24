/** Display-only escape ranges. Match escaped backslashes as one unit so a
 * literal backslash followed by n is not mistaken for a newline escape. */
export function jsonEscapeRanges(text: string): { from: number; to: number }[] {
  return Array.from(
    text.matchAll(/\\(?:u[\da-fA-F]{4}|["\\/bfnrt])/g),
    match => ({
      from: match.index,
      to: match.index + match[0].length,
    })
  )
}

export function splitJsonEscapes(text: string) {
  const parts: { text: string; escape: boolean }[] = []
  let from = 0
  for (const range of jsonEscapeRanges(text)) {
    if (range.from > from)
      parts.push({ text: text.slice(from, range.from), escape: false })
    parts.push({ text: text.slice(range.from, range.to), escape: true })
    from = range.to
  }
  if (from < text.length) parts.push({ text: text.slice(from), escape: false })
  return parts
}
