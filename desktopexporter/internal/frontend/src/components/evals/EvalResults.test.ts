// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { render, screen, within, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import EvalResults from './EvalResults.svelte'
import { exampleRuns } from './examples'
import { telemetryAPI } from '@/services/telemetry-service'
import type { SpanTiming, TraceData } from '@/types/api-types'

function traceViews(timings: SpanTiming[]) {
  const traces: TraceData[] = []
  for (const timing of timings) {
    let trace = traces.find(trace => trace.traceID === timing.traceID)
    if (!trace) {
      trace = { traceID: timing.traceID, unplacedSpanCount: 0, spans: [] }
      traces.push(trace)
    }
    const parent = timing.name.startsWith('tool')
      ? (trace.spans[0]?.spanData.spanID ?? null)
      : null
    trace.spans.push({
      depth: parent ? 1 : 0,
      matched: true,
      spanData: {
        ...timing,
        parentSpanID: parent,
        traceState: '',
        flags: 0,
        kind: 'Internal',
        kindCode: 1,
        attributes: [],
        events: [],
        links: [],
        droppedAttributesCount: 0,
        droppedEventsCount: 0,
        droppedLinksCount: 0,
        statusCode: 'Unset',
        statusMessage: '',
        resource: {
          attributes: [
            {
              key: 'service.name',
              value: { kind: 'string', value: timing.serviceName },
            },
          ],
          droppedAttributesCount: 0,
        },
        scope: {
          name: '',
          version: '',
          attributes: [],
          droppedAttributesCount: 0,
        },
      },
    })
  }
  return { traces, unavailableTraceIDs: [] }
}

async function showOnlyTime() {
  const user = userEvent.setup()
  await user.click(screen.getByRole('radio', { name: 'Tool calls' }))
}

describe('EvalResults repetitions', () => {
  it('disables missing views, skips them with arrows and retains recorded zero costs', async () => {
    const lookup = vi
      .spyOn(telemetryAPI, 'getReferencedTraceViews')
      .mockResolvedValue(traceViews([]))
    try {
      const rep = {
        ...exampleRuns[0].reps[0],
        telemetry: undefined,
        repTelemetry: undefined,
        tools: [],
        toolsKnown: false,
        assessments: [],
        investigatorCost: undefined,
        graderCost: undefined,
      }
      const run = { ...exampleRuns[0], telemetry: undefined, reps: [rep] }
      const { rerender } = render(EvalResults, {
        run,
        reps: run.reps,
        viewKey: 'cost',
      })
      const results = screen.getByRole('radio', { name: 'Results' })
      const tools = screen.getByRole('radio', { name: 'Tool calls' })
      const cost = screen.getByRole('radio', { name: 'Cost' })
      await waitFor(() =>
        expect(results).toHaveAttribute('aria-checked', 'true')
      )
      expect(tools).toBeDisabled()
      expect(cost).toBeDisabled()
      const user = userEvent.setup()
      results.focus()
      await user.keyboard('{ArrowRight}')
      expect(results).toHaveFocus()

      const withCost = { ...run, reps: [{ ...rep, investigatorCost: 0 }] }
      await rerender({ run: withCost, reps: withCost.reps })
      expect(cost).toBeEnabled()
      await user.keyboard('{ArrowRight}')
      expect(cost).toHaveFocus()
      expect(cost).toHaveAttribute('aria-checked', 'true')
      await rerender({ reps: [] })
      expect(cost).toBeEnabled()
      expect(cost).toHaveAttribute('aria-checked', 'true')
      await user.keyboard('{ArrowLeft}')
      expect(results).toHaveFocus()

      const knownZero = { ...run, reps: [{ ...rep, toolsKnown: true }] }
      await rerender({ run: knownZero, reps: knownZero.reps })
      expect(cost).toBeDisabled()
      expect(tools).toBeEnabled()
      await user.click(tools)
      expect(tools).toHaveAttribute('aria-checked', 'true')
      await rerender({ run, reps: run.reps })
      await waitFor(() =>
        expect(results).toHaveAttribute('aria-checked', 'true')
      )
      expect(tools).toBeDisabled()
    } finally {
      lookup.mockRestore()
    }
  })

  it('retains selected tool telemetry when filtering its case out of the table', async () => {
    const rep = exampleRuns[0].reps[0]
    const tool = rep.tools[0]
    const reference = tool.telemetry!
    const lookup = vi
      .spyOn(telemetryAPI, 'getReferencedTraceViews')
      .mockResolvedValue(
        traceViews([
          {
            traceID: reference.traceID,
            spanID: reference.spanID!,
            name: 'tool selected',
            serviceName: 'runner',
            startTime: 10n,
            endTime: 30n,
            statusCodeValue: 2,
          },
        ])
      )
    const onResolveSelection = vi.fn()
    try {
      const run = {
        ...exampleRuns[0],
        telemetry: undefined,
        reps: [rep],
      }
      const { rerender, container } = render(EvalResults, {
        run,
        reps: run.reps,
        selectedRowID: `${rep.id}/tool/0`,
        onResolveSelection,
      })
      const evidence = expect.objectContaining({
        id: `${rep.id}/tool/0`,
        state: 'Error',
        spanError: true,
        window: { startTime: 10n, endTime: 30n },
      })
      await waitFor(() =>
        expect(onResolveSelection).toHaveBeenLastCalledWith(evidence)
      )
      await rerender({ reps: [] })
      expect(container.querySelectorAll('[data-outline-id]')).toHaveLength(1)
      await waitFor(() =>
        expect(onResolveSelection).toHaveBeenLastCalledWith(evidence)
      )
    } finally {
      lookup.mockRestore()
    }
  })

  it('selects one summary view, combines totals and preserves manual disclosure', async () => {
    const rep = exampleRuns[0].reps[0]
    const telemetry = { traceID: 'a'.repeat(32), spanID: 'b'.repeat(16) }
    const lookup = vi
      .spyOn(telemetryAPI, 'getReferencedTraceViews')
      .mockResolvedValue(traceViews([]))
    try {
      const run = { ...exampleRuns[0], telemetry, reps: [rep] }
      const { container } = render(EvalResults, {
        run,
        reps: run.reps,
        selectedID: rep.id,
      })
      const user = userEvent.setup()
      const toggles = within(
        screen.getByRole('radiogroup', { name: 'Eval view' })
      )
      const toggle = (name: string) => toggles.getByRole('radio', { name })
      await user.click(toggle('Cost'))
      await user.click(toggle('Tool calls'))
      for (const name of ['Results', 'Tool calls', 'Cost'])
        expect(toggle(name)).toHaveAttribute(
          'aria-checked',
          String(name === 'Tool calls')
        )
      expect(screen.queryByRole('tablist')).not.toBeInTheDocument()
      const headers = [...container.querySelectorAll('thead button')].map(
        button => button.getAttribute('aria-label')
      )
      expect(headers).toEqual(
        ['Test case', 'Started', 'Duration', 'Tool calls'].map(
          label => `Sort by ${label}`
        )
      )
      const expectedColumns = 4
      for (const row of container.querySelectorAll('tr')) {
        expect(
          [...row.children].reduce(
            (total, cell) => total + (cell as HTMLTableCellElement).colSpan,
            0
          )
        ).toBe(expectedColumns)
      }
      const toolCount = rep.tools.length
      expect(container.querySelectorAll('[data-tool-rep-id]')).toHaveLength(
        toolCount
      )
      await user.click(screen.getByRole('button', { name: /^Collapse Result/ }))
      await user.click(toggle('Cost'))
      expect(container.querySelectorAll('col')).toHaveLength(6)
      expect(
        [...container.querySelectorAll('tfoot td')].map(cell =>
          cell.textContent?.trim()
        )
      ).toEqual([
        '',
        '',
        rep.investigatorCost!.toFixed(3),
        rep.graderCost!.toFixed(3),
        (rep.investigatorCost! + rep.graderCost!).toFixed(3),
      ])
      expect(
        screen.getByRole('button', { name: 'Sort by Total cost' })
      ).toBeInTheDocument()
      expect(container.querySelectorAll('[data-tool-rep-id]')).toHaveLength(0)
      await user.click(toggle('Tool calls'))
      expect(container.querySelectorAll('[data-tool-rep-id]')).toHaveLength(0)
      await user.click(screen.getByRole('button', { name: /^Expand Result/ }))
      expect(container.querySelectorAll('[data-tool-rep-id]')).toHaveLength(
        toolCount
      )
      expect(container.querySelector(`[data-rep-id="${rep.id}"]`)).toHaveClass(
        'table-row--selected'
      )
    } finally {
      lookup.mockRestore()
    }
  })

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
      const rep = {
        ...exampleRuns[0].reps[0],
        telemetry: reference(1),
        tools: [2, 3].map(id => ({
          name: `tool ${id}`,
          state: 'OK' as const,
          message: '',
          telemetry: reference(id),
        })),
      }
      const lookup = vi
        .spyOn(telemetryAPI, 'getReferencedTraceViews')
        .mockResolvedValue(
          traceViews([
            timing(1, 'question', 10n, 20n),
            timing(2, 'tool 2', 12n, 14n),
            timing(3, 'tool 3', 18n, 26n),
            ...(withRun ? [timing(4, 'run', 0n, 40n)] : []),
          ])
        )
      try {
        render(EvalResults, {
          run: {
            ...exampleRuns[0],
            reps: [rep],
            telemetry: withRun ? reference(4) : undefined,
          },
          reps: [rep],
        })
        await showOnlyTime()
        await screen.findByRole('button', {
          name: 'Span question, 10.000 s',
        })
        const early = screen.getByRole('button', {
          name: 'Span tool 2, 2.000 s',
        })
        const late = screen.getByRole('button', {
          name: 'Span tool 3, 8.000 s',
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

  it('aligns reps from different traces to one origin without a run span', async () => {
    const epoch = 1700000000000000001n
    const reps = [0, 1].map(index => ({
      ...exampleRuns[0].reps[0],
      id: `rep-${index}`,
      caseID: `case-${index}`,
      telemetry: undefined,
      repTelemetry: {
        traceID: String(index + 1).repeat(32),
        spanID: 'a'.repeat(16),
      },
      tools: [],
      assessments: [],
    }))
    const lookup = vi
      .spyOn(telemetryAPI, 'getReferencedTraceViews')
      .mockResolvedValue(
        traceViews(
          reps.map((rep, index) => ({
            ...rep.repTelemetry,
            name: `rep span ${index}`,
            serviceName: 'runner',
            startTime: epoch + BigInt(index * 20) * 1_000_000_000n,
            endTime: epoch + BigInt(index * 20 + 10) * 1_000_000_000n,
            statusCodeValue: 1,
          }))
        )
      )
    try {
      const { container } = render(EvalResults, {
        run: { ...exampleRuns[0], telemetry: undefined, reps },
        reps,
      })
      const second = await screen.findByRole('button', {
        name: 'Span rep span 1, 10.000 s',
      })
      expect(second.querySelector('.span-bar')).toHaveStyle({
        left: '66.66%',
        width: '33.33%',
      })
      expect(second).toHaveAttribute(
        'title',
        expect.stringContaining('Start offset 20.000 s')
      )
      expect(
        container.querySelector('[data-outline-id="case/case-1"] .span-bar')
      ).toHaveStyle({ left: '66.66%', width: '33.33%' })
    } finally {
      lookup.mockRestore()
    }
  })

  it('keeps case and missing-run fallback bars across spaced repetitions, including filtered reps', async () => {
    const epoch = 1700000000000000001n
    const reps = [0, 1].map(index => ({
      ...exampleRuns[0].reps[0],
      id: `rep-${index}`,
      caseID: 'shared-case',
      repetition: index + 1,
      telemetry: undefined,
      repTelemetry: {
        traceID: String(index + 1).repeat(32),
        spanID: 'a'.repeat(16),
      },
      tools: [],
      assessments: [],
    }))
    const lookup = vi
      .spyOn(telemetryAPI, 'getReferencedTraceViews')
      .mockResolvedValue(
        traceViews(
          reps.map((rep, index) => ({
            ...rep.repTelemetry,
            name: `rep span ${index}`,
            serviceName: 'runner',
            startTime: epoch + BigInt(index * 20) * 1_000_000_000n,
            endTime: epoch + BigInt(index * 20 + 10) * 1_000_000_000n,
            statusCodeValue: 1,
          }))
        )
      )
    try {
      const { container } = render(EvalResults, {
        run: {
          ...exampleRuns[0],
          telemetry: { traceID: 'f'.repeat(32), spanID: 'f'.repeat(16) },
          reps,
        },
        reps: [reps[0]],
      })
      const root = await screen.findByRole('button', {
        name: `${reps[0].name}, 30.000 s`,
      })
      expect(root.querySelector('.span-bar')).toHaveStyle({
        left: '0%',
        width: '100%',
      })
      expect(container.querySelectorAll('[data-rep-id]')).toHaveLength(1)
      expect(root).toHaveAttribute(
        'title',
        expect.stringContaining('elapsed time across available references')
      )
      const runBar = screen.getByRole('button', {
        name: `${exampleRuns[0].name}, 30.000 s`,
      })
      expect(runBar.querySelector('.span-bar')).toHaveStyle({
        left: '0%',
        width: '100%',
      })
      expect(runBar).toHaveAttribute(
        'title',
        expect.stringContaining('Timing fallback')
      )
    } finally {
      lookup.mockRestore()
    }
  })

  it('shows a referenced run span and keeps an absent question span as an unlinked recorded duration', async () => {
    const telemetry = { traceID: 'a'.repeat(32), spanID: 'b'.repeat(16) }
    const lookup = vi
      .spyOn(telemetryAPI, 'getReferencedTraceViews')
      .mockResolvedValue(
        traceViews([
          {
            ...telemetry,
            name: 'eval run',
            serviceName: 'runner',
            startTime: 1700000000000000001n,
            endTime: 1700000001000000001n,
            statusCodeValue: 1,
          },
        ])
      )
    try {
      const reps = [exampleRuns[0].reps[0]]
      render(EvalResults, {
        run: { ...exampleRuns[0], telemetry, reps },
        reps,
      })
      await showOnlyTime()
      expect(
        await screen.findByRole('button', { name: 'Span eval run, 1.000 s' })
      ).not.toHaveAttribute('href')
      expect(screen.getAllByText('24.000 s')).toHaveLength(2)
      expect(
        screen.getAllByTitle('Recorded eval duration')[0].querySelector('a')
      ).toBeNull()
      expect(lookup.mock.calls[0][0]).toContainEqual(telemetry)
    } finally {
      lookup.mockRestore()
    }
  })

  it.each([false, true])(
    'renders unlinked durations and leaves trivial or absent values empty (lookup failed: %s)',
    async failed => {
      const lookup = vi.spyOn(telemetryAPI, 'getReferencedTraceViews')
      if (failed) lookup.mockRejectedValue(new Error('unavailable'))
      else lookup.mockResolvedValue(traceViews([]))
      try {
        const reps = [24, 0, undefined, 1e-9].map((seconds, index) => ({
          ...exampleRuns[0].reps[0],
          id: `duration-${index}`,
          repetition: index + 1,
          seconds,
          telemetry: index === 0 ? exampleRuns[0].reps[0].telemetry : undefined,
          tools: [],
        }))
        const { container } = render(EvalResults, {
          run: { ...exampleRuns[0], reps },
          reps,
        })
        await showOnlyTime()
        const rows = [...container.querySelectorAll('[data-rep-id]')]
        const cells = rows.map(row => row.querySelector('td')!)
        expect(cells[0]).toHaveTextContent('24.000 s')
        expect(cells[0].querySelector('.span-bar')).toHaveStyle({
          width: '100%',
        })
        for (const cell of cells.slice(1)) {
          expect(cell).toHaveTextContent('')
          expect(cell.querySelector('.span-bar')).toBeNull()
        }
        expect(cells[2].querySelector('.timing-cell__unavailable')).toBeNull()
        expect(container.querySelector('.eval-results__span-bar')).toBeNull()
        expect(screen.getByText('Partial total')).toBeInTheDocument()
      } finally {
        lookup.mockRestore()
      }
    }
  )

  it('selects eval details from labels without selecting through collapse controls', async () => {
    const [first, second, third] = exampleRuns[0].reps.filter(
      item => item.caseID === 'request-latency'
    )
    const single = {
      ...first,
      id: 'single',
      caseID: 'single',
      name: 'Single case',
    }
    const reps = [single, first, second, third]
    const onSelect = vi.fn()
    const { container } = render(EvalResults, {
      run: { ...exampleRuns[0], reps },
      reps,
      selectedID: second.id,
      onSelect,
    })
    expect(container.querySelectorAll('tbody tr')).toHaveLength(7)
    const singleton = container.querySelector(
      '[data-rep-id="single"]'
    )! as HTMLElement
    expect(within(singleton).getByText('Result')).toBeInTheDocument()
    expect(singleton.querySelector('.seg__arm--elbow')).not.toBeNull()
    // The Run root and both Case roots use the aggregate-row treatment.
    expect(container.querySelectorAll('.eval-results__case')).toHaveLength(3)
    expect(
      screen.queryByText(exampleRuns[0].evaluation)
    ).not.toBeInTheDocument()
    const labels = [
      ...container.querySelectorAll(
        '[data-row-kind="rep"] button[aria-pressed]'
      ),
    ]
    expect(labels.map(label => label.textContent)).toEqual([
      'Result',
      'Result',
      'Result',
      'Result',
    ])
    expect(labels[2].closest('tr')).toHaveClass('table-row--selected')
    expect(labels[2]).toHaveAttribute('aria-pressed', 'true')
    expect(labels[1]).toHaveAttribute('aria-pressed', 'false')
    expect(labels[2]).toHaveAttribute('title', 'Result · Span error')
    const parent = container.querySelector(
      '.eval-results__case[data-case-id="request-latency"]'
    )! as HTMLElement
    const toggle = within(parent).getByRole('button', {
      name: `Collapse ${first.name}, 3 children`,
    })
    const aggregateValues = [...parent.querySelectorAll('td')].map(
      cell => cell.textContent
    )
    await userEvent.setup().click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    expect(container.querySelectorAll('[data-rep-id]')).toHaveLength(1)
    expect(
      [...parent.querySelectorAll('td')].map(cell => cell.textContent)
    ).toEqual(aggregateValues)
    await userEvent.setup().keyboard('[Enter]')
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(container.querySelector(`[data-rep-id="${second.id}"]`)).toHaveClass(
      'table-row--selected'
    )
    expect(onSelect).not.toHaveBeenCalled()
    await userEvent
      .setup()
      .click(
        container.querySelector(
          `[data-rep-id="${second.id}"] button[aria-pressed]`
        )!
      )
    expect(onSelect).toHaveBeenCalledWith(
      expect.objectContaining({ kind: 'rep', rep: second })
    )
  })
})
