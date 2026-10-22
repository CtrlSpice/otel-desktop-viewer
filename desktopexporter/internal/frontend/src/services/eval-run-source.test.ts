import { expect, it } from 'vitest'
import { assembleEvalRunSource } from './eval-run-source'
import type { EvalRunSourcePage } from './eval-run-types'

const page: EvalRunSourcePage = {
  summary: {
    id: 'run',
    adapterRunID: 'adapter',
    datasetID: 'dataset',
    name: 'Evaluation · Provider',
    evaluation: 'Evaluation',
    source: 'received',
    format: 'promptfoo-v3',
    state: 'Unknown',
    stateReason: 'Unknown',
    pauseReason: null,
    startedAt: null,
    plannedCount: null,
    repCount: 2,
    assessmentName: 'Recorded outcome',
    outcomes: [],
  },
  contextJSON:
    '{"evalId":"eval","unknown":9007199254740993,"config":{},"results":{"version":3,"results":[]}}',
  rows: [{ ordinal: 2, json: '{"id":"a","exact":18446744073709551615}' }],
  nextAfter: 2,
  hasMore: true,
}

it('assembles only a complete selected run without rounding context or row numbers', () => {
  const source = assembleEvalRunSource([
    page,
    {
      ...page,
      rows: [{ ordinal: 7, json: '{"id":"b","decimal":1.2300e9999}' }],
      nextAfter: 7,
      hasMore: false,
    },
  ])
  expect(source.rowOrdinals).toEqual([2, 7])
  expect(source.payload).toContain('9007199254740993')
  expect(source.payload).toContain('18446744073709551615')
  expect(source.payload).toContain('1.2300e9999')
  expect(source.id).toBe('dataset')
  expect(() => assembleEvalRunSource([page])).toThrow('incomplete')
  expect(() => assembleEvalRunSource([{ ...page, hasMore: false }])).toThrow(
    'complete recorded run'
  )
  expect(() =>
    assembleEvalRunSource([page, { ...page, hasMore: false }])
  ).toThrow('strictly increasing')
})
