import { describe, expect, it } from 'vitest'
import { dedupeAttributes } from '@/components/metrics/utils/dedupe-attributes'

describe('dedupeAttributes', () => {
  it('returns empty for empty input', () => {
    expect(dedupeAttributes([])).toEqual([])
  })

  it('keeps every entry, including duplicate keys', () => {
    expect(
      dedupeAttributes([
        { key: 'b', value: { kind: 'string', value: '1' } },
        { key: 'a', value: { kind: 'string', value: '2' } },
        { key: 'b', value: { kind: 'int64', value: 3n } },
      ])
    ).toEqual([
      { key: 'b', value: { kind: 'string', value: '1' } },
      { key: 'a', value: { kind: 'string', value: '2' } },
      { key: 'b', value: { kind: 'int64', value: 3n } },
    ])
  })

  it('does not collapse distinct typed duplicate values', () => {
    expect(
      dedupeAttributes([
        { key: 'service.name', value: { kind: 'string', value: '1' } },
        { key: 'service.name', value: { kind: 'int64', value: 1n } },
      ])
    ).toHaveLength(2)
  })
})
