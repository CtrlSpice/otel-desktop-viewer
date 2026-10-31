// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { fireEvent, screen, waitFor, within } from '@testing-library/svelte'
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

function log(owner: SpanData, body: string): LogData {
  return {
    logRef: `log-${owner.spanID}`,
    traceID: owner.traceID,
    spanID: owner.spanID,
    timestamp: 1n,
    observedTimestamp: 1n,
    severityText: '',
    severityNumber: 0,
    body: { kind: 'string', value: body },
    eventName: 'tool.output',
    resource: owner.resource,
    scope: owner.scope,
    flags: 0,
    attributes: [],
    droppedAttributesCount: 0,
  }
}

it.each(['root-first', 'child-first', 'child-failure'])(
  'publishes each ready log owner independently with deterministic order and exact Copy: %s',
  async order => {
    const a = span('1'.repeat(16)),
      b = span('2'.repeat(16))
    b.attributes = [
      {
        key: 'gen_ai.operation.name',
        value: { kind: 'string', value: 'execute_tool' },
      },
    ]
    const child = { ...span('3'.repeat(16)), parentSpanID: b.spanID }
    const nodes = new Map(
      [a, b, child].map(spanData => [
        referenceKey(spanData),
        { spanData, depth: 0, matched: true },
      ])
    )
    const pending = new Map<
      string,
      { resolve: (logs: LogData[]) => void; reject: (reason: Error) => void }
    >()
    vi.spyOn(telemetryAPI, 'getTraceLogSummaries').mockResolvedValue([])
    vi.spyOn(telemetryAPI, 'getSpanLogs').mockImplementation((_trace, id) =>
      id === a.spanID
        ? Promise.resolve([log(a, 'A retained log')])
        : new Promise((resolve, reject) => pending.set(id, { resolve, reject }))
    )
    const view = render(EvalRowDetail, {
      experiment,
      row: row(a),
      telemetryNodes: nodes,
    })
    await screen.findByText('A retained log')
    await view.rerender({
      componentProps: { experiment, row: row(b), telemetryNodes: nodes },
    })
    await waitFor(() => expect(pending.size).toBe(2))
    const first = order === 'child-first' ? child : b
    pending.get(first.spanID)!.resolve([log(first, `received-${first.spanID}`)])
    const ready = await screen.findByText(`received-${first.spanID}`)
    expect(
      screen
        .getByText('A retained log')
        .closest('table')
        ?.querySelector('caption')
    ).toHaveTextContent(a.spanID)
    const telemetry = [...view.container.querySelectorAll('summary')].find(
      element => element.textContent?.includes('Telemetry')
    )!
    expect(telemetry).not.toHaveTextContent(/\d+ logs?/)
    await fireEvent.click(
      within(ready.closest('table')!).getByRole('button', { name: 'Copy body' })
    )
    expect(copyText).toHaveBeenLastCalledWith(`received-${first.spanID}`)
    if (order === 'child-failure')
      pending.get(child.spanID)!.reject(new Error('unavailable child'))
    else {
      const last = first === child ? b : child
      pending.get(last.spanID)!.resolve([log(last, `received-${last.spanID}`)])
    }
    if (order === 'child-failure') {
      const retained = screen.getByText('A retained log').closest('table')!
      expect(retained.querySelector('caption')).toHaveTextContent(a.spanID)
      await fireEvent.click(
        within(retained).getByRole('button', { name: 'Copy body' })
      )
      expect(copyText).toHaveBeenLastCalledWith('A retained log')
      expect(
        screen.getByText('Linked log evidence could not be loaded.')
      ).toBeVisible()
      expect(telemetry).not.toHaveTextContent(/\d+ logs?/)
    } else {
      await waitFor(() =>
        expect(screen.queryByText('A retained log')).toBeNull()
      )
      const bodies = [...view.container.querySelectorAll('table')].flatMap(
        table =>
          table.textContent?.includes('received-') ? [table.textContent] : []
      )
      expect(bodies).toHaveLength(2)
      expect(bodies[0]).toContain(`received-${b.spanID}`)
      expect(bodies[1]).toContain(`received-${child.spanID}`)
      await waitFor(() => expect(telemetry).toHaveTextContent('2 logs'))
    }
  }
)

it.each(['case', 'result', 'no-reference', 'trace-only'])(
  'clears inapplicable prior evidence immediately despite global pending telemetry: %s',
  async kind => {
    const a = span('1'.repeat(16))
    const nodes = new Map([
      [referenceKey(a), { spanData: a, depth: 0, matched: true }],
    ])
    const lookup = vi
      .spyOn(telemetryAPI, 'getSpanLogs')
      .mockResolvedValue([log(a, 'A prior log')])
    vi.spyOn(telemetryAPI, 'getTraceLogSummaries').mockResolvedValue([])
    const view = render(EvalRowDetail, {
      experiment,
      row: row(a),
      telemetryNodes: nodes,
    })
    await screen.findByText('A prior log')
    const next = {
      ...row(a),
      id: 'B',
      name: 'B',
      kind: (kind === 'case' || kind === 'result'
        ? kind
        : 'tool') as EvalOutlineRow['kind'],
      reference: kind === 'trace-only' ? { traceID: a.traceID } : undefined,
    }
    await view.rerender({
      componentProps: {
        experiment,
        row: next,
        telemetryNodes: nodes,
        telemetryLoading: true,
      },
    })
    expect(screen.queryByText(`output-${a.spanID}`)).toBeNull()
    expect(screen.queryByText('A prior log')).toBeNull()
    expect(lookup).toHaveBeenCalledTimes(1)
  }
)

it('rejects both late multi-owner success and failure across A → B → fresh A', async () => {
  const a = span('1'.repeat(16)),
    b = span('2'.repeat(16))
  b.attributes = [
    {
      key: 'gen_ai.operation.name',
      value: { kind: 'string', value: 'execute_tool' },
    },
  ]
  const child = { ...span('3'.repeat(16)), parentSpanID: b.spanID }
  const nodes = new Map(
    [a, b, child].map(spanData => [
      referenceKey(spanData),
      { spanData, depth: 0, matched: true },
    ])
  )
  const pending: Array<{
    id: string
    resolve: (logs: LogData[]) => void
    reject: (reason: Error) => void
  }> = []
  const lookup = vi
    .spyOn(telemetryAPI, 'getSpanLogs')
    .mockImplementation(
      (_trace, id) =>
        new Promise((resolve, reject) => pending.push({ id, resolve, reject }))
    )
  const view = render(EvalRowDetail, {
    experiment,
    row: row(a),
    telemetryNodes: nodes,
  })
  await waitFor(() => expect(pending).toHaveLength(1))
  pending[0].resolve([log(a, 'A original')])
  await screen.findByText('A original')
  await view.rerender({
    componentProps: { experiment, row: row(b), telemetryNodes: nodes },
  })
  await waitFor(() => expect(pending).toHaveLength(3))
  const fresh = { ...row(a), name: 'A refreshed' }
  await view.rerender({
    componentProps: { experiment, row: fresh, telemetryNodes: new Map(nodes) },
  })
  await waitFor(() => expect(pending).toHaveLength(4))
  pending[3].resolve([log(a, 'A fresh')])
  await screen.findByText('A fresh')
  pending[1].resolve([log(b, 'B stale')])
  pending[2].reject(new Error('stale child failure'))
  await Promise.resolve()
  await Promise.resolve()
  expect(lookup.mock.calls[1][2]?.aborted).toBe(true)
  expect(lookup.mock.calls[2][2]?.aborted).toBe(true)
  expect(screen.queryByText('B stale')).toBeNull()
  expect(
    screen.queryByText('Linked log evidence could not be loaded.')
  ).toBeNull()
  await fireEvent.click(screen.getByRole('button', { name: 'Copy body' }))
  expect(copyText).toHaveBeenLastCalledWith('A fresh')
})

it('publishes ready B span output without waiting for logs, retaining only identified A logs and their Copy target', async () => {
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
    .mockResolvedValueOnce([log(first, 'A log body')])
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
  await screen.findByText(`output-${first.spanID}`)
  const field = await screen.findByText('A log body')
  await waitFor(() => expect(loader).toHaveBeenCalledTimes(1))
  const heading = view.container.querySelector(
    'summary.detail-bubble__heading'
  )!
  await view.rerender({
    componentProps: { experiment, row: row(second), telemetryNodes: nodes },
  })
  await waitFor(() => expect(loader).toHaveBeenCalledTimes(2))
  expect(heading).toHaveTextContent(second.name)
  expect(screen.getByText(`case-${second.spanID}`)).toBeVisible()
  expect(field.closest('table')?.querySelector('caption')).toHaveTextContent(
    first.name
  )
  expect(screen.getByText('A log body')).toBe(field)
  expect(screen.queryByText(`output-${first.spanID}`)).toBeNull()
  expect(screen.getByText(`output-${second.spanID}`)).toBeVisible()
  expect(screen.queryByText('Loading linked log evidence…')).toBeNull()
  expect(await screen.findByText('Loading linked log evidence…')).toBeVisible()
  expect(heading).toHaveTextContent(second.name)
  await fireEvent.click(screen.getByRole('button', { name: 'Copy output' }))
  expect(copyText).toHaveBeenLastCalledWith(`output-${second.spanID}`)
  await fireEvent.click(screen.getByRole('button', { name: 'Copy body' }))
  expect(copyText).toHaveBeenLastCalledWith('A log body')
  finish([])
  await screen.findByText(`output-${second.spanID}`)
  expect(view.container.querySelector('summary.detail-bubble__heading')).toBe(
    heading
  )
  expect(heading).toHaveTextContent(second.name)
  expect(screen.getByText(`case-${second.spanID}`)).toBeVisible()
  expect(screen.queryByText(`case-${first.spanID}`)).toBeNull()
  expect(screen.queryByText(`output-${first.spanID}`)).toBeNull()
  await waitFor(() => expect(screen.queryByText('A log body')).toBeNull())
  await fireEvent.click(screen.getByRole('button', { name: 'Copy output' }))
  expect(copyText).toHaveBeenLastCalledWith(`output-${second.spanID}`)
})

it.each(['success', 'failure'])(
  'updates Cost immediately without waiting for unrelated evidence %s',
  async outcome => {
    const first = span('1'.repeat(16))
    const second = span('2'.repeat(16))
    const nodes = new Map(
      [first, second].map(spanData => [
        referenceKey(spanData),
        { spanData, depth: 0, matched: true },
      ])
    )
    const firstRow = {
      ...row(first),
      estimate: { total: 1, incomplete: false },
    }
    const secondRow = {
      ...row(second),
      estimate: { total: 2, incomplete: false },
    }
    let finish!: () => void
    const loader = vi
      .spyOn(telemetryAPI, 'getSpanLogs')
      .mockResolvedValueOnce([])
      .mockImplementationOnce(
        () =>
          new Promise((resolve, reject) => {
            finish = () =>
              outcome === 'success'
                ? resolve([])
                : reject(new Error('unavailable'))
          })
      )
    const view = render(EvalRowDetail, {
      experiment,
      row: firstRow,
      telemetryNodes: nodes,
      mode: 'cost',
    })
    await waitFor(() => expect(loader).toHaveBeenCalledTimes(1))
    const accounting = screen.getByRole('table', {
      name: `${first.name} accounting`,
    })
    expect(accounting).toHaveTextContent('C$1.000')
    await view.rerender({
      componentProps: {
        experiment,
        row: secondRow,
        telemetryNodes: nodes,
        mode: 'cost',
      },
    })
    await waitFor(() => expect(loader).toHaveBeenCalledTimes(2))
    expect(screen.queryByText('Loading linked log evidence…')).toBeNull()
    expect(
      screen.getByRole('table', { name: `${second.name} accounting` })
    ).toBe(accounting)
    expect(accounting).toHaveTextContent('C$2.000')
    finish()
    const replacement = await screen.findByRole('table', {
      name: `${second.name} accounting`,
    })
    expect(replacement).toHaveTextContent('C$2.000')
    expect(screen.queryByText('Loading linked log evidence…')).toBeNull()
    if (outcome === 'failure') {
      expect(
        screen.getByText('Linked log evidence could not be loaded.')
      ).not.toBeVisible()
    }
  }
)

it('retains identified span content only while the replacement span itself is unavailable', async () => {
  const first = span('1'.repeat(16)),
    second = span('2'.repeat(16))
  const nodes = new Map(
    [first, second].map(spanData => [
      referenceKey(spanData),
      { spanData, depth: 0, matched: true },
    ])
  )
  const pending: Array<(logs: LogData[]) => void> = []
  vi.spyOn(telemetryAPI, 'getSpanLogs').mockImplementation(
    () => new Promise(resolve => pending.push(resolve))
  )
  const view = render(EvalRowDetail, {
    experiment,
    row: row(first),
    telemetryNodes: nodes,
  })
  const field = await screen.findByText(`output-${first.spanID}`)
  await view.rerender({
    componentProps: {
      experiment,
      row: row(second),
      telemetryNodes: new Map(),
      telemetryLoading: true,
    },
  })
  expect(screen.getByText(`output-${first.spanID}`)).toBe(field)
  expect(field.closest('table')?.querySelector('caption')).toHaveTextContent(
    first.name
  )
  await view.rerender({
    componentProps: {
      experiment,
      row: row(second),
      telemetryNodes: nodes,
      telemetryLoading: false,
    },
  })
  expect(screen.getByText(`output-${second.spanID}`)).toBeVisible()
  expect(screen.queryByText(`output-${first.spanID}`)).toBeNull()
  expect(pending).toHaveLength(2)
  pending[0]([log(first, 'Stale A log')])
  pending[1]([])
  await waitFor(() =>
    expect(screen.queryByText('Loading linked log evidence…')).toBeNull()
  )
  expect(screen.queryByText('Stale A log')).toBeNull()
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
  expect(heading).toHaveTextContent(refreshed.name)
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

it('keeps identified log evidence and Copy when all replacement owners fail, until a successful replacement', async () => {
  const a = span('1'.repeat(16)),
    b = span('2'.repeat(16))
  const nodes = new Map(
    [a, b].map(spanData => [
      referenceKey(spanData),
      { spanData, depth: 0, matched: true },
    ])
  )
  let fail!: (error: Error) => void
  vi.spyOn(telemetryAPI, 'getTraceLogSummaries').mockResolvedValue([])
  const lookup = vi
    .spyOn(telemetryAPI, 'getSpanLogs')
    .mockResolvedValueOnce([log(a, 'retained A log')])
    .mockReturnValueOnce(
      new Promise((_, reject) => {
        fail = reject
      })
    )
    .mockResolvedValueOnce([])
  const view = render(EvalRowDetail, {
    experiment,
    row: row(a),
    telemetryNodes: nodes,
  })
  const value = await screen.findByText('retained A log')
  const table = value.closest('table')!
  await view.rerender({
    componentProps: { experiment, row: row(b), telemetryNodes: nodes },
  })
  await waitFor(() => expect(lookup).toHaveBeenCalledTimes(2))
  expect(table.isConnected).toBe(true)
  fail(new Error('replacement logs failed'))
  await screen.findByText('Linked log evidence could not be loaded.')
  expect(table.isConnected).toBe(true)
  expect(table.querySelector('caption')).toHaveTextContent(a.spanID)
  expect(screen.getByText(`output-${b.spanID}`)).toBeVisible()
  await fireEvent.click(
    within(table).getByRole('button', { name: 'Copy body' })
  )
  expect(copyText).toHaveBeenLastCalledWith('retained A log')
  const telemetry = [...view.container.querySelectorAll('summary')].find(
    element => element.textContent?.includes('Telemetry')
  )!
  expect(telemetry).not.toHaveTextContent(/\d+ logs?/)
  await view.rerender({
    componentProps: { experiment, row: row(b), telemetryNodes: new Map(nodes) },
  })
  await waitFor(() => expect(lookup).toHaveBeenCalledTimes(3))
  await waitFor(() => expect(screen.queryByText('retained A log')).toBeNull())
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
