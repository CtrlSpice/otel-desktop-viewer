// @vitest-environment jsdom
import { expect, it, vi } from 'vitest'
import { fireEvent, screen, within } from '@testing-library/svelte'
import { renderWithContexts as render } from '@/test/render-helpers'
import { copyText } from '@/utils/clipboard'
import EvalRowDetail from './EvalRowDetail.svelte'
import { exampleExperiments } from './examples'
import { buildEvalOutline } from './eval-outline'
import { groupResults } from './group-results'
import { indexEvalTraces } from './eval-traces'

vi.mock('@/utils/clipboard', () => ({ copyText: vi.fn() }))

it('promotes only an explicit Overall judgment and preserves its evidence and input Copy', async () => {
  const result = exampleExperiments[0].results[0]
  const experiment = { ...exampleExperiments[0], results: [result] }
  const row = buildEvalOutline(
    groupResults(experiment.results, experiment.results),
    experiment,
    indexEvalTraces([])
  ).find(row => row.kind === 'result')!
  const view = render(EvalRowDetail, { experiment, row })
  const heading = () =>
    [...view.container.querySelectorAll('summary.detail-bubble__heading')].find(
      el => el.textContent?.includes('Assessments')
    )!
  expect(heading()).not.toHaveTextContent('Pass')
  const overall = result.assessments.find(
    item => item.id === experiment.headlineAssessment
  )!
  await view.rerender({
    componentProps: {
      experiment: {
        ...experiment,
        assessments: experiment.assessments.map(item =>
          item.id === overall.id ? { ...item, name: 'Overall' } : item
        ),
      },
      row,
    },
  })
  expect(heading()).toHaveTextContent(overall.label)
  expect(heading()).not.toHaveTextContent('Overall')
  expect(screen.queryByText('Overall', { exact: true })).toBeNull()
  expect(screen.getByText(overall.explanation)).toBeVisible()
  expect(heading().querySelector('button')).toBeNull()
  const action = heading()
    .closest('details')!
    .parentElement!.querySelector(':scope > .detail-bubble__action')!
  await fireEvent.click(
    within(action as HTMLElement).getByRole('button', {
      name: 'Copy grader input',
    })
  )
  expect(copyText).toHaveBeenLastCalledWith(overall.rubric)
})

it('keeps an explicitly empty tool output inspectable and copyable', async () => {
  const original = exampleExperiments[0].results[0]
  const result = {
    ...original,
    tools: [{ ...original.tools[0], message: '' }],
  }
  const experiment = { ...exampleExperiments[0], results: [result] }
  const row = buildEvalOutline(
    groupResults(experiment.results, experiment.results),
    experiment,
    indexEvalTraces([])
  ).find(row => row.kind === 'tool')!
  render(EvalRowDetail, { experiment, row, mode: 'time' })
  const output = screen.getByRole('table', { name: `${row.name} output` })
  expect(within(output).getByText('""')).toBeVisible()
  expect(screen.getByRole('button', { name: 'Copy output' })).toBeEnabled()
  await fireEvent.click(screen.getByRole('button', { name: 'Copy output' }))
  expect(copyText).toHaveBeenCalledExactlyOnceWith('')
})

it('omits output and its copy actions when a tool has no recorded message', () => {
  const original = exampleExperiments[0].results[0]
  const result = {
    ...original,
    tools: [{ ...original.tools[0], message: undefined }],
  }
  const experiment = { ...exampleExperiments[0], results: [result] }
  const row = buildEvalOutline(
    groupResults(experiment.results, experiment.results),
    experiment,
    indexEvalTraces([])
  ).find(row => row.kind === 'tool')!
  render(EvalRowDetail, { experiment, row, mode: 'time' })
  expect(
    screen.queryByRole('table', { name: `${row.name} output` })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: 'Copy output' })
  ).not.toBeInTheDocument()
})

it('keeps Details and Telemetry stable while the selected mode surfaces its own evidence', async () => {
  const result = {
    ...exampleExperiments[0].results[0],
    answer: 'Recorded answer',
    seconds: 0,
    investigatorCost: 0,
    graderCost: undefined,
  }
  const experiment = { ...exampleExperiments[0], results: [result] }
  const row = buildEvalOutline(
    groupResults(experiment.results, experiment.results),
    experiment,
    indexEvalTraces([])
  ).find(row => row.kind === 'result')!
  const view = render(EvalRowDetail, { experiment, row, mode: 'results' })
  const sections = [
    ...view.container.querySelectorAll<HTMLDetailsElement>('details.collapse'),
  ]
  expect(
    sections.map(
      section => section.querySelector('summary > span')?.textContent
    )
  ).toEqual(['Result', 'Assessments', 'Telemetry'])
  expect(screen.getByText('Recorded answer')).toBeVisible()
  expect(screen.getByText(result.assessments[0].explanation)).toBeVisible()
  await fireEvent.click(sections[2].querySelector('summary')!)
  expect(sections[2].open).toBe(false)

  await view.rerender({ componentProps: { experiment, row, mode: 'time' } })
  expect(screen.queryByText('Recorded answer')).toBeNull()
  expect(screen.getByRole('group', { name: 'tool calls' })).toBeVisible()
  expect(screen.queryByRole('group', { name: 'duration' })).toBeNull()
  expect(sections[2].open).toBe(false)

  await view.rerender({ componentProps: { experiment, row, mode: 'cost' } })
  const accounting = screen.getByRole('table', {
    name: 'Result accounting',
  })
  expect(
    within(accounting).getByRole('group', {
      name: 'investigator API estimate (CAD)',
    })
  ).toHaveTextContent('C$0.000')
  expect(
    within(accounting).getByRole('group', {
      name: 'grader API estimate (CAD)',
    })
  ).toHaveTextContent('Unknown')
  expect(
    within(accounting).getByRole('group', {
      name: 'total API estimate (CAD)',
    })
  ).toHaveTextContent('C$0.000 · partial')
  expect(sections[2].open).toBe(false)
})

it('retains the recorded result start and exact Copy when its spans are unavailable', async () => {
  const startedAt = '2026-10-20T12:34:56.123456789-07:00'
  const result = { ...exampleExperiments[0].results[0], startedAt }
  const experiment = { ...exampleExperiments[0], results: [result] }
  const row = buildEvalOutline(
    groupResults(experiment.results, experiment.results),
    experiment,
    indexEvalTraces([])
  ).find(row => row.kind === 'result')!
  const view = render(EvalRowDetail, { experiment, row, mode: 'results' })
  expect(screen.getAllByText('Span unavailable').length).toBeGreaterThan(0)

  for (const mode of ['results', 'time'] as const) {
    await view.rerender({ componentProps: { experiment, row, mode } })
    const started = screen.getByRole('group', { name: 'started' })
    expect(within(started).getByTitle(startedAt)).toBeVisible()
    expect(started.querySelector('.detail-property__raw-timestamp')).toBeNull()
    await fireEvent.click(
      within(started).getByRole('button', { name: 'Copy started' })
    )
    expect(copyText).toHaveBeenLastCalledWith(startedAt)
  }

  await view.rerender({
    componentProps: {
      experiment,
      row: { ...row, result: { ...result, startedAt: undefined } },
      mode: 'time',
    },
  })
  expect(screen.queryByRole('group', { name: 'started' })).toBeNull()
})
