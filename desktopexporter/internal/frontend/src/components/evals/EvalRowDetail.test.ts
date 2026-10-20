// @vitest-environment jsdom
import { expect, it } from 'vitest'
import { render, screen, within } from '@testing-library/svelte'
import EvalRowDetail from './EvalRowDetail.svelte'
import { exampleRuns } from './examples'
import { buildEvalOutline } from './eval-outline'
import { groupAttempts } from './group-attempts'
import { indexEvalTraces } from './eval-traces'

it('keeps an explicitly empty tool output inspectable and copyable', () => {
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
})
