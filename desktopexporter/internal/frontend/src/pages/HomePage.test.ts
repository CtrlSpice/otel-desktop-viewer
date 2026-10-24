// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from 'vitest'
import { fireEvent, screen, waitFor, within } from '@testing-library/svelte'
import HomePage, { rejectionLabel } from './HomePage.svelte'
import { telemetryAPI } from '@/services/telemetry-service'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

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
  afterEach(() => vi.restoreAllMocks())

  it('scopes discovery by namespace and service and refreshes only on request', async () => {
    vi.spyOn(telemetryAPI, 'getTraceAttributeDefinitions').mockResolvedValue([])
    vi.spyOn(telemetryAPI, 'getMetricAttributeDefinitions').mockResolvedValue(
      []
    )
    vi.spyOn(telemetryAPI, 'getLogAttributeDefinitions').mockResolvedValue([])
    vi.spyOn(telemetryAPI, 'searchTraceSummaries').mockResolvedValue([])
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
    const keys = vi
      .spyOn(telemetryAPI, 'discoverAttributeKeys')
      .mockImplementation(async options => ({
        items:
          options.ownerType === 'resource' || options.ownerType === 'span'
            ? [
                {
                  key: 'shared.name',
                  kind: 'string',
                  foundOn: [
                    { signal: options.signal!, ownerType: options.ownerType },
                  ],
                },
              ]
            : [],
        truncated: false,
      }))
    setTestUrl('/?time=all')
    vi.spyOn(telemetryAPI, 'discoverAttributeValues').mockResolvedValue({
      items: [],
      truncated: false,
    })
    renderWithContexts(HomePage)
    const card = await screen.findByRole('button', {
      name: /^checkout\s*· storefront/,
    })
    await waitFor(() =>
      expect(keys).toHaveBeenCalledWith(
        expect.objectContaining({
          serviceName: 'checkout',
          namespace: 'storefront',
          signal: 'traces',
          ownerType: 'resource',
        }),
        expect.any(AbortSignal)
      )
    )
    await waitFor(() =>
      expect(screen.getAllByText('shared.name', { exact: false })).toHaveLength(
        2
      )
    )
    expect(
      screen.queryByRole('combobox', { name: 'Attribute owner' })
    ).not.toBeInTheDocument()
    expect(card).toHaveAttribute('aria-pressed', 'true')
    expect(within(card).queryByRole('link')).not.toBeInTheDocument()
    expect(within(card).queryByRole('button')).not.toBeInTheDocument()
    await fireEvent.click(
      screen.getByRole('button', { name: /^checkout\s*· internal/ })
    )
    await waitFor(() =>
      expect(keys).toHaveBeenCalledWith(
        expect.objectContaining({ namespace: 'internal' }),
        expect.any(AbortSignal)
      )
    )
    await waitFor(() =>
      expect(screen.getByRole('tab', { name: /^Logs/ })).toBeDisabled()
    )
    await waitFor(() =>
      expect(keys).toHaveBeenCalledWith(
        expect.objectContaining({ namespace: 'internal', signal: 'logs' }),
        expect.any(AbortSignal)
      )
    )
    expect(services).toHaveBeenCalledTimes(1)
    expect(window.location.pathname).toBe('/')
    await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
    await waitFor(() => expect(services).toHaveBeenCalledTimes(2))
  })
})
