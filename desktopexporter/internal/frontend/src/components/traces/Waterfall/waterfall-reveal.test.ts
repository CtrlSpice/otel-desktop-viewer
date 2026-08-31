import { describe, expect, it } from 'vitest'
import { ancestorIdsOf, keyboardAnchorSpanID } from './waterfall-reveal'

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

describe('ancestorIdsOf with parent cycles', () => {
  // parentSpanID is reported data, not verified structure. A salvaged trace
  // can put two spans in each other's ancestry; the walk must terminate and
  // return the chain up to the first repeat, not hang the main thread.
  it('terminates on a two-span cycle and reports the single honest step', () => {
    const parentOf = new Map<string, string | null>([
      ['old-biff', 'young-biff'],
      ['young-biff', 'old-biff'],
    ])
    expect(ancestorIdsOf('old-biff', parentOf)).toEqual(['young-biff'])
    expect(ancestorIdsOf('young-biff', parentOf)).toEqual(['old-biff'])
  })

  it('terminates on a self-parenting span with no ancestors', () => {
    const parentOf = new Map<string, string | null>([['fry', 'fry']])
    expect(ancestorIdsOf('fry', parentOf)).toEqual([])
  })

  it('walks a healthy chain exactly as before', () => {
    const parentOf = new Map<string, string | null>([
      ['grandchild', 'child'],
      ['child', 'root'],
      ['root', null],
    ])
    expect(ancestorIdsOf('grandchild', parentOf)).toEqual(['child', 'root'])
  })

  it('stops where a long chain feeds into a cycle', () => {
    const parentOf = new Map<string, string | null>([
      ['leaf', 'mid'],
      ['mid', 'loop-a'],
      ['loop-a', 'loop-b'],
      ['loop-b', 'loop-a'],
    ])
    expect(ancestorIdsOf('leaf', parentOf)).toEqual(['mid', 'loop-a', 'loop-b'])
  })
})

describe('keyboardAnchorSpanID', () => {
  const parentOf = new Map<string, string | null>([
    ['d', 'c'],
    ['c', 'b'],
    ['b', 'a'],
    ['a', null],
  ])

  it('uses the first visible span before a span is selected', () => {
    expect(keyboardAnchorSpanID(null, ['a', 'b', 'c', 'd'], parentOf)).toBe('a')
  })

  it('uses the selected span when it is visible', () => {
    expect(keyboardAnchorSpanID('c', ['a', 'b', 'c', 'd'], parentOf)).toBe('c')
  })

  it('uses the nearest visible ancestor of a hidden selection', () => {
    expect(keyboardAnchorSpanID('d', ['a', 'b'], parentOf)).toBe('b')
  })

  it('falls back to the first visible span for an unknown selection', () => {
    expect(keyboardAnchorSpanID('unknown', ['a', 'b'], parentOf)).toBe('a')
  })

  it('returns null when no spans are visible', () => {
    expect(keyboardAnchorSpanID('d', [], parentOf)).toBeNull()
  })
})
