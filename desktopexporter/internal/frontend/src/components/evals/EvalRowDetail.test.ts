// @vitest-environment jsdom
import { expect, it, vi } from 'vitest'
import { fireEvent, screen, within } from '@testing-library/svelte'
import { renderWithContexts as render } from '@/test/render-helpers'
import { copyText } from '@/utils/clipboard'
import EvalRowDetail from './EvalRowDetail.svelte'
import { exampleRuns } from './examples'
import { buildEvalOutline } from './eval-outline'
import { groupAttempts } from './group-attempts'
import { indexEvalTraces } from './eval-traces'

vi.mock('@/utils/clipboard', () => ({ copyText: vi.fn() }))

it('keeps an explicitly empty tool output inspectable and copyable', async () => {
  const original = exampleRuns[0].attempts[0]
  const attempt = {
    ...original,
    tools: [{ ...original.tools[0], message: '' }],
  }
  const run = { ...exampleRuns[0], attempts: [attempt] }
  const row = buildEvalOutline(
    groupAttempts(run.attempts, run.attempts),
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
  const original = exampleRuns[0].attempts[0]
  const attempt = {
    ...original,
    tools: [{ ...original.tools[0], message: undefined }],
  }
  const run = { ...exampleRuns[0], attempts: [attempt] }
  const row = buildEvalOutline(
    groupAttempts(run.attempts, run.attempts),
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
  const attempt = {
    ...exampleRuns[0].attempts[0],
    answer: 'Recorded answer',
    seconds: 0,
    investigatorCost: 0,
    graderCost: undefined,
  }
  const run = { ...exampleRuns[0], attempts: [attempt] }
  const row = buildEvalOutline(
    groupAttempts(run.attempts, run.attempts),
    run,
    indexEvalTraces([])
  ).find(row => row.kind === 'attempt')!
  const view = render(EvalRowDetail, { run, row, mode: 'results' })
  const sections = [
    ...view.container.querySelectorAll<HTMLDetailsElement>('details.collapse'),
  ]
  expect(
    sections.map(
      section => section.querySelector('summary > span')?.textContent
    )
  ).toEqual(['Details', 'Telemetry'])
  expect(screen.getByText('Recorded answer')).toBeVisible()
  expect(screen.getByText(attempt.assessments[0].explanation)).toBeVisible()
  await fireEvent.click(sections[1].querySelector('summary')!)
  expect(sections[1].open).toBe(false)

  await view.rerender({ componentProps: { run, row, mode: 'time' } })
  expect(screen.queryByText('Recorded answer')).toBeNull()
  expect(screen.getByRole('group', { name: 'tool calls' })).toBeVisible()
  expect(screen.getByText('0 s')).toBeVisible()
  expect(sections[1].open).toBe(false)

  await view.rerender({ componentProps: { run, row, mode: 'cost' } })
  const accounting = screen.getByRole('table', {
    name: 'Attempt 1 accounting',
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
  expect(sections[1].open).toBe(false)
})
