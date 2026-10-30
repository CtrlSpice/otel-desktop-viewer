// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { fireEvent, screen, waitFor } from '@testing-library/svelte'
import { renderWithContexts } from '@/test/render-helpers'
import type { MetricViewData } from '@/types/api-types'
import TimeseriesPanelHarness from '@/test/TimeseriesPanelHarness.svelte'

const { getMetricSeries } = vi.hoisted(() => ({ getMetricSeries: vi.fn() }))
const { copyText } = vi.hoisted(() => ({
  copyText: vi.fn().mockResolvedValue(undefined),
}))
vi.mock('@/utils/clipboard', () => ({ copyText }))
vi.mock('@/services/telemetry-service', () => ({
  telemetryAPI: { getMetricSeries },
}))

function metric(): MetricViewData {
  const resource = { attributes: [], droppedAttributesCount: 0 }
  return {
    metricRef: 'metric',
    name: 'metric',
    description: '',
    metadata: [],
    unit: '',
    metricType: 'Gauge',
    aggregationTemporality: null,
    aggregationTemporalityCode: null,
    isMonotonic: null,
    resource,
    resourceDroppedAttributesCount: 0,
    resourceSchemaUrl: '',
    scope: { ...resource, name: '', version: '' },
    scopeName: '',
    scopeVersion: '',
    scopeSchemaUrl: '',
    scopeDroppedAttributesCount: 0,
    lastSeenNs: 1n,
    datapointCount: 1,
    boundsMismatch: null,
    window: {
      requested: { startNs: null, endNs: null },
      effective: { startNs: 1n, endNs: 2n },
    },
    timeseries: [
      {
        seriesRef: 'series',
        attributes: [],
        attributesComplete: false,
        labelPreviews: [],
        resource,
        datapoints: [],
        stats: null,
        views: null,
        rateStats: null,
        sparkline: null,
        datapointCount: 1,
        lastSeenNs: 1n,
      },
    ],
  }
}

describe('TimeseriesPanel exact attribute refresh', () => {
  it('retains the mounted table and Copy controls until a fresh same-ID response arrives', async () => {
    let finish!: (value: unknown) => void
    getMetricSeries
      .mockResolvedValueOnce({
        attributes: [
          { key: 'context', value: { kind: 'string', value: 'old context' } },
        ],
      })
      .mockImplementationOnce(
        () =>
          new Promise(resolve => {
            finish = resolve
          })
      )
    const view = renderWithContexts(TimeseriesPanelHarness, {
      metric: metric(),
    })
    await fireEvent.click(
      await screen.findByRole('button', {
        name: 'Expand attributes for default series',
      })
    )
    const value = await screen.findByText('old context')
    const table = value.closest('table')!
    const copy = table.querySelector('button[aria-label^="Copy"]')!
    expect(copy).not.toBeNull()
    await view.rerender({
      component: TimeseriesPanelHarness,
      componentProps: { metric: metric() },
    })
    await waitFor(() => expect(getMetricSeries).toHaveBeenCalledTimes(2))
    expect(screen.getByText('old context').closest('table')).toBe(table)
    expect(table.querySelector('button[aria-label^="Copy"]')).toBe(copy)
    expect(screen.queryByText('Loading series attributes…')).toBeNull()
    expect(await screen.findByText('Loading series attributes…')).toBeVisible()
    expect(screen.getByText('old context').closest('table')).toBe(table)
    await fireEvent.click(copy)
    await waitFor(() =>
      expect(copyText).toHaveBeenLastCalledWith('old context')
    )
    finish({
      attributes: [
        { key: 'context', value: { kind: 'string', value: 'fresh context' } },
      ],
    })
    await screen.findByText('fresh context')
    expect(screen.queryByText('old context')).toBeNull()
    expect(screen.getByText('fresh context').closest('table')).toBe(table)
    await fireEvent.click(table.querySelector('button[aria-label^="Copy"]')!)
    await waitFor(() =>
      expect(copyText).toHaveBeenLastCalledWith('fresh context')
    )
    expect(screen.queryByText('Loading series attributes…')).toBeNull()
    expect(
      screen.getByRole('button', {
        name: 'Collapse attributes for default series',
      })
    ).toHaveAttribute('aria-expanded', 'true')
  })
})
