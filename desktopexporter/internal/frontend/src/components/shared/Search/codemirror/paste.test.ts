import { describe, expect, it } from 'vitest'
import { getFieldsBySignal } from '@/constants/fields'
import { parseSearchRequest } from '../queryParser'
import { normalizeQueryPaste } from './paste'

describe('multiline query paste', () => {
  it('joins formatting and plain text without inventing boolean operators', () => {
    expect(
      normalizeQueryPaste('body contains "disk"\r\n  and severityNumber >= 17')
    ).toBe('body contains "disk" and severityNumber >= 17')
    expect(normalizeQueryPaste("don't\n\tlose\r\nthese words")).toBe(
      "don't lose these words"
    )
    expect(normalizeQueryPaste('body = "a"\nbody = "b"')).toBe(
      'body = "a" body = "b"'
    )
  })

  it.each(['"', "'"] as const)(
    'preserves received newlines and indentation inside %s quotes',
    quote => {
      const raw = `body = ${quote}first\r\n  second\rthird\nfourth${quote}`
      const normalized = normalizeQueryPaste(raw)
      expect(normalized).toBe(
        `body = ${quote}first\\r\\n  second\\rthird\\nfourth${quote}`
      )
      expect(
        parseSearchRequest(normalized, getFieldsBySignal('logs'), 'logs')
          ?.predicate
      ).toMatchObject({ query: { value: 'first\r\n  second\rthird\nfourth' } })
    }
  )

  it('uses the insertion context when pasting into an existing quoted value', () => {
    expect(normalizeQueryPaste('first\n second', 'body = "', '"')).toBe(
      'first\\n second'
    )
    expect(normalizeQueryPaste('\n  and body = "b"', 'body = "a"')).toBe(
      ' and body = "b"'
    )
    expect(normalizeQueryPaste('first\n second', 'body = "')).toBe(
      'first\\n second'
    )
  })

  it('keeps escaped quotes and escaped newlines exact across paste boundaries', () => {
    const before = 'body = "first\\'
    const insert = normalizeQueryPaste('\r\nsecond', before, '"')
    expect(insert).toBe('r\\nsecond')
    expect(
      parseSearchRequest(
        before + insert + '"',
        getFieldsBySignal('logs'),
        'logs'
      )?.predicate
    ).toMatchObject({ query: { value: 'first\r\nsecond' } })
    const raw = 'body = "say \\"hello\\"\nnext"'
    expect(
      parseSearchRequest(
        normalizeQueryPaste(raw),
        getFieldsBySignal('logs'),
        'logs'
      )?.predicate
    ).toMatchObject({ query: { value: 'say "hello"\nnext' } })
  })

  it('preserves quoted text-search arguments and attribute keys', () => {
    expect(
      normalizeQueryPaste('search("first\nsecond")\n and body != null')
    ).toBe('search("first\\nsecond") and body != null')
    const query = normalizeQueryPaste(
      'attr(log, "first\nsecond", string) = "value"'
    )
    expect(parseSearchRequest(query, [], 'logs')?.predicate).toMatchObject({
      query: { field: { name: 'first\nsecond' }, value: 'value' },
    })
  })

  it('leaves single-line pastes and existing escape sequences unchanged', () => {
    const text = 'body = "first\\nsecond"  and severityNumber >= 17'
    expect(normalizeQueryPaste(text)).toBe(text)
  })
})
