// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/svelte'
import LogsPage from './LogsPage.svelte'
import type { LogSummary, LogData, Stats } from '@/types/api-types'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

const {
  searchLogPage,
  getLog,
  getStats,
  getLogAttributeDefinitions,
  getLogErrorNavigation,
} = vi.hoisted(() => ({
  searchLogPage: vi.fn(),
  getLog: vi.fn(),
  getStats: vi.fn(),
  getLogAttributeDefinitions: vi.fn(),
  getLogErrorNavigation: vi.fn(),
}))

vi.mock('@/services/telemetry-service', async importOriginal => {
  const actual =
    await importOriginal<typeof import('@/services/telemetry-service')>()
  return {
    ...actual,
    telemetryAPI: {
      ...actual.telemetryAPI,
      searchLogPage,
      getLog,
      getStats,
      getLogAttributeDefinitions,
      getLogErrorNavigation,
    },
  }
})

function makeLogSummary(): LogSummary {
  return {
    logRef: 'log-1',
    timestamp: 1_700_000_000_000_000_000n,
    severityText: 'ERROR',
    severityNumber: 17,
    serviceName: 'checkout-api',
    bodyPreview: 'payment declined',
  }
}

function makeLogData(body: string): LogData {
  return {
    logRef: 'log-1',
    timestamp: 1_700_000_000_000_000_000n,
    observedTimestamp: 1_700_000_000_000_000_000n,
    traceID: null,
    spanID: null,
    severityText: 'ERROR',
    severityNumber: 17,
    body: { kind: 'string', value: body },
    resource: { attributes: [], droppedAttributesCount: 0 },
    scope: { name: '', version: '', attributes: [], droppedAttributesCount: 0 },
    attributes: [],
    droppedAttributesCount: 0,
    flags: 0,
    eventName: '',
  }
}

function makeStats(): Stats {
  return {
    traces: {
      traceCount: 0,
      spanCount: 0,
      serviceCount: 0,
      errorCount: 0,
      lastReceived: null,
    },
    logs: { logCount: 1, errorCount: 1, lastReceived: null },
    metrics: { metricCount: 0, dataPointCount: 0, lastReceived: null },
    rejections: [],
  }
}

beforeEach(() => {
  searchLogPage.mockReset()
  getLog.mockReset()
  getStats.mockReset()
  getLogAttributeDefinitions.mockReset()
  getLogAttributeDefinitions.mockResolvedValue([])
  getLogErrorNavigation.mockReset()
  getLogErrorNavigation.mockResolvedValue({
    count: 0,
    index: null,
    previous: null,
    next: null,
  })
})

async function renderSelectedLog() {
  searchLogPage.mockResolvedValue({ logs: [makeLogSummary()], total: 1 })
  getStats.mockResolvedValue(makeStats())
  getLog.mockResolvedValue(makeLogData('payment declined'))
  setTestUrl('/logs/log-1')
  renderWithContexts(LogsPage)
  await waitFor(() => expect(getLog).toHaveBeenCalledTimes(1))
}

describe('LogsPage refresh', () => {
  it('cancels superseded and unmounted detail requests', async () => {
    searchLogPage.mockResolvedValue({ logs: [makeLogSummary()], total: 1 })
    getStats.mockResolvedValue(makeStats())
    const signals: AbortSignal[] = []
    const resolve: ((data: LogData) => void)[] = []
    getLog.mockImplementation((_id: string, signal: AbortSignal) => {
      signals.push(signal)
      return new Promise<LogData>(done => resolve.push(done))
    })
    setTestUrl('/logs/log-1')
    const view = renderWithContexts(LogsPage)
    await waitFor(() => expect(signals).toHaveLength(1))
    screen.getByRole('button', { name: /refresh/i }).click()
    await waitFor(() => expect(signals).toHaveLength(2))
    expect(signals[0].aborted).toBe(true)
    expect(signals[1].aborted).toBe(false)
    resolve[1](makeLogData('replacement'))
    await waitFor(() =>
      expect(
        screen.getByRole('table', { name: 'Log fields' })
      ).toHaveTextContent('replacement')
    )
    resolve[0](makeLogData('stale'))
    await view.unmount()
    expect(signals[1].aborted).toBe(true)
  })

  it('queries all logs with numeric severity descending by default', async () => {
    await renderSelectedLog()
    expect(searchLogPage).toHaveBeenCalledWith(
      null,
      null,
      100,
      0,
      undefined,
      undefined,
      { field: 'severity', direction: 'desc' },
      [],
      expect.any(AbortSignal)
    )
  })

  it('refetches the open record, not just the list', async () => {
    await renderSelectedLog()

    const refresh = screen.getByRole('button', { name: /refresh/i })
    refresh.click()

    await waitFor(() =>
      expect(searchLogPage.mock.calls.length).toBeGreaterThan(1)
    )
    await waitFor(() => expect(getLog).toHaveBeenCalledTimes(2))
  })

  it('does not refetch a record nobody has selected', async () => {
    searchLogPage.mockResolvedValue({ logs: [], total: 0 })
    getStats.mockResolvedValue(makeStats())
    getLog.mockResolvedValue(makeLogData('payment declined'))
    setTestUrl('/logs')
    renderWithContexts(LogsPage)
    await waitFor(() => expect(searchLogPage).toHaveBeenCalled())

    const refresh = screen.getByRole('button', { name: /refresh/i })
    refresh.click()

    await waitFor(() =>
      expect(searchLogPage.mock.calls.length).toBeGreaterThan(1)
    )
    // A detail fetch with no selection would race the pane's empty state.
    expect(getLog).not.toHaveBeenCalled()
  })
})

describe('LogsPage direct selection outside the list range', () => {
  it('keeps original resource and scope keys inline with separate provenance badges', async () => {
    const log = makeLogData('flat record')
    log.resource.attributes = [
      { key: 'service.name', value: { kind: 'string', value: 'checkout-api' } },
    ]
    log.scope.name = 'test-scope'
    log.scope.version = '1.2'
    searchLogPage.mockResolvedValue({ logs: [makeLogSummary()], total: 1 })
    getStats.mockResolvedValue(makeStats())
    getLog.mockResolvedValue(log)
    setTestUrl('/logs/log-1')
    renderWithContexts(LogsPage)
    const table = await screen.findByRole('table', { name: 'Log fields' })
    for (const [key, origin, value] of [
      ['service.name', 'resource', 'checkout-api'],
      ['name', 'scope', 'test-scope'],
      ['version', 'scope', '1.2'],
    ]) {
      const row = table.querySelector(`[data-field="${key}"]`)!
      expect(row.querySelector('.detail-property__key')).toHaveTextContent(key!)
      expect(row.querySelector('.detail-property__origin')).toHaveTextContent(
        origin!
      )
      expect(row.querySelector('.detail-property__type')).toHaveTextContent(
        'string'
      )
      expect(row.querySelector('.detail-property__body')).toHaveTextContent(
        value!
      )
    }
    expect(table).not.toHaveTextContent('resource.service.name')
    expect(log.resource.attributes[0]!.key).toBe('service.name')
    expect(table.closest('details')).toBeNull()
    expect(screen.queryByText('Resource', { selector: 'summary' })).toBeNull()
    expect(screen.queryByText('Scope', { selector: 'summary' })).toBeNull()
  })

  it('keeps the requested ID and renders its fetched detail', async () => {
    searchLogPage.mockResolvedValue({ logs: [], total: 0 })
    getStats.mockResolvedValue(makeStats())
    getLog.mockResolvedValue(makeLogData('outside current range'))
    setTestUrl('/logs/log-1?start=1&end=2')

    renderWithContexts(LogsPage)

    await waitFor(() =>
      expect(getLog).toHaveBeenCalledWith('log-1', expect.any(AbortSignal))
    )
    expect(window.location.pathname).toBe('/logs/log-1')
    await waitFor(() =>
      expect(
        screen.getByRole('table', { name: 'Log fields' })
      ).toHaveTextContent('outside current range')
    )
    expect(screen.queryByText('No logs in this time range')).toBeNull()
  })
})
