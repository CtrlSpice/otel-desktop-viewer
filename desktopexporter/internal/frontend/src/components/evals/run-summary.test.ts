import { describe, expect, it } from 'vitest'
import { exampleRuns } from './examples'
import { runSummary } from './run-summary'

describe('run summaries', () => {
  it('keeps the mean separate from outcome counts and recorded tool failures', () => {
    const summary = runSummary(exampleRuns[0])
    expect(summary.assessment).toEqual({
      scored: 16,
      denominator: 16,
      percent: 78.125,
    })
    expect(summary.parts).toEqual([
      { label: 'Pass', tone: 'success', count: 11 },
      { label: 'Partial', tone: 'warning', count: 3 },
      { label: 'Fail', tone: 'error', count: 2 },
    ])
    expect(summary.toolCalls).toBe(35)
    expect(summary.failedTools).toBe(3)
    expect(summary.investigatorCost.total).toBeCloseTo(1.187)
    expect(summary.investigatorCost.incomplete).toBe(false)
  })

  it('includes planned and execution-error slots without turning them into failed grades', () => {
    const summary = runSummary(exampleRuns[1])
    expect(summary.assessment).toEqual({
      scored: 10,
      denominator: 16,
      percent: 40.625,
    })
    expect(summary.started).toBe(12)
    expect(summary.parts).toEqual([
      { label: 'Pass', tone: 'success', count: 5 },
      { label: 'Partial', tone: 'warning', count: 3 },
      { label: 'Fail', tone: 'error', count: 2 },
      { label: 'Internal fault', tone: 'papaya', count: 2 },
      { label: 'Planned', tone: 'neutral', count: 4 },
    ])
    expect(summary.investigatorCost.total).toBeCloseTo(0.592)
    expect(summary.investigatorCost.incomplete).toBe(true)
  })

  it('keeps an unstarted run unscored and its cost unknown', () => {
    const summary = runSummary(exampleRuns[2])
    expect(summary.assessment).toEqual({
      scored: 0,
      denominator: 16,
      percent: null,
    })
    expect(summary.parts).toEqual([
      { label: 'Planned', tone: 'neutral', count: 16 },
    ])
    expect(summary.started).toBe(0)
    expect(summary.investigatorCost.total).toBeNull()
  })

  it('counts completed investigations separately from recorded grades', () => {
    const run = structuredClone(exampleRuns[0])
    run.attempts[0].assessments = []
    const summary = runSummary(run)
    expect(summary.assessed).toBe(16)
    expect(summary.assessment?.scored).toBe(15)
    expect(summary.planned).toBe(16)
  })
})
