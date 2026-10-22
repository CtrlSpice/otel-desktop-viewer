import { describe, expect, it } from 'vitest'
import { exampleExperiments } from './examples'
import { completionState, groupResults } from './group-results'
import type { EvalResult } from './model'

describe('case repetition groups', () => {
  const sample = exampleExperiments[0].results[0]
  const result = (
    id: string,
    caseID: string,
    seconds: number | undefined,
    state: EvalResult['state'] = 'Complete'
  ): EvalResult => ({
    ...sample,
    id,
    caseID,
    seconds,
    state,
    name: 'Same name',
  })

  it('keeps source identity, singleton cases and source order', () => {
    const results = [
      result('a1', 'a', 10),
      result('b1', 'b', 8),
      result('a2', 'a', 3),
    ]
    const groups = groupResults(results, results)
    expect(
      groups.map(group => [group.id, group.results.map(item => item.id)])
    ).toEqual([
      ['a', ['a1', 'a2']],
      ['b', ['b1']],
    ])
    expect(results.map(item => item.id)).toEqual(['a1', 'b1', 'a2'])
  })

  it('ranks cases by the leading sorted result, keeps siblings together and missing values last within each case', () => {
    const results = [
      result('a1', 'a', 10),
      result('b1', 'b', 8),
      result('a2', 'a', 3),
      result('a3', 'a', undefined),
      result('c1', 'c', undefined),
    ]
    for (const direction of ['asc', 'desc'] as const) {
      const groups = groupResults(results, results, {
        key: 'seconds',
        direction,
      })
      expect(groups.map(group => group.id)).toEqual(['a', 'b', 'c'])
      expect(groups[0].results.map(item => item.id)).toEqual(
        direction === 'asc' ? ['a2', 'a1', 'a3'] : ['a1', 'a2', 'a3']
      )
    }
  })

  it('does not declare a filtered case complete when a hidden result errored', () => {
    const results = [
      result('a1', 'a', 10),
      result('a2', 'a', undefined, 'Error'),
    ]
    const [group] = groupResults([results[0]], results)
    expect(group.state).toBe('Error')
    expect(group.results).toEqual([results[0]])
    expect(group.allResults).toEqual(results)
  })

  it('uses execution completion rather than assessment success', () => {
    const failed = {
      ...sample,
      state: 'Complete' as const,
      assessments: sample.assessments.map(score => ({ ...score, value: 0 })),
    }
    expect(completionState([failed])).toBe('Complete')
    expect(completionState([result('a', 'a', undefined, 'Planned')])).toBe(
      'Planned'
    )
    expect(
      completionState([failed, result('b', 'a', undefined, 'Planned')])
    ).toBe('Running')
    expect(completionState([failed, result('b', 'a', 3, 'Error')])).toBe(
      'Error'
    )
  })
})
