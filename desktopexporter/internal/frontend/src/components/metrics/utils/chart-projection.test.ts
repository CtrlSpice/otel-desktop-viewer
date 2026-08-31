import { describe, expect, it } from 'vitest'
import {
  rateBucketStartForSourceDatapoint,
  timeseriesToChartTimeseries,
} from './chart-projection'
import type { MetricTimeseries } from '@/types/api-types'

describe('timeseriesToChartTimeseries', () => {
  it('preserves exact source identity when Date coordinates collide', () => {
    const base = 1_700_000_000_000_000_000n
    const timeseries = [
      {
        attributesKey: 'series-a',
        attributes: [],
        resource: { attributes: [], droppedAttributesCount: 0 },
        datapoints: [
          {
            id: 'later',
            timestamp: base + 900n,
            timestampMs: 1_700_000_000_000,
            startTime: base,
            flags: 0,
            exemplars: [],
            metricType: 'Gauge',
            doubleValue: 2,
            intValue: null,
            valueType: 'double',
          },
          {
            id: 'earlier',
            timestamp: base + 100n,
            timestampMs: 1_700_000_000_000,
            startTime: base,
            flags: 0,
            exemplars: [],
            metricType: 'Gauge',
            doubleValue: 1,
            intValue: null,
            valueType: 'double',
          },
        ],
        stats: null,
        datapointCount: 2,
        lastSeenNs: base + 900n,
        views: null,
        rateStats: null,
        sparkline: null,
      },
    ] satisfies MetricTimeseries[]

    const [line] = timeseriesToChartTimeseries(timeseries).chartTimeseries

    expect(line!.points.map(point => point.date.getTime())).toEqual([
      1_700_000_000_000, 1_700_000_000_000,
    ])
    expect(
      line!.points.map(point => [point.sourceDatapointID, point.timestampNs])
    ).toEqual([
      ['earlier', base + 100n],
      ['later', base + 900n],
    ])
  })

  it('maps an exact same-ms raw identity onto its synthetic rate bucket', () => {
    const base = 1_700_000_000_000_000_000n
    const series = {
      attributesKey: 'series-a',
      attributes: [],
      resource: { attributes: [], droppedAttributesCount: 0 },
      datapoints: [
        {
          id: 'later',
          timestamp: base + 900n,
          timestampMs: 1_700_000_000_000,
          startTime: base,
          flags: 0,
          exemplars: [],
          metricType: 'Sum',
          doubleValue: 9,
          intValue: null,
          valueType: 'double',
          isMonotonic: true,
          aggregationTemporality: 'Cumulative',
        },
        {
          id: 'middle',
          timestamp: base + 400n,
          timestampMs: 1_700_000_000_000,
          startTime: base,
          flags: 0,
          exemplars: [],
          metricType: 'Sum',
          doubleValue: 5,
          intValue: null,
          valueType: 'double',
          isMonotonic: true,
          aggregationTemporality: 'Cumulative',
        },
        {
          id: 'first',
          timestamp: base + 100n,
          timestampMs: 1_700_000_000_000,
          startTime: base,
          flags: 0,
          exemplars: [],
          metricType: 'Sum',
          doubleValue: 1,
          intValue: null,
          valueType: 'double',
          isMonotonic: true,
          aggregationTemporality: 'Cumulative',
        },
      ],
      stats: null,
      datapointCount: 3,
      lastSeenNs: base + 900n,
      views: [
        {
          bucketStart: base,
          sampleCount: 1,
          sum: 1,
          avg: 1,
          rate: null,
          slope: null,
          hasReset: false,
        },
        {
          bucketStart: base + 300n,
          sampleCount: 1,
          sum: 5,
          avg: 5,
          rate: 2,
          slope: null,
          hasReset: false,
        },
        {
          bucketStart: base + 600n,
          sampleCount: 1,
          sum: 9,
          avg: 9,
          rate: 4,
          slope: 6,
          hasReset: false,
        },
      ],
      rateStats: { min: 2, max: 4, avg: 3 },
      sparkline: null,
    } satisfies MetricTimeseries

    expect(rateBucketStartForSourceDatapoint(series, 'later')).toBe(base + 600n)
    expect(rateBucketStartForSourceDatapoint(series, 'middle')).toBe(
      base + 300n
    )
    expect(rateBucketStartForSourceDatapoint(series, 'first')).toBeUndefined()
    expect(
      rateBucketStartForSourceDatapoint(series, 'raw-only-at-same-time')
    ).toBeUndefined()
  })
})
