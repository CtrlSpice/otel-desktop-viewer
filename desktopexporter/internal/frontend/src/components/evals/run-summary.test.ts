import { describe, expect, it } from 'vitest'
import { exampleRuns } from './examples'
import { runSummary } from './run-summary'

describe('run summaries', () => {
  it('keeps the mean separate from outcome counts and recorded tool failures', () => {
    const summary = runSummary(exampleRuns[0])
    expect(summary.assessment).toEqual({
      scored: 8,
      denominator: 8,
      percent: 81.25,
    })
    expect(summary.parts).toEqual([
      { label: 'Pass', tone: 'success', count: 6 },
      { label: 'Partial', tone: 'warning', count: 1 },
      { label: 'Fail', tone: 'error', count: 1 },
    ])
    expect(summary.toolCalls).toBe(17)
    expect(summary.failedTools).toBe(1)
    expect(summary.investigatorCost.total).toBeCloseTo(0.588)
    expect(summary.investigatorCost.incomplete).toBe(false)
  })

  it('includes planned and execution-error slots without turning them into failed grades', () => {
    const summary = runSummary(exampleRuns[1])
    expect(summary.assessment).toEqual({
      scored: 5,
      denominator: 8,
      percent: 43.75,
    })
    expect(summary.started).toBe(6)
    expect(summary.parts).toEqual([
      { label: 'Pass', tone: 'success', count: 3 },
      { label: 'Partial', tone: 'warning', count: 1 },
      { label: 'Fail', tone: 'error', count: 1 },
      { label: 'Execution error', tone: 'neutral', count: 1 },
      { label: 'Planned', tone: 'neutral', count: 2 },
    ])
    expect(summary.investigatorCost.total).toBeCloseTo(0.285)
    expect(summary.investigatorCost.incomplete).toBe(true)
  })

  it('keeps an unstarted run unscored and its cost unknown', () => {
    const summary = runSummary(exampleRuns[2])
    expect(summary.assessment).toEqual({
      scored: 0,
      denominator: 8,
      percent: null,
    })
    expect(summary.parts).toEqual([
      { label: 'Planned', tone: 'neutral', count: 8 },
    ])
    expect(summary.started).toBe(0)
    expect(summary.investigatorCost.total).toBeNull()
  })
})
