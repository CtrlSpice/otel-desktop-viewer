import { expect, it } from 'vitest'
import { attributeMatchesSQL } from './attribute-matches'

it('matches stored JSON escapes without changing the received shell command', () => {
  const value = "echo 'exact' 2>&1 < input\u2028next\u2029line"
  const sql = attributeMatchesSQL(
    {
      signal: 'traces',
      ownerType: 'event',
      key: 'bash_command',
      kind: 'string',
      valueIdentity: JSON.stringify({ kind: 'string', value }),
    },
    0
  )
  const stored = String.raw`{"kind":"string","value":"echo 'exact' 2\u003e\u00261 \u003c input\u2028next\u2029line"}`
  expect(sql).toContain(`AND a.value = '${stored.replaceAll("'", "''")}'::JSON`)
  expect(JSON.parse(stored).value).toBe(value)
})

it('retains typed array order, exact int64s, and negative-zero double bits', () => {
  const identity =
    '{"kind":"array","value":[{"kind":"int64","value":"9007199254740993"},{"kind":"double","value":"0x8000000000000000"},{"kind":"empty","value":null}]}'
  const sql = attributeMatchesSQL(
    {
      signal: 'traces',
      ownerType: 'event',
      key: "quote'key",
      kind: 'array',
      valueIdentity: identity,
      startTime: '9007199254740993',
      endTime: '9007199254740994',
    },
    500
  )
  expect(sql).toContain(`a.key = 'quote''key'`)
  expect(sql).toContain(`AND a.value = '${identity}'::JSON`)
  expect(sql).toContain(`e.timestamp >= '9007199254740993'::UBIGINT`)
  expect(sql).toContain(`s.start_time <= '9007199254740994'::UBIGINT`)
  expect(sql).toContain('ORDER BY item_ref OFFSET 500')
})
