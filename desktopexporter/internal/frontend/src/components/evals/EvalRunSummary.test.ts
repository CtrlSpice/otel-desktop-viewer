// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import EvalRunSummary from './EvalRunSummary.svelte'
import { exampleRuns } from './examples'

describe('EvalRunSummary', () => {
  it('uses continuous working-size metadata rows and a regular-weight score', () => {
    const { container } = render(EvalRunSummary, {
      run: exampleRuns[0],
      selected: true,
      onclick: vi.fn(),
    })
    const card = screen.getByRole('button', { name: /Guide-assisted agent/ })
    expect(card).toHaveAttribute('aria-pressed', 'true')
    expect(card).toHaveClass('signal-row', 'item-state')
    const score = screen.getByText('78.1%')
    expect(score.tagName).toBe('SPAN')
    expect(container.querySelector('strong, b')).toBeNull()
    expect(screen.getAllByRole('term')).toHaveLength(3)
    expect(card).not.toHaveTextContent('16/16 complete')
    expect(screen.queryByText('Graded:')).not.toBeInTheDocument()
    expect(card).toHaveTextContent('35 tool calls (3 failures)')
  })

  it('shows every outcome together and selects the run through its card', async () => {
    const user = userEvent.setup()
    const onclick = vi.fn()
    render(EvalRunSummary, { run: exampleRuns[0], onclick })
    const group = screen.getByRole('group', {
      name: 'Task completion outcomes',
    })
    const outcomes = within(group).getAllByRole('listitem')
    expect(outcomes).toHaveLength(3)
    expect(outcomes[0]).toHaveTextContent('Pass (11)')
    expect(outcomes[1]).toHaveTextContent('Partial (3)')
    expect(outcomes[2]).toHaveTextContent('Fail (2)')
    expect(within(group).queryByRole('button')).not.toBeInTheDocument()
    expect(onclick).not.toHaveBeenCalled()
    expect(document.querySelector('button button')).toBeNull()
    await user.click(
      screen.getByRole('button', { name: /Guide-assisted agent/ })
    )
    expect(onclick).toHaveBeenCalledWith('guide-assisted')
    expect(screen.getByText('78.1%')).toBeInTheDocument()
  })

  it('keeps planned work and execution errors separate from failed grades', () => {
    render(EvalRunSummary, { run: exampleRuns[1], onclick: vi.fn() })
    const group = screen.getByRole('group', {
      name: 'Task completion outcomes',
    })
    const outcomes = within(group).getAllByRole('listitem')
    expect(
      outcomes.find(item => item.textContent?.includes('Internal fault'))
    ).toHaveTextContent('Internal fault (2)')
    expect(
      outcomes.find(item => item.textContent?.includes('Planned'))
    ).toHaveTextContent('Planned (4)')
    expect(
      outcomes.find(item => item.textContent?.includes('Fail'))
    ).toHaveTextContent('Fail (2)')
    expect(
      screen.getByText('Partial est. cost:').parentElement
    ).toHaveTextContent('Partial est. cost: C$0.592')
    expect(screen.getByText('10/16 complete')).toBeInTheDocument()
  })

  it('uses a plain label for a single outcome without inventing grades or cost', () => {
    render(EvalRunSummary, { run: exampleRuns[2], onclick: vi.fn() })
    const card = screen.getByRole('button', { name: /Revised instructions/ })
    expect(card).toHaveTextContent('Planned (16)')
    expect(
      screen.queryByRole('group', { name: 'Task completion outcomes' })
    ).not.toBeInTheDocument()
    expect(screen.queryByText('Unscored')).not.toBeInTheDocument()
    expect(screen.queryByText(/Est\. cost:/)).not.toBeInTheDocument()
    expect(screen.queryByText('Tool activity:')).not.toBeInTheDocument()
    expect(screen.queryByText('Progress:')).not.toBeInTheDocument()
  })
})
