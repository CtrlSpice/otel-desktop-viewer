import { afterEach, expect, it, vi } from 'vitest'
import { evalAPI } from './eval-service'
import { parseQuery } from '@/components/shared/Search/queryParser'
import { evalSearchDefinition } from '@/components/evals/search-fields'

afterEach(() => vi.unstubAllGlobals())

it('retains exact source JSON through the typed experiment page response and passes cancellation', async () => {
  const source = '{"exact":9007199254740993,"small":-0.00,"unknown":null}'
  const fetcher = vi.fn<typeof fetch>(
    async () =>
      new Response(
        JSON.stringify({
          summary: { id: 'a'.repeat(64), resultCount: 2 },
          contextJSON: '{"results":{"results":[]}}',
          rows: [{ ordinal: 7, json: source }],
          nextAfter: 7,
          hasMore: true,
        })
      )
  )
  vi.stubGlobal('fetch', fetcher)
  const controller = new AbortController()
  const page = await evalAPI.getEvalExperimentPage(
    'a'.repeat(64),
    4,
    1,
    controller.signal
  )
  expect(page.rows[0].json).toBe(source)
  expect(page.nextAfter).toBe(7)
  expect(fetcher).toHaveBeenCalledWith(
    `/api/evals/experiments/${'a'.repeat(64)}/rows?after=4&limit=1`,
    { signal: controller.signal }
  )
})

it('sends outcome choices as repeated server-side filters and preserves global totals', async () => {
  const fetcher = vi.fn<typeof fetch>(
    async () =>
      new Response(
        JSON.stringify({
          experiments: [],
          total: 300,
          hasMore: true,
          facets: [],
          indexing: { remaining: 2 },
        })
      )
  )
  vi.stubGlobal('fetch', fetcher)
  const page = await evalAPI.searchEvalExperiments({
    criteria: parseQuery('name = "Experiment 2"', evalSearchDefinition.fields),
    outcomes: ['success:Pass', 'error:Fail'],
    sort: 'name',
    limit: 10,
    offset: 20,
  })
  const url = new URL(String(fetcher.mock.calls[0][0]), 'http://viewer.test')
  expect(url.searchParams.getAll('outcome')).toEqual([
    'success:Pass',
    'error:Fail',
  ])
  expect(url.searchParams.has('q')).toBe(false)
  expect(JSON.parse(url.searchParams.get('criteria')!)).toMatchObject({
    type: 'condition',
    query: {
      field: { owner: 'experiment', path: ['name'] },
      fieldOperator: '=',
      value: 'Experiment 2',
    },
  })
  expect(page.total).toBe(300)
  expect(page.indexing.remaining).toBe(2)
})
