// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from 'vitest'
import {
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import { navigate } from '@/route'
import { setTestUrl } from '@/test/render-helpers'
import type { Stats } from '@/types/api-types'
import type { ImportFailure } from '@/types/import-types'

const {
  getStats,
  searchTraceSummaries,
  searchLogSummaries,
  getLog,
  searchMetricSummaries,
  getMetricView,
  getTraceAttributeDefinitions,
  getLogAttributeDefinitions,
  getMetricAttributeDefinitions,
} = vi.hoisted(() => ({
  getStats: vi.fn(),
  searchTraceSummaries: vi.fn(),
  searchLogSummaries: vi.fn(),
  getLog: vi.fn(),
  searchMetricSummaries: vi.fn(),
  getMetricView: vi.fn(),
  getTraceAttributeDefinitions: vi.fn(),
  getLogAttributeDefinitions: vi.fn(),
  getMetricAttributeDefinitions: vi.fn(),
}))

vi.mock('@/services/telemetry-service', async importOriginal => {
  const actual =
    await importOriginal<typeof import('@/services/telemetry-service')>()
  return {
    ...actual,
    telemetryAPI: {
      ...actual.telemetryAPI,
      getStats,
      searchTraceSummaries,
      searchLogSummaries,
      getLog,
      searchMetricSummaries,
      getMetricView,
      getTraceAttributeDefinitions,
      getLogAttributeDefinitions,
      getMetricAttributeDefinitions,
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
  vi.clearAllMocks()
  getStats.mockResolvedValue(EMPTY_STATS)
  searchTraceSummaries.mockResolvedValue([])
  searchLogSummaries.mockResolvedValue([])
  getLog.mockResolvedValue(null)
  searchMetricSummaries.mockResolvedValue([])
  getMetricView.mockResolvedValue(null)
  getTraceAttributeDefinitions.mockResolvedValue([])
  getLogAttributeDefinitions.mockResolvedValue([])
  getMetricAttributeDefinitions.mockResolvedValue([])
})

describe('App real-page composition', () => {
  it('keeps failed-file counts separate from rejected telemetry in Home Overview', async () => {
    setTestUrl('/')
    const timestamp = BigInt(Date.now()) * 1_000_000n
    const rejectedStats: Stats = {
      ...EMPTY_STATS,
      rejections: [
        {
          signal: 'traces',
          kind: 'span_already_stored',
          occurrences: 2,
          samples: [
            {
              traceID: '00000000000000000000000000000001',
              spanID: '0000000000000001',
            },
          ],
          firstSeen: timestamp,
          lastSeen: timestamp,
        },
      ],
    }
    getStats.mockResolvedValue(rejectedStats)
    render(App, {
      importFailures: [
        {
          fileName: 'example.json',
          reason: 'Invalid JSON.',
          occurredAt: timestamp,
        },
      ],
    })
    const panel = await screen.findByRole('region', {
      name: 'Ingestion issues',
    })
    expect(within(panel).getByText('1 file')).toBeInTheDocument()
    expect(await within(panel).findByText('2 records')).toBeInTheDocument()
    expect(within(panel).getByText(/Invalid JSON\./)).toBeInTheDocument()
    expect(
      within(panel).getByRole('link', { name: '0000000000000001' })
    ).toHaveAttribute(
      'href',
      '/traces/00000000000000000000000000000001?span=0000000000000001'
    )
    expect(screen.queryByRole('alert')).toBeNull()
  })

  it.each(['/logs', '/traces', '/metrics'])(
    'shows an inline import error on %s and links to Home issues',
    async path => {
      setTestUrl(path)
      render(App, {
        importFailures: [
          {
            fileName: 'example.json',
            reason: 'Invalid JSON.',
            occurredAt: BigInt(Date.now()) * 1_000_000n,
          },
        ],
      })
      const main = screen.getByRole('main')
      expect(
        await within(main).findByText('Import issue in example.json.')
      ).toBeInTheDocument()
      expect(window.location.pathname).toBe(path)
      const link = within(main).getByRole('link', { name: 'View issue' })
      expect(link).toHaveAttribute('href', '/#ingestion-issues')
      await userEvent.click(link)
      const panel = await screen.findByRole('region', {
        name: 'Ingestion issues',
      })
      expect(window.location.pathname).toBe('/')
      expect(window.location.hash).toBe('#ingestion-issues')
      await waitFor(() => expect(panel).toHaveFocus())
      expect(within(panel).getByText(/Invalid JSON\./)).toBeInTheDocument()
      expect(screen.queryByRole('alert')).toBeNull()
    }
  )

  it('keeps the issue on Home, clears the inline error after visiting Home and shows a later failure', async () => {
    setTestUrl('/logs')
    const failure: ImportFailure = {
      fileName: 'example.json',
      reason: 'Invalid JSON.',
      occurredAt: BigInt(Date.now()) * 1_000_000n,
    }
    const view = render(App, { importFailures: [failure] })
    await screen.findByText('Import issue in example.json.')
    navigate('/')
    const panel = await screen.findByRole('region', {
      name: 'Ingestion issues',
    })
    expect(within(panel).getByText('example.json')).toBeInTheDocument()
    navigate('/logs')
    await screen.findByText('No logs in this time range')
    expect(screen.queryByText('Import issue in example.json.')).toBeNull()
    await view.rerender({
      importFailures: [
        failure,
        {
          ...failure,
          fileName: 'another.json',
          occurredAt: failure.occurredAt + 1n,
        },
      ],
    })
    expect(
      await within(screen.getByRole('main')).findByText(
        'Import issue in another.json.'
      )
    ).toBeInTheDocument()
    expect(window.location.pathname).toBe('/logs')
  })

  it.each([
    { path: '/logs', fetchList: searchLogSummaries },
    { path: '/traces', fetchList: searchTraceSummaries },
    { path: '/metrics', fetchList: searchMetricSummaries },
  ])(
    'preserves the existing page error on $path when an import also fails',
    async ({ path, fetchList }) => {
      setTestUrl(path)
      fetchList.mockRejectedValue(new Error('Unable to load telemetry'))
      render(App, {
        importFailures: [
          {
            fileName: 'example.json',
            reason: 'Invalid JSON.',
            occurredAt: BigInt(Date.now()) * 1_000_000n,
          },
        ],
      })
      expect(
        await screen.findByText('Error: Unable to load telemetry')
      ).toBeInTheDocument()
      expect(screen.queryByText('Import issue in example.json.')).toBeNull()
      navigate('/')
      const panel = await screen.findByRole('region', {
        name: 'Ingestion issues',
      })
      expect(within(panel).getByText('example.json')).toBeInTheDocument()
    }
  )

  it('offers the home picker and accepts dropped files after navigating to another signal', async () => {
    setTestUrl('/')
    const importFiles = vi.fn()
    render(App, { importFiles })
    expect(
      screen.getByRole('button', { name: 'Import files' })
    ).toBeInTheDocument()
    navigate('/logs')
    await screen.findByText('No logs in this time range')
    expect(screen.queryByRole('button', { name: 'Import files' })).toBeNull()
    const file = new File(['{"resourceLogs":[]}'], 'renamed.json')
    const dataTransfer = { types: ['Files'], files: [file] }
    await fireEvent.dragEnter(window, { dataTransfer })
    expect(
      screen.getByText('Drop OTLP JSON files to import')
    ).toBeInTheDocument()
    await fireEvent.drop(window, { dataTransfer })
    expect(importFiles).toHaveBeenCalledWith([file])
    expect(window.location.pathname).toBe('/logs')
  })

  it('selects real pages for route prefixes and falls back to Home', async () => {
    setTestUrl('/traces/trace-1')
    render(App)

    await screen.findByText('No traces in this time range')
    expect(searchTraceSummaries).toHaveBeenCalled()

    navigate('/metrics/metric-1')
    await screen.findByText('No metrics in this time range')
    expect(searchMetricSummaries).toHaveBeenCalled()

    navigate('/logs/log-1')
    await screen.findByText('No logs in this time range')
    expect(searchLogSummaries).toHaveBeenCalled()

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
