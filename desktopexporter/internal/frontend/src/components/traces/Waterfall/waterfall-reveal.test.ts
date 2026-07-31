import { describe, expect, it } from 'vitest'
import {
  ancestorIdsOf,
  expandAncestorsForSpan,
} from './waterfall-reveal'

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

describe('expandAncestorsForSpan', () => {
  it('removes collapsed markers on every ancestor', () => {
    const parentOf = new Map<string, string | null>([
      ['f', 'e'],
      ['e', 'd'],
      ['d', null],
    ])
    const collapsed = new Set(['e', 'd', 'other'])
    const changed = expandAncestorsForSpan(collapsed, 'f', parentOf)
    expect(changed).toBe(true)
    expect(collapsed.has('e')).toBe(false)
    expect(collapsed.has('d')).toBe(false)
    expect(collapsed.has('other')).toBe(true)
  })

  it('returns false when no ancestors are collapsed', () => {
    const parentOf = new Map<string, string | null>([['f', 'e'], ['e', null]])
    const collapsed = new Set<string>()
    expect(expandAncestorsForSpan(collapsed, 'f', parentOf)).toBe(false)
  })
})
