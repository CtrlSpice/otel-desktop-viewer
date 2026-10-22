import { TokenType } from '@streamparser/json'
import { scanJSONFile } from './json-file'

/** Each JSONL record needs its own envelope; bare rows cannot establish run context. */
export async function detectImportFile(
  file: Blob,
  signal?: AbortSignal
): Promise<'otlp' | 'eval'> {
  let depth = 0
  let key = false
  let keys = new Set<string>()
  let kind: 'otlp' | 'eval' | undefined
  await scanJSONFile(
    file,
    ({ token, value }) => {
      if (token === TokenType.SEPARATOR) return
      if (depth === 0 && token !== TokenType.LEFT_BRACE)
        throw new Error(
          'Input must contain JSON objects or JSONL object records.'
        )
      if (token === TokenType.LEFT_BRACE || token === TokenType.LEFT_BRACKET) {
        if (depth === 0) keys = new Set()
        depth++
        if (depth === 1) key = true
      } else if (
        token === TokenType.RIGHT_BRACE ||
        token === TokenType.RIGHT_BRACKET
      ) {
        depth--
        if (depth !== 0) return
        const telemetry = [...keys].some(k =>
          /^resource(Spans|Logs|Metrics|Profiles)$/.test(k)
        )
        const evaluation =
          keys.has('evalId') || keys.has('results') || keys.has('config')
        if (telemetry && evaluation)
          throw new Error('Ambiguous input mixes OTLP and evaluation fields.')
        const next = telemetry ? 'otlp' : evaluation ? 'eval' : undefined
        if (!next)
          throw new Error(
            'Unsupported JSON object. Eval imports require a full Promptfoo v3 envelope; bare result rows lack run context.'
          )
        if (kind && kind !== next)
          throw new Error('A file cannot mix telemetry and evaluation records.')
        kind = next
      } else if (depth === 1) {
        if (token === TokenType.COMMA) key = true
        else if (token === TokenType.COLON) key = false
        else if (token === TokenType.STRING && key) keys.add(value as string)
      }
    },
    signal
  )
  if (!kind) throw new Error('No JSON records found.')
  return kind
}
