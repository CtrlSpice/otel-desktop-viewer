import { expect, it } from 'vitest'
import { formatRawJSON, rawJsonFormatLimits } from './raw-json-format'

it('changes only whitespace around received JSON tokens', () => {
  const raw =
    '{"n":9007199254740993,"n":-0,"exp":1.2300e9999,"text":"brace } quote \\" literal \\\\n"}'
  const formatted = formatRawJSON(raw)
  expect(formatted).toBe(
    '{\n  "n": 9007199254740993,\n  "n": -0,\n  "exp": 1.2300e9999,\n  "text": "brace } quote \\" literal \\\\n"\n}'
  )
})

it('returns the unchanged source when indentation depth exceeds the budget', () => {
  const raw =
    '['.repeat(1000) + Array(100000).fill('0').join(',') + ']'.repeat(1000)
  expect(raw.length).toBeLessThan(rawJsonFormatLimits.characters)
  expect(formatRawJSON(raw)).toBe(raw)
})

it('bounds total formatted text even within the supported nesting depth', () => {
  const raw =
    '['.repeat(63) + Array(25000).fill('-0').join(',') + ']'.repeat(63)
  expect(raw.length).toBeLessThan(rawJsonFormatLimits.characters)
  expect(formatRawJSON(raw)).toBe(raw)
})
