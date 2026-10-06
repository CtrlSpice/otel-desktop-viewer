// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/svelte'
import LogsPage from './LogsPage.svelte'
import type { LogSummary, LogData, Stats } from '@/types/api-types'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

const { searchLogSummaries, getLog, getStats, getLogAttributeDefinitions } = vi.hoisted(() => ({
  searchLogSummaries: vi.fn(),
  getLog: vi.fn(),
  getStats: vi.fn(),
  getLogAttributeDefinitions: vi.fn(),
}))

vi.mock('@/services/telemetry-service', async importOriginal => {
  const actual =
    await importOriginal<typeof import('@/services/telemetry-service')>()
  return {
    ...actual,
    telemetryAPI: {
      ...actual.telemetryAPI,
      searchLogSummaries,
      getLog,
      getStats,
      getLogAttributeDefinitions,
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
  searchLogSummaries.mockReset()
  getLog.mockReset()
  getStats.mockReset()
  getLogAttributeDefinitions.mockReset()
  getLogAttributeDefinitions.mockResolvedValue([])
})

async function renderSelectedLog() {
  searchLogSummaries.mockResolvedValue([makeLogSummary()])
  getStats.mockResolvedValue(makeStats())
  getLog.mockResolvedValue(makeLogData('payment declined'))
  setTestUrl('/logs/log-1')
  renderWithContexts(LogsPage)
  await waitFor(() => expect(getLog).toHaveBeenCalledTimes(1))
}

describe('LogsPage refresh', () => {
  it('queries the list with null bounds for the default All selection', async () => {
    await renderSelectedLog()
    expect(searchLogSummaries).toHaveBeenCalledWith(null, null, undefined)
  })

  it('refetches the open record, not just the list', async () => {
    await renderSelectedLog()

    const refresh = screen.getByRole('button', { name: /refresh/i })
    refresh.click()

    await waitFor(() => expect(searchLogSummaries.mock.calls.length).toBeGreaterThan(1))
    await waitFor(() => expect(getLog).toHaveBeenCalledTimes(2))
  })

  it('does not refetch a record nobody has selected', async () => {
    searchLogSummaries.mockResolvedValue([makeLogSummary()])
    getStats.mockResolvedValue(makeStats())
    getLog.mockResolvedValue(makeLogData('payment declined'))
    setTestUrl('/logs')
    renderWithContexts(LogsPage)
    await waitFor(() => expect(searchLogSummaries).toHaveBeenCalled())

    const refresh = screen.getByRole('button', { name: /refresh/i })
    refresh.click()

    await waitFor(() => expect(searchLogSummaries.mock.calls.length).toBeGreaterThan(1))
    // A detail fetch with no selection would race the pane's empty state.
    expect(getLog).not.toHaveBeenCalled()
  })
})

describe('LogsPage direct selection outside the list range', () => {
  it('keeps the requested ID and renders its fetched detail', async () => {
    searchLogSummaries.mockResolvedValue([])
    getStats.mockResolvedValue(makeStats())
    getLog.mockResolvedValue(makeLogData('outside current range'))
    setTestUrl('/logs/log-1?start=1&end=2')

    renderWithContexts(LogsPage)

    await waitFor(() => expect(getLog).toHaveBeenCalledWith('log-1'))
    expect(window.location.pathname).toBe('/logs/log-1')
    await waitFor(() =>
      expect(
        screen.getByRole('table', { name: 'Log fields' })
      ).toHaveTextContent('outside current range')
    )
    expect(screen.queryByText('No logs in this time range')).toBeNull()
  })
})
