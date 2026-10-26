import { JSONParser, TokenType } from '@streamparser/json'

type ParserToken = Parameters<JSONParser['onToken']>[0]
export type JSONFileToken = Omit<ParserToken, 'token' | 'value'> &
  (
    | { token: TokenType.STRING; value: string }
    | {
        token: Exclude<ParserToken['token'], TokenType.STRING>
        value: ParserToken['value']
      }
  )

/** Shared syntax scan. Offsets address the Blob; numeric token values are not storage values. */
export async function scanJSONFile(
  file: Blob,
  onToken: (token: JSONFileToken) => void,
  signal?: AbortSignal
): Promise<void> {
  const prefix = new Uint8Array(await file.slice(0, 3).arrayBuffer())
  const base =
    prefix[0] === 0xef && prefix[1] === 0xbb && prefix[2] === 0xbf ? 3 : 0
  const parser = new JSONParser({
    paths: [],
    keepStack: false,
    separator: '\n',
  })
  parser.onToken = token => {
    const offset = token.offset + base
    if (token.token === TokenType.STRING) {
      // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Check the third-party tokenizer's STRING/value invariant before publishing a discriminated token.
      if (typeof token.value !== 'string')
        throw new Error('Invalid JSON string token.')
      onToken({ ...token, token: TokenType.STRING, value: token.value, offset })
    } else {
      onToken({ ...token, token: token.token, offset })
    }
  }
  const reader = file.stream().getReader()
  const utf8 = new TextDecoder('utf-8', { fatal: true })
  try {
    while (true) {
      signal?.throwIfAborted()
      const next = await reader.read()
      if (next.done) break
      utf8.decode(next.value, { stream: true })
      parser.write(next.value)
    }
    utf8.decode()
    parser.end()
    signal?.throwIfAborted()
  } finally {
    await reader.cancel()
    reader.releaseLock()
  }
}
