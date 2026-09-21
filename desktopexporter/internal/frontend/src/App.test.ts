// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/svelte'
import { navigate } from '@/route'
import { setTestUrl } from '@/test/render-helpers'
import type { Stats } from '@/types/api-types'

const {
  getStats,
  searchTraces,
  searchLogs,
  getLog,
  searchMetricSummaries,
  getMetric,
  getTraceAttributes,
  getLogAttributes,
  getMetricAttributes,
} = vi.hoisted(() => ({
  getStats: vi.fn(),
  searchTraces: vi.fn(),
  searchLogs: vi.fn(),
  getLog: vi.fn(),
  searchMetricSummaries: vi.fn(),
  getMetric: vi.fn(),
  getTraceAttributes: vi.fn(),
  getLogAttributes: vi.fn(),
  getMetricAttributes: vi.fn(),
}))

vi.mock('@/services/telemetry-service', async importOriginal => {
  const actual =
    await importOriginal<typeof import('@/services/telemetry-service')>()
  return {
    ...actual,
    telemetryAPI: {
      ...actual.telemetryAPI,
      getStats,
      searchTraces,
      searchLogs,
      getLog,
      searchMetricSummaries,
      getMetric,
      getTraceAttributes,
      getLogAttributes,
      getMetricAttributes,
    },
  }
})

import App from './App.svelte'

const EMPTY_STATS: Stats = {
  traces: {
    traceCount: 0,
    spanCount: 0,
    serviceCount: 0,
    errorCount: 0,
    lastReceived: null,
  },
  logs: { logCount: 0, errorCount: 0, lastReceived: null },
  metrics: { metricCount: 0, dataPointCount: 0, lastReceived: null },
  rejections: [],
}

beforeEach(() => {
  if (typeof Element.prototype.scrollTo !== 'function') {
    Element.prototype.scrollTo = () => {}
  }
  if (typeof Element.prototype.scrollIntoView !== 'function') {
    Element.prototype.scrollIntoView = () => {}
  }
  vi.clearAllMocks()
  getStats.mockResolvedValue(EMPTY_STATS)
  searchTraces.mockResolvedValue([])
  searchLogs.mockResolvedValue([])
  getLog.mockResolvedValue(null)
  searchMetricSummaries.mockResolvedValue([])
  getMetric.mockResolvedValue(null)
  getTraceAttributes.mockResolvedValue([])
  getLogAttributes.mockResolvedValue([])
  getMetricAttributes.mockResolvedValue([])
})

describe('App real-page composition', () => {
  it('selects real pages for route prefixes and falls back to Home', async () => {
    setTestUrl('/traces/trace-1')
    render(App)

    await screen.findByText('No traces in this time range')
    expect(searchTraces).toHaveBeenCalled()

    navigate('/metrics/metric-1')
    await screen.findByText('No metrics in this time range')
    expect(searchMetricSummaries).toHaveBeenCalled()

    navigate('/logs/log-1')
    await screen.findByText('No logs in this time range')
    expect(searchLogs).toHaveBeenCalled()

    navigate('/unknown')
    await screen.findByRole('heading', {
      level: 1,
      name: 'OpenTelemetry Desktop Viewer',
    })
    expect(getStats).toHaveBeenCalled()
  })

  it('preserves focus across repeated master-detail pathname changes', async () => {
    setTestUrl('/metrics/a')
    render(App)
    await screen.findByText('No metrics in this time range')
    const control = document.createElement('button')
    document.body.appendChild(control)
    control.focus()

    for (const path of ['/metrics/b', '/metrics/c']) {
      navigate(path)
      await waitFor(() => expect(window.location.pathname).toBe(path))
      expect(control).toHaveFocus()
    }
    control.remove()
  })

  it('does not move focus for a query-only update', async () => {
    setTestUrl('/metrics')
    render(App)
    await screen.findByText('No metrics in this time range')
    const outside = document.createElement('button')
    document.body.appendChild(outside)
    outside.focus()

    navigate('/metrics?start=10&end=20', 'replace')
    await waitFor(() => expect(outside).toHaveFocus())
    outside.remove()
  })

  it('recovers focus after a real cross-page trigger is destroyed', async () => {
    setTestUrl('/metrics/a')
    render(App)
    const main = screen.getByRole('main')
    const trigger = await screen.findByRole('button', {
      name: /change time range/i,
    })
    trigger.focus()
    let oldPageWasDestroyedBeforeFocus: boolean | undefined
    main.addEventListener(
      'focus',
      () => {
        oldPageWasDestroyedBeforeFocus = !document.body.contains(trigger)
      },
      { once: true }
    )

    navigate('/logs')
    await waitFor(() => expect(trigger).not.toBeInTheDocument())
    await waitFor(() => expect(main).toHaveFocus())
    expect(oldPageWasDestroyedBeforeFocus).toBe(true)
  })
})
