// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import type { LogData, SpanData } from '@/types/api-types'
import { telemetryAPI } from '@/services/telemetry-service'
import EvalSpanEvidence from './EvalSpanEvidence.svelte'
import { referenceKey } from './eval-traces'

function span(spanID: string): SpanData {
  return {
    traceID: 'a'.repeat(32),
    spanID,
    parentSpanID: null,
    traceState: '',
    flags: 0,
    name: 'tool',
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

it('retains evidence fields while replacement logs load', async () => {
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
  const view = render(EvalSpanEvidence, { span: first, nodes })
  const field = await screen.findByText(`output-${first.spanID}`)
  await waitFor(() => expect(loader).toHaveBeenCalledTimes(1))
  await view.rerender({ span: second, nodes })
  expect(screen.getByText(`output-${first.spanID}`)).toBe(field)
  expect(screen.queryByText(`output-${second.spanID}`)).toBeNull()
  finish([])
  await screen.findByText(`output-${second.spanID}`)
  expect(screen.queryByText(`output-${first.spanID}`)).toBeNull()
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
  const { rerender } = render(EvalSpanEvidence, { span: first, nodes })
  expect(screen.getByText(`output-${first.spanID}`)).toBeVisible()
  await waitFor(() => expect(loader).toHaveBeenCalledTimes(1))
  const signal = loader.mock.calls[0][2]!
  await rerender({ span: second, nodes })
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
