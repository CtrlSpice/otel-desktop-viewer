import { afterEach, describe, expect, it, vi } from 'vitest'
import { telemetryAPI, JsonRpcError } from './telemetry-service'
import type { MetricFixture } from '@/test/telemetry-fixture-types'
import type { ReceivedHistogramDataPoint } from '@/types/api-types'
import {
  stubRpcResponse,
  stubRpcResult,
} from '@/test/telemetry-service-fixtures'

function metricResult(overrides: Partial<MetricFixture> = {}): MetricFixture {
  return {
    lastSeenNs: null,
    metricRef: 'some-metric',
    name: 'test.gauge',
    description: '',
    metadata: [],
    unit: '1',
    metricType: 'Gauge',
    aggregationTemporalityCode: null,
    aggregationTemporality: null,
    isMonotonic: null,
    resourceDroppedAttributesCount: 0,
    resourceSchemaUrl: '',
    resource: { attributes: [], droppedAttributesCount: 0 },
    scopeName: '',
    scopeSchemaUrl: '',
    scopeVersion: '',
    scopeDroppedAttributesCount: 0,
    scope: { name: '', version: '', attributes: [], droppedAttributesCount: 0 },
    timeseries: [],
    aggregate: null,
    scalarAggregate: null,
    datapointCount: 0,
    boundsMismatch: null,
    window: {
      requested: { startNs: null, endNs: null },
      effective: { startNs: null, endNs: null },
    },
    ...overrides,
  }
}

afterEach(() => {
  vi.unstubAllGlobals()
  vi.restoreAllMocks()
})

describe('telemetryAPI.getMetricView', () => {
  it('returns null when the backend reports metric not found (-32003)', async () => {
    stubRpcResponse({
      jsonrpc: '2.0',
      error: { code: -32003, message: 'Metric not found' },
    })
    await expect(
      telemetryAPI.getMetricView('some-metric', 0n, 1n)
    ).resolves.toBeNull()
  })

  it('rethrows JSON-RPC errors other than metric not found', async () => {
    stubRpcResponse({
      jsonrpc: '2.0',
      error: { code: -32009, message: 'Invalid Metric reference' },
    })
    const call = telemetryAPI.getMetricView('not-a-metric', 0n, 1n)
    await expect(call).rejects.toBeInstanceOf(JsonRpcError)
    await expect(call).rejects.toMatchObject({ code: -32009 })
  })

  it('parses a successful result into MetricViewData', async () => {
    stubRpcResult(
      metricResult({
        unit: 'bytes',
        resourceSchemaUrl: 'https://example.test/resource/1.0',
        scopeSchemaUrl: 'https://example.test/scope/1.0',
        window: {
          requested: { startNs: null, endNs: null },
          effective: { startNs: '10', endNs: '20' },
        },
      })
    )
    const metric = await telemetryAPI.getMetricView('some-metric', 0n, 1n)
    expect(metric).not.toBeNull()
    expect(metric!.name).toBe('test.gauge')
    expect(metric!.resourceSchemaUrl).toBe('https://example.test/resource/1.0')
    expect(metric!.scopeSchemaUrl).toBe('https://example.test/scope/1.0')
    expect(metric!.timeseries).toEqual([])
    expect(metric!.window).toEqual({
      requested: { startNs: null, endNs: null },
      effective: { startNs: 10n, endNs: 20n },
    })
  })

  it('revives typed exemplar values and chart breaks without losing int64 precision', async () => {
    stubRpcResult(
      metricResult({
        name: 'test.gauge',
        unit: '1',
        metricType: 'Gauge',
        timeseries: [
          {
            seriesRef: 'series-1',
            attributes: [],
            resource: { attributes: [], droppedAttributesCount: 0 },
            datapoints: [
              {
                id: 'dp-1',
                timestamp: '100',
                breakBefore: true,
                timestampMs: 0,
                startTime: '0',
                flags: 0,
                metricType: 'Gauge',
                doubleValue: -0,
                intValue: null,
                valueType: 'Double',
                exemplars: [
                  {
                    timestamp: '101',
                    valueType: 'Int',
                    doubleValue: null,
                    intValue: '9223372036854775807',
                    traceID: null,
                    spanID: null,
                    filteredAttributes: [],
                  },
                  {
                    timestamp: '102',
                    valueType: 'Double',
                    doubleValue: 1.25,
                    intValue: null,
                    traceID: null,
                    spanID: null,
                    filteredAttributes: [],
                  },
                  {
                    timestamp: '103',
                    valueType: 'Empty',
                    doubleValue: null,
                    intValue: null,
                    traceID: null,
                    spanID: null,
                    filteredAttributes: [],
                  },
                  {
                    timestamp: '104',
                    valueType: 'Double',
                    doubleValue: NaN,
                    intValue: null,
                    traceID: null,
                    spanID: null,
                    filteredAttributes: [],
                  },
                  {
                    timestamp: '105',
                    valueType: 'Double',
                    doubleValue: Infinity,
                    intValue: null,
                    traceID: null,
                    spanID: null,
                    filteredAttributes: [],
                  },
                  {
                    timestamp: '106',
                    valueType: 'Double',
                    doubleValue: -Infinity,
                    intValue: null,
                    traceID: null,
                    spanID: null,
                    filteredAttributes: [],
                  },
                  {
                    timestamp: '107',
                    valueType: 'Double',
                    doubleValue: NaN,
                    intValue: null,
                    traceID: null,
                    spanID: null,
                    filteredAttributes: [],
                  },
                ],
              },
            ],
            stats: null,
            datapointCount: 1,
            lastSeenNs: '100',
            views: null,
            rateStats: null,
            sparkline: [
              { timestamp: '0', value: 0 },
              { timestamp: '100', value: 1, breakBefore: true },
            ],
          },
        ],
        window: {
          requested: { startNs: null, endNs: null },
          effective: { startNs: '100', endNs: '100' },
        },
      })
    )

    const metric = await telemetryAPI.getMetricView('some-metric', 0n, 1n)
    expect(metric!.timeseries[0]!.datapoints[0]).toMatchObject({
      intValue: null,
    })
    const datapoint = metric!.timeseries[0]!.datapoints[0]!
    expect(datapoint.metricType).toBe('Gauge')
    if (datapoint.metricType !== 'Gauge') throw new Error('expected Gauge')
    expect(datapoint.breakBefore).toBe(true)
    expect(metric!.timeseries[0]!.sparkline![0]).not.toHaveProperty(
      'breakBefore'
    )
    expect(metric!.timeseries[0]!.sparkline![1]!.breakBefore).toBe(true)
    expect(Object.is(datapoint.doubleValue, -0)).toBe(true)
    const exemplars = metric!.timeseries[0]!.datapoints[0]!.exemplars
    expect(exemplars).toEqual([
      expect.objectContaining({
        timestamp: 101n,
        valueType: 'Int',
        doubleValue: null,
        intValue: 9_223_372_036_854_775_807n,
      }),
      expect.objectContaining({
        timestamp: 102n,
        valueType: 'Double',
        doubleValue: 1.25,
        intValue: null,
      }),
      expect.objectContaining({
        timestamp: 103n,
        valueType: 'Empty',
        doubleValue: null,
        intValue: null,
      }),
      expect.objectContaining({
        timestamp: 104n,
        valueType: 'Double',
        doubleValue: Number.NaN,
        intValue: null,
      }),
      expect.objectContaining({
        timestamp: 105n,
        valueType: 'Double',
        doubleValue: Number.POSITIVE_INFINITY,
        intValue: null,
      }),
      expect.objectContaining({
        timestamp: 106n,
        valueType: 'Double',
        doubleValue: Number.NEGATIVE_INFINITY,
        intValue: null,
      }),
      // JavaScript exposes both payloads as NaN; stored/wire bit identity is
      // asserted by the store tests rather than inferred from Number.NaN.
      expect.objectContaining({
        timestamp: 107n,
        valueType: 'Double',
        doubleValue: Number.NaN,
        intValue: null,
      }),
    ])
  })

  it('revives received metric numeric fields without losing precision', async () => {
    const wire = metricResult({
      timeseries: [
        {
          seriesRef: 'series-1',
          attributes: [],
          resource: { attributes: [], droppedAttributesCount: 0 },
          datapoints: [
            {
              id: 'gauge-min',
              timestamp: '100',
              timestampMs: 0,
              startTime: '0',
              flags: 0,
              exemplars: [],
              metricType: 'Gauge',
              doubleValue: null,
              intValue: '-9223372036854775808',
              valueType: 'Int',
            },
            {
              id: 'sum-max',
              timestamp: '101',
              timestampMs: 0,
              startTime: '0',
              flags: 0,
              exemplars: [],
              metricType: 'Sum',
              doubleValue: null,
              intValue: '9223372036854775807',
              valueType: 'Int',
              isMonotonic: true,
              aggregationTemporalityCode: 2,
              aggregationTemporality: 'Cumulative',
              delta: '18446744073709551615',
              isReset: false,
            },
            {
              id: 'histogram-max',
              timestamp: '102',
              timestampMs: 0,
              startTime: '0',
              flags: 0,
              exemplars: [],
              metricType: 'Histogram',
              count: '18446744073709551615',
              sum: NaN,
              min: -Infinity,
              max: Infinity,
              bucketCounts: ['0', '9007199254740993', '18446744073709551615'],
              explicitBounds: [-0, 1],
              quantiles: { '0.5': Infinity },
              aggregationTemporalityCode: -1,
              aggregationTemporality: 'Unknown (-1)',
            },
            {
              id: 'exponential-max',
              timestamp: '103',
              timestampMs: 0,
              startTime: '0',
              flags: 0,
              exemplars: [],
              metricType: 'ExponentialHistogram',
              count: '18446744073709551615',
              sum: 1,
              min: 1,
              max: 1,
              scale: 0,
              zeroCount: '9007199254740993',
              zeroThreshold: -0,
              positiveBucketOffset: 0,
              positiveBucketCounts: ['18446744073709551615'],
              negativeBucketOffset: 0,
              negativeBucketCounts: [],
              quantiles: null,
              aggregationTemporalityCode: 1,
              aggregationTemporality: 'Delta',
            },
            {
              id: 'sum-overflow',
              timestamp: '104',
              timestampMs: 0,
              startTime: '0',
              flags: 0,
              exemplars: [],
              metricType: 'Sum',
              doubleValue: 1,
              intValue: null,
              valueType: 'Double',
              isMonotonic: true,
              aggregationTemporalityCode: 2,
              aggregationTemporality: 'Cumulative',
              delta: Infinity,
              isReset: false,
            },
          ],
          stats: null,
          datapointCount: 5,
          lastSeenNs: '104',
          views: null,
          rateStats: null,
          sparkline: null,
        },
      ],
    })
    stubRpcResult(wire)

    const metric = await telemetryAPI.getMetricView('some-metric', 0n, 1n)
    const datapoints = metric!.timeseries[0]!.datapoints

    expect(datapoints[0]).toMatchObject({
      intValue: -9_223_372_036_854_775_808n,
    })
    expect(datapoints[1]).toMatchObject({
      intValue: 9_223_372_036_854_775_807n,
      delta: 18_446_744_073_709_551_615n,
    })
    expect(datapoints[2]).toMatchObject({
      count: 18_446_744_073_709_551_615n,
      bucketCounts: [0n, 9_007_199_254_740_993n, 18_446_744_073_709_551_615n],
      aggregationTemporalityCode: -1,
      aggregationTemporality: 'Unknown (-1)',
    })
    const histogram = datapoints[2]!
    if (histogram.metricType !== 'Histogram') {
      throw new Error('expected Histogram')
    }
    expect(Number.isNaN(histogram.sum)).toBe(true)
    expect(histogram.min).toBe(Number.NEGATIVE_INFINITY)
    expect(histogram.max).toBe(Number.POSITIVE_INFINITY)
    expect(Object.is(histogram.explicitBounds[0], -0)).toBe(true)
    expect(histogram.quantiles).toEqual({
      '0.5': Number.POSITIVE_INFINITY,
    })
    expect(datapoints[3]).toMatchObject({
      count: 18_446_744_073_709_551_615n,
      zeroCount: 9_007_199_254_740_993n,
      positiveBucketCounts: [18_446_744_073_709_551_615n],
      negativeBucketCounts: [],
    })
    const exponential = datapoints[3]!
    if (exponential.metricType !== 'ExponentialHistogram') {
      throw new Error('expected ExponentialHistogram')
    }
    expect(Object.is(exponential.zeroThreshold, -0)).toBe(true)
    expect(datapoints[4]).toMatchObject({
      delta: Number.POSITIVE_INFINITY,
    })
    expect(wire.timeseries[0]!.datapoints[2]).toMatchObject({
      count: '18446744073709551615',
      bucketCounts: ['0', '9007199254740993', '18446744073709551615'],
    })
  })

  it('keeps absent histogram statistics distinct from present zero', async () => {
    const base = {
      timestampMs: 0,
      startTime: '0',
      flags: 0,
      metricType: 'Histogram' as const,
      count: '0',
      bucketCounts: ['0'],
      explicitBounds: [],
      quantiles: null,
      aggregationTemporalityCode: 1,
      aggregationTemporality: 'Delta',
      exemplars: [],
    }
    stubRpcResult(
      metricResult({
        metricType: 'Histogram',
        timeseries: [
          {
            seriesRef: 'series-1',
            attributes: [],
            resource: { attributes: [], droppedAttributesCount: 0 },
            datapoints: [
              {
                ...base,
                id: 'absent',
                timestamp: '1',
                sum: null,
                min: null,
                max: null,
              },
              {
                ...base,
                id: 'zero',
                timestamp: '2',
                sum: 0,
                min: 0,
                max: 0,
              },
            ],
            stats: null,
            datapointCount: 2,
            lastSeenNs: '2',
            views: null,
            rateStats: null,
            sparkline: null,
          },
        ],
      })
    )

    const metric = await telemetryAPI.getMetricView('some-metric', 0n, 2n)
    const [absent, zero] = metric!.timeseries[0]!.datapoints
    expect(absent).toMatchObject({ sum: null, min: null, max: null })
    expect(zero).toMatchObject({ sum: 0, min: 0, max: 0 })
  })

  it.each(['Histogram', 'ExponentialHistogram'] as const)(
    'loads received %s counts when optional quantiles were not computed',
    async metricType => {
      stubRpcResult(
        metricResult({
          metricType,
          timeseries: [
            {
              seriesRef: 'series-1',
              attributes: [],
              resource: { attributes: [], droppedAttributesCount: 0 },
              datapointCount: 1,
              lastSeenNs: '2',
              stats: null,
              views: null,
              rateStats: null,
              sparkline: null,
              datapoints: [
                {
                  id: 'received',
                  timestamp: '2',
                  timestampMs: 0,
                  startTime: '1',
                  flags: 0,
                  exemplars: [],
                  count: '9007199254740993',
                  sum: null,
                  min: 0,
                  max: null,
                  aggregationTemporalityCode: 2,
                  aggregationTemporality: 'Cumulative',
                  ...(metricType === 'Histogram'
                    ? {
                        metricType,
                        explicitBounds: [],
                        bucketCounts: ['9007199254740993'],
                      }
                    : {
                        metricType,
                        scale: 0,
                        zeroCount: '9007199254740993',
                        zeroThreshold: 0,
                        positiveBucketOffset: 0,
                        positiveBucketCounts: [],
                        negativeBucketOffset: 0,
                        negativeBucketCounts: [],
                      }),
                },
              ],
            },
          ],
        })
      )
      const metric = await telemetryAPI.getMetricView(
        'some-metric',
        null,
        null,
        0
      )
      expect(metric!.timeseries[0]!.datapoints[0]).toMatchObject({
        id: 'received',
        metricType,
        timestamp: 2n,
        count: 9007199254740993n,
        sum: null,
        min: 0,
        max: null,
        quantiles: null,
      })
    }
  )

  it('decodes recursive attribute int64 values and exceptional double bits once', async () => {
    stubRpcResult(
      metricResult({
        metadata: [
          {
            id: 'metadata-1',
            key: 'payload',
            value: {
              kind: 'map',
              value: [
                {
                  key: 'count',
                  value: { kind: 'int64', value: '9223372036854775807' },
                },
                {
                  key: 'negativeZero',
                  value: { kind: 'double', value: -0 },
                },
                {
                  key: 'list',
                  value: {
                    kind: 'array',
                    value: [{ kind: 'double', value: Infinity }],
                  },
                },
              ],
            },
          },
        ],
      })
    )

    const value = (await telemetryAPI.getMetricView('some-metric', 0n, 1n))!
      .metadata[0]!.value
    expect(value).toMatchObject({ kind: 'map' })
    if (value.kind !== 'map') throw new Error('Expected a map value')
    expect(value.value.find(entry => entry.key === 'count')!.value).toEqual({
      kind: 'int64',
      value: 9_223_372_036_854_775_807n,
    })
    const zero = value.value.find(entry => entry.key === 'negativeZero')!.value
    expect(zero.kind).toBe('double')
    expect(Object.is(zero.value, -0)).toBe(true)
    const list = value.value.find(entry => entry.key === 'list')!.value
    if (list.kind !== 'array') throw new Error('Expected an array value')
    expect(list.value[0]!.value).toBe(Number.POSITIVE_INFINITY)
  })

  it('retains NaN in scalar and nested native values', async () => {
    stubRpcResult(
      metricResult({
        metadata: [
          {
            id: 'metadata-1',
            key: 'payload',
            value: { kind: 'double', value: NaN },
          },
          {
            id: 'metadata-3',
            key: 'nested',
            value: {
              kind: 'map',
              value: [
                {
                  key: 'payload',
                  value: { kind: 'double', value: NaN },
                },
              ],
            },
          },
        ],
      })
    )

    const metadata = (await telemetryAPI.getMetricView('some-metric', 0n, 1n))!
      .metadata
    expect(metadata.find(entry => entry.key === 'payload')!.value).toEqual({
      kind: 'double',
      value: NaN,
    })
    expect(metadata.find(entry => entry.key === 'nested')!.value).toMatchObject(
      {
        kind: 'map',
        value: [{ key: 'payload', value: { kind: 'double', value: NaN } }],
      }
    )
  })
})

describe('exact received metric API', () => {
  it('decodes exact identity and computed series summaries', async () => {
    stubRpcResult({
      metricRef: 'metric-1',
      name: 'requests',
      description: '',
      unit: '1',
      metadata: [],
      metricType: 'Sum',
      aggregationTemporalityCode: 2,
      isMonotonic: true,
      resource: {
        droppedAttributesCount: 0,
        schemaUrl: '',
        attributes: [
          {
            key: 'nested',
            value: {
              kind: 'map',
              value: [
                {
                  key: 'limit',
                  value: { kind: 'int64', value: '9007199254740993' },
                },
              ],
            },
          },
        ],
      },
      scope: {
        name: 'sdk',
        version: '1',
        attributes: [],
        droppedAttributesCount: 0,
        schemaUrl: 'scope-schema',
      },
      series: [
        {
          seriesRef: 'series-1',
          attributes: [],
          datapointCount: '18446744073709551615',
          firstDatapointTimestamp: '1',
          lastDatapointTimestamp: null,
        },
      ],
    })

    const metric = await telemetryAPI.getMetric('metric-1')
    expect(metric).toMatchObject({
      metricRef: 'metric-1',
      aggregationTemporalityCode: 2,
      isMonotonic: true,
    })
    expect(metric!.series[0]).toMatchObject({
      datapointCount: 18446744073709551615n,
      firstDatapointTimestamp: 1n,
      lastDatapointTimestamp: null,
    })
    expect(metric!.resource.attributes[0]!.value).toMatchObject({
      kind: 'map',
      value: [
        { key: 'limit', value: { kind: 'int64', value: 9007199254740993n } },
      ],
    })
  })

  it('decodes exact histogram values, all exemplars, and optional presence', async () => {
    stubRpcResult({
      metricRef: 'metric-1',
      name: 'latency',
      description: 'received report',
      unit: 'ms',
      metadata: [],
      metricType: 'Histogram',
      aggregationTemporalityCode: 1,
      resource: {
        attributes: [],
        droppedAttributesCount: 2,
        schemaUrl: 'resource-schema',
      },
      scope: {
        name: 'sdk',
        version: '1',
        attributes: [],
        droppedAttributesCount: 3,
        schemaUrl: 'scope-schema',
      },
      seriesRef: 'series-1',
      attributes: [],
      datapoints: [
        {
          datapointRef: 'point-1',
          timestamp: '18446744073709551615',
          startTime: '0',
          flags: 1,
          count: '18446744073709551615',
          sum: 0,
          max: -0,
          bucketCounts: ['18446744073709551615'],
          explicitBounds: [Infinity],
          exemplars: [
            {
              timestamp: '9',
              valueType: 'Int',
              intValue: '-9223372036854775808',
              doubleValue: null,
              traceID: null,
              spanID: null,
              filteredAttributes: [],
            },
            {
              timestamp: '10',
              valueType: 'Double',
              intValue: null,
              doubleValue: -0,
              traceID: '00000000000000000000000000000001',
              spanID: '0000000000000001',
              filteredAttributes: [],
            },
          ],
        },
      ],
    })

    const selected = await telemetryAPI.getMetricSeries(
      'metric-1',
      'series-1',
      null,
      18446744073709551615n
    )
    const point = selected!.datapoints[0]! as ReceivedHistogramDataPoint
    expect(point).toMatchObject({
      datapointRef: 'point-1',
      timestamp: 18446744073709551615n,
      count: 18446744073709551615n,
      sum: 0,
      bucketCounts: [18446744073709551615n],
    })
    expect('min' in point).toBe(false)
    expect(Object.is(point.max, -0)).toBe(true)
    expect(point.exemplars).toHaveLength(2)
    expect(point.exemplars[0]).toMatchObject({
      intValue: -9223372036854775808n,
    })
    expect(Object.is(point.exemplars[1]!.doubleValue, -0)).toBe(true)
  })

  it('decodes every received number union arm', async () => {
    stubRpcResult({
      metricRef: 'metric-1',
      name: 'counter',
      description: '',
      unit: '1',
      metadata: [],
      metricType: 'Sum',
      aggregationTemporalityCode: 2,
      isMonotonic: false,
      resource: { attributes: [], droppedAttributesCount: 0, schemaUrl: '' },
      scope: {
        name: '',
        version: '',
        attributes: [],
        droppedAttributesCount: 0,
        schemaUrl: '',
      },
      seriesRef: 'series-1',
      attributes: [],
      datapoints: [
        {
          datapointRef: 'i',
          timestamp: '1',
          startTime: '0',
          flags: 0,
          exemplars: [],
          valueType: 'Int',
          intValue: '9223372036854775807',
          doubleValue: null,
        },
        {
          datapointRef: 'd',
          timestamp: '2',
          startTime: '0',
          flags: 0,
          exemplars: [],
          valueType: 'Double',
          intValue: null,
          doubleValue: NaN,
        },
        {
          datapointRef: 'e',
          timestamp: '3',
          startTime: '0',
          flags: 0,
          exemplars: [],
          valueType: 'Empty',
          intValue: null,
          doubleValue: null,
        },
      ],
    })

    const selected = await telemetryAPI.getMetricSeries(
      'metric-1',
      'series-1',
      null,
      null
    )
    const points = selected!.datapoints
    expect(points.map(point => point.datapointRef)).toEqual(['i', 'd', 'e'])
    expect(points.every(point => !('datapointID' in point))).toBe(true)
    expect(points[0]).toMatchObject({
      valueType: 'Int',
      intValue: 9223372036854775807n,
    })
    expect(
      Number.isNaN((points[1] as { doubleValue: number }).doubleValue)
    ).toBe(true)
    expect(points[2]).toMatchObject({
      valueType: 'Empty',
      intValue: null,
      doubleValue: null,
    })
  })

  it('decodes received exponential histogram buckets without chart fields', async () => {
    stubRpcResult({
      metricRef: 'metric-1',
      name: 'distribution',
      description: '',
      unit: '1',
      metadata: [],
      metricType: 'ExponentialHistogram',
      aggregationTemporalityCode: 1,
      resource: { attributes: [], droppedAttributesCount: 0, schemaUrl: '' },
      scope: {
        name: '',
        version: '',
        attributes: [],
        droppedAttributesCount: 0,
        schemaUrl: '',
      },
      seriesRef: 'series-1',
      attributes: [],
      datapoints: [
        {
          datapointRef: 'point-1',
          timestamp: '1',
          startTime: '0',
          flags: 0,
          exemplars: [],
          count: '18446744073709551615',
          scale: -10,
          zeroCount: '9007199254740993',
          zeroThreshold: -0,
          positive: {
            offset: -2,
            bucketCounts: ['1', '18446744073709551615'],
          },
          negative: { offset: 3, bucketCounts: [] },
        },
      ],
    })

    const selected = await telemetryAPI.getMetricSeries(
      'metric-1',
      'series-1',
      0n,
      1n
    )
    expect(selected!.datapoints[0]).toMatchObject({
      datapointRef: 'point-1',
      count: 18446744073709551615n,
      scale: -10,
      zeroCount: 9007199254740993n,
      positive: { offset: -2, bucketCounts: [1n, 18446744073709551615n] },
      negative: { offset: 3, bucketCounts: [] },
    })
  })
})

describe('telemetryAPI.getMetricAggregateView', () => {
  it('decodes aggregate doubles while preserving omitted extents', async () => {
    stubRpcResult({
      aggregate: [
        {
          timestamp: '100',
          startTime: '0',
          count: 0,
          sum: -0,
          bucketCounts: [0, 0],
          explicitBounds: [Infinity],
          quantiles: { '0.5': NaN },
        },
      ],
      scalarAggregate: null,
    })

    const result = await telemetryAPI.getMetricAggregateView(
      'some-metric',
      0n,
      1n,
      1,
      null,
      [0.5],
      0
    )

    const aggregate = result!.aggregate![0]!
    expect(Object.is(aggregate.sum, -0)).toBe(true)
    expect(aggregate.explicitBounds).toEqual([Number.POSITIVE_INFINITY])
    expect(Number.isNaN(aggregate.quantiles!['0.5'])).toBe(true)
    expect(aggregate).not.toHaveProperty('min')
    expect(aggregate).not.toHaveProperty('max')
  })
})

describe('telemetryAPI metric bigint boundary', () => {
  it('keeps only the established lastSeenNs missing-field normalizations', async () => {
    // SAFETY: This fixture intentionally omits typed lastSeenNs fields to exercise their parser normalization.
    const result = metricResult({
      timeseries: [
        {
          seriesRef: 'series-1',
          attributes: [],
          resource: { attributes: [], droppedAttributesCount: 0 },
          datapoints: [],
          stats: null,
          datapointCount: 0,
          lastSeenNs: undefined as never,
          views: [
            {
              bucketStart: '9223372036854775807',
              sampleCount: 1,
              sum: 1,
              avg: 1,
              rate: 1,
              slope: null,
              hasReset: false,
            },
          ],
          rateStats: null,
          sparkline: [{ timestamp: '-1', value: 1 }],
        },
      ],
    }) as { lastSeenNs?: unknown; timeseries: { lastSeenNs?: unknown }[] }
    delete result.lastSeenNs
    delete result.timeseries[0]!.lastSeenNs
    stubRpcResult(result)

    const metric = await telemetryAPI.getMetricView('some-metric', 0n, 1n)
    expect(metric!.lastSeenNs).toBeNull()
    expect(metric!.timeseries[0]!.lastSeenNs).toBeNull()
    expect(metric!.timeseries[0]!.views![0]!.bucketStart).toBe(
      9_223_372_036_854_775_807n
    )
    expect(metric!.timeseries[0]!.sparkline![0]!.timestamp).toBe(-1n)
  })
})
