import { describe, expect, it } from 'vitest'
import {
  acquireColor,
  remapColorAssignments,
  seedColorAssignments,
  syncColorAssignments,
} from './metric-timeseries-colors'

describe('metric timeseries colours', () => {
  it('seeds in legend order and preserves existing assignments while reconciling', () => {
    const assigned = seedColorAssignments(
      ['a', 'b', 'c'],
      new Set(['two', 'three']),
      ['one', 'two', 'three']
    )
    expect([...assigned]).toEqual([
      ['two', 'a'],
      ['three', 'b'],
    ])

    syncColorAssignments(['a', 'b', 'c'], assigned, new Set(['one', 'three']), [
      'one',
      'two',
      'three',
    ])
    expect([...assigned]).toEqual([
      ['three', 'b'],
      ['one', 'a'],
    ])
  })

  it('reuses the approved pool deterministically after exhaustion', () => {
    const assigned = new Map<string, string>()
    expect(acquireColor(['a', 'b'], assigned, 'one')).toBe('a')
    expect(acquireColor(['a', 'b'], assigned, 'two')).toBe('b')
    expect(acquireColor(['a', 'b'], assigned, 'three')).toBe('a')
    expect(acquireColor([], assigned, 'four')).toBeNull()
  })

  it('keeps key slots while applying the current theme palette', () => {
    const assigned = new Map([
      ['one', '#old-b'],
      ['two', '#old-a'],
      ['three', '#old-b'],
    ])
    expect([
      ...remapColorAssignments(
        assigned,
        ['#old-a', '#old-b'],
        ['#new-a', '#new-b']
      ),
    ]).toEqual([
      ['one', '#new-b'],
      ['two', '#new-a'],
      ['three', '#new-b'],
    ])
  })
})
