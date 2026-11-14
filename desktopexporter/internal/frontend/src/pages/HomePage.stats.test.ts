// @vitest-environment jsdom
import { afterEach, expect, it, vi } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/svelte'
import HomePage from './HomePage.svelte'
import { telemetryAPI } from '@/services/telemetry-service'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import { notifyDataChanged } from '@/test/event-source'
import type { Stats } from '@/types/api-types'

const initial: Stats = {
  traces: {
    traceCount: 0,
    spanCount: 0,
    serviceCount: 0,
    errorCount: 0,
    lastReceived: null,
  },
  metrics: { metricCount: 0, dataPointCount: 0, lastReceived: null },
  logs: { logCount: 0, errorCount: 0, lastReceived: null },
  storage: { sizeBytes: 123, maxSizeBytes: 0, inMemory: true },
  rejections: [],
}
const updated: Stats = {
  ...initial,
  storage: { sizeBytes: 456, maxSizeBytes: 0, inMemory: true },
  rejections: [
    {
      signal: 'traces',
      kind: 'span_already_stored',
      occurrences: 3,
      samples: [],
      firstSeen: null,
      lastSeen: null,
    },
  ],
}

afterEach(() => vi.restoreAllMocks())

function renderPendingHome() {
  const stats = vi.spyOn(telemetryAPI, 'getStats').mockResolvedValue(initial)
  const services = vi
    .spyOn(telemetryAPI, 'discoverServices')
    .mockImplementation(() => new Promise(() => {}))
  vi.spyOn(telemetryAPI, 'getImportConfig').mockResolvedValue({
    otlpHttpPort: 4318,
  })
  setTestUrl('/?time=all')
  const view = renderWithContexts(HomePage)
  return { stats, services, view }
}

it.each(['commit', 'manual Refresh'])(
  'updates storage and ingestion issues on %s with Live off and page reads pending',
  async trigger => {
    const { stats, services } = renderPendingHome()
    const storageValue = await screen.findByText('123 bytes')
    const pageSignal = services.mock.calls[0]![1]!
    expect(screen.getByRole('button', { name: 'Live' })).toHaveAttribute(
      'aria-pressed',
      'false'
    )

    stats.mockResolvedValue(updated)
    if (trigger === 'commit') await notifyDataChanged()
    else await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))

    expect(await screen.findByText('456 bytes')).toBe(storageValue)
    expect(
      screen.getByText('Ingestion issues').closest('summary')
    ).toHaveTextContent('[3]')
    expect(stats).toHaveBeenCalledTimes(2)
    expect(services).toHaveBeenCalledTimes(1)
    expect(pageSignal.aborted).toBe(false)
  }
)

it('coalesces commit and manual refresh requests and discards queued work on unmount', async () => {
  const { stats, services, view } = renderPendingHome()
  const storageValue = await screen.findByText('123 bytes')
  const current = Promise.withResolvers<Stats>()
  const followUp = Promise.withResolvers<Stats>()
  stats
    .mockReturnValueOnce(current.promise)
    .mockReturnValueOnce(followUp.promise)

  await notifyDataChanged()
  await waitFor(() => expect(stats).toHaveBeenCalledTimes(2))
  const signal = stats.mock.calls[1]![0]!
  for (let i = 0; i < 5; i++) await notifyDataChanged()
  await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }))
  expect(stats).toHaveBeenCalledTimes(2)
  expect(signal.aborted).toBe(false)
  expect(screen.getByText('123 bytes')).toBe(storageValue)
  expect(services).toHaveBeenCalledTimes(1)

  current.resolve(updated)
  await waitFor(() => expect(stats).toHaveBeenCalledTimes(3))
  expect(await screen.findByText('456 bytes')).toBe(storageValue)
  const followUpSignal = stats.mock.calls[2]![0]!
  await notifyDataChanged()
  expect(stats).toHaveBeenCalledTimes(3)
  expect(followUpSignal.aborted).toBe(false)

  view.unmount()
  expect(followUpSignal.aborted).toBe(true)
  followUp.resolve(updated)
  await notifyDataChanged()
  expect(stats).toHaveBeenCalledTimes(3)
})
