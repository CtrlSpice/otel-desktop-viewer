// @vitest-environment jsdom
import { beforeEach, afterEach, it, expect, vi } from 'vitest'
import {
  cleanup,
  fireEvent,
  screen,
  waitFor,
  act,
} from '@testing-library/svelte'
import { EditorView } from '@codemirror/view'
import { tick } from 'svelte'
import LogsPage from './LogsPage.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import { navigateToItem, navigateCurrentRoute } from '@/route'

const api = vi.hoisted(() => ({
  searchLogPage: vi.fn(),
  getLog: vi.fn(),
  getStats: vi.fn(),
  getLogAttributeDefinitions: vi.fn(),
  getLogErrorNavigation: vi.fn(),
}))
vi.mock('@/services/telemetry-service', async original => {
  const actual = await original<typeof import('@/services/telemetry-service')>()
  return { ...actual, telemetryAPI: { ...actual.telemetryAPI, ...api } }
})
const summary = (id = 'A') => ({
  logRef: id,
  timestamp: 1700000000000000000n,
  severityText: 'ERROR',
  severityNumber: 17,
  serviceName: 'service-' + id,
  bodyPreview: 'row-' + id,
})
const result = (id = 'A') => ({ logs: [summary(id)], total: 300 })
const detail = (id: string) => ({
  ...summary(id),
  observedTimestamp: 1700000000000000000n,
  traceID: null,
  spanID: null,
  body: { kind: 'string', value: 'detail-' + id },
  resource: { attributes: [], droppedAttributesCount: 0 },
  scope: { name: '', version: '', attributes: [], droppedAttributesCount: 0 },
  attributes: [],
  droppedAttributesCount: 0,
  flags: 0,
  eventName: '',
})
function deferred() {
  let resolve!: (value: unknown) => void
  let reject!: (reason: unknown) => void
  const promise = new Promise((yes, no) => {
    resolve = yes
    reject = no
  })
  return { promise, resolve, reject }
}
beforeEach(() => {
  Object.values(api).forEach(fn => fn.mockReset())
  api.searchLogPage.mockResolvedValue(result())
  api.getLog.mockImplementation((id: string) => Promise.resolve(detail(id)))
  api.getStats.mockResolvedValue({ logs: { logCount: 300 } })
  api.getLogAttributeDefinitions.mockResolvedValue([])
  api.getLogErrorNavigation.mockResolvedValue({
    count: 1,
    index: 0,
    previous: null,
    next: null,
  })
  setTestUrl('/logs/A')
})
afterEach(() => {
  cleanup()
  vi.useRealTimers()
  vi.unstubAllGlobals()
})
async function mount() {
  const view = renderWithContexts(LogsPage)
  await waitFor(() =>
    expect(document.querySelector('[data-log-ref="A"]')).not.toBeNull()
  )
  await waitFor(() =>
    expect(screen.getByRole('table', { name: 'Log fields' })).toHaveTextContent(
      'detail-A'
    )
  )
  return view
}
async function query(text: string) {
  const element = screen.getByRole('textbox', { name: 'Search logs' })
  const editor = EditorView.findFromDOM(element)!
  await act(() =>
    editor.dispatch({
      changes: { from: 0, to: editor.state.doc.length, insert: text },
    })
  )
  await fireEvent.keyDown(element, { key: 'Enter', code: 'Enter' })
  await tick()
}

it('does not report an unknown initial count as zero', async () => {
  api.searchLogPage.mockReturnValue(deferred().promise)
  renderWithContexts(LogsPage)
  await waitFor(() => expect(api.searchLogPage).toHaveBeenCalled())
  expect(
    screen.getByRole('status', { name: 'Log count' })
  ).not.toHaveTextContent('0 logs')
})

it('failed replacement retains displayed query ownership for paging and error navigation', async () => {
  await mount()
  const pending = deferred()
  api.searchLogPage.mockReturnValueOnce(pending.promise)
  await query('body = "B"')
  await waitFor(() => expect(api.searchLogPage).toHaveBeenCalledTimes(2))
  const predicateB = api.searchLogPage.mock.calls[1][4]
  expect(predicateB).toBeTruthy()
  expect(document.querySelector('[data-log-ref="A"]')).not.toBeNull()
  expect(screen.getByRole('button', { name: 'Next page' })).not.toBeDisabled()
  pending.reject(new Error('B failed'))
  await waitFor(() =>
    expect(screen.getByText('Error: B failed')).toBeInTheDocument()
  )
  expect(document.querySelector('[data-log-ref="A"]')).not.toBeNull()
  expect(api.getLogErrorNavigation.mock.calls.at(-1)?.[3]).toBeUndefined()
  await fireEvent.click(screen.getByRole('button', { name: 'Next page' }))
  await waitFor(() => expect(api.searchLogPage).toHaveBeenCalledTimes(3))
  expect(api.searchLogPage.mock.calls[2][3]).toBe(100)
  expect(api.searchLogPage.mock.calls[2][4]).toBeUndefined()
})

for (const staleFailure of [false, true]) {
  for (const replacementFailure of [false, true]) {
    it(`page before replacement: obsolete ${staleFailure ? 'failure' : 'success'}, replacement ${replacementFailure ? 'failure' : 'success'}`, async () => {
      const view = await mount()
      const page = deferred(),
        replacement = deferred()
      api.searchLogPage
        .mockReturnValueOnce(page.promise)
        .mockReturnValueOnce(replacement.promise)
      await fireEvent.click(screen.getByRole('button', { name: 'Next page' }))
      await waitFor(() => expect(api.searchLogPage).toHaveBeenCalledTimes(2))
      await query('body = "B"')
      await waitFor(() => expect(api.searchLogPage).toHaveBeenCalledTimes(3))
      expect(api.searchLogPage.mock.calls[1].at(-1).aborted).toBe(false)
      expect(api.searchLogPage.mock.calls[2][3]).toBe(0)
      if (replacementFailure)
        replacement.reject(new Error('replacement failed'))
      else replacement.resolve(result('B'))
      await waitFor(() =>
        replacementFailure
          ? expect(
              screen.getByText('Error: replacement failed')
            ).toBeInTheDocument()
          : expect(document.querySelector('[data-log-ref="B"]')).not.toBeNull()
      )
      if (staleFailure) page.reject(new Error('obsolete failed'))
      else page.resolve(result('obsolete'))
      await act(async () => {
        await Promise.resolve()
        await tick()
      })
      expect(document.querySelector('[data-log-ref="obsolete"]') !== null).toBe(
        replacementFailure && !staleFailure
      )
      if (!replacementFailure)
        expect(screen.queryByText('Error: obsolete failed')).toBeNull()
      await view.unmount()
      expect(api.searchLogPage.mock.calls[2].at(-1).aborted).toBe(true)
    })
  }
}

for (const failed of [false, true]) {
  it(`replacement before page starts independently; replacement failure=${failed}`, async () => {
    await mount()
    const pending = deferred()
    api.searchLogPage.mockReturnValueOnce(pending.promise)
    await query('body = "B"')
    await waitFor(() => expect(api.searchLogPage).toHaveBeenCalledTimes(2))
    const predicateB = api.searchLogPage.mock.calls[1][4]
    expect(screen.getByRole('button', { name: 'Next page' })).not.toBeDisabled()
    await fireEvent.click(screen.getByRole('button', { name: 'Next page' }))
    await waitFor(() => expect(api.searchLogPage).toHaveBeenCalledTimes(3))
    expect(api.searchLogPage.mock.calls[2][4]).toBeUndefined()
    expect(api.searchLogPage.mock.calls[1].at(-1).aborted).toBe(false)
    if (failed) pending.reject(new Error('B failed'))
    else pending.resolve(result('B'))
    await waitFor(() =>
      expect(
        screen.getByRole('button', { name: 'Next page' })
      ).not.toBeDisabled()
    )
    expect(
      document.querySelector(`[data-log-ref="${failed ? 'A' : 'B'}"]`)
    ).not.toBeNull()
    await fireEvent.click(screen.getByRole('button', { name: 'Next page' }))
    await waitFor(() => expect(api.searchLogPage).toHaveBeenCalledTimes(4))
    expect(api.searchLogPage.mock.calls[3][4]).toEqual(
      failed ? undefined : predicateB
    )
  })
}

it('starts and publishes list/detail independently; shared 200ms notice resets on replacement', async () => {
  await mount()
  const list1 = deferred(),
    list2 = deferred(),
    record1 = deferred(),
    record2 = deferred()
  api.searchLogPage
    .mockReturnValueOnce(list1.promise)
    .mockReturnValueOnce(list2.promise)
  api.getLog
    .mockReturnValueOnce(record1.promise)
    .mockReturnValueOnce(record2.promise)
  vi.useFakeTimers()
  await fireEvent.click(screen.getByRole('button', { name: /refresh/i }))
  await tick()
  expect(api.searchLogPage).toHaveBeenCalledTimes(2)
  expect(api.getLog).toHaveBeenCalledTimes(2)
  await act(() => vi.advanceTimersByTimeAsync(199))
  expect(screen.queryByText('Loading selected log…')).toBeNull()
  await act(() => vi.advanceTimersByTimeAsync(1))
  expect(screen.getByText('Loading selected log…')).toBeInTheDocument()
  await fireEvent.click(screen.getByRole('button', { name: /refresh/i }))
  await tick()
  expect(api.searchLogPage).toHaveBeenCalledTimes(3)
  expect(api.getLog).toHaveBeenCalledTimes(3)
  expect(screen.queryByText('Loading selected log…')).toBeNull()
  list2.resolve(result('fresh-list'))
  await act(async () => {
    await Promise.resolve()
    await tick()
  })
  expect(document.querySelector('[data-log-ref="fresh-list"]')).not.toBeNull()
  expect(screen.getByRole('table', { name: 'Log fields' })).toHaveTextContent(
    'detail-A'
  )
  await act(() => vi.advanceTimersByTimeAsync(199))
  expect(screen.queryByText('Loading selected log…')).toBeNull()
  record2.resolve(detail('fresh-detail'))
  await act(async () => {
    await Promise.resolve()
    await tick()
  })
  expect(screen.getByRole('table', { name: 'Log fields' })).toHaveTextContent(
    'detail-fresh-detail'
  )
  await act(() => vi.advanceTimersByTimeAsync(300))
  expect(screen.queryByText('Loading selected log…')).toBeNull()
})

for (const staleFailure of [false, true]) {
  it(`A-B-A rejects obsolete detail ${staleFailure ? 'failure' : 'success'}; retained displayed A owns full Copy/Download`, async () => {
    const view = await mount()
    const b = deferred(),
      a = deferred()
    api.getLog.mockReturnValueOnce(b.promise).mockReturnValueOnce(a.promise)
    const fetch = vi.fn<(url: string) => Promise<Response>>(
      () => new Promise(() => {})
    )
    vi.stubGlobal('fetch', fetch)
    await act(() => navigateToItem('logs', 'B'))
    await waitFor(() => expect(api.getLog).toHaveBeenCalledTimes(2))
    expect(screen.getByRole('table', { name: 'Log fields' })).toHaveTextContent(
      'detail-A'
    )
    await fireEvent.click(screen.getByRole('button', { name: 'Copy log' }))
    await fireEvent.click(screen.getByRole('button', { name: 'Download log' }))
    expect(fetch.mock.calls.map(call => call[0])).toEqual([
      '/export/logs/A',
      '/export/logs/A',
    ])
    await act(() => navigateToItem('logs', 'A'))
    await waitFor(() => expect(api.getLog).toHaveBeenCalledTimes(3))
    expect(api.getLog.mock.calls[1][1].aborted).toBe(true)
    a.resolve(detail('A-fresh'))
    await waitFor(() =>
      expect(
        screen.getByRole('table', { name: 'Log fields' })
      ).toHaveTextContent('detail-A-fresh')
    )
    if (staleFailure) b.reject(new Error('stale-B-failure'))
    else b.resolve(detail('B-obsolete'))
    await act(async () => {
      await Promise.resolve()
      await tick()
    })
    expect(screen.getByRole('table', { name: 'Log fields' })).toHaveTextContent(
      'detail-A-fresh'
    )
    expect(screen.queryByText('Error: stale-B-failure')).toBeNull()
    await view.unmount()
    expect(api.getLog.mock.calls[2][1].aborted).toBe(true)
  })
}

it('failed detail replacement preserves mounted fields and full-action owner', async () => {
  await mount()
  const b = deferred()
  api.getLog.mockReturnValueOnce(b.promise)
  await act(() => navigateToItem('logs', 'B'))
  await waitFor(() => expect(api.getLog).toHaveBeenCalledTimes(2))
  const table = screen.getByRole('table', { name: 'Log fields' })
  b.reject(new Error('B detail failed'))
  await waitFor(() =>
    expect(screen.getByText('Error: B detail failed')).toBeInTheDocument()
  )
  expect(screen.getByRole('table', { name: 'Log fields' })).toBe(table)
  expect(table).toHaveTextContent('detail-A')
  const fetch = vi.fn<(url: string) => Promise<Response>>(
    () => new Promise(() => {})
  )
  vi.stubGlobal('fetch', fetch)
  await fireEvent.click(screen.getByRole('button', { name: 'Copy log' }))
  await fireEvent.click(screen.getByRole('button', { name: 'Download log' }))
  expect(fetch.mock.calls.map(call => call[0])).toEqual([
    '/export/logs/A',
    '/export/logs/A',
  ])
})

it('failed time replacement keeps the displayed unbounded window for paging', async () => {
  await mount()
  const replacement = deferred()
  api.searchLogPage.mockReturnValueOnce(replacement.promise)
  await act(() =>
    navigateCurrentRoute(
      { start: '1700000000000', end: '1700000010000' },
      'replace'
    )
  )
  await waitFor(() => expect(api.searchLogPage).toHaveBeenCalledTimes(2))
  expect(api.searchLogPage.mock.calls[1].slice(0, 2)).toEqual([
    1700000000000000000n,
    1700000010000000000n,
  ])
  replacement.reject(new Error('time failed'))
  await waitFor(() =>
    expect(screen.getByText('Error: time failed')).toBeInTheDocument()
  )
  expect(document.querySelector('[data-log-ref="A"]')).not.toBeNull()
  await fireEvent.click(screen.getByRole('button', { name: 'Next page' }))
  await waitFor(() => expect(api.searchLogPage).toHaveBeenCalledTimes(3))
  expect(api.searchLogPage.mock.calls[2].slice(0, 2)).toEqual(
    api.searchLogPage.mock.calls[0].slice(0, 2)
  )
  expect(api.searchLogPage.mock.calls[2][3]).toBe(100)
})

for (const staleFailure of [false, true]) {
  it(`list A-B-A rejects stale ${staleFailure ? 'failure' : 'success'} after newest A publishes`, async () => {
    await mount()
    const b = deferred(),
      a = deferred()
    api.searchLogPage
      .mockReturnValueOnce(b.promise)
      .mockReturnValueOnce(a.promise)
    await query('body = "B"')
    await waitFor(() => expect(api.searchLogPage).toHaveBeenCalledTimes(2))
    await query('')
    await waitFor(() => expect(api.searchLogPage).toHaveBeenCalledTimes(3))
    expect(api.searchLogPage.mock.calls[1].at(-1).aborted).toBe(true)
    a.resolve(result('A-fresh'))
    await waitFor(() =>
      expect(document.querySelector('[data-log-ref="A-fresh"]')).not.toBeNull()
    )
    if (staleFailure) b.reject(new Error('obsolete B failed'))
    else b.resolve(result('B-obsolete'))
    await act(async () => {
      await Promise.resolve()
      await tick()
    })
    expect(document.querySelector('[data-log-ref="A-fresh"]')).not.toBeNull()
    expect(document.querySelector('[data-log-ref="B-obsolete"]')).toBeNull()
    expect(screen.queryByText('Error: obsolete B failed')).toBeNull()
  })
}

async function flush() {
  await act(async () => {
    await Promise.resolve()
    await tick()
  })
}

it('keeps native error navigation usable and owned by displayed rows while replacement is pending or failed', async () => {
  api.getLogErrorNavigation.mockResolvedValue({
    count: 3,
    index: 1,
    previous: null,
    next: { logRef: 'A-error-2', offset: 150 },
  })
  await mount()
  await waitFor(() =>
    expect(
      screen.getByRole('button', { name: 'Next error' })
    ).not.toBeDisabled()
  )
  const replacement = deferred(),
    paging = deferred()
  api.searchLogPage
    .mockReturnValueOnce(replacement.promise)
    .mockReturnValueOnce(paging.promise)
  await query('body = "B"')
  expect(screen.getByRole('button', { name: 'Next error' })).not.toBeDisabled()
  await fireEvent.click(screen.getByRole('button', { name: 'Next error' }))
  expect(api.searchLogPage.mock.calls[2].slice(2, 7)).toEqual([
    100,
    100,
    undefined,
    undefined,
    { field: 'severity', direction: 'desc' },
  ])
  await waitFor(() =>
    expect(api.getLogErrorNavigation.mock.calls.at(-1)?.[2]).toBe('A-error-2')
  )
  expect(api.getLogErrorNavigation.mock.calls.at(-1)?.[3]).toBeUndefined()
  replacement.reject(new Error('replacement failed'))
  await flush()
  expect(api.searchLogPage.mock.calls[2].at(-1).aborted).toBe(false)
  paging.resolve(result('A-error-2'))
  await flush()
  expect(document.querySelector('[data-log-ref="A-error-2"]')).not.toBeNull()
})

it('keeps displayed sort and page request sort after a failed sort replacement', async () => {
  await mount()
  const replacement = deferred()
  api.searchLogPage.mockReturnValueOnce(replacement.promise)
  const sort = screen.getByRole('button', { name: 'Sort by severity' })
  await fireEvent.click(sort)
  expect(api.searchLogPage.mock.calls[1][6]).toEqual({
    field: 'severity',
    direction: 'asc',
  })
  expect(sort).toHaveTextContent('↓')
  replacement.reject(new Error('sort failed'))
  await flush()
  expect(sort).toHaveTextContent('↓')
  await fireEvent.click(screen.getByRole('button', { name: 'Next page' }))
  expect(api.searchLogPage.mock.calls[2][6]).toEqual({
    field: 'severity',
    direction: 'desc',
  })
})

for (const replacementFirst of [false, true]) {
  for (const replacementSettlesFirst of [false, true]) {
    for (const pageFails of [false, true]) {
      for (const replacementFails of [false, true]) {
        it(`independent requests: replacement starts first=${replacementFirst}, settles first=${replacementSettlesFirst}, page fails=${pageFails}, replacement fails=${replacementFails}`, async () => {
          await mount()
          const paging = deferred()
          const replacement = deferred()
          const pageIndex = replacementFirst ? 2 : 1
          const replacementIndex = replacementFirst ? 1 : 2
          api.searchLogPage
            .mockReturnValueOnce(
              replacementFirst ? replacement.promise : paging.promise
            )
            .mockReturnValueOnce(
              replacementFirst ? paging.promise : replacement.promise
            )
          const startPage = () =>
            fireEvent.click(screen.getByRole('button', { name: 'Next page' }))
          if (replacementFirst) {
            await query('body = "B"')
            await startPage()
          } else {
            await startPage()
            await query('body = "B"')
          }
          expect(api.searchLogPage).toHaveBeenCalledTimes(3)
          expect(api.searchLogPage.mock.calls[pageIndex].slice(0, 8)).toEqual([
            null,
            null,
            100,
            100,
            undefined,
            undefined,
            { field: 'severity', direction: 'desc' },
            [],
          ])
          const pageSignal = api.searchLogPage.mock.calls[pageIndex].at(-1)
          const replacementSignal =
            api.searchLogPage.mock.calls[replacementIndex].at(-1)
          expect(pageSignal.aborted).toBe(false)
          expect(replacementSignal.aborted).toBe(false)
          const settlePage = async () => {
            if (pageFails) paging.reject(new Error('page failed'))
            else paging.resolve(result('A-page-2'))
            await flush()
          }
          const settleReplacement = async () => {
            if (replacementFails)
              replacement.reject(new Error('replacement failed'))
            else replacement.resolve({ logs: [summary('B')], total: 42 })
            await flush()
          }
          if (replacementSettlesFirst) {
            await settleReplacement()
            expect(pageSignal.aborted).toBe(!replacementFails)
            await settlePage()
          } else {
            await settlePage()
            expect(
              document.querySelector(
                `[data-log-ref="${pageFails ? 'A' : 'A-page-2'}"]`
              )
            ).not.toBeNull()
            expect(replacementSignal.aborted).toBe(false)
            await settleReplacement()
          }
          const owner = replacementFails ? (pageFails ? 'A' : 'A-page-2') : 'B'
          expect(
            document.querySelector(`[data-log-ref="${owner}"]`)
          ).not.toBeNull()
          expect(
            screen.getByRole('status', { name: 'Log count' })
          ).toHaveTextContent(replacementFails ? '300 logs' : '42 logs')
          expect(
            screen.getByRole('button', {
              name: `Page ${replacementFails && !pageFails ? 2 : 1}`,
            })
          ).toHaveAttribute('aria-current', 'page')
          if (!replacementFails)
            expect(screen.queryByText('Error: page failed')).toBeNull()
        })
      }
    }
  }
}

it('keeps page failure retry and size changes attached to displayed rows during a replacement', async () => {
  await mount()
  const replacement = deferred(),
    first = deferred(),
    retry = deferred(),
    size = deferred()
  api.searchLogPage
    .mockReturnValueOnce(replacement.promise)
    .mockReturnValueOnce(first.promise)
    .mockReturnValueOnce(retry.promise)
    .mockReturnValueOnce(size.promise)
  await query('body = "B"')
  await fireEvent.click(screen.getByRole('button', { name: 'Next page' }))
  first.reject(new Error('page failed'))
  await waitFor(() =>
    expect(screen.getByText('Error: page failed')).toBeInTheDocument()
  )
  await fireEvent.click(screen.getByRole('button', { name: 'Next page' }))
  retry.resolve(result('A-2'))
  await flush()
  expect(document.querySelector('[data-log-ref="A-2"]')).not.toBeNull()
  await fireEvent.change(
    screen.getByRole('combobox', { name: 'Logs per page' }),
    { target: { value: '25' } }
  )
  expect(api.searchLogPage.mock.calls[4].slice(2, 5)).toEqual([
    25,
    0,
    undefined,
  ])
  expect(api.searchLogPage.mock.calls[1].at(-1).aborted).toBe(false)
  size.resolve(result('A-size'))
  await flush()
  expect(screen.getByRole('combobox', { name: 'Logs per page' })).toHaveValue(
    '25'
  )
  replacement.reject(new Error('replacement failed'))
  await flush()
  expect(document.querySelector('[data-log-ref="A-size"]')).not.toBeNull()
  expect(screen.getByRole('combobox', { name: 'Logs per page' })).toHaveValue(
    '25'
  )
})

it('keeps the displayed page size selected while a size request is pending or fails', async () => {
  await mount()
  const size = deferred()
  api.searchLogPage.mockReturnValueOnce(size.promise)
  const select = screen.getByRole('combobox', { name: 'Logs per page' })
  await fireEvent.change(select, { target: { value: '25' } })
  expect(select).toHaveValue('100')
  size.reject(new Error('size failed'))
  await flush()
  expect(select).toHaveValue('100')
  expect(screen.getByRole('button', { name: 'Page 1' })).toHaveAttribute(
    'aria-current',
    'page'
  )
  expect(screen.getByRole('status', { name: 'Log count' })).toHaveTextContent(
    '300 logs'
  )
})

it('publishes a successful empty replacement and ignores a late displayed-page response', async () => {
  await mount()
  const paging = deferred(),
    replacement = deferred()
  api.searchLogPage
    .mockReturnValueOnce(paging.promise)
    .mockReturnValueOnce(replacement.promise)
  await fireEvent.click(screen.getByRole('button', { name: 'Next page' }))
  await query('body = "B"')
  replacement.resolve({ logs: [], total: 0 })
  await flush()
  expect(screen.getByRole('status', { name: 'Log count' })).toHaveTextContent(
    '0 logs'
  )
  expect(document.querySelector('[data-log-ref="A"]')).toBeNull()
  expect(screen.getByRole('button', { name: 'Next page' })).toBeDisabled()
  paging.resolve(result('late-page'))
  await flush()
  expect(document.querySelector('[data-log-ref="late-page"]')).toBeNull()
})

it('uses the shared 200ms feedback for independent page requests and cancels it on unmount', async () => {
  const view = await mount()
  api.searchLogPage.mockReturnValue(deferred().promise)
  vi.useFakeTimers()
  await query('body = "B"')
  await act(() => vi.advanceTimersByTimeAsync(200))
  expect(screen.getByText('Loading logs…')).toBeInTheDocument()
  await fireEvent.click(screen.getByRole('button', { name: 'Next page' }))
  expect(api.searchLogPage).toHaveBeenCalledTimes(3)
  expect(screen.queryByText('Loading logs…')).toBeNull()
  await act(() => vi.advanceTimersByTimeAsync(199))
  expect(screen.queryByText('Loading logs…')).toBeNull()
  await act(() => vi.advanceTimersByTimeAsync(1))
  expect(screen.getByText('Loading logs…')).toBeInTheDocument()
  await view.unmount()
  await act(() => vi.advanceTimersByTimeAsync(300))
  expect(screen.queryByText('Loading logs…')).toBeNull()
})

for (const fails of [false, true]) {
  it(`page A-B-A ignores late success/failure=${fails} from superseded page requests`, async () => {
    await mount()
    const a = deferred(),
      b = deferred(),
      fresh = deferred()
    api.searchLogPage
      .mockReturnValueOnce(a.promise)
      .mockReturnValueOnce(b.promise)
      .mockReturnValueOnce(fresh.promise)
    await fireEvent.click(screen.getByRole('button', { name: 'Page 2' }))
    await fireEvent.click(screen.getByRole('button', { name: 'Page 3' }))
    await fireEvent.click(screen.getByRole('button', { name: 'Page 2' }))
    expect(api.searchLogPage.mock.calls.slice(1).map(call => call[3])).toEqual([
      100, 200, 100,
    ])
    expect(api.searchLogPage.mock.calls[1].at(-1).aborted).toBe(true)
    expect(api.searchLogPage.mock.calls[2].at(-1).aborted).toBe(true)
    fresh.resolve(result('fresh-page-2'))
    await flush()
    if (fails) {
      a.reject(new Error('old A failed'))
      b.reject(new Error('old B failed'))
    } else {
      a.resolve(result('old-A'))
      b.resolve(result('old-B'))
    }
    await flush()
    expect(
      document.querySelector('[data-log-ref="fresh-page-2"]')
    ).not.toBeNull()
    expect(screen.queryByText(/Error: old/)).toBeNull()
  })

  it(`aborts independent page and replacement requests on unmount; late failure=${fails}`, async () => {
    const view = await mount()
    const paging = deferred(),
      replacement = deferred()
    api.searchLogPage
      .mockReturnValueOnce(paging.promise)
      .mockReturnValueOnce(replacement.promise)
    await fireEvent.click(screen.getByRole('button', { name: 'Next page' }))
    await query('body = "B"')
    await view.unmount()
    expect(api.searchLogPage.mock.calls[1].at(-1).aborted).toBe(true)
    expect(api.searchLogPage.mock.calls[2].at(-1).aborted).toBe(true)
    if (fails) {
      paging.reject(new Error('late page'))
      replacement.reject(new Error('late replacement'))
    } else {
      paging.resolve(result('late-page'))
      replacement.resolve(result('late-replacement'))
    }
    await flush()
    expect(screen.queryByText(/late page|late replacement/)).toBeNull()
  })
}
