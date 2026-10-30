// @vitest-environment jsdom
import { expect, it, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte'
import EvalAssessmentDetail from './EvalAssessmentDetail.svelte'
import { exampleExperiments } from './examples'
import { copyText } from '@/utils/clipboard'
import { tick } from 'svelte'

vi.mock('@/utils/clipboard', () => ({ copyText: vi.fn() }))

it('keeps the exact full-input Copy mounted outside a collapsed assessment disclosure', async () => {
  const assessment = {
    ...exampleExperiments[0].results[0].assessments[0],
    rubric: 'Complete grader input\nwith instructions and evidence',
  }
  const view = render(EvalAssessmentDetail, { assessment })
  const copy = screen.getByRole('button', {
    name: 'Copy grader input',
  })
  const disclosure = view.container.querySelector(
    '.eval-assessment-detail > details'
  )!
  expect(copy.closest('details')).toBeNull()
  expect(copy.closest('summary')).toBeNull()
  await fireEvent.click(disclosure.querySelector(':scope > summary')!)
  expect(disclosure).not.toHaveAttribute('open')
  expect(screen.getByRole('button', { name: 'Copy grader input' })).toBe(copy)
  await fireEvent.click(copy)
  await waitFor(() =>
    expect(copyText).toHaveBeenLastCalledWith(assessment.rubric)
  )
  expect(disclosure).not.toHaveAttribute('open')
})

it('updates scores and explanations immediately while retaining identified grader input and Copy', async () => {
  const assessment = {
    ...exampleExperiments[0].results[0].assessments[0],
    rubric: 'Original criterion',
  }
  const view = render(EvalAssessmentDetail, {
    assessment,
    contextLabel: 'Result A',
  })
  const field = await screen.findByText('Original criterion')
  let finish!: (text: string) => void
  const loadRubric = vi.fn(
    () =>
      new Promise<string>(resolve => {
        finish = resolve
      })
  )
  await view.rerender({
    assessment: {
      ...assessment,
      rubric: 'Replacement preview',
      value: 0.25,
      explanation: 'New explanation',
    },
    contextLabel: 'Result B',
    loadRubric,
  })
  expect(screen.getByText('Original criterion')).toBe(field)
  expect(screen.getByText('New explanation')).toBeVisible()
  expect(screen.getByText('Grader input · Result A')).toBeVisible()
  await fireEvent.click(
    screen.getByRole('button', { name: 'Copy grader input' })
  )
  await waitFor(() =>
    expect(copyText).toHaveBeenLastCalledWith('Original criterion')
  )
  expect(loadRubric).toHaveBeenCalledTimes(1)
  finish('Complete replacement criterion')
  await screen.findByText('Complete replacement criterion')
  expect(screen.getByText('Grader input · Result B')).toBeVisible()
  expect(screen.queryByText('Original criterion')).toBeNull()
})

it('guards A → B → A rubric loads and resets the shared 200ms notice for each request', async () => {
  vi.useFakeTimers()
  try {
    const assessment = {
      ...exampleExperiments[0].results[0].assessments[0],
      rubric: 'Initial input',
    }
    const pending: Array<(text: string) => void> = []
    const signals: AbortSignal[] = []
    const load = () => (signal?: AbortSignal) =>
      new Promise<string>(resolve => {
        if (signal) signals.push(signal)
        pending.push(resolve)
      })
    const view = render(EvalAssessmentDetail, {
      assessment,
      loadRubric: load(),
      contextLabel: 'A',
    })
    await tick()
    expect(pending).toHaveLength(1)
    await vi.advanceTimersByTimeAsync(199)
    expect(screen.queryByText('Loading grader input…')).toBeNull()
    await view.rerender({ assessment, loadRubric: load(), contextLabel: 'B' })
    await vi.advanceTimersByTimeAsync(199)
    expect(screen.queryByText('Loading grader input…')).toBeNull()
    await view.rerender({ assessment, loadRubric: load(), contextLabel: 'A' })
    expect(signals[0].aborted).toBe(true)
    expect(signals[1].aborted).toBe(true)
    await vi.advanceTimersByTimeAsync(200)
    expect(screen.getByText('Loading grader input…')).toBeVisible()
    pending[2]('Fresh A')
    await tick()
    await tick()
    pending[1]('Stale B')
    pending[0]('Stale A')
    await tick()
    await tick()
    expect(screen.getByText('Fresh A')).toBeVisible()
    expect(screen.queryByText('Stale A')).toBeNull()
    expect(screen.queryByText('Stale B')).toBeNull()
    view.unmount()
  } finally {
    vi.useRealTimers()
  }
})
