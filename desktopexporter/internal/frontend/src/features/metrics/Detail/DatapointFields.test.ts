// @vitest-environment jsdom
import { expect, it, vi } from 'vitest'
import { screen } from '@testing-library/svelte'
import userEvent from '@testing-library/user-event'
import { renderWithContexts } from '@/test/render-helpers'
import type { DataPoint } from '@/types/api-types'
import DatapointFields from './DatapointFields.svelte'

it.each(['Gauge', 'Histogram'] as const)(
  'preserves negative zero in %s fields and explicit Copy',
  async metricType => {
    const base = {
      id: 'point',
      timestamp: 1n,
      timestampMs: 0,
      startTime: 0n,
      flags: 0,
      exemplars: [],
    }
    const datapoint: DataPoint =
      metricType === 'Gauge'
        ? {
            ...base,
            metricType,
            doubleValue: -0,
            intValue: null,
            valueType: 'double',
          }
        : {
            ...base,
            metricType,
            count: 1n,
            sum: -0,
            min: -0,
            max: -0,
            explicitBounds: [-0, 1],
            bucketCounts: [1n, 0n, 0n],
            aggregationTemporality: 'Cumulative',
            aggregationTemporalityCode: 2,
          }
    const user = userEvent.setup()
    const copy = vi.spyOn(navigator.clipboard, 'writeText').mockResolvedValue()
    renderWithContexts(DatapointFields, { datapoint })
    for (const [field, expected] of metricType === 'Gauge'
      ? [['asDouble', '-0']]
      : [
          ['sum', '-0'],
          ['min', '-0'],
          ['max', '-0'],
          ['explicitBounds', '[-0,1]'],
        ]) {
      await user.click(screen.getByRole('button', { name: `Copy ${field}` }))
      expect(copy).toHaveBeenLastCalledWith(expected)
    }
  }
)
