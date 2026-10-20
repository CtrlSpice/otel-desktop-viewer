// @vitest-environment jsdom
import { expect, it } from 'vitest'
import { render, screen } from '@testing-library/svelte'
import type { SpanNode } from '@/types/api-types'
import EvalTelemetrySummaries from './EvalTelemetrySummaries.svelte'
import { evalTelemetrySummaries } from './eval-telemetry-summaries'
import { exampleRuns } from './examples'
import { buildEvalOutline } from './eval-outline'
import { groupAttempts } from './group-attempts'
import { indexEvalTraces, referenceKey } from './eval-traces'
import { traceHref } from './model'

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
  expect(screen.getByText('Rep 1 · Execution · eval-agent')).toBeVisible()
  expect(screen.getByText('Error')).toBeVisible()
  expect(screen.getByText('0 ns')).toBeVisible()
})

it('keeps unavailable references navigable and does not borrow a same-ID span from another trace', () => {
  const other = { ...reference, traceID: 'c'.repeat(32) }
  render(EvalTelemetrySummaries, {
    entries: [{ reference: other, label: 'Execution' }],
    nodes: new Map([[referenceKey(reference), node]]),
  })
  expect(screen.getByText('Span summary unavailable')).toBeVisible()
  expect(screen.queryByText('recorded operation')).toBeNull()
  expect(screen.getByRole('link')).toHaveAttribute('href', traceHref(other))
})

it('keeps all case repetitions without inventing a case span or repeating tool lists', () => {
  const first = {
    ...exampleRuns[0].attempts[0],
    attemptTelemetry: undefined,
    telemetry: reference,
  }
  const second = {
    ...first,
    id: 'second',
    repetition: 2,
    telemetry: { ...reference, traceID: 'c'.repeat(32) },
  }
  const run = { ...exampleRuns[0], attempts: [first, second] }
  const rows = buildEvalOutline(
    groupAttempts(run.attempts, run.attempts),
    run,
    indexEvalTraces([])
  )
  const entries = evalTelemetrySummaries(
    run,
    rows.find(row => row.kind === 'case')
  )
  expect(entries).toEqual([
    { reference, label: 'Rep 1 · Execution' },
    { reference: second.telemetry, label: 'Rep 2 · Execution' },
  ])
  const tool = rows.find(row => row.kind === 'tool')!
  expect(evalTelemetrySummaries(run, tool)).toEqual([
    { reference: tool.reference, label: tool.name },
  ])
})

it('deduplicates an attempt and execution that reference the same stored span', () => {
  const attempt = {
    ...exampleRuns[0].attempts[0],
    attemptTelemetry: reference,
    telemetry: { ...reference, traceID: reference.traceID.toUpperCase() },
  }
  expect(evalTelemetrySummaries(exampleRuns[0], undefined, attempt)).toEqual([
    { reference, label: 'Rep 1 · Attempt' },
  ])
})
