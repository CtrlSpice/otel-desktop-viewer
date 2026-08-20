import { describe, expect, it } from 'vitest'
import { ancestorIdsOf } from './waterfall-reveal'

describe('ancestorIdsOf', () => {
  it('returns ancestors from immediate parent to root', () => {
    const parentOf = new Map<string, string | null>([
      ['f', 'e'],
      ['e', 'd'],
      ['d', 'c'],
      ['c', 'b'],
      ['b', 'a'],
      ['a', null],
    ])
    expect(ancestorIdsOf('f', parentOf)).toEqual(['e', 'd', 'c', 'b', 'a'])
  })

  it('returns an empty list for the root span', () => {
    const parentOf = new Map<string, string | null>([['a', null]])
    expect(ancestorIdsOf('a', parentOf)).toEqual([])
  })
})
