// @vitest-environment jsdom
import { expect, it, vi } from 'vitest'
import { fireEvent, screen, within } from '@testing-library/svelte'
import { renderWithContexts as render } from '@/test/render-helpers'
import { copyText } from '@/utils/clipboard'
import EvalRowDetail from './EvalRowDetail.svelte'
import { exampleRuns } from './examples'
import { buildEvalOutline } from './eval-outline'
import { groupReps } from './group-reps'
import { indexEvalTraces } from './eval-traces'

vi.mock('@/utils/clipboard', () => ({ copyText: vi.fn() }))

it('keeps an explicitly empty tool output inspectable and copyable', async () => {
  const original = exampleRuns[0].reps[0]
  const rep = {
    ...original,
    tools: [{ ...original.tools[0], message: '' }],
  }
  const run = { ...exampleRuns[0], reps: [rep] }
  const row = buildEvalOutline(
    groupReps(run.reps, run.reps),
    run,
    indexEvalTraces([])
  ).find(row => row.kind === 'tool')!
  render(EvalRowDetail, { run, row, mode: 'time' })
  const output = screen.getByRole('table', { name: `${row.name} output` })
  expect(within(output).getByText('""')).toBeVisible()
  expect(screen.getByRole('button', { name: `Copy ${row.name}` })).toBeEnabled()
  await fireEvent.click(
    screen.getByRole('button', { name: `Copy ${row.name}` })
  )
  expect(copyText).toHaveBeenCalledExactlyOnceWith('')
})

it('omits output and its copy actions when a tool has no recorded message', () => {
  const original = exampleRuns[0].reps[0]
  const rep = {
    ...original,
    tools: [{ ...original.tools[0], message: undefined }],
  }
  const run = { ...exampleRuns[0], reps: [rep] }
  const row = buildEvalOutline(
    groupReps(run.reps, run.reps),
    run,
    indexEvalTraces([])
  ).find(row => row.kind === 'tool')!
  render(EvalRowDetail, { run, row, mode: 'time' })
  expect(
    screen.queryByRole('table', { name: `${row.name} output` })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: `Copy ${row.name}` })
  ).not.toBeInTheDocument()
})

it('keeps Details and Telemetry stable while the selected mode surfaces its own evidence', async () => {
  const rep = {
    ...exampleRuns[0].reps[0],
    answer: 'Recorded answer',
    seconds: 0,
    investigatorCost: 0,
    graderCost: undefined,
  }
  const run = { ...exampleRuns[0], reps: [rep] }
  const row = buildEvalOutline(
    groupReps(run.reps, run.reps),
    run,
    indexEvalTraces([])
  ).find(row => row.kind === 'rep')!
  const view = render(EvalRowDetail, { run, row, mode: 'results' })
  const sections = [
    ...view.container.querySelectorAll<HTMLDetailsElement>('details.collapse'),
  ]
  expect(
    sections.map(
      section => section.querySelector('summary > span')?.textContent
    )
  ).toEqual(['Result', 'Assessments', 'Telemetry'])
  expect(screen.getByText('Recorded answer')).toBeVisible()
  expect(screen.getByText(rep.assessments[0].explanation)).toBeVisible()
  await fireEvent.click(sections[2].querySelector('summary')!)
  expect(sections[2].open).toBe(false)

  await view.rerender({ componentProps: { run, row, mode: 'time' } })
  expect(screen.queryByText('Recorded answer')).toBeNull()
  expect(screen.getByRole('group', { name: 'tool calls' })).toBeVisible()
  expect(screen.getByText('0 s')).toBeVisible()
  expect(sections[2].open).toBe(false)

  await view.rerender({ componentProps: { run, row, mode: 'cost' } })
  const accounting = screen.getByRole('table', {
    name: 'Result accounting',
  })
  expect(
    within(accounting).getByRole('group', {
      name: 'investigator estimate (CAD)',
    })
  ).toHaveTextContent('C$0.000')
  expect(
    within(accounting).getByRole('group', {
      name: 'grader estimate (CAD)',
    })
  ).toHaveTextContent('Unknown')
  expect(
    within(accounting).getByRole('group', {
      name: 'total estimate (CAD)',
    })
  ).toHaveTextContent('C$0.000 · partial')
  expect(sections[2].open).toBe(false)
})

it('retains the recorded rep start and exact Copy when its spans are unavailable', async () => {
  const startedAt = '2026-10-20T12:34:56.123456789-07:00'
  const rep = { ...exampleRuns[0].reps[0], startedAt }
  const run = { ...exampleRuns[0], reps: [rep] }
  const row = buildEvalOutline(
    groupReps(run.reps, run.reps),
    run,
    indexEvalTraces([])
  ).find(row => row.kind === 'rep')!
  const view = render(EvalRowDetail, { run, row, mode: 'results' })
  expect(screen.getAllByText('Span unavailable').length).toBeGreaterThan(0)

  for (const mode of ['results', 'time'] as const) {
    await view.rerender({ componentProps: { run, row, mode } })
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
      run,
      row: { ...row, rep: { ...rep, startedAt: undefined } },
      mode: 'time',
    },
  })
  expect(screen.queryByRole('group', { name: 'started' })).toBeNull()
})
