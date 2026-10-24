import { describe, expect, it } from 'vitest'
import {
  acquireColor,
  releaseColor,
  remapColorAssignments,
  seedColorAssignments,
  syncColorAssignments,
} from './metric-timeseries-colors'

describe('metric timeseries colours', () => {
  it('retains slots while hiding, reconciling and restoring native series keys', () => {
    const assigned = seedColorAssignments(
      ['a', 'b', 'c'],
      new Set(['two', 'three']),
      ['one', 'two', 'three'],
      'metric:seed-test'
    )
    const two = assigned.get('two')
    const three = assigned.get('three')
    expect(two).not.toBe(three)

    syncColorAssignments(
      ['a', 'b', 'c'],
      assigned,
      new Set(['one', 'three']),
      ['one', 'two', 'three'],
      'metric:seed-test'
    )
    expect(assigned.has('two')).toBe(false)
    expect(assigned.get('three')).toBe(three)
    expect(assigned.get('one')).not.toBe(two)
    releaseColor(assigned, 'one')
    expect(
      acquireColor(['a', 'b', 'c'], assigned, 'two', 'metric:seed-test')
    ).toBe(two)
  })

  it('reuses the approved pool deterministically after exhaustion', () => {
    const assigned = new Map<string, string>()
    const one = acquireColor(['a', 'b'], assigned, 'one', 'metric:reuse-test')
    const two = acquireColor(['a', 'b'], assigned, 'two', 'metric:reuse-test')
    expect(one).not.toBe(two)
    expect(['a', 'b']).toContain(
      acquireColor(['a', 'b'], assigned, 'three', 'metric:reuse-test')
    )
    expect(assigned.get('one')).toBe(one)
    expect(assigned.get('two')).toBe(two)
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
