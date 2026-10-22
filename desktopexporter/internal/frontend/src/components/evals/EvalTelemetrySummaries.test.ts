// @vitest-environment jsdom
import { afterEach, beforeEach, expect, it, vi } from 'vitest'
import { screen, waitFor, within } from '@testing-library/svelte'
import { telemetryAPI } from '@/services/telemetry-service'
import { renderWithContexts as render } from '@/test/render-helpers'
import type { SpanNode } from '@/types/api-types'
import EvalTelemetrySummaries from './EvalTelemetrySummaries.svelte'
import { evalTelemetrySummaries } from './eval-telemetry-summaries'
import { exampleRuns } from './examples'
import { buildEvalOutline } from './eval-outline'
import { groupReps } from './group-reps'
import { indexEvalTraces, referenceKey } from './eval-traces'
import { traceHref } from './model'

beforeEach(() => {
  vi.spyOn(telemetryAPI, 'getTraceSummary').mockRejectedValue(
    new Error('Lookup unavailable')
  )
})
afterEach(() => vi.restoreAllMocks())

const reference = { traceID: 'a'.repeat(32), spanID: 'b'.repeat(16) }
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

it('summarizes stored evidence and provides a native exact-span link, preserving zero duration', () => {
  render(EvalTelemetrySummaries, {
    entries: [{ reference, label: 'Rep 1 · Execution' }],
    nodes: new Map([[referenceKey(reference), node]]),
  })
  const link = screen.getByRole('link', { name: /recorded operation/ })
  expect(link).toHaveAttribute('href', traceHref(reference))
  expect(link).not.toHaveAttribute('aria-pressed')
  expect(screen.getByText('Rep 1 · Execution')).toHaveClass('text-secondary')
  expect(screen.getByTitle('eval-agent')).toBeVisible()
  expect(screen.getByText('Error')).toBeVisible()
  expect(screen.getByText('0 ns')).toBeVisible()
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
    ...exampleRuns[0].reps[0],
    repTelemetry: undefined,
    telemetry: reference,
  }
  const second = {
    ...first,
    id: 'second',
    repetition: 2,
    telemetry: { ...reference, traceID: 'c'.repeat(32) },
  }
  const run = { ...exampleRuns[0], reps: [first, second] }
  const rows = buildEvalOutline(
    groupReps(run.reps, run.reps),
    run,
    indexEvalTraces([])
  )
  const entries = evalTelemetrySummaries(
    run,
    rows.find(row => row.kind === 'case')
  )
  expect(entries).toEqual([
    { reference, label: 'Result · Execution' },
    { reference: second.telemetry, label: 'Result · Execution' },
  ])
  const tool = rows.find(row => row.kind === 'tool')!
  expect(evalTelemetrySummaries(run, tool)).toEqual([
    { reference: tool.reference, label: tool.name },
  ])
})

it('deduplicates an rep and execution that reference the same stored span', () => {
  const rep = {
    ...exampleRuns[0].reps[0],
    repTelemetry: reference,
    telemetry: { ...reference, traceID: reference.traceID.toUpperCase() },
    assessments: [],
  }
  expect(evalTelemetrySummaries(exampleRuns[0], undefined, rep)).toEqual([
    { reference, label: 'Result' },
  ])
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
  expect(screen.queryByText('Started:')).toBeNull()
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
    expect(screen.queryByText('Unavailable', { exact: true })).toBeNull()
    finish(null)
    await waitFor(() => expect(link).toHaveAttribute('aria-disabled', 'true'))
    expect(link).not.toHaveAttribute('href')
    expect(link).toHaveAttribute('tabindex', '-1')
    const ids = screen.getByRole('group', { name: 'Execution identifiers' })
    expect(within(ids).getByText('Unavailable', { exact: true })).toBeVisible()
    expect(within(ids).getByTitle(received.traceID)).toBeVisible()
    if (spanID) expect(within(ids).getByTitle(spanID)).toBeVisible()
    expect(screen.queryByText('Started:')).toBeNull()
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
  expect(screen.getByText('Started:')).toBeVisible()
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
  expect(screen.queryByText('Unavailable', { exact: true })).toBeNull()
})
