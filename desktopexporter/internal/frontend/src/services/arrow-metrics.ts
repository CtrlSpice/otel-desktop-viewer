import type {
  ExactMetric,
  ExactMetricIdentity,
  ExactMetricResource,
  ExactMetricScope,
  ExactMetricSeries,
  Exemplar,
  MetricSeriesSummary,
  ReceivedNumberDataPoint,
  ReceivedHistogramDataPoint,
  ReceivedExponentialHistogramDataPoint,
  DataPoint,
  ScalarViewBucket,
  MetricSeriesViewData,
  MetricViewData,
  MetricAggregateViewData,
  AggregateBucket,
} from '@/types/api-types'
import { ArrowCell, ArrowFrame } from './arrow-data'
import { attributes, rowViews } from './arrow-values'
import {
  cachedColumnView as columnView,
  extendView,
  memoize,
  text,
  number,
  integer,
  boolean,
  nullableNumber,
  nullableInteger,
  nullableText,
  listValues,
} from './arrow-record'

function resource(cell: ArrowCell): ExactMetricResource {
  return columnView(cell.struct(), 0, {
    attributes,
    droppedAttributesCount: number,
    schemaUrl: text,
  })
}
function scope(cell: ArrowCell): ExactMetricScope {
  return columnView(cell.struct(), 0, {
    name: text,
    version: text,
    attributes,
    droppedAttributesCount: number,
    schemaUrl: text,
  })
}

function identity(frame: ArrowFrame): ExactMetricIdentity {
  const base = columnView(frame, 0, {
    metricRef: text,
    name: text,
    description: text,
    unit: text,
    metadata: attributes,
    resource,
    scope,
  })
  const metricType = frame.cell('metricType', 0).text()
  switch (metricType) {
    case 'Gauge':
      return extendView(base, { metricType })
    case 'Sum':
      return extendView(base, {
        metricType,
        get aggregationTemporalityCode() {
          return frame.cell('aggregationTemporalityCode', 0).number()
        },
        get isMonotonic() {
          return frame.cell('isMonotonic', 0).boolean()
        },
      })
    case 'Histogram':
    case 'ExponentialHistogram':
      return extendView(base, {
        metricType,
        get aggregationTemporalityCode() {
          return frame.cell('aggregationTemporalityCode', 0).number()
        },
      })
    default:
      throw new Error(`Unsupported metric type: ${metricType}`)
  }
}

function seriesSummary(frame: ArrowFrame, row: number): MetricSeriesSummary {
  return columnView(frame, row, {
    seriesRef: text,
    attributes,
    datapointCount: integer,
    firstDatapointTimestamp: nullableInteger,
    lastDatapointTimestamp: nullableInteger,
  })
}

export function metricSeriesOrder(frame: ArrowFrame): string[] {
  return rowViews(frame, (frame, row) => text(frame.cell('seriesRef', row)))
}

export function exactMetric(frame: ArrowFrame): ExactMetric {
  return extendView(
    identity(frame),
    columnView(frame, 0, {
      series: cell => rowViews(new ArrowFrame(cell.list()), seriesSummary),
    })
  )
}

function numberValue(frame: ArrowFrame, row: number) {
  const valueType = frame.cell('valueType', row).text()
  switch (valueType) {
    case 'Int':
      return {
        valueType,
        get intValue() {
          return frame.cell('intValue', row).integer()
        },
        doubleValue: null,
      }
    case 'Double':
      return {
        valueType,
        intValue: null,
        get doubleValue() {
          return frame.cell('doubleValue', row).number()
        },
      }
    case 'Empty':
      return { valueType, intValue: null, doubleValue: null }
    default:
      throw new Error(`Unsupported metric value type: ${valueType}`)
  }
}

function exemplar(frame: ArrowFrame, row: number): Exemplar {
  return extendView(
    columnView(frame, row, {
      timestamp: integer,
      traceID: nullableText,
      spanID: nullableText,
      filteredAttributes: attributes,
    }),
    numberValue(frame, row)
  )
}
function exemplars(cell: ArrowCell): Exemplar[] {
  return rowViews(new ArrowFrame(cell.list()), exemplar)
}
const integers = (cell: ArrowCell) => listValues(cell, integer)
const numbers = (cell: ArrowCell) => listValues(cell, number)

function receivedBase(frame: ArrowFrame, row: number) {
  return columnView(frame, row, {
    datapointRef: text,
    timestamp: integer,
    startTime: integer,
    flags: number,
    exemplars,
  })
}

function receivedNumber(
  frame: ArrowFrame,
  row: number
): ReceivedNumberDataPoint {
  return extendView(receivedBase(frame, row), numberValue(frame, row))
}

/** Omitted received statistics stay absent, including when other rows have them. */
function receivedStatistics(frame: ArrowFrame, row: number) {
  const values: Pick<ReceivedHistogramDataPoint, 'sum' | 'min' | 'max'> = {}
  for (const name of ['sum', 'min', 'max'] as const) {
    const cell = frame.cell(name, row)
    if (!cell.isNull)
      Object.defineProperty(values, name, {
        enumerable: true,
        get: () => cell.number(),
      })
  }
  return values
}

function receivedHistogram(
  frame: ArrowFrame,
  row: number
): ReceivedHistogramDataPoint {
  return extendView(
    extendView(
      receivedBase(frame, row),
      columnView(frame, row, {
        count: integer,
        bucketCounts: integers,
        explicitBounds: numbers,
      })
    ),
    receivedStatistics(frame, row)
  )
}

function exponentialBuckets(cell: ArrowCell) {
  return columnView(cell.struct(), 0, {
    offset: number,
    bucketCounts: integers,
  })
}
function receivedExponentialHistogram(
  frame: ArrowFrame,
  row: number
): ReceivedExponentialHistogramDataPoint {
  return extendView(
    extendView(
      receivedBase(frame, row),
      columnView(frame, row, {
        count: integer,
        scale: number,
        zeroCount: integer,
        zeroThreshold: number,
        positive: exponentialBuckets,
        negative: exponentialBuckets,
      })
    ),
    receivedStatistics(frame, row)
  )
}

export function exactMetricSeries(frame: ArrowFrame): ExactMetricSeries {
  const metric = identity(frame)
  return extendView(
    metric,
    columnView(frame, 0, {
      seriesRef: text,
      attributes,
      datapoints: cell => {
        const points = new ArrowFrame(cell.list())
        switch (metric.metricType) {
          case 'Gauge':
          case 'Sum':
            return rowViews(points, receivedNumber)
          case 'Histogram':
            return rowViews(points, receivedHistogram)
          case 'ExponentialHistogram':
            return rowViews(points, receivedExponentialHistogram)
        }
      },
    })
  )
}

function temporality(code: number): string {
  return ['Unspecified', 'Delta', 'Cumulative'][code] ?? `Unknown (${code})`
}
function quantiles(cell: ArrowCell): Record<string, number | null> | null {
  if (cell.isNull) return null
  const entries = cell.map()
  const result: Record<string, number | null> = {}
  for (let row = 0; row < entries.length; row++)
    Object.defineProperty(result, entries.cell('key', row).text(), {
      enumerable: true,
      get: () => nullableNumber(entries.cell('value', row)),
    })
  return result
}
function point(frame: ArrowFrame, row: number): DataPoint {
  const base = extendView(
    columnView(frame, row, {
      id: text,
      timestamp: integer,
      startTime: integer,
      flags: number,
      exemplars,
    }),
    {
      get timestampMs() {
        return Number(frame.cell('timestamp', row).integer() / 1000000n)
      },
      get exemplarCount() {
        const cell = frame.cell('exemplarCount', row)
        return cell.isNull ? undefined : cell.number()
      },
    }
  )
  const metricType = frame.cell('metricType', row).text()
  if (metricType === 'Gauge')
    return extendView(
      base,
      extendView(numberValue(frame, row), { metricType } as const)
    )
  const code = frame.cell('aggregationTemporalityCode', row).number()
  const temporal = extendView(base, {
    aggregationTemporalityCode: code,
    aggregationTemporality: temporality(code),
  })
  if (metricType === 'Sum')
    return extendView(
      temporal,
      extendView(numberValue(frame, row), {
        metricType: 'Sum' as const,
        get isMonotonic() {
          return frame.cell('isMonotonic', row).boolean()
        },
        get delta() {
          if (code !== 2) return undefined
          const int = frame.cell('deltaInt', row)
          return int.isNull
            ? nullableNumber(frame.cell('deltaDouble', row))
            : int.integer()
        },
        get isReset() {
          const cell = frame.cell('isReset', row)
          return code !== 2 ? undefined : cell.isNull ? null : cell.boolean()
        },
      })
    )
  const histogram = extendView(
    temporal,
    columnView(frame, row, {
      count: integer,
      sum: nullableNumber,
      min: nullableNumber,
      max: nullableNumber,
      quantiles,
    })
  )
  if (metricType === 'Histogram')
    return extendView(
      histogram,
      extendView(
        columnView(frame, row, {
          bucketCounts: integers,
          explicitBounds: numbers,
        }),
        { metricType } as const
      )
    )
  if (metricType === 'ExponentialHistogram')
    return extendView(
      histogram,
      extendView(
        columnView(frame, row, {
          scale: number,
          zeroCount: integer,
          zeroThreshold: number,
          positiveBucketOffset: number,
          positiveBucketCounts: integers,
          negativeBucketOffset: number,
          negativeBucketCounts: integers,
        }),
        { metricType } as const
      )
    )
  throw new Error(`Unsupported metric type: ${metricType}`)
}

function scalarBucket(frame: ArrowFrame, row: number): ScalarViewBucket {
  return columnView(frame, row, {
    bucketStart: integer,
    sampleCount: number,
    sum: nullableNumber,
    avg: nullableNumber,
    rate: nullableNumber,
    slope: nullableNumber,
    hasReset: boolean,
  })
}
function scalarBuckets(cell: ArrowCell): ScalarViewBucket[] {
  return rowViews(new ArrowFrame(cell.list()), scalarBucket)
}

function seriesView(
  frame: ArrowFrame,
  row: number,
  metricResource: ExactMetricResource
): MetricSeriesViewData {
  const readResource = memoize(() =>
    frame.has('resource') && !frame.cell('resource', row).isNull
      ? resource(frame.cell('resource', row))
      : metricResource
  )
  const readLabelPreviews = memoize(() =>
    frame.has('labelPreviews') && !frame.cell('labelPreviews', row).isNull
      ? rowViews(
          new ArrowFrame(frame.cell('labelPreviews', row).list()),
          (rows, i) => columnView(rows, i, { key: text, text, kind: text })
        )
      : undefined
  )
  return extendView(
    columnView(frame, row, {
      seriesRef: text,
      attributes,
      datapoints: cell => rowViews(new ArrowFrame(cell.list()), point),
      stats: cell =>
        cell.isNull
          ? null
          : columnView(cell.struct(), 0, {
              count: number,
              min: number,
              max: number,
              sum: number,
              avg: number,
            }),
      datapointCount: number,
      lastSeenNs: nullableInteger,
      views: cell => (cell.isNull ? null : scalarBuckets(cell)),
      rateStats: cell =>
        cell.isNull
          ? null
          : columnView(cell.struct(), 0, {
              min: number,
              max: number,
              avg: number,
            }),
      sparkline: cell =>
        cell.isNull
          ? null
          : rowViews(new ArrowFrame(cell.list()), (rows, i) =>
              columnView(rows, i, { timestamp: integer, value: number })
            ),
    }),
    {
      get resource() {
        return readResource()
      },
      get attributesComplete() {
        return (
          !frame.has('attributesComplete') ||
          frame.cell('attributesComplete', row).boolean()
        )
      },
      get labelPreviews() {
        return readLabelPreviews()
      },
    }
  )
}

function bounds(cell: ArrowCell) {
  return columnView(cell.struct(), 0, {
    startNs: nullableInteger,
    endNs: nullableInteger,
  })
}
export function metricView(frame: ArrowFrame): MetricViewData {
  const metric = identity(frame)
  const base = extendView(
    metric,
    columnView(frame, 0, {
      lastSeenNs: nullableInteger,
      datapointCount: number,
      timeseries: cell => {
        const sharedResource = metric.resource
        return rowViews(new ArrowFrame(cell.list()), (rows, row) =>
          seriesView(rows, row, sharedResource)
        )
      },
      boundsMismatch: cell =>
        cell.isNull
          ? null
          : columnView(cell.struct(), 0, {
              seriesBuckets: number,
              aggregateBuckets: number,
            }),
      window: cell =>
        columnView(cell.struct(), 0, { requested: bounds, effective: bounds }),
    })
  )
  return extendView(base, {
    get resourceDroppedAttributesCount() {
      return metric.resource.droppedAttributesCount
    },
    get resourceSchemaUrl() {
      return metric.resource.schemaUrl
    },
    get scopeName() {
      return metric.scope.name
    },
    get scopeVersion() {
      return metric.scope.version
    },
    get scopeSchemaUrl() {
      return metric.scope.schemaUrl
    },
    get scopeDroppedAttributesCount() {
      return metric.scope.droppedAttributesCount
    },
    get aggregationTemporalityCode() {
      return nullableNumber(frame.cell('aggregationTemporalityCode', 0))
    },
    get aggregationTemporality() {
      const code = nullableNumber(frame.cell('aggregationTemporalityCode', 0))
      return code === null ? null : temporality(code)
    },
  })
}

function aggregateBucket(frame: ArrowFrame, row: number): AggregateBucket {
  const base = columnView(frame, row, {
    timestamp: integer,
    startTime: integer,
    count: integer,
    sum: nullableNumber,
    quantiles,
  })
  const extents: Pick<AggregateBucket, 'min' | 'max'> = {}
  for (const name of ['min', 'max'] as const) {
    const cell = frame.cell(name, row)
    if (!cell.isNull)
      Object.defineProperty(extents, name, {
        enumerable: true,
        get: () => cell.number(),
      })
  }
  if (!frame.cell('explicitBounds', row).isNull)
    return extendView(
      extendView(base, extents),
      columnView(frame, row, {
        explicitBounds: numbers,
        bucketCounts: integers,
      })
    )
  return extendView(
    extendView(base, extents),
    columnView(frame, row, {
      scale: number,
      zeroThreshold: number,
      zeroCount: integer,
      positiveBucketOffset: number,
      positiveBucketCounts: integers,
      negativeBucketOffset: number,
      negativeBucketCounts: integers,
    })
  )
}
function aggregates(cell: ArrowCell): AggregateBucket[] | null {
  return cell.isNull
    ? null
    : rowViews(new ArrowFrame(cell.list()), aggregateBucket)
}
export function metricAggregateView(
  frame: ArrowFrame
): MetricAggregateViewData {
  const result: MetricAggregateViewData = columnView(frame, 0, {
    aggregate: aggregates,
    scalarAggregate: cell =>
      cell.isNull
        ? null
        : columnView(cell.struct(), 0, {
            selected: scalarBuckets,
            all: scalarBuckets,
          }),
  })
  if (frame.has('contribution'))
    Object.defineProperty(result, 'contribution', {
      enumerable: true,
      get: memoize(() => aggregates(frame.cell('contribution', 0))),
    })
  return result
}
