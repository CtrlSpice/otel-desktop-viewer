import { describe, expect, it } from 'vitest'
import { exampleRuns } from './examples'
import { completionState, groupAttempts } from './group-attempts'
import type { EvalAttempt } from './model'

describe('case repetition groups', () => {
  const sample = exampleRuns[0].attempts[0]
  const attempt = (
    id: string,
    caseID: string,
    seconds: number | undefined,
    state: EvalAttempt['state'] = 'Complete'
  ): EvalAttempt => ({
    ...sample,
    id,
    caseID,
    seconds,
    state,
    name: 'Same name',
  })

  it('keeps source identity, singleton cases and source order', () => {
    const attempts = [
      attempt('a1', 'a', 10),
      attempt('b1', 'b', 8),
      attempt('a2', 'a', 3),
    ]
    const groups = groupAttempts(attempts, attempts)
    expect(
      groups.map(group => [group.id, group.attempts.map(item => item.id)])
    ).toEqual([
      ['a', ['a1', 'a2']],
      ['b', ['b1']],
    ])
    expect(attempts.map(item => item.id)).toEqual(['a1', 'b1', 'a2'])
  })

  it('ranks cases by the leading sorted attempt, keeps siblings together and missing values last within each case', () => {
    const attempts = [
      attempt('a1', 'a', 10),
      attempt('b1', 'b', 8),
      attempt('a2', 'a', 3),
      attempt('a3', 'a', undefined),
      attempt('c1', 'c', undefined),
    ]
    for (const direction of ['asc', 'desc'] as const) {
      const groups = groupAttempts(attempts, attempts, {
        key: 'seconds',
        direction,
      })
      expect(groups.map(group => group.id)).toEqual(['a', 'b', 'c'])
      expect(groups[0].attempts.map(item => item.id)).toEqual(
        direction === 'asc' ? ['a2', 'a1', 'a3'] : ['a1', 'a2', 'a3']
      )
    }
  })

  it('does not declare a filtered case complete when a hidden attempt errored', () => {
    const attempts = [
      attempt('a1', 'a', 10),
      attempt('a2', 'a', undefined, 'Error'),
    ]
    const [group] = groupAttempts([attempts[0]], attempts)
    expect(group.state).toBe('Error')
    expect(group.attempts).toEqual([attempts[0]])
    expect(group.allAttempts).toEqual(attempts)
  })

  it('uses execution completion rather than assessment success', () => {
    const failed = {
      ...sample,
      state: 'Complete' as const,
      assessments: sample.assessments.map(score => ({ ...score, value: 0 })),
    }
    expect(completionState([failed])).toBe('Complete')
    expect(completionState([attempt('a', 'a', undefined, 'Planned')])).toBe(
      'Planned'
    )
    expect(
      completionState([failed, attempt('b', 'a', undefined, 'Planned')])
    ).toBe('Running')
    expect(completionState([failed, attempt('b', 'a', 3, 'Error')])).toBe(
      'Error'
    )
  })
})
