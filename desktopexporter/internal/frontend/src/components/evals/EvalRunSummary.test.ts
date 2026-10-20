// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { screen, within } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import EvalRunSummary from './EvalRunSummary.svelte'
import { exampleRuns } from './examples'
import { renderWithContexts as render } from '@/test/render-helpers'
import { formatTimestampParts, normalizeTimezone } from '@/utils/time'

describe('EvalRunSummary', () => {
  it('keeps run summaries concise without putting run statistics in segment tooltips', () => {
    const { container } = render(EvalRunSummary, {
      run: exampleRuns[0],
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
      name: /^Task completion by attempt count/,
    })
    expect(bar).not.toHaveTextContent(/complete|Score:|tool calls|cost:/)
    expect(
      container.querySelector('.legend-rows, .signal-row__meta-row')
    ).toBeNull()
  })

  it('exposes each outcome in its own tooltip and selects only through the card', async () => {
    const user = userEvent.setup()
    const onclick = vi.fn()
    render(EvalRunSummary, { run: exampleRuns[0], onclick })
    const group = screen.getByRole('group', {
      name: 'Task completion outcomes',
    })
    const bar = within(group).getByRole('button', { name: 'Pass: 11 attempts' })
    expect(
      [...group.querySelectorAll('[role="tooltip"]')].map(node =>
        node.textContent?.trim()
      )
    ).toEqual(['Pass [11]', 'Partial [3]', 'Fail [2]'])
    expect(group.querySelectorAll('.breakdown-segment')).toHaveLength(3)
    expect(within(group).queryByRole('listitem')).not.toBeInTheDocument()
    await user.click(bar)
    expect(onclick).not.toHaveBeenCalled()
    expect(document.querySelector('button button')).toBeNull()
    await user.click(
      screen.getByRole('button', { name: /Guide-assisted agent/ })
    )
    expect(onclick).toHaveBeenCalledWith('guide-assisted')
    expect(onclick).toHaveBeenCalledTimes(1)
  })

  it('keeps planned work and execution errors separate from failed grades', () => {
    render(EvalRunSummary, { run: exampleRuns[1], onclick: vi.fn() })
    const group = screen.getByRole('group', {
      name: 'Task completion outcomes',
    })
    expect(
      within(group).getByRole('button', { name: 'Internal fault: 2 attempts' })
    ).toBeInTheDocument()
    expect(
      within(group).getByRole('button', { name: 'Planned: 4 attempts' })
    ).toBeInTheDocument()
    expect(
      within(group).getByRole('button', { name: 'Fail: 2 attempts' })
    ).toBeInTheDocument()
  })

  it('uses a plain label for a single outcome without inventing grades or cost', () => {
    render(EvalRunSummary, { run: exampleRuns[2], onclick: vi.fn() })
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

  it('keeps tooltip context keyboard reachable without selecting the run', async () => {
    const user = userEvent.setup()
    const onclick = vi.fn()
    render(EvalRunSummary, { run: exampleRuns[0], onclick })
    const card = screen.getByRole('button', { name: /Guide-assisted agent/ })
    const bar = screen.getByRole('button', {
      name: 'Pass: 11 attempts',
    })
    await user.tab()
    expect(card).toHaveFocus()
    await user.tab()
    expect(bar).toHaveFocus()
    expect(bar).toHaveAccessibleName('Pass: 11 attempts')
    await user.keyboard('{Enter}')
    expect(onclick).not.toHaveBeenCalled()
    expect(document.querySelector('button button')).toBeNull()
    await user.tab({ shift: true })
    await user.keyboard('{Enter}')
    expect(onclick).toHaveBeenCalledExactlyOnceWith('guide-assisted')
  })

  it('does not turn missing grades and cost into received zeros', () => {
    const run = {
      ...exampleRuns[0],
      attempts: exampleRuns[0].attempts.map(attempt => ({
        ...attempt,
        assessments: [],
        tools: [],
        investigatorCost: undefined,
      })),
    }
    render(EvalRunSummary, { run, onclick: vi.fn() })
    const bar = screen.getByRole('button', {
      name: 'Unscored: 16 attempts',
    })
    const tooltip = document.querySelector('[role="tooltip"]')
    expect(bar).toHaveTextContent(/^Unscored$/)
    expect(bar.querySelector('.breakdown-segment__fill')).toHaveAttribute(
      'aria-hidden',
      'true'
    )
    expect(tooltip).toHaveTextContent(/^Unscored \[16\]$/)
  })

  it('retains received zero scores and costs without calling failed grades execution faults', () => {
    const run = {
      ...exampleRuns[0],
      attempts: exampleRuns[0].attempts.map(attempt => ({
        ...attempt,
        assessments: attempt.assessments.map(assessment => ({
          ...assessment,
          value: 0,
          label: 'Fail',
        })),
        investigatorCost: 0,
      })),
    }
    render(EvalRunSummary, { run, onclick: vi.fn() })
    expect(
      screen.getByRole('button', { name: 'Fail: 16 attempts' })
    ).toBeInTheDocument()
    expect(document.querySelector('[role="tooltip"]')).toHaveTextContent(
      /^Fail \[16\]$/
    )
  })

  it.each(['UTC', 'America/Toronto'] as const)(
    'labels the received start in the selected %s timezone at the bottom of the card',
    tz => {
      localStorage.setItem('time-tz', tz)
      const { container } = render(EvalRunSummary, {
        run: exampleRuns[0],
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
  ])(
    'shows an em dash and preserves $label in the title when no valid start was received',
    ({ run, label }) => {
      const { container } = render(EvalRunSummary, { run, onclick: vi.fn() })
      const date = container.querySelector('.signal-date')
      expect(date).toHaveTextContent('Started: —')
      expect(date).toHaveAttribute('title', `Started: ${label}`)
    }
  )
})
