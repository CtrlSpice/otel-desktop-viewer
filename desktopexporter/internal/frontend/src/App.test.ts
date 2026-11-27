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
import { telemetryAPI } from '@/services/telemetry-service'
import { notifyDataChanged } from '@/test/event-source'

const {
  getStats,
  listDatabases,
  openDatabase,
  newDatabase,
  searchTraceSummaries,
  getTraceView,
  getTraceLogSummaries,
  searchLogPage,
  getLogErrorNavigation,
  getLog,
  searchMetricSummaries,
  getMetric,
  getMetricView,
  getTraceAttributeDefinitions,
  getLogAttributeDefinitions,
  getMetricAttributeDefinitions,
} = vi.hoisted(() => ({
  getStats: vi.fn(),
  listDatabases: vi.fn(),
  openDatabase: vi.fn(),
  newDatabase: vi.fn(),
  searchTraceSummaries: vi.fn(),
  getTraceView: vi.fn(),
  getTraceLogSummaries: vi.fn(),
  searchLogPage: vi.fn(),
  getLogErrorNavigation: vi.fn(),
  getLog: vi.fn(),
  searchMetricSummaries: vi.fn(),
  getMetric: vi.fn(),
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
      getServiceCount: vi.fn().mockResolvedValue(0),
      discoverServices: vi
        .fn()
        .mockResolvedValue({ items: [], truncated: false }),
      getImportConfig: vi.fn().mockResolvedValue({ otlpHttpPort: 4318 }),
      getStats,
      listDatabases,
      openDatabase,
      newDatabase,
      searchTraceSummaries,
      getTraceView,
      getTraceLogSummaries,
      searchLogPage,
      getLogErrorNavigation,
      getLog,
      searchMetricSummaries,
      getMetric,
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
  listDatabases.mockResolvedValue({ managed: false, active: '', databases: [] })
  openDatabase.mockResolvedValue({ managed: false, active: '', databases: [] })
  newDatabase.mockResolvedValue({ managed: false, active: '', databases: [] })
  searchTraceSummaries.mockResolvedValue([])
  getTraceView.mockImplementation(async (traceID: string) => ({
    traceID,
    spans: [],
    unplacedSpanCount: 0,
  }))
  getTraceLogSummaries.mockResolvedValue([])
  searchLogPage.mockResolvedValue({ total: 0, logs: [] })
  getLogErrorNavigation.mockResolvedValue({
    count: 0,
    index: null,
    previous: null,
    next: null,
  })
  getLog.mockResolvedValue(null)
  searchMetricSummaries.mockResolvedValue([])
  getMetric.mockResolvedValue(null)
  getMetricView.mockResolvedValue(null)
  getTraceAttributeDefinitions.mockResolvedValue([])
  getLogAttributeDefinitions.mockResolvedValue([])
  getMetricAttributeDefinitions.mockResolvedValue([])
})

describe('App real-page composition', () => {
  it('updates navigation counts on commits with Live off while page reads are pending', async () => {
    setTestUrl('/logs?time=all')
    searchLogPage.mockImplementationOnce(() => new Promise(() => {}))
    render(App)
    const navigation = within(
      screen.getByRole('navigation', { name: 'Primary' })
    )
    await navigation.findByRole('link', { name: 'Logs [0]' })
    expect(screen.getByRole('button', { name: 'Live' })).toHaveAttribute(
      'aria-pressed',
      'false'
    )

    getStats.mockResolvedValue({
      ...EMPTY_STATS,
      traces: { ...EMPTY_STATS.traces, traceCount: 7 },
      logs: { ...EMPTY_STATS.logs, logCount: 8 },
      metrics: { ...EMPTY_STATS.metrics, metricCount: 9 },
    })
    await notifyDataChanged()
    await navigation.findByRole('link', { name: 'Logs [8]' })
    expect(
      navigation.getByRole('link', { name: 'Traces [7]' })
    ).toBeInTheDocument()
    expect(
      navigation.getByRole('link', { name: 'Metrics [9]' })
    ).toBeInTheDocument()
    expect(searchLogPage).toHaveBeenCalledTimes(1)
  })

  it('keeps Live and manual refresh progressing while navigation counts are pending', async () => {
    setTestUrl('/logs?time=all')
    render(App)
    await screen.findByText('No logs match this search and time range.')
    await waitFor(() => expect(searchLogPage).toHaveBeenCalledTimes(1))
    vi.mocked(telemetryAPI.getServiceCount).mockImplementationOnce(
      () => new Promise<number>(() => {})
    )
    await fireEvent.click(screen.getByRole('button', { name: 'Live' }))
    await waitFor(() => expect(searchLogPage).toHaveBeenCalledTimes(2))
    for (let calls = 3; calls <= 5; calls++) {
      notifyDataChanged()
      await waitFor(() => expect(searchLogPage).toHaveBeenCalledTimes(calls))
    }
    await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() => expect(searchLogPage).toHaveBeenCalledTimes(6))
    expect(telemetryAPI.getServiceCount).toHaveBeenCalledTimes(2)
  })

  it('publishes available navigation counts without waiting for service counts', async () => {
    setTestUrl('/traces')
    vi.mocked(telemetryAPI.getServiceCount).mockImplementationOnce(
      () => new Promise(() => {})
    )
    getStats.mockResolvedValue({
      ...EMPTY_STATS,
      traces: { ...EMPTY_STATS.traces, traceCount: 7 },
      logs: { ...EMPTY_STATS.logs, logCount: 8 },
      metrics: { ...EMPTY_STATS.metrics, metricCount: 9 },
    })
    const { unmount } = render(App)
    const navigation = within(
      screen.getByRole('navigation', { name: 'Primary' })
    )
    await navigation.findByRole('link', { name: 'Traces [7]' })
    expect(
      navigation.getByRole('link', { name: 'Logs [8]' })
    ).toBeInTheDocument()
    expect(
      navigation.getByRole('link', { name: 'Metrics [9]' })
    ).toBeInTheDocument()
    const signal = vi.mocked(telemetryAPI.getServiceCount).mock.calls[0]![0]!
    await unmount()
    expect(signal.aborted).toBe(true)
  })

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
    await userEvent.click(
      screen.getByText('Ingestion issues').closest('summary')!
    )
    expect(within(panel).getByText('1 file')).toBeInTheDocument()
    expect(await within(panel).findByText('2 records')).toBeInTheDocument()
    const files = within(panel).getByRole('button', {
      name: /^File import issues 1 file Last seen /,
    })
    const records = within(panel).getByRole('button', {
      name: /^duplicate span id 2 records Last seen /,
    })
    expect(files).toHaveAttribute('aria-expanded', 'false')
    expect(records).toHaveAttribute('aria-expanded', 'false')
    expect(within(panel).getByText(/Invalid JSON\./)).not.toBeVisible()
    await userEvent.click(files)
    expect(within(panel).getByText(/Invalid JSON\./)).toBeVisible()
    expect(records).toHaveAttribute('aria-expanded', 'false')
    await userEvent.click(records)
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
    await screen.findByText('No logs match this search and time range.')
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
    { path: '/logs', fetchList: searchLogPage },
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
      screen.getByRole('button', { name: /^Import files\./ })
    ).toBeInTheDocument()
    navigate('/logs')
    await screen.findByText('No logs match this search and time range.')
    expect(screen.queryByRole('button', { name: /^Import files\./ })).toBeNull()
    const file = new File(['{"resourceLogs":[]}'], 'renamed.json')
    const dataTransfer = { types: ['Files'], files: [file] }
    await fireEvent.dragEnter(window, { dataTransfer })
    expect(
      screen.getByText(
        'Drop telemetry or evaluation JSON / JSONL files to import'
      )
    ).toBeInTheDocument()
    await fireEvent.drop(window, { dataTransfer })
    expect(importFiles).toHaveBeenCalledWith([file])
    expect(window.location.pathname).toBe('/logs')
  })

  it('selects real pages for route prefixes and falls back to Home', async () => {
    setTestUrl('/traces/trace-1')
    render(App)

    await waitFor(() =>
      expect(getTraceView).toHaveBeenCalledWith(
        'trace-1',
        undefined,
        expect.any(AbortSignal),
        expect.any(Array)
      )
    )
    expect(window.location.pathname).toBe('/traces/trace-1')
    expect(searchTraceSummaries).toHaveBeenCalled()
    navigate('/traces')
    await screen.findByText('No traces in this time range')

    navigate('/metrics/metric-1')
    await screen.findByText('The selected metric was not found.')
    expect(searchMetricSummaries).toHaveBeenCalled()
    expect(getMetric).toHaveBeenCalledWith('metric-1', expect.any(AbortSignal))

    navigate('/logs/log-1')
    await screen.findByText('No logs match this search and time range.')
    expect(searchLogPage).toHaveBeenCalled()

    navigate('/unknown')
    await screen.findByRole('region', { name: 'Service map' })
    expect(getStats).toHaveBeenCalled()
  })

  it('preserves focus across repeated master-detail pathname changes', async () => {
    setTestUrl('/metrics/a')
    render(App)
    await screen.findByText('The selected metric was not found.')
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
      name: 'Delete all metrics',
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

  it('remounts the active page, normalizes the URL, and refreshes navigation counts on database switch', async () => {
    const user = userEvent.setup()
    const managedSessions = {
      managed: true,
      active: 'current.duckdb',
      databases: [
        {
          name: 'current.duckdb',
          sizeBytes: 12000,
          modifiedAt: '2026-11-26T18:00:00Z',
          active: true,
        },
        {
          name: 'incident.duckdb',
          sizeBytes: 24000,
          modifiedAt: '2026-11-26T17:00:00Z',
          active: false,
        },
      ],
    }
    listDatabases.mockResolvedValue(managedSessions)
    openDatabase.mockResolvedValue({
      ...managedSessions,
      active: 'incident.duckdb',
      databases: [
        { ...managedSessions.databases[1]!, active: true },
        { ...managedSessions.databases[0]!, active: false },
      ],
    })
    getStats.mockResolvedValue({
      ...EMPTY_STATS,
      traces: { ...EMPTY_STATS.traces, traceCount: 5 },
      logs: { ...EMPTY_STATS.logs, logCount: 12 },
      metrics: { ...EMPTY_STATS.metrics, metricCount: 3 },
    })

    setTestUrl('/traces/trace-1?start=100&end=200&q=err&span=s1')
    render(App)
    const navigation = within(
      screen.getByRole('navigation', { name: 'Primary' })
    )
    await navigation.findByRole('link', { name: 'Traces [5]' })
    await waitFor(() => expect(searchTraceSummaries).toHaveBeenCalled())
    const traceCallsBeforeSwitch = searchTraceSummaries.mock.calls.length

    getStats.mockResolvedValue({
      ...EMPTY_STATS,
      traces: { ...EMPTY_STATS.traces, traceCount: 1 },
      logs: { ...EMPTY_STATS.logs, logCount: 2 },
      metrics: { ...EMPTY_STATS.metrics, metricCount: 0 },
    })

    await user.click(
      await screen.findByRole('button', { name: 'Session: current' })
    )
    await user.click(await screen.findByRole('button', { name: /incident/ }))

    await waitFor(() =>
      expect(window.location.pathname + window.location.search).toBe(
        '/traces?start=100&end=200&q=err'
      )
    )
    await navigation.findByRole('link', { name: 'Traces [1]' })
    expect(
      navigation.getByRole('link', { name: 'Logs [2]' })
    ).toBeInTheDocument()
    expect(
      navigation.getByRole('link', { name: 'Metrics [0]' })
    ).toBeInTheDocument()
    await waitFor(() =>
      expect(searchTraceSummaries.mock.calls.length).toBeGreaterThan(
        traceCallsBeforeSwitch
      )
    )
  })

  it('resets SearchEditor dynamic schema and LogsPage state when switching databases', async () => {
    const user = userEvent.setup()
    const managedSessions = {
      managed: true,
      active: 'current.duckdb',
      databases: [
        {
          name: 'current.duckdb',
          sizeBytes: 12000,
          modifiedAt: '2026-11-26T18:00:00Z',
          active: true,
        },
        {
          name: 'empty.duckdb',
          sizeBytes: 4000,
          modifiedAt: '2026-11-26T17:00:00Z',
          active: false,
        },
      ],
    }
    listDatabases.mockResolvedValue(managedSessions)
    openDatabase.mockResolvedValue({
      ...managedSessions,
      active: 'empty.duckdb',
      databases: [
        { ...managedSessions.databases[1]!, active: true },
        { ...managedSessions.databases[0]!, active: false },
      ],
    })
    getLogAttributeDefinitions.mockResolvedValue([
      {
        name: 'old.attr',
        type: 'string',
        path: ['attributes', 'old.attr'],
        owner: 'log',
      },
    ])

    setTestUrl('/logs?time=all')
    render(App)
    await screen.findByText('No logs match this search and time range.')
    await waitFor(() => expect(searchLogPage).toHaveBeenCalledTimes(1))

    // Focusing the search editor loads dynamic attribute definitions for session 1.
    const editor = screen.getByRole('textbox', { name: 'Search logs' })
    await fireEvent.focus(editor)
    await waitFor(() =>
      expect(getLogAttributeDefinitions).toHaveBeenCalledTimes(1)
    )

    getLogAttributeDefinitions.mockResolvedValue([
      {
        name: 'new.attr',
        type: 'string',
        path: ['attributes', 'new.attr'],
        owner: 'log',
      },
    ])

    await user.click(
      await screen.findByRole('button', { name: 'Session: current' })
    )
    await user.click(await screen.findByRole('button', { name: /empty/ }))

    await waitFor(() => expect(searchLogPage).toHaveBeenCalledTimes(2))
    // After remount, focusing the fresh SearchEditor loads definitions for session 2.
    const remountedEditor = screen.getByRole('textbox', {
      name: 'Search logs',
    })
    await fireEvent.focus(remountedEditor)
    await waitFor(() =>
      expect(getLogAttributeDefinitions).toHaveBeenCalledTimes(2)
    )
  })

  it('remounts HomePage and reloads storage stats and services on database switch', async () => {
    const user = userEvent.setup()
    const managedSessions = {
      managed: true,
      active: 'current.duckdb',
      databases: [
        {
          name: 'current.duckdb',
          sizeBytes: 12000,
          modifiedAt: '2026-11-26T18:00:00Z',
          active: true,
        },
        {
          name: 'staging.duckdb',
          sizeBytes: 4000,
          modifiedAt: '2026-11-26T17:00:00Z',
          active: false,
        },
      ],
    }
    listDatabases.mockResolvedValue(managedSessions)
    openDatabase.mockResolvedValue({
      ...managedSessions,
      active: 'staging.duckdb',
      databases: [
        { ...managedSessions.databases[1]!, active: true },
        { ...managedSessions.databases[0]!, active: false },
      ],
    })

    setTestUrl('/')
    render(App)
    await screen.findByRole('region', { name: 'Service map' })
    const discoverBefore = vi.mocked(telemetryAPI.discoverServices).mock.calls
      .length
    const statsBefore = getStats.mock.calls.length

    await user.click(
      await screen.findByRole('button', { name: 'Session: current' })
    )
    await user.click(await screen.findByRole('button', { name: /staging/ }))

    await waitFor(() =>
      expect(
        vi.mocked(telemetryAPI.discoverServices).mock.calls.length
      ).toBeGreaterThan(discoverBefore)
    )
    await waitFor(() =>
      expect(getStats.mock.calls.length).toBeGreaterThan(statsBefore)
    )
  })
})
