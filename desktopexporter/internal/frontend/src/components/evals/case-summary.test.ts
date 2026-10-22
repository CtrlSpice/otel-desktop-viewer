import { describe, expect, it } from 'vitest'
import { caseSummary } from './case-summary'
import { exampleRuns } from './examples'
import type { EvalRep } from './model'

describe('case totals', () => {
  const run = exampleRuns[0]
  const definition = run.assessments.find(
    item => item.id === run.headlineAssessment
  )!
  const complete: EvalRep = {
    ...run.reps[0],
    seconds: 12,
    investigatorCost: 0,
    graderCost: 0.2,
    assessments: [
      {
        ...run.reps[0].assessments[0],
        id: definition.id,
        value: definition.maximum!,
      },
    ],
    tools: [{ name: 'query', state: 'Error', message: 'Failed' }],
  }
  const planned: EvalRep = {
    ...complete,
    state: 'Planned',
    seconds: undefined,
    investigatorCost: undefined,
    graderCost: undefined,
    assessments: [],
    tools: [],
  }

  it('retains the assessment denominator and marks incomplete sums instead of inventing zero measurements', () => {
    const summary = caseSummary(
      [complete, planned],
      [
        {
          ...definition,
          aggregation: 'planned',
        },
      ]
    )
    expect(summary.scores[definition.id]).toEqual({
      scored: 1,
      denominator: 2,
      percent: 50,
    })
    expect(summary.duration).toEqual({ total: 12, incomplete: true })
    expect(summary.investigatorCost).toEqual({ total: 0, incomplete: true })
    expect(summary.graderCost).toEqual({ total: 0.2, incomplete: true })
    expect([summary.calls, summary.failures]).toEqual([1, 1])
    expect(
      caseSummary(
        [complete, planned],
        [{ ...definition, aggregation: 'scored' }]
      ).scores[definition.id].percent
    ).toBe(100)
    expect(caseSummary([planned], [definition]).duration.total).toBeNull()
    expect(
      caseSummary([planned], [definition]).scores[definition.id].percent
    ).toBeNull()
  })

  it('sums repetition durations, costs and failures rather than averaging them', () => {
    const summary = caseSummary(
      [complete, { ...complete, seconds: 8 }],
      [definition]
    )
    expect(summary.duration).toEqual({ total: 20, incomplete: false })
    expect(summary.graderCost).toEqual({ total: 0.4, incomplete: false })
    expect([summary.calls, summary.failures]).toEqual([2, 2])
  })
})
