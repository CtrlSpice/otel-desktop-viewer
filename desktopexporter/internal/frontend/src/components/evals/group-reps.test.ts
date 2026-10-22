import { describe, expect, it } from 'vitest'
import { exampleRuns } from './examples'
import { completionState, groupReps } from './group-reps'
import type { EvalRep } from './model'

describe('case repetition groups', () => {
  const sample = exampleRuns[0].reps[0]
  const rep = (
    id: string,
    caseID: string,
    seconds: number | undefined,
    state: EvalRep['state'] = 'Complete'
  ): EvalRep => ({
    ...sample,
    id,
    caseID,
    seconds,
    state,
    name: 'Same name',
  })

  it('keeps source identity, singleton cases and source order', () => {
    const reps = [rep('a1', 'a', 10), rep('b1', 'b', 8), rep('a2', 'a', 3)]
    const groups = groupReps(reps, reps)
    expect(
      groups.map(group => [group.id, group.reps.map(item => item.id)])
    ).toEqual([
      ['a', ['a1', 'a2']],
      ['b', ['b1']],
    ])
    expect(reps.map(item => item.id)).toEqual(['a1', 'b1', 'a2'])
  })

  it('ranks cases by the leading sorted rep, keeps siblings together and missing values last within each case', () => {
    const reps = [
      rep('a1', 'a', 10),
      rep('b1', 'b', 8),
      rep('a2', 'a', 3),
      rep('a3', 'a', undefined),
      rep('c1', 'c', undefined),
    ]
    for (const direction of ['asc', 'desc'] as const) {
      const groups = groupReps(reps, reps, {
        key: 'seconds',
        direction,
      })
      expect(groups.map(group => group.id)).toEqual(['a', 'b', 'c'])
      expect(groups[0].reps.map(item => item.id)).toEqual(
        direction === 'asc' ? ['a2', 'a1', 'a3'] : ['a1', 'a2', 'a3']
      )
    }
  })

  it('does not declare a filtered case complete when a hidden rep errored', () => {
    const reps = [rep('a1', 'a', 10), rep('a2', 'a', undefined, 'Error')]
    const [group] = groupReps([reps[0]], reps)
    expect(group.state).toBe('Error')
    expect(group.reps).toEqual([reps[0]])
    expect(group.allReps).toEqual(reps)
  })

  it('uses execution completion rather than assessment success', () => {
    const failed = {
      ...sample,
      state: 'Complete' as const,
      assessments: sample.assessments.map(score => ({ ...score, value: 0 })),
    }
    expect(completionState([failed])).toBe('Complete')
    expect(completionState([rep('a', 'a', undefined, 'Planned')])).toBe(
      'Planned'
    )
    expect(completionState([failed, rep('b', 'a', undefined, 'Planned')])).toBe(
      'Running'
    )
    expect(completionState([failed, rep('b', 'a', 3, 'Error')])).toBe('Error')
  })
})
