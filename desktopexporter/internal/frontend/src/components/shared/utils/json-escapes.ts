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

/** Only the delimiters of a quoted token, never an escaped closing quote. */
export function stringQuoteRanges(text: string): { from: number; to: number }[] {
  const quote = text[0]
  if (quote !== '"' && quote !== "'") return []
  const ranges = [{ from: 0, to: 1 }]
  const last = text.length - 1
  if (last > 0 && text[last] === quote) {
    let beforeSlashes = last - 1
    while (text[beforeSlashes] === '\\') beforeSlashes--
    if ((last - beforeSlashes - 1) % 2 === 0)
      ranges.push({ from: last, to: last + 1 })
  }
  return ranges
}

export function splitJsonString(text: string) {
  const parts: { text: string; kind: 'text' | 'escape' | 'quote' }[] = []
  const ranges = [
    ...jsonEscapeRanges(text).map(range => ({ ...range, kind: 'escape' as const })),
    ...stringQuoteRanges(text).map(range => ({ ...range, kind: 'quote' as const })),
  ].sort((a, b) => a.from - b.from)
  let from = 0
  for (const range of ranges) {
    if (range.from > from)
      parts.push({ text: text.slice(from, range.from), kind: 'text' })
    parts.push({ text: text.slice(range.from, range.to), kind: range.kind })
    from = range.to
  }
  if (from < text.length) parts.push({ text: text.slice(from), kind: 'text' })
  return parts
}
