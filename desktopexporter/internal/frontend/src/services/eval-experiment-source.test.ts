import { afterEach, expect, it, vi } from 'vitest'
import {
  assembleEvalExperimentSource,
  loadEvalExperimentSource,
} from './eval-experiment-source'
import { evalAPI } from './eval-service'
import { evalProjectionLimits } from '@/components/evals/promptfoo-adapter'
import type { EvalExperimentSourcePage } from './eval-experiment-types'

afterEach(() => vi.restoreAllMocks())

const page: EvalExperimentSourcePage = {
  summary: {
    id: 'experiment',
    adapterExperimentID: 'adapter',
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
    resultCount: 2,
    assessmentName: 'Recorded outcome',
    outcomes: [],
  },
  contextJSON:
    '{"evalId":"eval","unknown":9007199254740993,"config":{},"results":{"version":3,"results":[]}}',
  rows: [{ ordinal: 2, json: '{"id":"a","exact":18446744073709551615}' }],
  nextAfter: 2,
  hasMore: true,
}

it('assembles only a complete selected experiment without rounding context or row numbers', () => {
  const source = assembleEvalExperimentSource([
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
  expect(() => assembleEvalExperimentSource([page])).toThrow('incomplete')
  expect(() =>
    assembleEvalExperimentSource([{ ...page, hasMore: false }])
  ).toThrow('complete recorded experiment')
  expect(() =>
    assembleEvalExperimentSource([page, { ...page, hasMore: false }])
  ).toThrow('strictly increasing')
})

it('stops fetching when retained source text exceeds the display budget', async () => {
  const json = JSON.stringify({
    output: 'x'.repeat(evalProjectionLimits.payloadCharacters / 2),
  })
  const getPage = vi
    .spyOn(evalAPI, 'getEvalExperimentPage')
    .mockImplementation(async (_id, after = -1) => ({
      ...page,
      summary: { ...page.summary, resultCount: 3 },
      rows: [{ ordinal: after + 1, json }],
      nextAfter: after + 1,
      hasMore: true,
    }))
  await expect(
    loadEvalExperimentSource('experiment', new AbortController().signal)
  ).rejects.toThrow('text limit')
  expect(getPage).toHaveBeenCalledTimes(2)
})

it('rejects an oversized context before requesting another page', async () => {
  const getPage = vi.spyOn(evalAPI, 'getEvalExperimentPage').mockResolvedValue({
    ...page,
    contextJSON: ' '.repeat(evalProjectionLimits.payloadCharacters + 1),
    rows: [],
  })
  await expect(
    loadEvalExperimentSource('experiment', new AbortController().signal)
  ).rejects.toThrow('text limit')
  expect(getPage).toHaveBeenCalledOnce()
})

it('loads repeated large summary/context text while preserving complete source rows', async () => {
  const evaluation = 'x'.repeat(1024 * 1024)
  const contextJSON = page.contextJSON.replace(
    '"config":{}',
    `"config":{"description":"${evaluation}"}`
  )
  const getPage = vi
    .spyOn(evalAPI, 'getEvalExperimentPage')
    .mockImplementation(async (_id, after = -1) =>
      Response.json({
        ...page,
        contextJSON,
        summary: { ...page.summary, evaluation, resultCount: 9 },
        rows: [
          { ordinal: after + 1, json: '{"exact":9007199254740993,"zero":-0}' },
        ],
        nextAfter: after + 1,
        hasMore: after < 7,
      }).json()
    )
  const { source, summary } = await loadEvalExperimentSource(
    'experiment',
    new AbortController().signal
  )
  expect(getPage).toHaveBeenCalledTimes(9)
  expect(summary.evaluation).toBe(evaluation)
  expect(source.name).toBe(evaluation)
  expect(source.rowOrdinals).toEqual([0, 1, 2, 3, 4, 5, 6, 7, 8])
  expect(source.payload).toContain('"exact":9007199254740993,"zero":-0')
})

it('does not retain a page or fetch more pages after cancellation', async () => {
  const controller = new AbortController()
  const getPage = vi
    .spyOn(evalAPI, 'getEvalExperimentPage')
    .mockImplementation(async () => {
      controller.abort()
      return page
    })
  await expect(
    loadEvalExperimentSource('experiment', controller.signal)
  ).rejects.toMatchObject({ name: 'AbortError' })
  expect(getPage).toHaveBeenCalledOnce()
})
