// @vitest-environment jsdom
import { expect, it, vi } from 'vitest'
import { fireEvent, screen, within, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import { renderWithContexts as render } from '@/test/render-helpers'
import { copyText } from '@/utils/clipboard'
import EvalRowDetail from './EvalRowDetail.svelte'
import { exampleExperiments } from './examples'
import { buildEvalOutline } from './eval-outline'
import { groupResults } from './group-results'
import { indexEvalTraces } from './eval-traces'
import type { EvalSourceFieldCopy } from './eval-source-copy'

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
  expect(
    heading()
      .closest('details')!
      .parentElement!.querySelector(':scope > .detail-bubble__action')
  ).toBeNull()
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
  expect(action).not.toBeNull()
  await fireEvent.click(
    within(action as HTMLElement).getByRole('button', {
      name: 'Copy grader input',
    })
  )
  expect(copyText).toHaveBeenLastCalledWith(overall.rubric)
  const disclosure = heading().closest('details')!
  await fireEvent.click(heading())
  expect(disclosure).not.toHaveAttribute('open')
  const collapsedCopy = within(action as HTMLElement).getByRole('button', {
    name: 'Copy grader input',
  })
  expect(collapsedCopy.closest('details')).toBeNull()
  expect(collapsedCopy.closest('summary')).toBeNull()
  expect(
    disclosure.querySelector(
      'table[aria-label="Grader input"] button[aria-label="Copy grader input"]'
    )
  ).toBeNull()
  expect(
    within(action as HTMLElement).getAllByRole('button', {
      name: 'Copy grader input',
    })
  ).toHaveLength(1)
  await fireEvent.click(collapsedCopy)
  expect(copyText).toHaveBeenLastCalledWith(overall.rubric)
  expect(disclosure).not.toHaveAttribute('open')
})

it('keeps one Overall header Copy bound to retained input while closed through pending, failed, and fresh same-ID replacements', async () => {
  const original = exampleExperiments[0]
  const base = original.results[0]
  const id = original.headlineAssessment!
  const assessment = {
    ...base.assessments.find(item => item.id === id)!,
    rubric: 'Complete A input\n\t"exact" 🙂',
    explanation: 'A explanation',
  }
  const resultA = { ...base, assessments: [assessment] }
  const experiment = {
    ...original,
    results: [resultA],
    assessments: original.assessments.map(item =>
      item.id === id ? { ...item, name: 'Overall' } : item
    ),
  }
  const rowA = {
    ...buildEvalOutline(
      groupResults(experiment.results, experiment.results),
      experiment,
      indexEvalTraces([])
    ).find(row => row.kind === 'result')!,
    name: 'Result A',
  }
  const view = render(EvalRowDetail, { experiment, row: rowA })
  const heading = [
    ...view.container.querySelectorAll('summary.detail-bubble__heading'),
  ].find(element => element.textContent?.includes('Assessments'))!
  const disclosure = heading.closest('details')!
  const outer = disclosure.parentElement!
  await fireEvent.click(heading)
  const copy = within(outer).getByRole('button', { name: 'Copy grader input' })
  expect(copy.closest('details')).toBeNull()
  let rejectInput!: (reason: Error) => void
  const failedInput = vi.fn(
    (_signal?: AbortSignal) =>
      new Promise<string>((_resolve, reject) => {
        rejectInput = reject
      })
  )
  const resultB = {
    ...resultA,
    id: `${resultA.id}/B`,
    assessments: [
      { ...assessment, rubric: 'B preview', explanation: 'B explanation' },
    ],
  }
  const experimentB = { ...experiment, results: [resultB] }
  const rowB = {
    ...rowA,
    id: `${rowA.id}/B`,
    name: 'Result B',
    result: resultB,
  }
  const failedSource: EvalSourceFieldCopy = (_result, field) =>
    field === `assessment:${id}:rubric` ? failedInput : undefined
  await view.rerender({
    componentProps: {
      experiment: experimentB,
      row: rowB,
      sourceCopy: failedSource,
    },
  })
  expect(failedInput).toHaveBeenCalledTimes(1)
  expect(copy.closest('[role="group"]')).toHaveAttribute(
    'aria-label',
    `Grader input · Result A · ${resultA.id}`
  )
  expect(screen.getByText('B explanation')).toBeInTheDocument()
  expect(
    screen.getByText(`Grader input · Result A · ${resultA.id}`)
  ).toBeInTheDocument()
  expect(within(outer).getByRole('button', { name: 'Copy grader input' })).toBe(
    copy
  )
  await fireEvent.click(copy)
  expect(copyText).toHaveBeenLastCalledWith(assessment.rubric)
  expect(failedInput).toHaveBeenCalledTimes(1)
  rejectInput(new Error('replacement unavailable'))
  await screen.findByText('Could not load the full grader input.')
  expect(copy.closest('[role="group"]')).toHaveAttribute(
    'aria-label',
    `Grader input · Result A · ${resultA.id}`
  )
  await fireEvent.click(copy)
  expect(copyText).toHaveBeenLastCalledWith(assessment.rubric)
  expect(disclosure).not.toHaveAttribute('open')
  const exactB = 'Complete B input\r\n[full] 🙂'
  const freshInput = vi.fn(async (_signal?: AbortSignal) => exactB)
  const freshSource: EvalSourceFieldCopy = (_result, field) =>
    field === `assessment:${id}:rubric` ? freshInput : undefined
  await view.rerender({
    componentProps: {
      experiment: experimentB,
      row: rowB,
      sourceCopy: freshSource,
    },
  })
  await waitFor(() =>
    expect(
      screen.getByText(`Grader input · Result B · ${resultB.id}`)
    ).toBeInTheDocument()
  )
  expect(within(outer).getByRole('button', { name: 'Copy grader input' })).toBe(
    copy
  )
  expect(
    outer.querySelectorAll('button[aria-label="Copy grader input"]')
  ).toHaveLength(1)
  await fireEvent.click(copy)
  expect(copyText).toHaveBeenLastCalledWith(exactB)
  expect(freshInput).toHaveBeenCalledTimes(1)
  expect(failedInput.mock.calls[0][0]?.aborted).toBe(true)
  expect(copy.closest('[role="group"]')).toHaveAttribute(
    'aria-label',
    `Grader input · Result B · ${resultB.id}`
  )
  expect(disclosure).not.toHaveAttribute('open')
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
      section =>
        section.querySelector('.detail-bubble__title-row > [title]')
          ?.textContent
    )
  ).toEqual(['Result', 'Assessments', 'Telemetry'])
  expect(screen.getByText('Recorded answer')).toBeVisible()
  expect(screen.getByText(result.assessments[0].explanation)).toBeVisible()
  await userEvent.click(sections[2].querySelector('summary')!)
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
