// @vitest-environment jsdom
import { expect, it, vi } from 'vitest'
import { fireEvent, render, screen, within } from '@testing-library/svelte'
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
  render(EvalRowDetail, { run, row, traceLevel: true })
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
  render(EvalRowDetail, { run, row, traceLevel: true })
  expect(
    screen.queryByRole('table', { name: `${row.name} output` })
  ).not.toBeInTheDocument()
  expect(
    screen.queryByRole('button', { name: `Copy ${row.name}` })
  ).not.toBeInTheDocument()
})
