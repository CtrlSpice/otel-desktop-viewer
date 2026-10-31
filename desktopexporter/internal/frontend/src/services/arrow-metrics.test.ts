import { describe, expect, it, vi } from 'vitest'
import {
  Field,
  List,
  Struct,
  Utf8,
  Null,
  Table,
  vectorFromArray,
} from 'apache-arrow'
import { ArrowFrame } from './arrow-data'
import { metricAggregateView, metricView } from './arrow-metrics'
import { columnView, cachedColumnView } from './arrow-record'
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
    expect(view.aggregate === view.aggregate).toBe(true)
    expect(aggregate.bucketCounts === aggregate.bucketCounts).toBe(true)
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

  it('memoizes immutable response collections without decoding their scalars', () => {
    const type = new Struct([new Field('seriesRef', new Utf8())])
    const frame = new ArrowFrame(
      new Table({
        metricType: vectorFromArray(['Gauge'], new Utf8()),
        resource: vectorFromArray(
          [{ dummy: '' }],
          new Struct([new Field('dummy', new Utf8())])
        ),
        timeseries: vectorFromArray(
          [Array.from({ length: 5000 }, (_, i) => ({ seriesRef: String(i) }))],
          new List(new Field('item', type))
        ),
      })
    )
    const m = metricView(frame)
    const read = vi.spyOn(frame, 'cell')
    const first = m.timeseries
    read.mockClear()
    expect(m.timeseries === first).toBe(true)
    expect(m.timeseries[0] === first[0]).toBe(true)
    expect(read).not.toHaveBeenCalled()
    expect(Object.isFrozen(first)).toBe(true)
    expect(first[0].seriesRef).toBe('0')
    // Missing scalar columns are harmless until read; a new response owns new wrappers.
    expect(() => first[0].datapointCount).toThrow()
    expect(metricView(frame).timeseries === first).toBe(false)
  })

  it('caches null and empty collections only after their first read', () => {
    const frame = new ArrowFrame(new Table({ value: vectorFromArray([null]) }))
    const read = vi.fn(() => null)
    const view = cachedColumnView(frame, 0, { value: read })
    expect(read).not.toHaveBeenCalled()
    expect(view.value).toBeNull()
    expect(view.value).toBeNull()
    expect(read).toHaveBeenCalledOnce()
  })
})
