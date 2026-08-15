// @vitest-environment jsdom
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import type { SumDataPoint } from '@/types/api-types'
import SeriesDatapointListHarness from '@/test/SeriesDatapointListHarness.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import { SPAN_PARAM } from '@/route/query-params'

const navigateToItem = vi.hoisted(() => vi.fn())

vi.mock('@/route', async importOriginal => {
  const actual = await importOriginal<typeof import('@/route')>()
  return { ...actual, navigateToItem }
})

function makeDatapoint(overrides: Partial<SumDataPoint> = {}): SumDataPoint {
  return {
    id: 'dp-1',
    timestamp: 1_700_000_000_000_000_000n,
    timestampMs: 1_700_000_000_000,
    startTime: 1_700_000_000_000_000_000n,
    flags: 0,
    metricType: 'Sum',
    doubleValue: 42,
    intValue: null,
    valueType: 'double',
    isMonotonic: true,
    aggregationTemporality: 'Cumulative',
    exemplars: [
      {
        value: 42,
        timestamp: 1_700_000_000_000_000_000n,
        traceID: 'trace-ex',
        spanID: 'span-ex',
        filteredAttributes: [],
      },
    ],
    ...overrides,
  }
}

describe('SeriesDatapointList exemplar trace correlation', () => {
  beforeEach(() => {
    navigateToItem.mockClear()
    setTestUrl('/metrics/m1?start=0&end=1')
  })

  it('links exemplar trace and span ids with span in the href', () => {
    renderWithContexts(SeriesDatapointListHarness, {
      datapoints: [makeDatapoint()],
      expandDatapointId: 'dp-1',
    })
    expect(
      screen.getByRole('link', { name: 'trace: trace-ex' })
    ).toHaveAttribute('href', '/traces/trace-ex?start=0&end=1&span=span-ex')
    expect(screen.getByRole('link', { name: 'span: span-ex' })).toHaveAttribute(
      'href',
      '/traces/trace-ex?start=0&end=1&span=span-ex'
    )
  })

  it('navigates with span patch when an exemplar span link is clicked', async () => {
    renderWithContexts(SeriesDatapointListHarness, {
      datapoints: [makeDatapoint()],
      expandDatapointId: 'dp-1',
    })
    await userEvent.click(screen.getByRole('link', { name: 'span: span-ex' }))
    expect(navigateToItem).toHaveBeenCalledWith('traces', 'trace-ex', 'push', {
      [SPAN_PARAM]: 'span-ex',
    })
  })
})
