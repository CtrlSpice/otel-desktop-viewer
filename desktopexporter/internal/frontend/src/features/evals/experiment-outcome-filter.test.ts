import { describe, expect, it } from 'vitest'
import { exampleExperiments } from './examples'
import {
  readExperimentOutcomeFilter,
  experimentOutcomeOptions,
} from './experiment-outcome-filter'
import { toExperimentCard } from './experiment-summary'

describe('experiment outcome filters', () => {
  it('counts experiments, keeping failed assessments separate from internal faults and planned work', () => {
    expect(
      experimentOutcomeOptions(exampleExperiments.map(toExperimentCard))
    ).toEqual([
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
    const experiment = exampleExperiments[0]!
    const label = 'Result: A, "B" / ✓'
    const options = experimentOutcomeOptions([
      toExperimentCard({
        ...experiment,
        results: experiment.results.map(result => ({
          ...result,
          assessments: result.assessments.map(assessment => ({
            ...assessment,
            label,
          })),
        })),
      }),
    ])
    expect(options.map(option => [option.key, option.count])).toEqual([
      [`success:${label}`, 1],
      [`warning:${label}`, 1],
      [`error:${label}`, 1],
    ])
    expect(
      readExperimentOutcomeFilter(JSON.stringify([`error:${label}`]), options)
    ).toEqual([`error:${label}`])
  })

  it('ignores malformed, duplicate and stale URL selections', () => {
    const options = experimentOutcomeOptions(
      exampleExperiments.map(toExperimentCard)
    )
    for (const value of [
      undefined,
      '{',
      'null',
      '42',
      '{}',
      '[null,42,"missing"]',
    ]) {
      expect(readExperimentOutcomeFilter(value, options)).toEqual([])
    }
    expect(
      readExperimentOutcomeFilter(
        '["error:Fail","error:Fail","missing",null]',
        options
      )
    ).toEqual(['error:Fail'])
  })
})
