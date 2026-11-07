const ESCAPES = new Map([
  ['n', '\n'],
  ['t', '\t'],
  ['r', '\r'],
  ['b', '\b'],
  ['f', '\f'],
])

/** SQL-style quotes, with backslash escapes for control characters. */
export function quoteQueryString(value: string): string {
  const body = JSON.stringify(value).slice(1, -1).replace(/\\"/g, '"')
  return `'${body.replace(/'/g, "''")}'`
}

export function unquoteQueryString(text: string): string {
  const quote = text[0]
  const body = text.slice(1, text.endsWith(quote) ? -1 : undefined)
  let out = ''
  for (let i = 0; i < body.length; i++) {
    const c = body[i]
    if (c === quote && body[i + 1] === quote) {
      out += c
      i++
    } else if (c !== '\\') out += c
    else {
      const next = body[++i]
      if (next === undefined) break
      if (next === 'u' && /^[0-9a-fA-F]{4}$/.test(body.slice(i + 1, i + 5))) {
        out += String.fromCharCode(parseInt(body.slice(i + 1, i + 5), 16))
        i += 4
      } else out += ESCAPES.get(next) ?? next
    }
  }
  return out
}
