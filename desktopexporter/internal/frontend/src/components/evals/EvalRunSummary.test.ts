// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { screen, within } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import EvalRunSummary from './EvalRunSummary.svelte'
import { exampleRuns } from './examples'
import { renderWithContexts as render } from '@/test/render-helpers'
import { formatTimestampParts, normalizeTimezone } from '@/utils/time'
import { toRunCard } from './run-summary'

describe('EvalRunSummary', () => {
  it('keeps run summaries concise with outcome labels in compact tooltips', () => {
    const { container } = render(EvalRunSummary, {
      run: toRunCard(exampleRuns[0]),
      selected: true,
      onclick: vi.fn(),
    })
    const card = screen.getByRole('button', { name: /Guide-assisted agent/ })
    expect(card).toHaveAttribute('aria-pressed', 'true')
    expect(card).toHaveClass('signal-row', 'item-state')
    expect(card).toHaveTextContent('Guide-assisted agent')
    expect(card).toHaveTextContent('Complete')
    expect(container.querySelector('strong, b')).toBeNull()
    expect(screen.queryByRole('term')).not.toBeInTheDocument()
    expect(card).not.toHaveTextContent('16/16 complete')
    expect(screen.queryByText('Graded:')).not.toBeInTheDocument()
    expect(card).not.toHaveTextContent('35 tool calls (3 failures)')
    expect(screen.queryByText('78.1%')).not.toBeInTheDocument()
    const bar = screen.getByRole('group', {
      name: /^Task completion by rep count/,
    })
    expect(bar).not.toHaveTextContent(/complete|Score:|tool calls|cost:/)
    expect(
      screen.queryByRole('list', {
        name: 'Task completion outcomes legend',
      })
    ).not.toBeInTheDocument()
    expect(container.querySelector('.signal-row__meta-row')).toBeNull()
  })

  it('shows compact outcome tooltips and selects the run through its segments', async () => {
    const user = userEvent.setup()
    const onclick = vi.fn()
    render(EvalRunSummary, { run: toRunCard(exampleRuns[0]), onclick })
    const group = screen.getByRole('group', {
      name: 'Task completion outcomes',
    })
    within(group).getByRole('group', {
      name: /Task completion by rep count/,
    })
    expect(group.querySelectorAll('.breakdown-segment')).toHaveLength(3)
    expect(within(group).queryByRole('listitem')).not.toBeInTheDocument()
    expect(
      [...group.querySelectorAll('[role="tooltip"]')].map(node =>
        node.textContent?.trim()
      )
    ).toEqual(['Pass [11]', 'Partial [3]', 'Fail [2]'])
    const segment = within(group).getByRole('button', { name: 'Pass: 11 reps' })
    await user.tab()
    await user.tab()
    expect(segment).toHaveFocus()
    expect(onclick).not.toHaveBeenCalled()
    await user.keyboard('{Enter}')
    expect(onclick).toHaveBeenCalledWith('guide-assisted')
    expect(onclick).toHaveBeenCalledTimes(1)
    await user.click(segment)
    expect(onclick).toHaveBeenCalledTimes(2)
    expect(document.querySelector('button button')).toBeNull()
    await user.click(
      screen.getByRole('button', { name: /Guide-assisted agent/ })
    )
    expect(onclick).toHaveBeenCalledWith('guide-assisted')
    expect(onclick).toHaveBeenCalledTimes(3)
  })

  it('keeps planned work and execution errors separate from failed grades', () => {
    render(EvalRunSummary, {
      run: toRunCard(exampleRuns[1]),
      onclick: vi.fn(),
    })
    const group = screen.getByRole('group', {
      name: 'Task completion outcomes',
    })
    expect(group).toHaveTextContent('Internal fault [2]')
    expect(group).toHaveTextContent('Planned [4]')
    expect(group).toHaveTextContent('Fail [2]')
  })

  it('uses a plain label for a single outcome without inventing grades or cost', () => {
    render(EvalRunSummary, {
      run: toRunCard(exampleRuns[2]),
      onclick: vi.fn(),
    })
    const card = screen.getByRole('button', { name: /Revised instructions/ })
    expect(card).toHaveTextContent('Planned [16]')
    expect(
      screen.queryByRole('group', { name: 'Task completion outcomes' })
    ).not.toBeInTheDocument()
    expect(screen.queryByText('Unscored')).not.toBeInTheDocument()
    expect(screen.queryByText(/Est\. cost:/)).not.toBeInTheDocument()
    expect(screen.queryByText('Tool activity:')).not.toBeInTheDocument()
    expect(screen.queryByText('Progress:')).not.toBeInTheDocument()
  })

  it('does not turn missing grades and cost into received zeros', () => {
    const run = {
      ...exampleRuns[0],
      reps: exampleRuns[0].reps.map(rep => ({
        ...rep,
        assessments: [],
        tools: [],
        investigatorCost: undefined,
      })),
    }
    render(EvalRunSummary, { run: toRunCard(run), onclick: vi.fn() })
    const group = screen.getByRole('group', {
      name: 'Task completion outcomes',
    })
    expect(group).toHaveTextContent('Unscored [16]')
    expect(group.querySelector('[role="tooltip"]')).toBeNull()
  })

  it('retains received zero scores and costs without calling failed grades execution faults', () => {
    const run = {
      ...exampleRuns[0],
      reps: exampleRuns[0].reps.map(rep => ({
        ...rep,
        assessments: rep.assessments.map(assessment => ({
          ...assessment,
          value: 0,
          label: 'Fail',
        })),
        investigatorCost: 0,
      })),
    }
    render(EvalRunSummary, { run: toRunCard(run), onclick: vi.fn() })
    const group = screen.getByRole('group', {
      name: 'Task completion outcomes',
    })
    expect(group).toHaveTextContent('Fail [16]')
    expect(group.querySelector('[role="tooltip"]')).toBeNull()
  })

  it.each(['UTC', 'America/Toronto'] as const)(
    'labels the received start in the selected %s timezone at the bottom of the card',
    tz => {
      localStorage.setItem('time-tz', tz)
      const { container } = render(EvalRunSummary, {
        run: toRunCard(exampleRuns[0]),
        onclick: vi.fn(),
      })
      const timestamp =
        BigInt(Date.parse(exampleRuns[0].startedAt!)) * 1_000_000n
      const parts = formatTimestampParts(
        timestamp,
        normalizeTimezone(tz)!,
        'milliseconds'
      )
      const date = container.querySelector('.signal-date')
      expect(date).toHaveTextContent(`Started: ${parts.value}`)
      expect(date).toHaveAttribute(
        'title',
        `Started: ${parts.value} ${parts.unit}`
      )
      expect(date).not.toHaveTextContent(parts.unit)
      expect(parts.value).toContain('.123')
      expect(
        container.querySelector('.signal-row__body')?.lastElementChild
      ).toContainElement(date as HTMLElement)
    }
  )

  it.each([
    { run: { ...exampleRuns[0], startedAt: undefined }, label: 'Unavailable' },
    {
      run: { ...exampleRuns[0], startedAt: 'invalid date' },
      label: 'Unavailable',
    },
    { run: { ...exampleRuns[2], startedAt: undefined }, label: 'Not started' },
  ])('omits the date row when the start is $label', ({ run }) => {
    const { container } = render(EvalRunSummary, {
      run: toRunCard(run),
      onclick: vi.fn(),
    })
    expect(container.querySelector('.signal-row__date')).toBeNull()
    expect(screen.queryByText('Started:')).not.toBeInTheDocument()
  })
})
