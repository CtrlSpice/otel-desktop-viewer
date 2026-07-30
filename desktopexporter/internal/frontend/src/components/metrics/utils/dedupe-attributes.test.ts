import { describe, expect, it } from 'vitest'
import { dedupeAttributes } from '@/components/metrics/utils/dedupe-attributes'

describe('dedupeAttributes', () => {
  it('returns empty for empty input', () => {
    expect(dedupeAttributes([])).toEqual([])
  })

  it('keeps first-seen key order', () => {
    expect(
      dedupeAttributes([
        { key: 'b', value: '1', type: 'string' },
        { key: 'a', value: '2', type: 'string' },
        { key: 'b', value: '3', type: 'string' },
      ])
    ).toEqual([
      { key: 'b', value: '3', type: 'string' },
      { key: 'a', value: '2', type: 'string' },
    ])
  })

  it('last duplicate wins on value', () => {
    expect(
      dedupeAttributes([
        { key: 'service.name', value: 'old', type: 'string' },
        { key: 'service.name', value: 'new', type: 'string' },
      ])
    ).toEqual([{ key: 'service.name', value: 'new', type: 'string' }])
  })
})
