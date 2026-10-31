// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { fireEvent, screen, waitFor, within } from '@testing-library/svelte'
import { tick } from 'svelte'
import { renderWithContexts as render, setTestUrl } from '@/test/render-helpers'
import { navigate } from '@/route'
import { evalAPI } from '@/services/eval-service'
import { telemetryAPI } from '@/services/telemetry-service'
import { copyText } from '@/utils/clipboard'
import { projectPromptfooDataset } from '@/components/evals/promptfoo-adapter'
import type {
  EvalExperimentSearchPage,
  EvalExperimentSourcePage,
  EvalStoredExperimentSummary,
} from '@/services/eval-experiment-types'
import EvalsWorkspace from './EvalsWorkspace.svelte'

vi.mock('@/utils/clipboard', () => ({ copyText: vi.fn() }))
afterEach(() => {
  vi.useRealTimers()
  vi.restoreAllMocks()
})

const rows = ['Alpha', 'Beta'].flatMap(provider =>
  [0, 1].map(index => ({
    id: `${provider}-${index}`,
    provider: { id: provider },
    promptId: 'prompt',
    testIdx: index,
    testCase: { description: `${provider} case ${index}` },
    prompt: { raw: `Question ${index}` },
    response: {
      output: `${provider} output ${index} ` + 'Complete 🙂 '.repeat(4000),
    },
  }))
)
const context = {
  evalId: 'loading',
  config: {},
  results: { version: 3, results: [] },
}
const projection = projectPromptfooDataset({
  id: 'd'.repeat(64),
  source: 'local',
  name: 'loading.json',
  format: 'promptfoo-v3',
  payload: JSON.stringify({
    ...context,
    results: { version: 3, results: rows },
  }),
})
if (projection.status !== 'supported') throw new Error(projection.reason)
const experiments = projection.experiments
const summaries: EvalStoredExperimentSummary[] = experiments.map(
  (experiment, index) => ({
    id: String(index + 1).repeat(64),
    datasetID: 'd'.repeat(64),
    adapterExperimentID: experiment.id,
    name: experiment.name,
    evaluation: experiment.evaluation,
    source: experiment.source,
    format: 'promptfoo-v3',
    state: 'Unknown',
    stateReason: '',
    pauseReason: null,
    startedAt: null,
    plannedCount: null,
    resultCount: experiment.results.length,
    assessmentName: 'Recorded outcome',
    outcomes: [],
  })
)
function source(index: number): EvalExperimentSourcePage {
  return {
    summary: summaries[index],
    contextJSON: JSON.stringify(context),
    rows: rows.flatMap((row, ordinal) =>
      row.provider.id === summaries[index].name
        ? [{ ordinal, json: JSON.stringify(row) }]
        : []
    ),
    nextAfter: index * 2 + 1,
    hasMore: false,
  }
}
function page(
  experiments = summaries,
  hasMore = false
): EvalExperimentSearchPage {
  return {
    experiments,
    total: summaries.length,
    hasMore,
    facets: [],
    indexing: {
      remaining: 0,
      unsupported: 0,
      empty: 0,
      issues: [],
      hasMoreIssues: false,
    },
  }
}
function mount() {
  vi.spyOn(telemetryAPI, 'getReferencedTraceViews').mockResolvedValue({
    traces: [],
    unavailableTraceIDs: [],
  })
  const lists: Array<
    ReturnType<typeof Promise.withResolvers<EvalExperimentSearchPage>>
  > = []
  const searches = vi
    .spyOn(evalAPI, 'searchEvalExperiments')
    .mockImplementation(() => {
      const pending = Promise.withResolvers<EvalExperimentSearchPage>()
      lists.push(pending)
      return pending.promise
    })
  const sources: Array<
    ReturnType<typeof Promise.withResolvers<EvalExperimentSourcePage>>
  > = []
  const details = vi
    .spyOn(evalAPI, 'getEvalExperimentPage')
    .mockImplementation(() => {
      const pending = Promise.withResolvers<EvalExperimentSourcePage>()
      sources.push(pending)
      return pending.promise
    })
  setTestUrl(
    `/evals?experiment=${summaries[0].id}&result=${experiments[0].results[0].id}`
  )
  const view = render(EvalsWorkspace)
  return { view, lists, sources, searches, details }
}

it('publishes a selected experiment without waiting for its list and retains complete row actions during failed replacement', async () => {
  const { view, lists, sources, details } = mount()
  await waitFor(() => expect(sources).toHaveLength(1))
  sources[0].resolve(source(0))
  await screen.findByRole('table', { name: 'Results for Alpha' })
  const output = await screen.findByRole('group', { name: 'output' })
  const copy = within(output).getByRole('button', { name: 'Copy output' })
  lists[0].resolve(page())
  await screen.findByRole('button', { name: /^Beta/ })
  await fireEvent.click(screen.getByRole('button', { name: /^Beta/ }))
  expect(sources).toHaveLength(2)
  expect(screen.getByRole('group', { name: 'output' })).toBe(output)
  await fireEvent.click(copy)
  expect(copyText).toHaveBeenLastCalledWith(rows[0].response.output)
  sources[1].reject(new Error('Beta source unavailable'))
  await screen.findByText(/Beta source unavailable/)
  expect(screen.getByRole('group', { name: 'output' })).toBe(output)
  await fireEvent.click(screen.getByRole('button', { name: 'Alpha case 1' }))
  await waitFor(() => expect(output).toHaveTextContent('Alpha output 1'))
  await fireEvent.click(copy)
  expect(copyText).toHaveBeenLastCalledWith(rows[1].response.output)
  expect(details.mock.calls[1][3]?.aborted).toBe(true)
  view.unmount()
})

it.each(['success', 'failure'])(
  'cancels source pagination and rejects stale %s across A → B → A and unmount',
  async outcome => {
    const { view, lists, sources, details } = mount()
    lists[0].resolve(page())
    const first = source(0)
    sources[0].resolve({
      ...first,
      rows: first.rows.slice(0, 1),
      nextAfter: 0,
      hasMore: true,
    })
    await waitFor(() => expect(sources).toHaveLength(2))
    const stalePage = sources[1]
    await fireEvent.click(screen.getByRole('button', { name: /^Beta/ }))
    expect(details.mock.calls[1][3]?.aborted).toBe(true)
    await fireEvent.click(screen.getByRole('button', { name: /^Alpha/ }))
    expect(details.mock.calls[2][3]?.aborted).toBe(true)
    sources[3].resolve(first)
    await screen.findByRole('table', { name: 'Results for Alpha' })
    if (outcome === 'failure') {
      stalePage.reject(new Error('Stale A page error'))
      sources[2].reject(new Error('Stale B error'))
    } else {
      stalePage.resolve({ ...first, rows: first.rows.slice(1) })
      sources[2].resolve(source(1))
    }
    await tick()
    expect(screen.queryByRole('alert')).toBeNull()
    expect(
      screen.getByRole('table', { name: 'Results for Alpha' })
    ).toBeVisible()
    await fireEvent.click(screen.getByRole('button', { name: /^Beta/ }))
    view.unmount()
    expect(details.mock.calls[4][3]?.aborted).toBe(true)
    sources[4].reject(new Error('Unmounted B error'))
    await tick()
    expect(screen.queryByRole('alert')).toBeNull()
  }
)

it('resets the shared 200ms list notice on query replacement while keeping retained cards usable', async () => {
  const { view, lists, sources, searches } = mount()
  lists[0].resolve(page(summaries.slice(0, 1), true))
  sources[0].resolve(source(0))
  await screen.findByRole('table', { name: 'Results for Alpha' })
  const list = within(screen.getByRole('region', { name: 'Evals list' }))
  const card = list.getByRole('button', { name: /^Alpha/ })
  vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout'] })
  navigate(`/evals?experiment=${summaries[0].id}&esort=name`)
  await tick()
  expect(searches).toHaveBeenCalledTimes(2)
  expect(screen.queryByText('Loading experiments…')).toBeNull()
  await vi.advanceTimersByTimeAsync(199)
  expect(screen.queryByText('Loading experiments…')).toBeNull()
  await vi.advanceTimersByTimeAsync(1)
  expect(screen.getByText('Loading experiments…')).toBeVisible()
  navigate(`/evals?experiment=${summaries[0].id}&esort=started`)
  await tick()
  expect(searches).toHaveBeenCalledTimes(3)
  expect(searches.mock.calls[1][1]?.aborted).toBe(true)
  expect(screen.queryByText('Loading experiments…')).toBeNull()
  expect(list.getByRole('button', { name: /^Alpha/ })).toBe(card)
  expect(card).toBeEnabled()
  await vi.advanceTimersByTimeAsync(199)
  expect(screen.queryByText('Loading experiments…')).toBeNull()
  lists[2].resolve(page())
  await tick()
  await vi.advanceTimersByTimeAsync(1)
  expect(screen.queryByText('Loading experiments…')).toBeNull()
  view.unmount()
})

it('retains the replacement error and its own query while pagination continues the displayed list', async () => {
  const { lists, sources, searches } = mount()
  lists[0].resolve(page(summaries.slice(0, 1), true))
  sources[0].resolve(source(0))
  await screen.findByRole('table', { name: 'Results for Alpha' })
  navigate(`/evals?experiment=${summaries[0].id}&esort=name`)
  await waitFor(() => expect(lists).toHaveLength(2))
  lists[1].reject(new Error('Replacement list unavailable'))
  const failure = await screen.findByText('Replacement list unavailable')
  await fireEvent.click(
    screen.getByRole('button', { name: 'Load more experiments' })
  )
  expect(searches.mock.calls[2][0]).toMatchObject({
    sort: 'started',
    offset: 1,
  })
  expect(failure).toBeVisible()
  lists[2].resolve(page(summaries.slice(1)))
  await screen.findByRole('button', { name: /^Beta/ })
  expect(failure).toBeVisible()
  expect(searches).toHaveBeenCalledTimes(3)
})

it.each(['query', 'replacement', 'unmount'])(
  'aborts list pagination on %s and ignores its late success and failure',
  async action => {
    const { view, lists, sources, searches } = mount()
    lists[0].resolve(page(summaries.slice(0, 1), true))
    sources[0].resolve(source(0))
    await screen.findByRole('table', { name: 'Results for Alpha' })
    const more = screen.getByRole('button', { name: 'Load more experiments' })
    if (action === 'replacement') {
      navigate(`/evals?experiment=${summaries[0].id}&esort=name`)
      await waitFor(() => expect(lists).toHaveLength(2))
    }
    await fireEvent.click(more)
    const index = lists.length - 1
    expect(searches.mock.calls[index][0]).toMatchObject({
      sort: 'started',
      offset: 1,
    })
    if (action === 'unmount') view.unmount()
    else {
      if (action === 'query') {
        navigate(`/evals?experiment=${summaries[0].id}&esort=name`)
        await waitFor(() => expect(lists).toHaveLength(3))
        lists[2].resolve(page([], false))
      } else lists[1].resolve(page([], false))
      await waitFor(() => expect(more.isConnected).toBe(false))
    }
    expect(searches.mock.calls[index][1]?.aborted).toBe(true)
    if (action === 'replacement')
      lists[index].reject(new Error('Stale page error'))
    else lists[index].resolve(page(summaries.slice(1)))
    await tick()
    expect(screen.queryByText('Stale page error')).toBeNull()
    expect(screen.queryByRole('button', { name: /^Beta/ })).toBeNull()
  }
)
