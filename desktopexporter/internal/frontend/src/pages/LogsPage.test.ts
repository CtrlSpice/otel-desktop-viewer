// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { screen, waitFor } from '@testing-library/svelte'
import LogsPage from './LogsPage.svelte'
import type { LogSummary, LogData, Stats } from '@/types/api-types'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

// Refresh has to reach the detail pane, not just the list.
//
// Traces and metrics get this for free: their detail effects read the
// selected summary *object*, so reloading the list replaces that object and
// the effect re-runs. This page keys its fetcher on the log id -- a string a
// reload does not change -- so the wiring has to be explicit, and explicit
// wiring is exactly the kind that gets dropped in a refactor without anything
// failing.
//
// Nothing user-visible turns on it today, because a log record never changes
// after it is written and a refetch returns identical bytes. That is a
// property of the data rather than of this page, which is the reason to pin
// the behaviour rather than rely on it.

const { searchLogs, getLog, getStats, getLogAttributes } = vi.hoisted(() => ({
  searchLogs: vi.fn(),
  getLog: vi.fn(),
  getStats: vi.fn(),
  getLogAttributes: vi.fn(),
}))

vi.mock('@/services/telemetry-service', async importOriginal => {
  const actual =
    await importOriginal<typeof import('@/services/telemetry-service')>()
  return {
    ...actual,
    telemetryAPI: {
      ...actual.telemetryAPI,
      searchLogs,
      getLog,
      getStats,
      getLogAttributes,
    },
  }
})

function makeLogSummary(): LogSummary {
  return {
    id: 'log-1',
    timestamp: 1_700_000_000_000_000_000n,
    severityText: 'ERROR',
    severityNumber: 17,
    serviceName: 'checkout-api',
    bodyPreview: 'payment declined',
  }
}

function makeLogData(body: string): LogData {
  return {
    id: 'log-1',
    timestamp: 1_700_000_000_000_000_000n,
    observedTimestamp: 1_700_000_000_000_000_000n,
    traceID: null,
    spanID: null,
    severityText: 'ERROR',
    severityNumber: 17,
    body,
    bodyType: 'string',
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
  if (typeof Element.prototype.scrollTo !== 'function') {
    Element.prototype.scrollTo = () => {}
  }
  searchLogs.mockReset()
  getLog.mockReset()
  getStats.mockReset()
  getLogAttributes.mockReset()
  getLogAttributes.mockResolvedValue([])
})

async function renderSelectedLog() {
  searchLogs.mockResolvedValue([makeLogSummary()])
  getStats.mockResolvedValue(makeStats())
  getLog.mockResolvedValue(makeLogData('payment declined'))
  setTestUrl('/logs/log-1')
  renderWithContexts(LogsPage)
  await waitFor(() => expect(getLog).toHaveBeenCalledTimes(1))
}

describe('LogsPage refresh', () => {
  it('queries the list with null bounds for the default All selection', async () => {
    await renderSelectedLog()
    expect(searchLogs).toHaveBeenCalledWith(null, null, undefined)
  })

  it('refetches the open record, not just the list', async () => {
    await renderSelectedLog()

    const refresh = screen.getByRole('button', { name: /refresh/i })
    refresh.click()

    // The list reloads -- that part was never in doubt.
    await waitFor(() => expect(searchLogs.mock.calls.length).toBeGreaterThan(1))
    // And so does the record the user is looking at.
    await waitFor(() => expect(getLog).toHaveBeenCalledTimes(2))
  })

  it('does not refetch a record nobody has selected', async () => {
    searchLogs.mockResolvedValue([makeLogSummary()])
    getStats.mockResolvedValue(makeStats())
    getLog.mockResolvedValue(makeLogData('payment declined'))
    setTestUrl('/logs')
    renderWithContexts(LogsPage)
    await waitFor(() => expect(searchLogs).toHaveBeenCalled())

    const refresh = screen.getByRole('button', { name: /refresh/i })
    refresh.click()

    await waitFor(() => expect(searchLogs.mock.calls.length).toBeGreaterThan(1))
    // refresh() is a no-op on a null key, and must stay one: a detail fetch
    // with nothing selected would race the pane's empty state.
    expect(getLog).not.toHaveBeenCalled()
  })
})
