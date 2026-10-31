// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { render, screen, within, waitFor } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import EvalResults from './EvalResults.svelte'
import { exampleExperiments } from './examples'
import { telemetryAPI } from '@/services/telemetry-service'
import type {
  LogData,
  SpanTiming,
  TraceData,
  TraceLogSummary,
} from '@/types/api-types'

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
  it('publishes per-trace success and failure while unrelated trace work is pending', async () => {
    const references = ['a', 'b', 'c'].map(letter => ({
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
    const pending =
      Promise.withResolvers<
        Awaited<ReturnType<typeof telemetryAPI.getReferencedTraceViews>>
      >()
    const lookup = vi
      .spyOn(telemetryAPI, 'getReferencedTraceViews')
      .mockReturnValue(pending.promise)
    const summaries = vi
      .spyOn(telemetryAPI, 'getTraceLogSummaries')
      .mockResolvedValue([])
    const onTraceViews = vi.fn()
    try {
      const view = render(EvalResults, { experiment, results, onTraceViews })
      await waitFor(() => expect(lookup).toHaveBeenCalledOnce())
      const publish = lookup.mock.calls[0][2]!
      const a: TraceData = {
        traceID: references[0].traceID,
        spans: [],
        unplacedSpanCount: 0,
      }
      publish({ status: 'ready', trace: a })
      publish({ status: 'failed', traceID: references[1].traceID })
      await waitFor(() =>
        expect(onTraceViews).toHaveBeenLastCalledWith(
          [a],
          true,
          [],
          new Map([
            [references[0].traceID, 'ready'],
            [references[1].traceID, 'failed'],
            [references[2].traceID, 'pending'],
          ])
        )
      )
      const c = { ...a, traceID: references[2].traceID }
      publish({ status: 'ready', trace: c })
      await waitFor(() =>
        expect(onTraceViews).toHaveBeenLastCalledWith(
          [a, c],
          false,
          [],
          new Map([
            [references[0].traceID, 'ready'],
            [references[1].traceID, 'failed'],
            [references[2].traceID, 'ready'],
          ])
        )
      )
      view.unmount()
      expect(lookup.mock.calls[0][1]?.aborted).toBe(true)
    } finally {
      pending.resolve({ traces: [], unavailableTraceIDs: [] })
      lookup.mockRestore()
      summaries.mockRestore()
    }
  })

  it.each(['explicit', 'native-ancestor'])(
    'preserves independently authorized owners across an empty summary and another trace failure: %s',
    async authorization => {
      const a = { traceID: 'a'.repeat(32), spanID: '1'.repeat(16) },
        child = { traceID: 'a'.repeat(32), spanID: '3'.repeat(16) },
        b = { traceID: 'b'.repeat(32), spanID: '2'.repeat(16) }
      const refs = authorization === 'explicit' ? [a, b] : [a, child, b]
      const views = traceViews(
        refs.map(reference => ({
          ...reference,
          name: reference === child ? 'tool child' : 'execution',
          serviceName: 'runner',
          startTime: 10n,
          endTime: 100n,
          statusCodeValue: 0,
        }))
      )
      if (authorization === 'native-ancestor')
        views.traces[0].spans[1].spanData.attributes = [
          {
            key: 'gen_ai.operation.name',
            value: { kind: 'string', value: 'execute_tool' },
          },
          {
            key: 'gen_ai.tool.call.id',
            value: { kind: 'string', value: 'recorded-call' },
          },
        ]
      const summary = (reference: typeof a): TraceLogSummary => ({
        logRef: reference.spanID,
        spanID: reference.spanID,
        timestamp: 20n,
        eventName: 'tool.execution',
        severityNumber: 0,
        severityText: '',
        serviceName: '',
        bodyPreview: 'call',
      })
      const owner = views.traces[0].spans[0].spanData
      const full = (reference: typeof a): LogData => ({
        ...reference,
        logRef: reference.spanID,
        timestamp: 20n,
        observedTimestamp: 20n,
        eventName: 'tool.execution',
        severityNumber: 0,
        severityText: '',
        body: { kind: 'string', value: 'old' },
        attributes: [
          {
            key: 'gen_ai.tool.call.id',
            value: { kind: 'string', value: 'recorded-call' },
          },
        ],
        resource: owner.resource,
        scope: owner.scope,
        flags: 0,
        droppedAttributesCount: 0,
      })
      let refresh = false
      const held: Array<(logs: LogData[]) => void> = []
      const lookup = vi
        .spyOn(telemetryAPI, 'getReferencedTraceViews')
        .mockImplementation(async () => {
          if (refresh && authorization === 'explicit')
            return {
              ...views,
              traces: views.traces.map(trace =>
                trace.traceID === a.traceID ? { ...trace, spans: [] } : trace
              ),
            }
          return views
        })
      const summaries = vi
        .spyOn(telemetryAPI, 'getTraceLogSummaries')
        .mockImplementation(id =>
          !refresh
            ? Promise.resolve([summary(id === a.traceID ? a : b)])
            : id === a.traceID
              ? Promise.resolve([])
              : Promise.reject(new Error('B unavailable'))
        )
      const logs = vi
        .spyOn(telemetryAPI, 'getSpanLogs')
        .mockImplementation(id =>
          refresh && id === a.traceID
            ? new Promise(resolve => held.push(resolve))
            : Promise.resolve([full(id === a.traceID ? a : b)])
        )
      const results = [a, b].map((reference, index) => ({
        ...exampleExperiments[0].results[0],
        id: `result-${index}`,
        caseID: `case-${index}`,
        telemetry:
          reference === a
            ? authorization === 'explicit'
              ? a
              : child
            : { traceID: b.traceID },
        resultTelemetry: undefined,
        tools: [],
        toolsKnown: false,
        assessments: [],
      }))
      const experiment = {
        ...exampleExperiments[0],
        telemetry: undefined,
        results,
      }
      const onTraceViews = vi.fn()
      const published = () => onTraceViews.mock.calls.at(-1)?.[2] as LogData[]
      try {
        const view = render(EvalResults, { experiment, results, onTraceViews })
        await waitFor(() => expect(published()).toHaveLength(2))
        refresh = true
        await view.rerender({
          experiment: { ...experiment, name: 'refreshed' },
          results,
          onTraceViews,
        })
        await waitFor(() => expect(held.length).toBeGreaterThan(0))
        await waitFor(() =>
          expect(summaries.mock.calls.length).toBeGreaterThanOrEqual(4)
        )
        expect(published().map(log => log.traceID)).toEqual([
          a.traceID,
          b.traceID,
        ])
        expect(logs.mock.calls.at(-1)?.[2]?.aborted).toBe(false)
        for (const release of held)
          release([{ ...full(a), body: { kind: 'string', value: 'fresh A' } }])
        await waitFor(() =>
          expect(published()[0].body).toEqual({
            kind: 'string',
            value: 'fresh A',
          })
        )
        expect(published()[1].traceID).toBe(b.traceID)
        await screen.findByText('Tool evidence could not be loaded.')
      } finally {
        for (const release of held) release([])
        lookup.mockRestore()
        summaries.mockRestore()
        logs.mockRestore()
      }
    }
  )

  it.each(['pending', 'failed'])(
    'invalidates each successful trace’s old owners while another trace is %s, including retry',
    async other => {
      const a = { traceID: 'a'.repeat(32), spanID: '1'.repeat(16) },
        b = { traceID: 'b'.repeat(32), spanID: '2'.repeat(16) }
      const views = traceViews(
        [a, b].map(reference => ({
          ...reference,
          name: 'execution',
          serviceName: 'runner',
          startTime: 10n,
          endTime: 100n,
          statusCodeValue: 0,
        }))
      )
      const summary = (reference: typeof a): TraceLogSummary => ({
        logRef: reference.spanID,
        spanID: reference.spanID,
        timestamp: 20n,
        eventName: 'tool.execution',
        severityNumber: 0,
        severityText: '',
        serviceName: '',
        bodyPreview: 'call',
      })
      const full = (reference: typeof a): LogData => ({
        ...reference,
        logRef: reference.spanID,
        timestamp: 20n,
        observedTimestamp: 20n,
        eventName: 'tool.execution',
        severityNumber: 0,
        severityText: '',
        body: { kind: 'string', value: reference.spanID },
        attributes: [
          {
            key: 'gen_ai.tool.call.id',
            value: { kind: 'string', value: reference.spanID },
          },
        ],
        resource: views.traces[0].spans[0].spanData.resource,
        scope: views.traces[0].spans[0].spanData.scope,
        flags: 0,
        droppedAttributesCount: 0,
      })
      let phase = 'initial'
      const held: Array<(logs: TraceLogSummary[]) => void> = []
      const lookup = vi
        .spyOn(telemetryAPI, 'getReferencedTraceViews')
        .mockResolvedValue(views)
      const summaries = vi
        .spyOn(telemetryAPI, 'getTraceLogSummaries')
        .mockImplementation(id => {
          if (phase === 'initial')
            return Promise.resolve([summary(id === a.traceID ? a : b)])
          if (id === a.traceID || phase === 'retry') return Promise.resolve([])
          return other === 'failed'
            ? Promise.reject(new Error('B unavailable'))
            : new Promise(resolve => held.push(resolve))
        })
      const logs = vi
        .spyOn(telemetryAPI, 'getSpanLogs')
        .mockImplementation(async id => [full(id === a.traceID ? a : b)])
      const results = [a, b].map((reference, index) => ({
        ...exampleExperiments[0].results[0],
        id: `result-${index}`,
        caseID: `case-${index}`,
        telemetry: { traceID: reference.traceID },
        resultTelemetry: undefined,
        tools: [],
        toolsKnown: false,
        assessments: [],
      }))
      const experiment = {
        ...exampleExperiments[0],
        telemetry: undefined,
        results,
      }
      const onTraceViews = vi.fn()
      const published = () => onTraceViews.mock.calls.at(-1)?.[2] as LogData[]
      try {
        const view = render(EvalResults, {
          experiment,
          results,
          onTraceViews,
          viewKey: 'time',
        })
        await waitFor(() => expect(published()).toHaveLength(2))
        phase = 'refresh'
        await view.rerender({
          experiment: { ...experiment, name: 'refresh' },
          results,
          onTraceViews,
          viewKey: 'time',
        })
        await waitFor(() =>
          expect(published().map(log => log.traceID)).toEqual([b.traceID])
        )
        expect(
          logs.mock.calls.every(
            call => call[0] === a.traceID || call[0] === b.traceID
          )
        ).toBe(true)
        if (other === 'failed')
          await screen.findByText('Tool evidence could not be loaded.')
        else
          expect(
            await screen.findByText('Loading tool evidence…')
          ).toBeVisible()
        phase = 'retry'
        await view.rerender({
          experiment: { ...experiment, name: 'retry' },
          results,
          onTraceViews,
          viewKey: 'time',
        })
        await waitFor(() => expect(published()).toEqual([]))
        expect(screen.getByRole('radio', { name: 'Tool calls' })).toBeDisabled()
        for (const release of held) release([summary(b)])
        await Promise.resolve()
        expect(published()).toEqual([])
      } finally {
        for (const release of held) release([])
        lookup.mockRestore()
        summaries.mockRestore()
        logs.mockRestore()
      }
    }
  )

  it('replaces a successful owner set immediately and rejects a late active owner removed by the next refresh', async () => {
    const a = { traceID: 'a'.repeat(32), spanID: '1'.repeat(16) },
      next = { traceID: 'a'.repeat(32), spanID: '3'.repeat(16) },
      b = { traceID: 'b'.repeat(32), spanID: '2'.repeat(16) }
    const views = traceViews(
      [a, next, b].map(reference => ({
        ...reference,
        name: 'execution',
        serviceName: 'runner',
        startTime: 10n,
        endTime: 100n,
        statusCodeValue: 0,
      }))
    )
    const summary = (reference: typeof a): TraceLogSummary => ({
      logRef: reference.spanID,
      spanID: reference.spanID,
      timestamp: 20n,
      eventName: 'tool.execution',
      severityNumber: 0,
      severityText: '',
      serviceName: '',
      bodyPreview: 'call',
    })
    const owner = views.traces[0].spans[0].spanData
    const full = (reference: typeof a): LogData => ({
      ...reference,
      logRef: reference.spanID,
      timestamp: 20n,
      observedTimestamp: 20n,
      eventName: 'tool.execution',
      severityNumber: 0,
      severityText: '',
      body: { kind: 'string', value: reference.spanID },
      attributes: [
        {
          key: 'gen_ai.tool.call.id',
          value: { kind: 'string', value: reference.spanID },
        },
      ],
      resource: owner.resource,
      scope: owner.scope,
      flags: 0,
      droppedAttributesCount: 0,
    })
    let phase = 0
    const pending: Array<(logs: LogData[]) => void> = []
    const lookup = vi
      .spyOn(telemetryAPI, 'getReferencedTraceViews')
      .mockResolvedValue(views)
    const summaries = vi
      .spyOn(telemetryAPI, 'getTraceLogSummaries')
      .mockImplementation(id => {
        if (phase === 0)
          return Promise.resolve([summary(id === a.traceID ? a : b)])
        if (id === b.traceID) return Promise.reject(new Error('B unavailable'))
        return Promise.resolve(phase === 1 ? [summary(next)] : [])
      })
    const logs = vi
      .spyOn(telemetryAPI, 'getSpanLogs')
      .mockImplementation((id, spanID) =>
        spanID === next.spanID
          ? new Promise(resolve => pending.push(resolve))
          : Promise.resolve([full(id === a.traceID ? a : b)])
      )
    const results = [a, b].map((reference, index) => ({
      ...exampleExperiments[0].results[0],
      id: `result-${index}`,
      caseID: `case-${index}`,
      telemetry: { traceID: reference.traceID },
      resultTelemetry: undefined,
      tools: [],
      toolsKnown: false,
      assessments: [],
    }))
    const experiment = {
      ...exampleExperiments[0],
      telemetry: undefined,
      results,
    }
    const onTraceViews = vi.fn()
    const published = () => onTraceViews.mock.calls.at(-1)?.[2] as LogData[]
    try {
      const view = render(EvalResults, { experiment, results, onTraceViews })
      await waitFor(() => expect(published()).toHaveLength(2))
      phase = 1
      await view.rerender({
        experiment: { ...experiment, name: 'new owner' },
        results,
        onTraceViews,
      })
      await waitFor(() => expect(pending.length).toBeGreaterThan(0))
      expect(published().map(log => log.spanID)).toEqual([b.spanID])
      phase = 2
      await view.rerender({
        experiment: { ...experiment, name: 'owner removed' },
        results,
        onTraceViews,
      })
      await screen.findByText('Tool evidence could not be loaded.')
      for (const release of pending) release([full(next)])
      await Promise.resolve()
      await Promise.resolve()
      expect(
        logs.mock.calls
          .filter(call => call[1] === next.spanID)
          .every(call => call[2]?.aborted)
      ).toBe(true)
      expect(published().map(log => log.spanID)).toEqual([b.spanID])
    } finally {
      for (const release of pending) release([])
      lookup.mockRestore()
      summaries.mockRestore()
      logs.mockRestore()
    }
  })

  it('rejects stale owner success/failure across A → B → A and retains fresh same-ID evidence on failure', async () => {
    const reference = { traceID: 'a'.repeat(32), spanID: '1'.repeat(16) }
    const views = traceViews([
      {
        ...reference,
        name: 'execution',
        serviceName: 'runner',
        startTime: 10n,
        endTime: 100n,
        statusCodeValue: 0,
      },
    ])
    const lookup = vi
      .spyOn(telemetryAPI, 'getReferencedTraceViews')
      .mockResolvedValue(views)
    const summaries = vi
      .spyOn(telemetryAPI, 'getTraceLogSummaries')
      .mockResolvedValue([
        {
          logRef: 'call',
          spanID: reference.spanID,
          timestamp: 20n,
          eventName: 'tool.execution',
          severityNumber: 0,
          severityText: '',
          serviceName: '',
          bodyPreview: 'call',
        },
      ])
    const pending: Array<{
      resolve: (logs: LogData[]) => void
      reject: (error: Error) => void
    }> = []
    const logs = vi
      .spyOn(telemetryAPI, 'getSpanLogs')
      .mockImplementation(
        () =>
          new Promise((resolve, reject) => pending.push({ resolve, reject }))
      )
    const result = {
      ...exampleExperiments[0].results[0],
      telemetry: reference,
      resultTelemetry: undefined,
      tools: [],
      assessments: [],
    }
    const a = { ...exampleExperiments[0], results: [result] },
      b = { ...a, id: 'experiment-B' }
    const onTraceViews = vi.fn()
    try {
      const view = render(EvalResults, {
        experiment: a,
        results: a.results,
        onTraceViews,
      })
      await waitFor(() => expect(pending).toHaveLength(1))
      await view.rerender({ experiment: b, results: b.results, onTraceViews })
      await waitFor(() => expect(pending).toHaveLength(2))
      await view.rerender({ experiment: a, results: a.results, onTraceViews })
      await waitFor(() => expect(pending).toHaveLength(3))
      const owner = views.traces[0].spans[0].spanData
      const fresh: LogData = {
        ...reference,
        logRef: 'fresh',
        timestamp: 20n,
        observedTimestamp: 20n,
        eventName: 'tool.execution',
        severityNumber: 0,
        severityText: '',
        body: { kind: 'string', value: 'fresh' },
        attributes: [],
        resource: owner.resource,
        scope: owner.scope,
        flags: 0,
        droppedAttributesCount: 0,
      }
      pending[2].resolve([fresh])
      await waitFor(() =>
        expect(onTraceViews).toHaveBeenLastCalledWith(
          views.traces,
          false,
          [fresh],
          expect.any(Map)
        )
      )
      pending[0].resolve([{ ...fresh, logRef: 'stale' }])
      pending[1].reject(new Error('stale B failure'))
      expect(logs.mock.calls[0][2]?.aborted).toBe(true)
      expect(logs.mock.calls[1][2]?.aborted).toBe(true)
      await Promise.resolve()
      expect(
        screen.queryByText('Tool evidence could not be loaded.')
      ).toBeNull()
      const refreshed = { ...a, name: 'A refreshed' }
      await view.rerender({
        experiment: refreshed,
        results: refreshed.results,
        onTraceViews,
      })
      await waitFor(() => expect(pending.length).toBeGreaterThan(3))
      // Only the latest un-aborted request owns the refreshed packet.
      await waitFor(() =>
        expect(logs.mock.calls.filter(call => !call[2]?.aborted)).toHaveLength(
          1
        )
      )
      for (let index = 3; index < pending.length; index++)
        pending[index].reject(new Error('refresh unavailable'))
      await screen.findByText('Tool evidence could not be loaded.')
      expect(onTraceViews).toHaveBeenLastCalledWith(
        views.traces,
        false,
        [fresh],
        expect.any(Map)
      )
    } finally {
      lookup.mockRestore()
      summaries.mockRestore()
      logs.mockRestore()
    }
  })

  it('starts and publishes each owner’s full logs while unrelated trace summaries remain pending', async () => {
    const a = { traceID: 'a'.repeat(32), spanID: '1'.repeat(16) },
      b = { traceID: 'b'.repeat(32), spanID: '2'.repeat(16) }
    const views = traceViews(
      [a, b].map(reference => ({
        ...reference,
        name: 'execution',
        serviceName: 'runner',
        startTime: 10n,
        endTime: 100n,
        statusCodeValue: 0,
      }))
    )
    const lookup = vi
      .spyOn(telemetryAPI, 'getReferencedTraceViews')
      .mockResolvedValue(views)
    let release!: (logs: TraceLogSummary[]) => void
    const summaries = vi
      .spyOn(telemetryAPI, 'getTraceLogSummaries')
      .mockImplementation(id =>
        id === a.traceID
          ? Promise.resolve([
              {
                logRef: 'A',
                spanID: a.spanID,
                timestamp: 20n,
                eventName: 'tool.execution',
                severityNumber: 0,
                severityText: '',
                serviceName: '',
                bodyPreview: 'A',
              },
            ])
          : new Promise(resolve => {
              release = resolve
            })
      )
    const owner = views.traces[0].spans[0].spanData
    const received: LogData = {
      ...a,
      logRef: 'A',
      timestamp: 20n,
      observedTimestamp: 20n,
      eventName: 'tool.execution',
      severityNumber: 0,
      severityText: '',
      body: { kind: 'string', value: 'A full log' },
      attributes: [
        {
          key: 'gen_ai.tool.call.id',
          value: { kind: 'string', value: 'call-A' },
        },
      ],
      resource: owner.resource,
      scope: owner.scope,
      flags: 0,
      droppedAttributesCount: 0,
    }
    const logs = vi
      .spyOn(telemetryAPI, 'getSpanLogs')
      .mockResolvedValue([received])
    const onTraceViews = vi.fn()
    try {
      const results = [a, b].map((telemetry, index) => ({
        ...exampleExperiments[0].results[0],
        id: `result-${index}`,
        caseID: `case-${index}`,
        telemetry,
        resultTelemetry: undefined,
        tools: [],
        assessments: [],
      }))
      const experiment = { ...exampleExperiments[0], results }
      render(EvalResults, { experiment, results, onTraceViews })
      await waitFor(() => expect(summaries).toHaveBeenCalledTimes(2))
      await waitFor(() =>
        expect(logs).toHaveBeenCalledWith(
          a.traceID,
          a.spanID,
          expect.any(AbortSignal)
        )
      )
      await waitFor(() =>
        expect(onTraceViews).toHaveBeenLastCalledWith(
          views.traces,
          false,
          [received],
          expect.any(Map)
        )
      )
      expect(await screen.findByText('Loading tool evidence…')).toBeVisible()
      release([])
      await waitFor(() =>
        expect(screen.queryByText('Loading tool evidence…')).toBeNull()
      )
    } finally {
      release?.([])
      lookup.mockRestore()
      summaries.mockRestore()
      logs.mockRestore()
    }
  })

  it.each(['present', 'empty', 'owner-failure', 'summary-failure'])(
    'keeps Tool calls selected and enabled until discovery establishes availability: %s',
    async outcome => {
      const reference = { traceID: 'a'.repeat(32), spanID: '1'.repeat(16) }
      const views = traceViews([
        {
          ...reference,
          name: 'execution',
          serviceName: 'runner',
          startTime: 10n,
          endTime: 100n,
          statusCodeValue: 0,
        },
      ])
      const lookup = vi
        .spyOn(telemetryAPI, 'getReferencedTraceViews')
        .mockResolvedValue(views)
      let summaryResolve!: (logs: TraceLogSummary[]) => void,
        summaryReject!: (reason: Error) => void
      const summaries = vi
        .spyOn(telemetryAPI, 'getTraceLogSummaries')
        .mockImplementation(
          () =>
            new Promise((resolve, reject) => {
              summaryResolve = resolve
              summaryReject = reject
            })
        )
      let ownerResolve!: (logs: LogData[]) => void,
        ownerReject!: (reason: Error) => void
      const logs = vi.spyOn(telemetryAPI, 'getSpanLogs').mockImplementation(
        () =>
          new Promise((resolve, reject) => {
            ownerResolve = resolve
            ownerReject = reject
          })
      )
      try {
        const result = {
          ...exampleExperiments[0].results[0],
          telemetry: { traceID: reference.traceID },
          resultTelemetry: undefined,
          tools: [],
          toolsKnown: false,
          assessments: [],
        }
        const experiment = { ...exampleExperiments[0], results: [result] }
        render(EvalResults, { experiment, results: [result], viewKey: 'time' })
        await waitFor(() => expect(summaries).toHaveBeenCalledTimes(1))
        const mode = screen.getByRole('radio', { name: 'Tool calls' })
        expect(mode).not.toBeDisabled()
        expect(mode).toHaveAttribute('aria-checked', 'true')
        if (outcome === 'summary-failure')
          summaryReject(new Error('summary unavailable'))
        else {
          summaryResolve([
            {
              logRef: 'call',
              spanID: reference.spanID,
              timestamp: 20n,
              eventName: 'tool.execution',
              severityNumber: 0,
              severityText: '',
              serviceName: '',
              bodyPreview: 'call',
            },
          ])
          await waitFor(() => expect(logs).toHaveBeenCalledTimes(1))
          expect(mode).not.toBeDisabled()
          expect(mode).toHaveAttribute('aria-checked', 'true')
          if (outcome === 'owner-failure')
            ownerReject(new Error('owner unavailable'))
          else {
            const owner = views.traces[0].spans[0].spanData
            ownerResolve(
              outcome === 'empty'
                ? []
                : [
                    {
                      ...reference,
                      logRef: 'call',
                      timestamp: 20n,
                      observedTimestamp: 20n,
                      eventName: 'tool.execution',
                      severityNumber: 0,
                      severityText: '',
                      body: { kind: 'empty', value: null },
                      attributes: [
                        {
                          key: 'gen_ai.tool.call.id',
                          value: { kind: 'string', value: 'call' },
                        },
                      ],
                      resource: owner.resource,
                      scope: owner.scope,
                      flags: 0,
                      droppedAttributesCount: 0,
                    },
                  ]
            )
          }
        }
        if (outcome === 'empty') {
          await waitFor(() => expect(mode).toBeDisabled())
          expect(
            screen.getByRole('radio', { name: 'Results' })
          ).toHaveAttribute('aria-checked', 'true')
        } else if (outcome.endsWith('failure')) {
          await screen.findByText('Tool evidence could not be loaded.')
          expect(mode).not.toBeDisabled()
          expect(mode).toHaveAttribute('aria-checked', 'true')
        } else {
          await waitFor(() =>
            expect(screen.queryByText('Loading tool evidence…')).toBeNull()
          )
          expect(mode).not.toBeDisabled()
          expect(mode).toHaveAttribute('aria-checked', 'true')
        }
      } finally {
        lookup.mockRestore()
        summaries.mockRestore()
        logs.mockRestore()
      }
    }
  )

  it('publishes ready timeline summary markers before full tool logs finish', async () => {
    const reference = { traceID: 'a'.repeat(32), spanID: '1'.repeat(16) }
    const lookup = vi
      .spyOn(telemetryAPI, 'getReferencedTraceViews')
      .mockResolvedValue(
        traceViews([
          {
            ...reference,
            name: 'execution',
            serviceName: 'runner',
            startTime: 10n,
            endTime: 100n,
            statusCodeValue: 0,
          },
        ])
      )
    const summaries = vi
      .spyOn(telemetryAPI, 'getTraceLogSummaries')
      .mockResolvedValue([
        {
          logRef: 'ready-log',
          spanID: reference.spanID,
          timestamp: 20n,
          eventName: 'tool.execution',
          severityNumber: 0,
          severityText: '',
          serviceName: '',
          bodyPreview: 'ready activity',
        },
      ])
    let finish!: (logs: []) => void
    const logs = vi.spyOn(telemetryAPI, 'getSpanLogs').mockImplementation(
      () =>
        new Promise(resolve => {
          finish = resolve
        })
    )
    try {
      const result = {
        ...exampleExperiments[0].results[0],
        telemetry: reference,
        resultTelemetry: undefined,
        tools: [],
        assessments: [],
      }
      const experiment = { ...exampleExperiments[0], results: [result] }
      const view = render(EvalResults, { experiment, results: [result] })
      await waitFor(() => expect(logs).toHaveBeenCalledTimes(1))
      await waitFor(() =>
        expect(
          view.container.querySelectorAll('.timeline-marker').length
        ).toBeGreaterThan(0)
      )
      expect(screen.queryByText('Loading tool evidence…')).toBeNull()
      const header = view.container.querySelector('.pane-header')
      expect(await screen.findByText('Loading tool evidence…')).toBeVisible()
      finish([])
      await waitFor(() =>
        expect(screen.queryByText('Loading tool evidence…')).toBeNull()
      )
      expect(view.container.querySelector('.pane-header')).toBe(header)
      expect(
        view.container.querySelectorAll('.timeline-marker').length
      ).toBeGreaterThan(0)
    } finally {
      lookup.mockRestore()
      summaries.mockRestore()
      logs.mockRestore()
    }
  })

  it('never publishes another experiment’s retained trace bounds or action context', async () => {
    const first = {
      ...exampleExperiments[0],
      telemetry: { traceID: 'a'.repeat(32) },
    }
    const traces: TraceData[] = [
      { traceID: 'a'.repeat(32), unplacedSpanCount: 0, spans: [] },
    ]
    let finish!: (value: ReturnType<typeof traceViews>) => void
    const lookup = vi
      .spyOn(telemetryAPI, 'getReferencedTraceViews')
      .mockResolvedValueOnce({ traces, unavailableTraceIDs: [] })
      .mockImplementationOnce(
        () =>
          new Promise(resolve => {
            finish = resolve
          })
      )
    const onTraceViews = vi.fn()
    try {
      const view = render(EvalResults, {
        experiment: first,
        results: first.results,
        onTraceViews,
      })
      await waitFor(() =>
        expect(onTraceViews).toHaveBeenLastCalledWith(
          traces,
          false,
          [],
          expect.any(Map)
        )
      )
      const second = {
        ...first,
        id: 'different-experiment',
        name: 'Experiment B',
      }
      await view.rerender({
        experiment: second,
        results: second.results,
        onTraceViews,
      })
      await waitFor(() =>
        expect(onTraceViews).toHaveBeenLastCalledWith(
          [],
          true,
          [],
          expect.any(Map)
        )
      )
      finish(traceViews([]))
      await waitFor(() =>
        expect(onTraceViews).toHaveBeenLastCalledWith(
          [],
          false,
          [],
          expect.any(Map)
        )
      )
    } finally {
      lookup.mockRestore()
    }
  })

  it('disables missing views, skips them with arrows and retains recorded zero costs', async () => {
    const lookup = vi
      .spyOn(telemetryAPI, 'getReferencedTraceViews')
      .mockResolvedValue(traceViews([]))
    try {
      const result = {
        ...exampleExperiments[0].results[0],
        telemetry: undefined,
        resultTelemetry: undefined,
        tools: [],
        toolsKnown: false,
        assessments: [],
        investigatorCost: undefined,
        graderCost: undefined,
      }
      const experiment = {
        ...exampleExperiments[0],
        telemetry: undefined,
        results: [result],
      }
      const { rerender } = render(EvalResults, {
        experiment,
        results: experiment.results,
        viewKey: 'cost',
      })
      const results = screen.getByRole('radio', { name: 'Results' })
      const tools = screen.getByRole('radio', { name: 'Tool calls' })
      const cost = screen.getByRole('radio', { name: 'Cost · CAD est.' })
      await waitFor(() =>
        expect(results).toHaveAttribute('aria-checked', 'true')
      )
      expect(tools).toBeDisabled()
      expect(cost).toBeDisabled()
      const user = userEvent.setup()
      results.focus()
      await user.keyboard('{ArrowRight}')
      expect(results).toHaveFocus()

      const withCost = {
        ...experiment,
        results: [{ ...result, investigatorCost: 0 }],
      }
      await rerender({ experiment: withCost, results: withCost.results })
      expect(cost).toBeEnabled()
      await user.keyboard('{ArrowRight}')
      expect(cost).toHaveFocus()
      expect(cost).toHaveAttribute('aria-checked', 'true')
      await rerender({ results: [] })
      expect(cost).toBeEnabled()
      expect(cost).toHaveAttribute('aria-checked', 'true')
      await user.keyboard('{ArrowLeft}')
      expect(results).toHaveFocus()

      const knownZero = {
        ...experiment,
        results: [{ ...result, toolsKnown: true }],
      }
      await rerender({ experiment: knownZero, results: knownZero.results })
      expect(cost).toBeDisabled()
      expect(tools).toBeEnabled()
      await user.click(tools)
      expect(tools).toHaveAttribute('aria-checked', 'true')
      await rerender({ experiment, results: experiment.results })
      await waitFor(() =>
        expect(results).toHaveAttribute('aria-checked', 'true')
      )
      expect(tools).toBeDisabled()
    } finally {
      lookup.mockRestore()
    }
  })

  it('retains selected tool telemetry when filtering its case out of the table', async () => {
    const result = exampleExperiments[0].results[0]
    const tool = result.tools[0]
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
      const experiment = {
        ...exampleExperiments[0],
        telemetry: undefined,
        results: [result],
      }
      const { rerender, container } = render(EvalResults, {
        experiment,
        results: experiment.results,
        selectedRowID: `${result.id}/tool/0`,
        onResolveSelection,
      })
      const evidence = expect.objectContaining({
        id: `${result.id}/tool/0`,
        state: 'Error',
        spanError: true,
        window: { startTime: 10n, endTime: 30n },
      })
      await waitFor(() =>
        expect(onResolveSelection).toHaveBeenLastCalledWith(evidence)
      )
      await rerender({ results: [] })
      expect(container.querySelectorAll('[data-outline-id]')).toHaveLength(1)
      await waitFor(() =>
        expect(onResolveSelection).toHaveBeenLastCalledWith(evidence)
      )
    } finally {
      lookup.mockRestore()
    }
  })

  it('selects one summary view, combines totals and preserves manual disclosure', async () => {
    const result = exampleExperiments[0].results[0]
    const telemetry = { traceID: 'a'.repeat(32), spanID: 'b'.repeat(16) }
    const lookup = vi
      .spyOn(telemetryAPI, 'getReferencedTraceViews')
      .mockResolvedValue(traceViews([]))
    try {
      const experiment = {
        ...exampleExperiments[0],
        telemetry,
        results: [result],
      }
      const { container } = render(EvalResults, {
        experiment,
        results: experiment.results,
        selectedID: result.id,
      })
      const user = userEvent.setup()
      const toggles = within(
        screen.getByRole('radiogroup', { name: 'Eval view' })
      )
      const toggle = (name: string) =>
        toggles.getByRole('radio', {
          name: name === 'Cost' ? 'Cost · CAD est.' : name,
        })
      await user.click(toggle('Cost'))
      await user.click(toggle('Tool calls'))
      for (const name of ['Results', 'Tool calls', 'Cost · CAD est.'])
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
      const toolCount = result.tools.length
      expect(container.querySelectorAll('[data-tool-result-id]')).toHaveLength(
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
        result.investigatorCost!.toFixed(3),
        result.graderCost!.toFixed(3),
        (result.investigatorCost! + result.graderCost!).toFixed(3),
      ])
      expect(
        screen.getByRole('button', { name: 'Sort by Total cost' })
      ).toBeInTheDocument()
      expect(container.querySelectorAll('[data-tool-result-id]')).toHaveLength(
        0
      )
      await user.click(toggle('Tool calls'))
      expect(container.querySelectorAll('[data-tool-result-id]')).toHaveLength(
        0
      )
      await user.click(screen.getByRole('button', { name: /^Expand Result/ }))
      expect(container.querySelectorAll('[data-tool-result-id]')).toHaveLength(
        toolCount
      )
      expect(
        container.querySelector(`[data-result-id="${result.id}"]`)
      ).toHaveClass('table-row--selected')
    } finally {
      lookup.mockRestore()
    }
  })

  it.each([false, true])(
    'positions tools by stored start time, retaining late ends (experiment span: %s)',
    async withExperiment => {
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
      const result = {
        ...exampleExperiments[0].results[0],
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
            ...(withExperiment ? [timing(4, 'experiment', 0n, 40n)] : []),
          ])
        )
      try {
        render(EvalResults, {
          experiment: {
            ...exampleExperiments[0],
            results: [result],
            telemetry: withExperiment ? reference(4) : undefined,
          },
          results: [result],
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
          left: withExperiment ? '30%' : '12.5%',
          width: withExperiment ? '5%' : '12.5%',
        })
        expect(late.querySelector('.span-bar')).toHaveStyle({
          left: withExperiment ? '45%' : '50%',
          width: withExperiment ? '20%' : '50%',
        })
        expect(early.querySelector('.duration-bar__label')).toHaveStyle({
          left: withExperiment ? 'calc(35% + 16px)' : 'calc(25% + 16px)',
        })
        expect(early).toHaveAttribute(
          'title',
          expect.stringContaining(
            withExperiment ? 'Start offset 12.000 s' : 'Start offset 2.000 s'
          )
        )
      } finally {
        lookup.mockRestore()
      }
    }
  )

  it('aligns results from different traces to one origin without a experiment span', async () => {
    const epoch = 1700000000000000001n
    const results = [0, 1].map(index => ({
      ...exampleExperiments[0].results[0],
      id: `result-${index}`,
      caseID: `case-${index}`,
      telemetry: undefined,
      resultTelemetry: {
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
          results.map((result, index) => ({
            ...result.resultTelemetry,
            name: `result span ${index}`,
            serviceName: 'runner',
            startTime: epoch + BigInt(index * 20) * 1_000_000_000n,
            endTime: epoch + BigInt(index * 20 + 10) * 1_000_000_000n,
            statusCodeValue: 1,
          }))
        )
      )
    try {
      const { container } = render(EvalResults, {
        experiment: { ...exampleExperiments[0], telemetry: undefined, results },
        results,
      })
      const second = await screen.findByRole('button', {
        name: 'Span result span 1, 10.000 s',
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

  it('keeps case and missing-experiment fallback bars across spaced repetitions, including filtered results', async () => {
    const epoch = 1700000000000000001n
    const results = [0, 1].map(index => ({
      ...exampleExperiments[0].results[0],
      id: `result-${index}`,
      caseID: 'shared-case',
      repetition: index + 1,
      telemetry: undefined,
      resultTelemetry: {
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
          results.map((result, index) => ({
            ...result.resultTelemetry,
            name: `result span ${index}`,
            serviceName: 'runner',
            startTime: epoch + BigInt(index * 20) * 1_000_000_000n,
            endTime: epoch + BigInt(index * 20 + 10) * 1_000_000_000n,
            statusCodeValue: 1,
          }))
        )
      )
    try {
      const { container } = render(EvalResults, {
        experiment: {
          ...exampleExperiments[0],
          telemetry: { traceID: 'f'.repeat(32), spanID: 'f'.repeat(16) },
          results,
        },
        results: [results[0]],
      })
      const root = await screen.findByRole('button', {
        name: `${results[0].name}, 30.000 s`,
      })
      expect(root.querySelector('.span-bar')).toHaveStyle({
        left: '0%',
        width: '100%',
      })
      expect(container.querySelectorAll('[data-result-id]')).toHaveLength(1)
      expect(root).toHaveAttribute(
        'title',
        expect.stringContaining('elapsed time across available references')
      )
      const experimentBar = screen.getByRole('button', {
        name: `${exampleExperiments[0].name}, 30.000 s`,
      })
      expect(experimentBar.querySelector('.span-bar')).toHaveStyle({
        left: '0%',
        width: '100%',
      })
      expect(experimentBar).toHaveAttribute(
        'title',
        expect.stringContaining('Timing fallback')
      )
    } finally {
      lookup.mockRestore()
    }
  })

  it('shows a referenced experiment span and keeps an absent question span as an unlinked recorded duration', async () => {
    const telemetry = { traceID: 'a'.repeat(32), spanID: 'b'.repeat(16) }
    const lookup = vi
      .spyOn(telemetryAPI, 'getReferencedTraceViews')
      .mockResolvedValue(
        traceViews([
          {
            ...telemetry,
            name: 'eval experiment',
            serviceName: 'runner',
            startTime: 1700000000000000001n,
            endTime: 1700000001000000001n,
            statusCodeValue: 1,
          },
        ])
      )
    try {
      const results = [exampleExperiments[0].results[0]]
      render(EvalResults, {
        experiment: { ...exampleExperiments[0], telemetry, results },
        results,
      })
      await showOnlyTime()
      expect(
        await screen.findByRole('button', {
          name: 'Span eval experiment, 1.000 s',
        })
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
        const results = [24, 0, undefined, 1e-9].map((seconds, index) => ({
          ...exampleExperiments[0].results[0],
          id: `duration-${index}`,
          repetition: index + 1,
          seconds,
          telemetry:
            index === 0
              ? exampleExperiments[0].results[0].telemetry
              : undefined,
          tools: [],
        }))
        const { container } = render(EvalResults, {
          experiment: { ...exampleExperiments[0], results },
          results,
        })
        await showOnlyTime()
        const rows = [...container.querySelectorAll('[data-result-id]')]
        const cells = rows.map(row => row.querySelector('td')!)
        expect(cells[0]).toHaveTextContent('24.000 s')
        expect(cells[0].querySelector('.span-bar')).toHaveStyle({
          width: '100%',
        })
        for (const cell of cells.slice(1)) {
          expect(cell.querySelector('.sr-only')).toHaveTextContent(
            'No recorded duration'
          )
          expect(cell.querySelector('[aria-label]')).toBeNull()
          expect(cell.querySelector('.span-bar')).toBeNull()
        }
        expect(cells[2].querySelector('.timing-cell__unavailable')).toBeNull()
        expect(container.querySelector('.eval-results__span-bar')).toBeNull()
        expect(screen.queryByText('Partial total')).not.toBeInTheDocument()
        const caseRow = container.querySelector('[data-outline-id^="case/"]')!
        expect(
          caseRow.querySelector('.timing-cell__value')
        ).toBeEmptyDOMElement()
      } finally {
        lookup.mockRestore()
      }
    }
  )

  it('selects eval details from labels without selecting through collapse controls', async () => {
    const [first, second, third] = exampleExperiments[0].results.filter(
      item => item.caseID === 'request-latency'
    )
    const single = {
      ...first,
      id: 'single',
      caseID: 'single',
      name: 'Single case',
    }
    const results = [single, first, second, third]
    const onSelect = vi.fn()
    const { container } = render(EvalResults, {
      experiment: { ...exampleExperiments[0], results },
      results,
      selectedID: second.id,
      onSelect,
    })
    expect(container.querySelectorAll('tbody tr')).toHaveLength(7)
    const singleton = container.querySelector(
      '[data-result-id="single"]'
    )! as HTMLElement
    expect(within(singleton).getByText('Result')).toBeInTheDocument()
    expect(singleton.querySelector('.seg__arm--elbow')).not.toBeNull()
    // The Experiment root and both Case roots use the aggregate-row treatment.
    expect(container.querySelectorAll('.eval-results__case')).toHaveLength(3)
    expect(
      screen.queryByText(exampleExperiments[0].evaluation)
    ).not.toBeInTheDocument()
    const labels = [
      ...container.querySelectorAll(
        '[data-row-kind="result"] button[aria-pressed]'
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
    expect(container.querySelectorAll('[data-result-id]')).toHaveLength(1)
    expect(
      [...parent.querySelectorAll('td')].map(cell => cell.textContent)
    ).toEqual(aggregateValues)
    await userEvent.setup().keyboard('[Enter]')
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(
      container.querySelector(`[data-result-id="${second.id}"]`)
    ).toHaveClass('table-row--selected')
    expect(onSelect).not.toHaveBeenCalled()
    await userEvent
      .setup()
      .click(
        container.querySelector(
          `[data-result-id="${second.id}"] button[aria-pressed]`
        )!
      )
    expect(onSelect).toHaveBeenCalledWith(
      expect.objectContaining({ kind: 'result', result: second })
    )
  })
})
