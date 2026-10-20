// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import EvalResults from './EvalResults.svelte'
import { exampleRuns } from './examples'
import { telemetryAPI } from '@/services/telemetry-service'

describe('EvalResults repetitions', () => {
  it.each([false, true])(
    'positions tools by stored start time, retaining late ends (run span: %s)',
    async withRun => {
      const traceID = 'a'.repeat(32)
      const reference = (id: number) => ({
        traceID,
        spanID: id.toString(16).padStart(16, '0'),
      })
      const epoch = 1700000000000000001n
      const timing = (
        id: number,
        name: string,
        start: bigint,
        end: bigint
      ) => ({
        ...reference(id),
        name,
        serviceName: 'agent',
        startTime: epoch + start * 1_000_000_000n,
        endTime: epoch + end * 1_000_000_000n,
        statusCodeValue: 1,
      })
      const attempt = {
        ...exampleRuns[0].attempts[0],
        telemetry: reference(1),
        tools: [2, 3].map(id => ({
          name: `tool ${id}`,
          state: 'OK' as const,
          message: '',
          telemetry: reference(id),
        })),
      }
      const lookup = vi
        .spyOn(telemetryAPI, 'getSpanTimings')
        .mockResolvedValue([
          timing(1, 'question', 10n, 20n),
          timing(2, 'tool 2', 12n, 14n),
          timing(3, 'tool 3', 18n, 26n),
          ...(withRun ? [timing(4, 'run', 0n, 40n)] : []),
        ])
      try {
        render(EvalResults, {
          run: {
            ...exampleRuns[0],
            attempts: [attempt],
            telemetry: withRun ? reference(4) : undefined,
          },
          attempts: [attempt],
          onSelect: vi.fn(),
        })
        const user = userEvent.setup()
        await user.click(screen.getByRole('tab', { name: 'Time' }))
        await screen.findByRole('link', {
          name: 'Open span question, 10.000 s',
        })
        await user.click(screen.getByRole('button', { name: /^Expand tools/ }))
        const early = screen.getByRole('link', {
          name: 'Open span tool 2, 2.000 s',
        })
        const late = screen.getByRole('link', {
          name: 'Open span tool 3, 8.000 s',
        })
        expect(early.querySelector('.span-bar')).toHaveStyle({
          left: withRun ? '30%' : '12.5%',
          width: withRun ? '5%' : '12.5%',
        })
        expect(late.querySelector('.span-bar')).toHaveStyle({
          left: withRun ? '45%' : '50%',
          width: withRun ? '20%' : '50%',
        })
        expect(early.querySelector('.duration-bar__label')).toHaveStyle({
          left: withRun ? 'calc(35% + 16px)' : 'calc(25% + 16px)',
        })
        expect(early).toHaveAttribute(
          'title',
          expect.stringContaining(
            withRun ? 'Start offset 12.000 s' : 'Start offset 2.000 s'
          )
        )
      } finally {
        lookup.mockRestore()
      }
    }
  )

  it('shows a referenced run span and keeps an absent question span as an unlinked recorded duration', async () => {
    const telemetry = { traceID: 'a'.repeat(32), spanID: 'b'.repeat(16) }
    const lookup = vi.spyOn(telemetryAPI, 'getSpanTimings').mockResolvedValue([
      {
        ...telemetry,
        name: 'eval run',
        serviceName: 'runner',
        startTime: 1700000000000000001n,
        endTime: 1700000001000000001n,
        statusCodeValue: 1,
      },
    ])
    try {
      const attempts = [exampleRuns[0].attempts[0]]
      render(EvalResults, {
        run: { ...exampleRuns[0], telemetry, attempts },
        attempts,
        onSelect: vi.fn(),
      })
      await userEvent.setup().click(screen.getByRole('tab', { name: 'Time' }))
      expect(
        await screen.findByRole('link', { name: 'Open span eval run, 1.000 s' })
      ).toHaveAttribute(
        'href',
        `/traces/${telemetry.traceID}?span=${telemetry.spanID}`
      )
      expect(screen.getAllByText('24.000 s')).toHaveLength(2)
      expect(
        screen.getByTitle('Recorded eval duration').querySelector('a')
      ).toBeNull()
      expect(lookup.mock.calls[0][0]).toContainEqual(telemetry)
    } finally {
      lookup.mockRestore()
    }
  })

  it.each([false, true])(
    'renders unlinked durations, preserves zero and leaves absent values empty (lookup failed: %s)',
    async failed => {
      const lookup = vi.spyOn(telemetryAPI, 'getSpanTimings')
      if (failed) lookup.mockRejectedValue(new Error('unavailable'))
      else lookup.mockResolvedValue([])
      try {
        const attempts = [24, 0, undefined].map((seconds, index) => ({
          ...exampleRuns[0].attempts[0],
          id: `duration-${index}`,
          repetition: index + 1,
          seconds,
          telemetry:
            index === 0 ? exampleRuns[0].attempts[0].telemetry : undefined,
          tools: [],
        }))
        const { container } = render(EvalResults, {
          run: { ...exampleRuns[0], attempts },
          attempts,
          onSelect: vi.fn(),
        })
        await userEvent.setup().click(screen.getByRole('tab', { name: 'Time' }))
        const rows = [...container.querySelectorAll('[data-attempt-id]')]
        const cells = rows.map(row => row.querySelector('td')!)
        expect(cells[0]).toHaveTextContent('24.000 s')
        expect(cells[0].querySelector('.span-bar')).toHaveStyle({
          width: '100%',
        })
        expect(cells[1]).toHaveTextContent('0 ns')
        expect(cells[1].querySelector('.span-bar')).toHaveStyle({ width: '0%' })
        expect(cells[2]).toHaveTextContent('')
        expect(cells[2].querySelector('.span-bar')).toBeNull()
        expect(container.querySelector('.correlated-span-bar')).toBeNull()
        expect(screen.getByText('Partial total')).toBeInTheDocument()
      } finally {
        lookup.mockRestore()
      }
    }
  )

  it('fans out single and repeated attempts, with each number selecting its exact attempt', async () => {
    const [first, second, third] = exampleRuns[0].attempts.filter(
      item => item.caseID === 'request-latency'
    )
    const single = {
      ...first,
      id: 'single',
      caseID: 'single',
      name: 'Single case',
    }
    const attempts = [single, first, second, third]
    const onSelect = vi.fn()
    const { container } = render(EvalResults, {
      run: { ...exampleRuns[0], attempts },
      attempts,
      onSelect,
      selectedID: second.id,
    })
    expect(container.querySelectorAll('tbody tr')).toHaveLength(6)
    const singleton = container.querySelector(
      '[data-attempt-id="single"]'
    )! as HTMLElement
    expect(
      within(singleton).getByRole('button', { name: 'Single case' })
    ).toHaveTextContent('1')
    expect(singleton.querySelector('.seg__arm--elbow')).not.toBeNull()
    expect(container.querySelectorAll('.eval-results__case')).toHaveLength(2)
    expect(
      screen.queryByText(exampleRuns[0].evaluation)
    ).not.toBeInTheDocument()
    const buttons = screen.getAllByRole('button', {
      name: first.name,
    })
    expect(buttons.map(button => button.textContent)).toEqual(['1', '2', '3'])
    expect(buttons[1]).toHaveAttribute('aria-pressed', 'true')
    expect(buttons[1]).toHaveAccessibleDescription('Rep 2 · Complete')
    const parent = container.querySelector(
      '.eval-results__case[data-case-id="request-latency"]'
    )! as HTMLElement
    const toggle = within(parent).getByRole('button', {
      name: `Collapse ${first.name}, 3 repetitions`,
    })
    const aggregateValues = [...parent.querySelectorAll('td')].map(
      cell => cell.textContent
    )
    await userEvent.setup().click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(container.querySelectorAll('[data-attempt-id]')).toHaveLength(1)
    expect(
      [...parent.querySelectorAll('td')].map(cell => cell.textContent)
    ).toEqual(aggregateValues)
    expect(onSelect).not.toHaveBeenCalled()
    await userEvent.setup().keyboard('[Enter]')
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(
      screen.getAllByRole('button', { name: first.name })[1]
    ).toHaveAttribute('aria-pressed', 'true')
    await userEvent
      .setup()
      .click(screen.getAllByRole('button', { name: first.name })[2])
    expect(onSelect).toHaveBeenCalledWith(third.id)
  })
})
