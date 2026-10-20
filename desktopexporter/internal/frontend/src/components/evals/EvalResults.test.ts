// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import EvalResults from './EvalResults.svelte'
import { exampleRuns } from './examples'
import { telemetryAPI } from '@/services/telemetry-service'

describe('EvalResults repetitions', () => {
  it('shows a referenced run span and distinguishes an absent question span from fixture duration', async () => {
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
      expect(await screen.findByText('Span not stored')).toBeInTheDocument()
      expect(screen.queryByText('24.000 s')).not.toBeInTheDocument()
      expect(lookup.mock.calls[0][0]).toContainEqual(telemetry)
    } finally {
      lookup.mockRestore()
    }
  })

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
