// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/svelte'
import HomePage, { rejectionLabel } from './HomePage.svelte'
import { telemetryAPI } from '@/services/telemetry-service'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import type { Stats } from '@/types/api-types'

describe('rejectionLabel', () => {
  it('labels known rejection kinds', () => {
    expect(rejectionLabel('span_already_stored')).toBe('duplicate span id')
    expect(rejectionLabel('span_refused')).toBe('rejected by the store')
  })

  it('falls back to unknown and prototype-named kinds', () => {
    expect(rejectionLabel('new_rejection')).toBe('new_rejection')
    expect(rejectionLabel('toString')).toBe('toString')
    expect(rejectionLabel('constructor')).toBe('constructor')
  })
})

describe('Home service discovery', () => {
  afterEach(() => {
    vi.useRealTimers()
    vi.restoreAllMocks()
  })

  it('passes each pending discovery identity to the map without waiting for results', async () => {
    vi.useFakeTimers()
    vi.spyOn(telemetryAPI, 'getStats').mockImplementation(
      () => new Promise(() => {})
    )
    vi.spyOn(telemetryAPI, 'getImportConfig').mockResolvedValue({
      otlpHttpPort: 4318,
    })
    const services = vi
      .spyOn(telemetryAPI, 'discoverServices')
      .mockImplementation(() => new Promise(() => {}))
    setTestUrl('/?time=all')
    const view = renderWithContexts(HomePage)
    await vi.advanceTimersByTimeAsync(200)
    expect(view.getByText('Loading service map…')).toBeInTheDocument()
    const search = view.getByRole('searchbox', { name: 'Search telemetry' })
    await fireEvent.submit(search.closest('form')!)
    expect(services).toHaveBeenCalledTimes(2)
    expect(services.mock.calls[0]![1]?.aborted).toBe(true)
    expect(view.queryByText('Loading service map…')).toBeNull()
    await vi.advanceTimersByTimeAsync(199)
    expect(view.queryByText('Loading service map…')).toBeNull()
    await vi.advanceTimersByTimeAsync(1)
    expect(view.getByText('Loading service map…')).toBeInTheDocument()
    view.unmount()
    expect(services.mock.calls[1]![1]?.aborted).toBe(true)
  })

  it('scopes telemetry by typed service identity and refreshes only on request', async () => {
    // Responsive graph layout needs a viewport; jsdom has no layout engine.
    vi.spyOn(HTMLElement.prototype, 'clientWidth', 'get').mockReturnValue(900)
    vi.spyOn(HTMLElement.prototype, 'clientHeight', 'get').mockReturnValue(800)
    vi.spyOn(telemetryAPI, 'getTraceAttributeDefinitions').mockResolvedValue([])
    vi.spyOn(telemetryAPI, 'getMetricAttributeDefinitions').mockResolvedValue(
      []
    )
    vi.spyOn(telemetryAPI, 'getLogAttributeDefinitions').mockResolvedValue([])
    const traces = vi
      .spyOn(telemetryAPI, 'searchTraceSummaries')
      .mockResolvedValue([])
    vi.spyOn(telemetryAPI, 'searchMetricSummaries').mockResolvedValue([])
    vi.spyOn(telemetryAPI, 'searchLogSummaries').mockResolvedValue([])
    vi.spyOn(telemetryAPI, 'getStats').mockResolvedValue({
      traces: {
        traceCount: 0,
        spanCount: 0,
        serviceCount: 0,
        errorCount: 0,
        lastReceived: null,
      },
      metrics: { metricCount: 0, dataPointCount: 0, lastReceived: null },
      logs: { logCount: 0, errorCount: 0, lastReceived: null },
      rejections: [],
    })
    vi.spyOn(telemetryAPI, 'getImportConfig').mockResolvedValue({
      otlpHttpPort: 4318,
    })
    const base = {
      serviceName: 'checkout',
      serviceNamespace: 'storefront',
      traceCount: 42,
      spanCount: 248,
      errorSpanCount: 3,
      logCount: 36,
      errorLogCount: 2,
      logSeverityCounts: {
        trace: 0,
        debug: 0,
        info: 30,
        warn: 4,
        error: 1,
        fatal: 1,
        other: 0,
      },
      metricCount: 8,
      dataPointCount: 640,
      exemplarCount: 12,
      lastSeen: '1792692000000000000',
    }
    const services = vi
      .spyOn(telemetryAPI, 'discoverServices')
      .mockResolvedValue({
        items: [base, { ...base, serviceNamespace: 'internal' }],
        truncated: false,
      })
    vi.spyOn(telemetryAPI, 'discoverServiceConnections').mockResolvedValue({
      items: [],
      truncated: false,
    })
    setTestUrl('/?time=all')
    renderWithContexts(HomePage)
    await waitFor(() => expect(traces).toHaveBeenCalled())
    const query = JSON.stringify(traces.mock.calls[0]![2])
    expect(query).toContain('checkout')
    expect(query).toContain('storefront')
    expect(query).not.toContain('internal')
    expect(
      screen.queryByRole('combobox', { name: 'Attribute owner' })
    ).not.toBeInTheDocument()
    await waitFor(() =>
      expect(screen.getByRole('tab', { name: /^Logs/ })).toBeDisabled()
    )
    expect(services).toHaveBeenCalledTimes(1)
    expect(traces).toHaveBeenCalledTimes(1)
    expect(telemetryAPI.searchMetricSummaries).toHaveBeenCalledTimes(1)
    expect(telemetryAPI.searchLogSummaries).toHaveBeenCalledTimes(1)
    expect(telemetryAPI.getTraceAttributeDefinitions).not.toHaveBeenCalled()
    expect(telemetryAPI.getMetricAttributeDefinitions).not.toHaveBeenCalled()
    expect(telemetryAPI.getLogAttributeDefinitions).not.toHaveBeenCalled()
    expect(window.location.pathname).toBe('/')
    await fireEvent.click(
      screen
        .getAllByRole('button', { name: /checkout/ })
        .find(button => button.textContent?.includes('internal'))!
    )
    await waitFor(() =>
      expect(JSON.stringify(traces.mock.lastCall?.[2])).toContain('internal')
    )
    await waitFor(() =>
      expect(screen.getByRole('tab', { name: /^Logs/ })).toBeDisabled()
    )
    expect(traces).toHaveBeenCalledTimes(2)
    expect(telemetryAPI.searchMetricSummaries).toHaveBeenCalledTimes(2)
    expect(telemetryAPI.searchLogSummaries).toHaveBeenCalledTimes(2)
    const calls = traces.mock.calls.length
    let finishDiscovery!: () => void
    services.mockImplementationOnce(
      () =>
        new Promise(resolve => {
          finishDiscovery = () =>
            resolve({
              items: [base, { ...base, serviceNamespace: 'internal' }],
              truncated: false,
            })
        })
    )
    const canvas = document.querySelector('.service-map__canvas svg')
    expect(canvas).not.toBeNull()
    await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() => expect(services).toHaveBeenCalledTimes(2))
    await waitFor(() => expect(traces).toHaveBeenCalledTimes(calls + 1))
    expect(document.querySelector('.service-map__canvas svg')).toBe(canvas)
    expect(JSON.stringify(traces.mock.lastCall?.[2])).toContain('internal')
    expect(JSON.stringify(traces.mock.lastCall?.[2])).not.toContain(
      'storefront'
    )
    finishDiscovery()
    await waitFor(() =>
      expect(screen.getByRole('tab', { name: /^Logs/ })).toBeDisabled()
    )
    expect(traces).toHaveBeenCalledTimes(calls + 1)
    expect(document.querySelector('.service-map__canvas svg')).toBe(canvas)
  })

  it('keeps pending statistics independent of service pagination without displaying an invented zero', async () => {
    let finish!: (stats: Stats) => void
    const stats = vi.spyOn(telemetryAPI, 'getStats').mockImplementation(
      () =>
        new Promise(resolve => {
          finish = resolve
        })
    )
    vi.spyOn(telemetryAPI, 'getImportConfig').mockResolvedValue({
      otlpHttpPort: 4318,
    })
    vi.spyOn(telemetryAPI, 'discoverServices').mockResolvedValue({
      items: [],
      truncated: true,
    })
    setTestUrl('/')
    renderWithContexts(HomePage)
    await screen.findByText('Loading storage information…')
    expect(screen.queryByText('Storage information unavailable.')).toBeNull()
    expect(
      screen.getByText('Ingestion issues').closest('summary')
    ).not.toHaveTextContent('[0]')
    const signal = stats.mock.calls[0]![0]!
    await fireEvent.click(screen.getByRole('button', { name: 'More services' }))
    expect(stats).toHaveBeenCalledTimes(1)
    expect(signal.aborted).toBe(false)
    finish({
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
      storage: { sizeBytes: 123, maxSizeBytes: 456, inMemory: true },
    })
    await screen.findByText('123 bytes')
    expect(screen.queryByText('Loading storage information…')).toBeNull()
  })
})
