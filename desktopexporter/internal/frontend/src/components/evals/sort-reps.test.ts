import { describe, expect, it } from 'vitest'
import { exampleRuns } from './examples'
import { sortReps, type RepSortKey } from './sort-reps'

describe('sortReps', () => {
  const reps = exampleRuns[1].reps.filter(rep => rep.repetition === 1)

  it('preserves source order, ties and source objects', () => {
    const original = [...reps]
    expect(sortReps(reps)).toEqual(original)
    expect(sortReps(reps, { key: 'repetition', direction: 'desc' })).toEqual(
      original
    )
    sortReps(reps, { key: 'name', direction: 'asc' })
    expect(reps).toEqual(original)
  })

  it('defaults to available start times, with stable ties and unknown times last', () => {
    const samples = [
      { ...reps[0], id: 'late', startedAt: '2026-10-21T12:02:00Z' },
      { ...reps[0], id: 'unknown', startedAt: undefined },
      { ...reps[0], id: 'early', startedAt: '2026-10-21T12:01:00Z' },
      { ...reps[0], id: 'tie', startedAt: '2026-10-21T12:01:00Z' },
    ]
    expect(sortReps(samples).map(item => item.id)).toEqual([
      'early',
      'tie',
      'late',
      'unknown',
    ])
    expect(
      sortReps(samples, { key: 'started', direction: 'desc' }).map(
        item => item.id
      )
    ).toEqual(['late', 'early', 'tie', 'unknown'])
    expect(samples[0].id).toBe('late')
  })

  it('sorts scores numerically, including zero, with missing grades last both ways', () => {
    const asc = sortReps(reps, {
      key: 'assessment:completion',
      direction: 'asc',
    })
    const desc = sortReps(reps, {
      key: 'assessment:completion',
      direction: 'desc',
    })
    expect(asc.slice(0, 2).map(item => item.id)).toEqual([
      'trace-logs',
      'request-latency',
    ])
    expect(desc[0].id).toBe('checkout-failure')
    for (const sorted of [asc, desc]) {
      expect(sorted.slice(-3).map(item => item.id)).toEqual([
        'cpu-evidence',
        'checkout-latency',
        'service-errors',
      ])
    }
  })

  it.each(['seconds', 'investigatorCost', 'graderCost'] as const)(
    'keeps unknown %s after measured values in both directions',
    key => {
      const samples = [
        { ...reps[0], id: 'zero', [key]: 0 },
        { ...reps[0], id: 'unknown', [key]: undefined },
        { ...reps[0], id: 'ten', [key]: 10 },
        { ...reps[0], id: 'two', [key]: 2 },
      ]
      expect(
        sortReps(samples, { key, direction: 'asc' }).map(item => item.id)
      ).toEqual(['zero', 'two', 'ten', 'unknown'])
      expect(
        sortReps(samples, { key, direction: 'desc' }).map(item => item.id)
      ).toEqual(['ten', 'two', 'zero', 'unknown'])
    }
  )

  it('uses natural case names and numeric tool counts', () => {
    const samples = [
      {
        ...reps[0],
        id: 'ten',
        name: 'Case 10',
        tools: Array(10).fill(reps[0].tools[0]),
      },
      {
        ...reps[0],
        id: 'two',
        name: 'Case 2',
        tools: Array(2).fill(reps[0].tools[0]),
      },
    ]
    for (const key of ['name', 'tools'] satisfies RepSortKey[]) {
      expect(
        sortReps(samples, { key, direction: 'asc' }).map(item => item.id)
      ).toEqual(['two', 'ten'])
    }
  })
})
