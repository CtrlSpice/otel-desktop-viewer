import { describe, expect, it, vi } from 'vitest'
import { Null, Table, vectorFromArray } from 'apache-arrow'
import { ArrowFrame } from './arrow-data'
import { metricAggregateView } from './arrow-metrics'
import { columnView } from './arrow-record'
import { telemetryDataset } from '@/test/native-telemetry-fixtures'

describe('native metric accessors', () => {
  it('retains aggregate counts beyond uint64 and omits unavailable extents', () => {
    const count = (1n << 65n) + 1n
    const frame = new ArrowFrame(
      telemetryDataset('getMetricAggregateView', {
        aggregate: [
          {
            timestamp: '18446744073709551615',
            startTime: '0',
            count: count.toString(),
            sum: null,
            bucketCounts: [count.toString()],
            explicitBounds: [],
            quantiles: null,
          },
        ],
        scalarAggregate: null,
      })
    )
    const view = metricAggregateView(frame)
    const aggregate = view.aggregate![0]
    expect(aggregate.count).toBe(count)
    expect(aggregate.bucketCounts).toEqual([count])
    expect(aggregate.timestamp).toBe(18446744073709551615n)
    expect(aggregate).not.toHaveProperty('min')
    expect(aggregate).not.toHaveProperty('max')
    expect(aggregate.sum).toBeNull()
  })

  it('accepts a native empty-list branch with no element schema', () => {
    const frame = new ArrowFrame(vectorFromArray([], new Null()))
    expect(frame.length).toBe(0)
    expect(() => new ArrowFrame(vectorFromArray([null], new Null()))).toThrow(
      'Expected Arrow row columns'
    )
  })

  it('creates enumerable getters without visiting a column', () => {
    const frame = new ArrowFrame(new Table({ value: vectorFromArray([1]) }))
    const read = vi.spyOn(frame, 'cell')
    const view = columnView(frame, 0, { value: cell => cell.number() })
    expect(Object.keys(view)).toEqual(['value'])
    expect(read).not.toHaveBeenCalled()
    expect(view.value).toBe(1)
    expect(read).toHaveBeenCalledOnce()
  })
})
