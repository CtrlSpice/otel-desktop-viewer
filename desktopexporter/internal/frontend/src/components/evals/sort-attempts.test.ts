import { describe, expect, it } from 'vitest'
import { exampleRuns } from './examples'
import { sortAttempts, type AttemptSortKey } from './sort-attempts'

describe('sortAttempts', () => {
  const attempts = exampleRuns[1].attempts.filter(
    attempt => attempt.repetition === 1
  )

  it('preserves source order, ties and source objects', () => {
    const original = [...attempts]
    expect(sortAttempts(attempts)).toEqual(original)
    expect(
      sortAttempts(attempts, { key: 'repetition', direction: 'desc' })
    ).toEqual(original)
    sortAttempts(attempts, { key: 'name', direction: 'asc' })
    expect(attempts).toEqual(original)
  })

  it('sorts scores numerically, including zero, with missing grades last both ways', () => {
    const asc = sortAttempts(attempts, {
      key: 'assessment:completion',
      direction: 'asc',
    })
    const desc = sortAttempts(attempts, {
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
        { ...attempts[0], id: 'zero', [key]: 0 },
        { ...attempts[0], id: 'unknown', [key]: undefined },
        { ...attempts[0], id: 'ten', [key]: 10 },
        { ...attempts[0], id: 'two', [key]: 2 },
      ]
      expect(
        sortAttempts(samples, { key, direction: 'asc' }).map(item => item.id)
      ).toEqual(['zero', 'two', 'ten', 'unknown'])
      expect(
        sortAttempts(samples, { key, direction: 'desc' }).map(item => item.id)
      ).toEqual(['ten', 'two', 'zero', 'unknown'])
    }
  )

  it('uses natural case names and numeric tool counts', () => {
    const samples = [
      {
        ...attempts[0],
        id: 'ten',
        name: 'Case 10',
        tools: Array(10).fill(attempts[0].tools[0]),
      },
      {
        ...attempts[0],
        id: 'two',
        name: 'Case 2',
        tools: Array(2).fill(attempts[0].tools[0]),
      },
    ]
    for (const key of ['name', 'tools'] satisfies AttemptSortKey[]) {
      expect(
        sortAttempts(samples, { key, direction: 'asc' }).map(item => item.id)
      ).toEqual(['two', 'ten'])
    }
  })
})
