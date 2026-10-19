// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import EvalRunSummary from './EvalRunSummary.svelte'
import { exampleRuns } from './examples'

describe('EvalRunSummary', () => {
  it('uses outcome bubbles and a stable label without selecting the run during exploration', async () => {
    const user = userEvent.setup()
    const onclick = vi.fn()
    render(EvalRunSummary, { run: exampleRuns[0], onclick })
    const group = screen.getByRole('group', {
      name: 'Task completion outcomes',
    })
    const label = () => group.querySelector('.breakdown-label')
    expect(label()).toHaveTextContent('Pass (6 attempts, 75%)')
    expect(group.querySelector('.ball-pit .identifier')).toHaveAttribute(
      'title',
      'guide-assisted'
    )
    const fail = within(group).getByRole('button', {
      name: 'Fail: 1 attempt, 12.5% of total',
    })
    await user.hover(fail)
    await waitFor(() =>
      expect(label()).toHaveTextContent('Fail (1 attempt, 12.5%)')
    )
    await user.click(fail)
    expect(fail).toHaveAttribute('aria-pressed', 'true')
    expect(onclick).not.toHaveBeenCalled()
    expect(document.querySelector('button button')).toBeNull()
    await user.click(
      screen.getByRole('button', { name: /Guide-assisted agent/ })
    )
    expect(onclick).toHaveBeenCalledWith('guide-assisted')
    expect(screen.getByText('81.3%')).toBeInTheDocument()
  })

  it('keeps planned work and execution errors separate from failed grades', () => {
    render(EvalRunSummary, { run: exampleRuns[1], onclick: vi.fn() })
    const group = screen.getByRole('group', {
      name: 'Task completion outcomes',
    })
    expect(
      within(group).getByRole('button', {
        name: 'Execution error: 1 attempt, 12.5% of total',
      })
    ).toHaveStyle('--part-color: var(--color-papaya)')
    expect(
      within(group).getByRole('button', {
        name: 'Planned: 2 attempts, 25% of total',
      })
    ).toBeInTheDocument()
    expect(
      within(group).getByRole('button', {
        name: 'Fail: 1 attempt, 12.5% of total',
      })
    ).toBeInTheDocument()
  })

  it('uses a plain label for a single outcome without inventing grades or cost', () => {
    render(EvalRunSummary, { run: exampleRuns[2], onclick: vi.fn() })
    const group = screen.getByRole('group', {
      name: 'Task completion outcomes',
    })
    expect(group.querySelector('.breakdown-label')).toHaveTextContent(
      'Planned (8 attempts)'
    )
    expect(within(group).queryByRole('button')).not.toBeInTheDocument()
    expect(screen.getByText('Unscored')).toBeInTheDocument()
    expect(screen.getByText('Est. cost: Unknown')).toBeInTheDocument()
  })
})
