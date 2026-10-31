// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { screen, waitFor, within } from '@testing-library/svelte'
import { tick } from 'svelte'
import { telemetryAPI } from '@/services/telemetry-service'
import { renderWithContexts as render } from '@/test/render-helpers'
import type { SpanNode, TraceSummary } from '@/types/api-types'
import EvalTelemetrySummaries from './EvalTelemetrySummaries.svelte'
import EvalTelemetrySection from './EvalTelemetrySection.svelte'
import { evalTelemetrySummaries } from './eval-telemetry-summaries'
import { exampleExperiments } from './examples'
import { buildEvalOutline } from './eval-outline'
import { groupResults } from './group-results'
import { indexEvalTraces, referenceKey } from './eval-traces'
import { traceHref } from './model'

beforeEach(() => {
  vi.spyOn(telemetryAPI, 'getTraceSummary').mockRejectedValue(
    new Error('Lookup unavailable')
  )
})
afterEach(() => vi.restoreAllMocks())

const reference = { traceID: 'a'.repeat(32), spanID: 'b'.repeat(16) }

it('publishes each trace lookup immediately and shows slow feedback only for the pending trace', async () => {
  const a = 'a'.repeat(32),
    b = 'b'.repeat(32)
  let finish!: (value: null) => void
  vi.mocked(telemetryAPI.getTraceSummary).mockImplementation(id =>
    id === a
      ? Promise.resolve(null)
      : new Promise(resolve => {
          finish = resolve
        })
  )
  const ontracecount = vi.fn()
  render(EvalTelemetrySummaries, {
    entries: [
      { reference: { traceID: a }, label: 'A' },
      { reference: { traceID: b }, label: 'B' },
    ],
    ontracecount,
  })
  expect(screen.queryByText('Loading trace…')).toBeNull()
  await screen.findByText('(unavailable)')
  expect(screen.getAllByRole('link')[0]).toHaveAttribute(
    'aria-disabled',
    'true'
  )
  expect(screen.getAllByRole('link')[0]).not.toHaveAttribute('href')
  expect(screen.getAllByRole('link')[1]).toHaveAttribute('href')
  expect(ontracecount).toHaveBeenLastCalledWith(undefined)
  expect(await screen.findByText('Loading trace…')).toBeVisible()
  finish(null)
  await waitFor(() =>
    expect(screen.getAllByText('(unavailable)')).toHaveLength(2)
  )
  expect(screen.queryByText('Loading trace…')).toBeNull()
  expect(ontracecount).toHaveBeenLastCalledWith(0)
})

it('rejects superseded trace lookups including A → B → A', async () => {
  const pending: Array<(value: null) => void> = []
  const lookup = vi
    .mocked(telemetryAPI.getTraceSummary)
    .mockImplementation(() => new Promise(resolve => pending.push(resolve)))
  const a = { reference: { traceID: 'a'.repeat(32) }, label: 'A' }
  const b = { reference: { traceID: 'b'.repeat(32) }, label: 'B' }
  const view = render(EvalTelemetrySummaries, { entries: [a] })
  await waitFor(() => expect(pending).toHaveLength(1))
  await view.rerender({ componentProps: { entries: [b] } })
  await view.rerender({ componentProps: { entries: [a] } })
  expect(lookup.mock.calls[0][1]?.aborted).toBe(true)
  expect(lookup.mock.calls[1][1]?.aborted).toBe(true)
  pending[0](null)
  pending[1](null)
  await Promise.resolve()
  expect(screen.queryByText('(unavailable)')).toBeNull()
  pending[2](null)
  await screen.findByText('(unavailable)')
})

it.each(['failure', 'success', 'missing'] as const)(
  'forgets an earlier absence during a fresh lookup that ends in %s',
  async outcome => {
    const a = { reference: { traceID: 'a'.repeat(32) }, label: 'A' }
    const b = { reference: { traceID: 'b'.repeat(32) }, label: 'B' }
    const found: TraceSummary = {
      traceID: b.reference.traceID,
      hasRootSpan: true,
      rootSpan: { name: 'Received root', serviceName: 'eval-agent' },
      startTime: 0n,
      durationNs: 100n,
      spanCount: 2,
      errorCount: 0,
    }
    let finish!: (value: TraceSummary | null) => void
    let fail!: (error: Error) => void
    const lookup = vi
      .mocked(telemetryAPI.getTraceSummary)
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce(found)
      .mockReturnValueOnce(
        new Promise((resolve, reject) => {
          finish = resolve
          fail = reject
        })
      )
    const ontracecount = vi.fn()
    const view = render(EvalTelemetrySummaries, { entries: [a], ontracecount })
    await screen.findByText('(unavailable)')
    expect(ontracecount).toHaveBeenLastCalledWith(0)
    await view.rerender({ componentProps: { entries: [b], ontracecount } })
    await screen.findByText('Received root')
    ontracecount.mockClear()
    await view.rerender({ componentProps: { entries: [a], ontracecount } })
    await waitFor(() => expect(lookup).toHaveBeenCalledTimes(3))
    const link = screen.getByRole('link')
    expect(link).toHaveAttribute('href', traceHref(a.reference))
    expect(link).not.toHaveAttribute('aria-disabled', 'true')
    expect(screen.queryByText('(unavailable)')).toBeNull()
    expect(ontracecount).toHaveBeenLastCalledWith(undefined)
    expect(ontracecount.mock.calls.every(([count]) => count !== 0)).toBe(true)
    if (outcome === 'failure') fail(new Error('fresh lookup failed'))
    else
      finish(
        outcome === 'missing'
          ? null
          : { ...found, traceID: a.reference.traceID }
      )
    await tick()
    await tick()
    if (outcome === 'missing') {
      await waitFor(() => expect(link).toHaveAttribute('aria-disabled', 'true'))
      expect(ontracecount).toHaveBeenLastCalledWith(0)
    } else {
      expect(link).toHaveAttribute('href', traceHref(a.reference))
      expect(link).not.toHaveAttribute('aria-disabled', 'true')
      expect(screen.queryByText('(unavailable)')).toBeNull()
      expect(ontracecount).toHaveBeenLastCalledWith(
        outcome === 'success' ? 1 : undefined
      )
    }
  }
)

const node: SpanNode = {
  depth: 0,
  matched: true,
  spanData: {
    ...reference,
    parentSpanID: null,
    name: 'recorded operation',
    traceState: '',
    flags: 0,
    kind: 'Internal',
    kindCode: 1,
    startTime: 10n,
    endTime: 10n,
    statusCode: 'Error',
    statusCodeValue: 2,
    statusMessage: 'failure',
    attributes: [],
    events: [],
    links: [],
    droppedAttributesCount: 0,
    droppedEventsCount: 0,
    droppedLinksCount: 0,
    resource: {
      attributes: [
        { key: 'service.name', value: { kind: 'string', value: 'eval-agent' } },
      ],
      droppedAttributesCount: 0,
    },
    scope: { name: '', version: '', attributes: [], droppedAttributesCount: 0 },
  },
}

it('keeps unfinished summary lookups when a sibling span arrives', async () => {
  const references = ['a', 'b', 'c'].map(letter => ({
    traceID: letter.repeat(32),
    spanID: reference.spanID,
  }))
  const pending = references.map(() =>
    Promise.withResolvers<TraceSummary | null>()
  )
  const lookup = vi
    .mocked(telemetryAPI.getTraceSummary)
    .mockImplementation(
      id =>
        pending[references.findIndex(reference => reference.traceID === id)]
          .promise
    )
  const entries = references.map((reference, index) => ({
    reference,
    label: `Result ${index}`,
  }))
  const view = render(EvalTelemetrySummaries, { entries })
  await waitFor(() => expect(lookup).toHaveBeenCalledTimes(3))
  await view.rerender({
    componentProps: {
      entries,
      nodes: new Map([[referenceKey(references[0]), node]]),
    },
  })
  expect(lookup).toHaveBeenCalledTimes(3)
  expect(lookup.mock.calls[0][1]?.aborted).toBe(true)
  expect(lookup.mock.calls[1][1]?.aborted).toBe(false)
  expect(lookup.mock.calls[2][1]?.aborted).toBe(false)
  pending[1].resolve(null)
  await screen.findByText('(unavailable)')
  expect(screen.getAllByRole('link')[2]).toHaveAttribute(
    'href',
    traceHref(references[2])
  )
  view.unmount()
  expect(lookup.mock.calls[2][1]?.aborted).toBe(true)
  pending[0].resolve(null)
  pending[2].resolve(null)
})

it('refreshes only the arriving trace log count and never publishes a partial count as complete', async () => {
  type Logs = Awaited<ReturnType<typeof telemetryAPI.getTraceLogSummaries>>
  const references = ['a', 'b', 'c'].map(letter => ({
    traceID: letter.repeat(32),
    spanID: reference.spanID,
  }))
  const pending: Array<ReturnType<typeof Promise.withResolvers<Logs>>> = []
  const lookup = vi
    .spyOn(telemetryAPI, 'getTraceLogSummaries')
    .mockImplementation(() => {
      const request = Promise.withResolvers<Logs>()
      pending.push(request)
      return request.promise
    })
  const entries = references.map((reference, index) => ({
    reference,
    label: `Result ${index}`,
  }))
  const props = { entries, evidenceLogs: [] }
  const view = render(EvalTelemetrySection, props)
  await waitFor(() => expect(lookup).toHaveBeenCalledTimes(3))
  expect(screen.queryByText(/\d+ logs?/)).toBeNull()
  const nodes = new Map([[referenceKey(references[0]), node]])
  await view.rerender({ componentProps: { ...props, nodes } })
  expect(lookup).toHaveBeenCalledTimes(4)
  expect(lookup.mock.calls[3][0]).toBe(references[0].traceID)
  expect(lookup.mock.calls[0][1]?.aborted).toBe(true)
  expect(lookup.mock.calls[1][1]?.aborted).toBe(false)
  expect(lookup.mock.calls[2][1]?.aborted).toBe(false)
  const log = (logRef: string): Logs => [
    {
      logRef,
      timestamp: 1n,
      spanID: reference.spanID,
      eventName: '',
      severityNumber: 0,
      severityText: '',
      serviceName: '',
      bodyPreview: '',
    },
  ]
  pending[0].resolve(log('stale'))
  pending[1].resolve(log('b'))
  pending[2].reject(new Error('unavailable'))
  pending[3].resolve(log('a'))
  await screen.findByText('Log count could not be loaded.')
  expect(screen.queryByText(/\d+ logs?/)).toBeNull()
  await view.rerender({
    componentProps: {
      ...props,
      nodes: new Map(nodes).set(referenceKey(references[2]), {
        ...node,
        spanData: { ...node.spanData, ...references[2] },
      }),
    },
  })
  expect(lookup).toHaveBeenCalledTimes(5)
  expect(lookup.mock.calls[4][0]).toBe(references[2].traceID)
  pending[4].resolve(log('c'))
  await screen.findByText(/3 logs/)
  view.unmount()
  expect(lookup.mock.calls.every(([, signal]) => signal?.aborted)).toBe(true)
})

it('counts distinct connected traces and scoped logs, retaining the last complete count on failure', async () => {
  const logs = vi
    .spyOn(telemetryAPI, 'getTraceLogSummaries')
    .mockResolvedValue([
      {
        logRef: 'one',
        timestamp: 1n,
        spanID: reference.spanID,
        eventName: 'tool.output',
        severityNumber: 0,
        severityText: '',
        serviceName: '',
        bodyPreview: '',
      },
      {
        logRef: 'two',
        timestamp: 2n,
        spanID: 'c'.repeat(16),
        eventName: '',
        severityNumber: 0,
        severityText: '',
        serviceName: '',
        bodyPreview: '',
      },
    ])
  const props = {
    entries: [
      { reference, label: 'Tool' },
      { reference: { ...reference, spanID: 'c'.repeat(16) }, label: 'Grader' },
    ],
    nodes: new Map([[referenceKey(reference), node]]),
    evidenceLogs: [],
    wholeTraces: true,
  }
  const { rerender } = render(EvalTelemetrySection, props)
  await waitFor(() =>
    expect(screen.getByText('[1 trace | 2 logs]')).toBeVisible()
  )
  await rerender({
    componentProps: {
      ...props,
      entries: props.entries.slice(0, 1),
      wholeTraces: false,
    },
  })
  await waitFor(() =>
    expect(screen.getByText('[1 trace | 1 log]')).toBeVisible()
  )
  logs.mockRejectedValueOnce(new Error('offline'))
  await rerender({ componentProps: { ...props, wholeTraces: true } })
  await waitFor(() => expect(logs).toHaveBeenCalledTimes(3))
  expect(screen.getByText('[1 trace | 1 log]')).toBeVisible()
  expect(screen.queryByText(/0 logs/)).toBeNull()
})

it('summarizes stored evidence and provides a native exact-span link, preserving zero duration', () => {
  render(EvalTelemetrySummaries, {
    entries: [{ reference, label: 'Result 1 · Execution' }],
    nodes: new Map([[referenceKey(reference), node]]),
  })
  const link = screen.getByRole('link', { name: /recorded operation/ })
  expect(link).toHaveAttribute('href', traceHref(reference))
  expect(link).not.toHaveAttribute('aria-pressed')
  expect(screen.getByText('Result 1 · Execution')).toHaveClass('text-secondary')
  expect(screen.getByTitle('eval-agent')).toBeVisible()
  expect(screen.getByText('Error')).toBeVisible()
  expect(screen.getByText('0 ns')).toBeVisible()
})

it('cancels obsolete log counts and refetches when telemetry nodes change', async () => {
  type Logs = Awaited<ReturnType<typeof telemetryAPI.getTraceLogSummaries>>
  let finish!: (logs: Logs) => void
  const logs = vi
    .spyOn(telemetryAPI, 'getTraceLogSummaries')
    .mockReturnValueOnce(
      new Promise<Logs>(resolve => {
        finish = resolve
      })
    )
    .mockResolvedValue([])
  const props = { entries: [{ reference, label: 'Tool' }], evidenceLogs: [] }
  const { rerender } = render(EvalTelemetrySection, props)
  await waitFor(() => expect(logs).toHaveBeenCalledOnce())
  const signal = logs.mock.calls[0][1]!
  const next = {
    ...props,
    entries: [
      { reference: { ...reference, spanID: 'c'.repeat(16) }, label: 'Other' },
    ],
  }
  await rerender({ componentProps: next })
  await waitFor(() => expect(logs).toHaveBeenCalledTimes(2))
  expect(signal.aborted).toBe(true)
  finish([
    {
      logRef: 'obsolete',
      timestamp: 1n,
      spanID: reference.spanID,
      eventName: '',
      severityNumber: 0,
      severityText: '',
      serviceName: '',
      bodyPreview: '',
    },
  ])
  await waitFor(() => expect(screen.getByText(/0 logs/)).toBeVisible())
  expect(screen.queryByText(/1 log/)).toBeNull()
  await rerender({
    componentProps: {
      ...next,
      nodes: new Map([[referenceKey(reference), node]]),
    },
  })
  await waitFor(() => expect(logs).toHaveBeenCalledTimes(3))
})

it('keeps unavailable references navigable and does not borrow a same-ID span from another trace', () => {
  const other = { ...reference, traceID: 'c'.repeat(32) }
  render(EvalTelemetrySummaries, {
    entries: [{ reference: other, label: 'Execution' }],
    nodes: new Map([[referenceKey(reference), node]]),
  })
  expect(screen.getByText('Span unavailable')).toBeVisible()
  expect(screen.queryByText('recorded operation')).toBeNull()
  expect(screen.getByRole('link')).toHaveAttribute('href', traceHref(other))
})

it('keeps all case repetitions without inventing a case span or repeating tool lists', () => {
  const first = {
    ...exampleExperiments[0].results[0],
    resultTelemetry: undefined,
    telemetry: reference,
  }
  const second = {
    ...first,
    id: 'second',
    repetition: 2,
    telemetry: { ...reference, traceID: 'c'.repeat(32) },
  }
  const experiment = { ...exampleExperiments[0], results: [first, second] }
  const rows = buildEvalOutline(
    groupResults(experiment.results, experiment.results),
    experiment,
    indexEvalTraces([])
  )
  const entries = evalTelemetrySummaries(
    experiment,
    rows.find(row => row.kind === 'case')
  )
  expect(entries).toEqual([
    { reference, label: 'Result · Execution' },
    { reference: second.telemetry, label: 'Result · Execution' },
  ])
  const tool = rows.find(row => row.kind === 'tool')!
  expect(evalTelemetrySummaries(experiment, tool)).toEqual([
    { reference: tool.reference, label: tool.name },
  ])
})

it('deduplicates an result and execution that reference the same stored span', () => {
  const result = {
    ...exampleExperiments[0].results[0],
    resultTelemetry: reference,
    telemetry: { ...reference, traceID: reference.traceID.toUpperCase() },
    assessments: [],
  }
  expect(
    evalTelemetrySummaries(exampleExperiments[0], undefined, result)
  ).toEqual([{ reference, label: 'Result' }])
})

it('keeps exact reference IDs in the native navigation link even when the span is unavailable', () => {
  const received = { traceID: 'A'.repeat(32), spanID: 'B'.repeat(16) }
  render(EvalTelemetrySummaries, {
    entries: [{ reference: received, label: 'Execution' }],
  })
  const identity = screen.getByRole('group', { name: 'Execution identifiers' })
  expect(within(identity).getByTitle(received.traceID)).toBeVisible()
  expect(within(identity).getByTitle(received.spanID)).toBeVisible()
  expect(within(identity).getByText('Traces:')).toBeVisible()
  expect(within(identity).queryByRole('button')).toBeNull()
  expect(identity.closest('a')).toBe(screen.getByRole('link'))
  expect(screen.getByRole('link')).toHaveAttribute('href', traceHref(received))
})

it('shows a trace-only identity without inventing an unavailable span', () => {
  const received = { traceID: reference.traceID }
  render(EvalTelemetrySummaries, {
    entries: [{ reference: received, label: 'Execution' }],
  })
  expect(screen.getByText('Trace', { exact: true })).toBeVisible()
  expect(screen.queryByText('Start:')).toBeNull()
  expect(screen.queryByRole('button', { name: 'Copy span ID' })).toBeNull()
  expect(screen.getByRole('link')).toHaveAttribute('href', traceHref(received))
  expect(screen.getByRole('link')).toHaveAttribute(
    'href',
    `/traces/${received.traceID}`
  )
})

it.each([undefined, reference.spanID])(
  'disables a confirmed absent trace while retaining its IDs (span: %s)',
  async spanID => {
    let finish!: (value: null) => void
    vi.mocked(telemetryAPI.getTraceSummary).mockReturnValue(
      new Promise(resolve => {
        finish = resolve
      })
    )
    const received = { traceID: reference.traceID, spanID }
    render(EvalTelemetrySummaries, {
      entries: [{ reference: received, label: 'Execution' }],
    })
    const link = screen.getByRole('link')
    expect(link).toHaveAttribute('href', traceHref(received))
    expect(screen.queryByText('(unavailable)', { exact: true })).toBeNull()
    finish(null)
    await waitFor(() => expect(link).toHaveAttribute('aria-disabled', 'true'))
    expect(link).not.toHaveAttribute('href')
    expect(link).toHaveAttribute('tabindex', '-1')
    const ids = screen.getByRole('group', { name: 'Execution identifiers' })
    expect(
      within(ids).getByText('(unavailable)', { exact: true })
    ).toBeVisible()
    expect(within(ids).getByTitle(received.traceID)).toBeVisible()
    if (spanID) expect(within(ids).getByTitle(spanID)).toBeVisible()
    expect(screen.queryByText('Start:')).toBeNull()
  }
)

it('shows the stored trace root summary and start without inventing a span identity', async () => {
  vi.mocked(telemetryAPI.getTraceSummary).mockResolvedValue({
    traceID: reference.traceID,
    hasRootSpan: true,
    rootSpan: { name: 'Recorded trace root', serviceName: 'eval-agent' },
    startTime: 0n,
    durationNs: 100n,
    spanCount: 2,
    errorCount: 0,
  })
  render(EvalTelemetrySummaries, {
    entries: [
      { reference: { traceID: reference.traceID }, label: 'Execution' },
    ],
  })
  await screen.findByText('Recorded trace root')
  expect(screen.getByText('Start:')).toBeVisible()
  expect(screen.getByRole('link')).toHaveAttribute(
    'href',
    `/traces/${reference.traceID}`
  )
  expect(screen.getByRole('link')).not.toHaveAttribute('aria-disabled')
  expect(screen.queryByText('Span ID')).toBeNull()
})

it('does not mark a failed lookup as a confirmed missing trace', async () => {
  render(EvalTelemetrySummaries, {
    entries: [
      { reference: { traceID: reference.traceID }, label: 'Execution' },
    ],
  })
  await waitFor(() => expect(telemetryAPI.getTraceSummary).toHaveBeenCalled())
  expect(screen.getByRole('link')).toHaveAttribute(
    'href',
    `/traces/${reference.traceID}`
  )
  expect(screen.queryByText('(unavailable)', { exact: true })).toBeNull()
})
