import { describe, expect, it } from 'vitest'
import { exampleRuns } from './examples'
import { readRunOutcomeFilter, runOutcomeOptions } from './run-outcome-filter'

describe('run outcome filters', () => {
  it('counts runs, keeping failed assessments separate from internal faults and planned work', () => {
    expect(runOutcomeOptions(exampleRuns)).toEqual([
      { key: 'success:Pass', label: 'Pass', tone: 'success', count: 2 },
      { key: 'warning:Partial', label: 'Partial', tone: 'warning', count: 2 },
      { key: 'error:Fail', label: 'Fail', tone: 'error', count: 2 },
      {
        key: 'papaya:Internal fault',
        label: 'Internal fault',
        tone: 'papaya',
        count: 1,
      },
      { key: 'neutral:Planned', label: 'Planned', tone: 'neutral', count: 2 },
    ])
  })

  it('preserves arbitrary source labels and their distinct score meanings through URL selection', () => {
    const run = exampleRuns[0]!
    const label = 'Result: A, "B" / ✓'
    const options = runOutcomeOptions([
      {
        ...run,
        attempts: run.attempts.map(attempt => ({
          ...attempt,
          assessments: attempt.assessments.map(assessment => ({
            ...assessment,
            label,
          })),
        })),
      },
    ])
    expect(options.map(option => [option.key, option.count])).toEqual([
      [`success:${label}`, 1],
      [`warning:${label}`, 1],
      [`error:${label}`, 1],
    ])
    expect(
      readRunOutcomeFilter(JSON.stringify([`error:${label}`]), options)
    ).toEqual([`error:${label}`])
  })

  it('ignores malformed, duplicate and stale URL selections', () => {
    const options = runOutcomeOptions(exampleRuns)
    for (const value of [
      undefined,
      '{',
      'null',
      '42',
      '{}',
      '[null,42,"missing"]',
    ]) {
      expect(readRunOutcomeFilter(value, options)).toEqual([])
    }
    expect(
      readRunOutcomeFilter(
        '["error:Fail","error:Fail","missing",null]',
        options
      )
    ).toEqual(['error:Fail'])
  })
})
