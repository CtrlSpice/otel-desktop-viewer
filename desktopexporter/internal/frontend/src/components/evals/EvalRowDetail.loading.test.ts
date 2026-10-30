// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/svelte'
import { renderWithContexts as render } from '@/test/render-helpers'
import type { LogData, SpanData } from '@/types/api-types'
import { telemetryAPI } from '@/services/telemetry-service'
import EvalRowDetail from './EvalRowDetail.svelte'
import { exampleExperiments } from './examples'
import type { EvalOutlineRow } from './eval-outline'
import { copyText } from '@/utils/clipboard'
import { referenceKey } from './eval-traces'

function span(spanID: string): SpanData {
  return {
    traceID: 'a'.repeat(32),
    spanID,
    parentSpanID: null,
    traceState: '',
    flags: 0,
    name: `tool-${spanID}`,
    kind: 'Internal',
    kindCode: 1,
    startTime: 1n,
    endTime: 2n,
    statusCode: 'Unset',
    statusCodeValue: 0,
    statusMessage: '',
    attributes: [],
    events: [
      {
        name: 'tool.output',
        timestamp: 2n,
        droppedAttributesCount: 0,
        attributes: [
          {
            key: 'output',
            value: { kind: 'string', value: `output-${spanID}` },
          },
        ],
      },
    ],
    links: [],
    droppedAttributesCount: 0,
    droppedEventsCount: 0,
    droppedLinksCount: 0,
    resource: { attributes: [], droppedAttributesCount: 0 },
    scope: { name: '', version: '', attributes: [], droppedAttributesCount: 0 },
  }
}
afterEach(() => vi.restoreAllMocks())
vi.mock('@/utils/clipboard', () => ({ copyText: vi.fn() }))

function row(span: SpanData): EvalOutlineRow {
  return {
    id: span.spanID,
    parentID: null,
    kind: 'tool',
    caseID: `case-${span.spanID}`,
    name: span.name,
    depth: 0,
    state: 'Unknown',
    internalFault: false,
    spanError: false,
    reference: { traceID: span.traceID, spanID: span.spanID },
    estimate: { total: null, incomplete: true },
    tree: {} as EvalOutlineRow['tree'],
  }
}

const experiment = { ...exampleExperiments[0], results: [] }

it('publishes the heading, context, evidence and Copy target together after replacement logs load', async () => {
  const first = span('1'.repeat(16))
  const second = span('2'.repeat(16))
  const nodes = new Map(
    [first, second].map(spanData => [
      referenceKey(spanData),
      { spanData, depth: 0, matched: true },
    ])
  )
  let finish!: (logs: LogData[]) => void
  const loader = vi
    .spyOn(telemetryAPI, 'getSpanLogs')
    .mockResolvedValueOnce([])
    .mockImplementationOnce(
      () =>
        new Promise(resolve => {
          finish = resolve
        })
    )
  const view = render(EvalRowDetail, {
    experiment,
    row: row(first),
    telemetryNodes: nodes,
  })
  const field = await screen.findByText(`output-${first.spanID}`)
  await waitFor(() => expect(loader).toHaveBeenCalledTimes(1))
  const heading = view.container.querySelector(
    'summary.detail-bubble__heading'
  )!
  await view.rerender({
    componentProps: { experiment, row: row(second), telemetryNodes: nodes },
  })
  await waitFor(() => expect(loader).toHaveBeenCalledTimes(2))
  expect(heading).toHaveTextContent(first.name)
  expect(screen.getByText(`case-${first.spanID}`)).toBeVisible()
  expect(screen.getByText(`output-${first.spanID}`)).toBe(field)
  expect(screen.queryByText(`output-${second.spanID}`)).toBeNull()
  expect(screen.queryByText('Loading linked log evidence…')).toBeNull()
  expect(await screen.findByText('Loading linked log evidence…')).toBeVisible()
  expect(heading).toHaveTextContent(first.name)
  await fireEvent.click(screen.getByRole('button', { name: 'Copy output' }))
  expect(copyText).toHaveBeenLastCalledWith(`output-${first.spanID}`)
  finish([])
  await screen.findByText(`output-${second.spanID}`)
  expect(view.container.querySelector('summary.detail-bubble__heading')).toBe(
    heading
  )
  expect(heading).toHaveTextContent(second.name)
  expect(screen.getByText(`case-${second.spanID}`)).toBeVisible()
  expect(screen.queryByText(`case-${first.spanID}`)).toBeNull()
  expect(screen.queryByText(`output-${first.spanID}`)).toBeNull()
  await fireEvent.click(screen.getByRole('button', { name: 'Copy output' }))
  expect(copyText).toHaveBeenLastCalledWith(`output-${second.spanID}`)
})

it('shows span output immediately and never lets a late log response replace the newly selected tool', async () => {
  const first = span('1'.repeat(16))
  const second = span('2'.repeat(16))
  const nodes = new Map(
    [first, second].map(spanData => [
      referenceKey(spanData),
      { spanData, depth: 0, matched: true },
    ])
  )
  let finish!: (logs: LogData[]) => void
  const loader = vi
    .spyOn(telemetryAPI, 'getSpanLogs')
    .mockReturnValueOnce(
      new Promise(resolve => {
        finish = resolve
      })
    )
    .mockResolvedValueOnce([])
  const { rerender } = render(EvalRowDetail, {
    experiment,
    row: row(first),
    telemetryNodes: nodes,
  })
  expect(screen.getByText(`output-${first.spanID}`)).toBeVisible()
  await waitFor(() => expect(loader).toHaveBeenCalledTimes(1))
  const signal = loader.mock.calls[0][2]!
  await rerender({
    componentProps: { experiment, row: row(second), telemetryNodes: nodes },
  })
  expect(signal.aborted).toBe(true)
  await waitFor(() =>
    expect(screen.queryByText('Loading linked log evidence…')).toBeNull()
  )
  finish([
    {
      logRef: 'late',
      traceID: first.traceID,
      spanID: first.spanID,
      timestamp: 1n,
      observedTimestamp: 1n,
      severityText: '',
      severityNumber: 0,
      body: { kind: 'string', value: 'stale log output' },
      eventName: 'tool.output',
      resource: first.resource,
      scope: first.scope,
      flags: 0,
      attributes: [],
      droppedAttributesCount: 0,
    },
  ])
  await waitFor(() =>
    expect(screen.getByText(`output-${second.spanID}`)).toBeVisible()
  )
  expect(screen.queryByText(`output-${first.spanID}`)).toBeNull()
  expect(screen.queryByText('stale log output')).toBeNull()
})

it('rejects superseded responses across A → B → A and same-ID refreshes', async () => {
  const first = span('1'.repeat(16))
  const second = span('2'.repeat(16))
  const nodes = new Map(
    [first, second].map(spanData => [
      referenceKey(spanData),
      { spanData, depth: 0, matched: true },
    ])
  )
  const pending: Array<(logs: LogData[]) => void> = []
  const loader = vi
    .spyOn(telemetryAPI, 'getSpanLogs')
    .mockImplementation(() => new Promise(resolve => pending.push(resolve)))
  const view = render(EvalRowDetail, {
    experiment,
    row: row(first),
    telemetryNodes: nodes,
  })
  await waitFor(() => expect(loader).toHaveBeenCalledTimes(1))
  await view.rerender({
    componentProps: { experiment, row: row(second), telemetryNodes: nodes },
  })
  await waitFor(() => expect(loader).toHaveBeenCalledTimes(2))
  const refreshed = {
    ...first,
    name: 'refreshed tool A',
    events: second.events,
  }
  const refreshedNodes = new Map(nodes)
  refreshedNodes.set(referenceKey(first), {
    spanData: refreshed,
    depth: 0,
    matched: true,
  })
  await view.rerender({
    componentProps: {
      experiment,
      row: row(refreshed),
      telemetryNodes: refreshedNodes,
    },
  })
  await waitFor(() => expect(loader).toHaveBeenCalledTimes(3))
  expect(loader.mock.calls[0][2]?.aborted).toBe(true)
  expect(loader.mock.calls[1][2]?.aborted).toBe(true)
  const heading = view.container.querySelector(
    'summary.detail-bubble__heading'
  )!
  expect(heading).toHaveTextContent(first.name)
  pending[2]([])
  await screen.findByText(`output-${second.spanID}`)
  expect(heading).toHaveTextContent(refreshed.name)
  pending[1]([])
  pending[0]([])
  await waitFor(() =>
    expect(screen.queryByText('Loading linked log evidence…')).toBeNull()
  )
  expect(heading).toHaveTextContent(refreshed.name)
  expect(screen.queryByText(`output-${first.spanID}`)).toBeNull()
})

it('publishes a failed replacement with its own span evidence and context', async () => {
  const first = span('1'.repeat(16))
  const second = span('2'.repeat(16))
  const nodes = new Map(
    [first, second].map(spanData => [
      referenceKey(spanData),
      { spanData, depth: 0, matched: true },
    ])
  )
  let fail!: (reason: Error) => void
  vi.spyOn(telemetryAPI, 'getSpanLogs')
    .mockResolvedValueOnce([])
    .mockImplementationOnce(
      () =>
        new Promise((_, reject) => {
          fail = reject
        })
    )
  const view = render(EvalRowDetail, {
    experiment,
    row: row(first),
    telemetryNodes: nodes,
  })
  await screen.findByText(`output-${first.spanID}`)
  await view.rerender({
    componentProps: { experiment, row: row(second), telemetryNodes: nodes },
  })
  fail(new Error('unavailable'))
  await screen.findByText('Linked log evidence could not be loaded.')
  expect(
    view.container.querySelector('summary.detail-bubble__heading')
  ).toHaveTextContent(second.name)
  expect(screen.getByText(`output-${second.spanID}`)).toBeVisible()
  expect(screen.queryByText(`output-${first.spanID}`)).toBeNull()
  await fireEvent.click(screen.getByRole('button', { name: 'Copy output' }))
  expect(copyText).toHaveBeenLastCalledWith(`output-${second.spanID}`)
})

it('cancels pending evidence when selecting a row without a span', async () => {
  const first = span('1'.repeat(16))
  const nodes = new Map([
    [referenceKey(first), { spanData: first, depth: 0, matched: true }],
  ])
  let finish!: (logs: LogData[]) => void
  const loader = vi.spyOn(telemetryAPI, 'getSpanLogs').mockImplementation(
    () =>
      new Promise(resolve => {
        finish = resolve
      })
  )
  const view = render(EvalRowDetail, {
    experiment,
    row: row(first),
    telemetryNodes: nodes,
  })
  await waitFor(() => expect(loader).toHaveBeenCalledTimes(1))
  const next = {
    ...row(first),
    id: 'without-span',
    name: 'Without span',
    reference: undefined,
  }
  await view.rerender({
    componentProps: { experiment, row: next, telemetryNodes: nodes },
  })
  expect(loader.mock.calls[0][2]?.aborted).toBe(true)
  expect(screen.queryByText(`output-${first.spanID}`)).toBeNull()
  expect(
    view.container.querySelector('summary.detail-bubble__heading')
  ).toHaveTextContent(next.name)
  finish([])
  await waitFor(() =>
    expect(screen.queryByText(`output-${first.spanID}`)).toBeNull()
  )
  expect(
    view.container.querySelector('summary.detail-bubble__heading')
  ).toHaveTextContent(next.name)
})
