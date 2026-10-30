// @vitest-environment jsdom
import { expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte'
import EvalAssessmentDetail from './EvalAssessmentDetail.svelte'
import { exampleExperiments } from './examples'
import { copyText } from '@/utils/clipboard'

vi.mock('@/utils/clipboard', () => ({ copyText: vi.fn() }))

it('publishes grader input, assessment metadata and Copy together', async () => {
  const assessment = {
    ...exampleExperiments[0].results[0].assessments[0],
    rubric: 'Original criterion',
  }
  const view = render(EvalAssessmentDetail, { assessment })
  const field = await screen.findByText('Original criterion')
  let finish!: (text: string) => void
  const loadRubric = vi.fn(
    () =>
      new Promise<string>(resolve => {
        finish = resolve
      })
  )
  await view.rerender({
    assessment: { ...assessment, rubric: 'Replacement preview' },
    loadRubric,
  })
  expect(screen.getByText('Original criterion')).toBe(field)
  await fireEvent.click(
    screen.getByRole('button', { name: 'Copy grader input' })
  )
  await waitFor(() =>
    expect(copyText).toHaveBeenLastCalledWith('Original criterion')
  )
  expect(loadRubric).toHaveBeenCalledTimes(1)
  finish('Complete replacement criterion')
  await screen.findByText('Complete replacement criterion')
  expect(screen.queryByText('Original criterion')).toBeNull()
})
