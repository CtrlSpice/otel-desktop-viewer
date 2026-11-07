// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import { tick } from 'svelte'
import EvalResults from './EvalResults.svelte'
import { exampleExperiments } from './examples'
import { telemetryAPI } from '@/services/telemetry-service'
import type { LogData, TraceData, TraceLogSummary } from '@/types/api-types'

afterEach(() => vi.restoreAllMocks())

const references = ['a', 'b', 'c', 'd', 'e'].map(letter => ({
  traceID: letter.repeat(32),
  spanID: '1'.repeat(16),
}))
const results = references.map((telemetry, index) => ({
  ...exampleExperiments[0].results[0],
  id: `result-${index}`,
  caseID: `case-${index}`,
  telemetry,
  resultTelemetry: undefined,
  tools: [],
  assessments: [],
}))
const experiment = {
  ...exampleExperiments[0],
  telemetry: undefined,
  results,
}

function log(index: number): LogData {
  return {
    ...references[index],
    logRef: `log-${index}`,
    timestamp: 1n,
    observedTimestamp: 1n,
    eventName: 'tool.output',
    severityNumber: 0,
    severityText: '',
    body: { kind: 'string', value: `Full output ${index}` },
    attributes: [],
    resource: { attributes: [], droppedAttributesCount: 0 },
    scope: { name: '', version: '', attributes: [], droppedAttributesCount: 0 },
    flags: 0,
    droppedAttributesCount: 0,
  }
}

it('starts all known activity requests before span lookup and publishes each owner without a sibling barrier', async () => {
  const spans = Promise.withResolvers<{
    traces: TraceData[]
    unavailableTraceIDs: string[]
  }>()
  const lookup = vi
    .spyOn(telemetryAPI, 'getReferencedTraceViews')
    .mockReturnValue(spans.promise)
  const summaryReplies = references.map(() =>
    Promise.withResolvers<TraceLogSummary[]>()
  )
  const summaries = vi
    .spyOn(telemetryAPI, 'getTraceLogSummaries')
    .mockImplementation(
      id =>
        summaryReplies[references.findIndex(ref => ref.traceID === id)].promise
    )
  const logReplies = references.map(() => Promise.withResolvers<LogData[]>())
  const logs = vi
    .spyOn(telemetryAPI, 'getSpanLogs')
    .mockImplementation(
      id => logReplies[references.findIndex(ref => ref.traceID === id)].promise
    )
  const onTraceViews = vi.fn()
  const view = render(EvalResults, { experiment, results, onTraceViews })
  expect(lookup).toHaveBeenCalledOnce()
  expect(summaries).toHaveBeenCalledTimes(5)
  expect(logs).toHaveBeenCalledTimes(5)
  logReplies[4].resolve([log(4)])
  await waitFor(() =>
    expect(onTraceViews.mock.calls.at(-1)?.[2]).toEqual([log(4)])
  )
  expect(onTraceViews.mock.calls.at(-1)?.[1]).toBe(true)
  const publish = lookup.mock.calls[0][2]!
  publish({
    status: 'ready',
    trace: { traceID: references[4].traceID, spans: [], unplacedSpanCount: 0 },
  })
  summaryReplies[4].resolve([])
  await tick()
  expect(summaries).toHaveBeenCalledTimes(5)
  expect(logs).toHaveBeenCalledTimes(5)
  expect(logs.mock.calls[0][2]?.aborted).toBe(false)
  logReplies[1].reject(new Error('Second owner failed'))
  await screen.findByText('Tool evidence could not be loaded.')
  expect(onTraceViews.mock.calls.at(-1)?.[2]).toEqual([log(4)])
  view.unmount()
  expect(lookup.mock.calls[0][1]?.aborted).toBe(true)
  expect(summaries.mock.calls.every(([, signal]) => signal?.aborted)).toBe(true)
  expect(logs.mock.calls.every(([, , signal]) => signal?.aborted)).toBe(true)
  logReplies[0].resolve([log(0)])
  summaryReplies[0].reject(new Error('Unmounted failure'))
  spans.resolve({ traces: [], unavailableTraceIDs: [] })
  const publications = onTraceViews.mock.calls.length
  await tick()
  expect(onTraceViews).toHaveBeenCalledTimes(publications)
})
