import type {
  TraceData,
  TraceSummary,
  LogData,
  LogSummary,
  TraceLogSummary,
  MetricViewData,
  MetricSeriesViewData,
  MetricSummary,
  Stats,
  Exemplar,
  DataPoint,
  ScalarAggregate,
  ScalarViewBucket,
  MetricAggregateViewData,
  ExactMetric,
  ExactMetricSeries,
  ReceivedNumberDataPoint,
  ReceivedHistogramDataPoint,
  ReceivedExponentialHistogramDataPoint,
  AggregateBucket,
} from '@/types/api-types'
import type {
  JsonAttributeDefinition,
  JsonDataPoint,
  JsonDeleteResult,
  JsonExemplar,
  JsonLogData,
  JsonLogSummary,
  JsonTraceLogSummary,
  JsonMetricViewData,
  JsonMetricSummary,
  JsonMetricSeriesViewData,
  JsonStats,
  JsonTraceData,
  JsonTraceSummary,
  JsonAttributeType,
  JsonQueryNode,
  JsonAttributeMatch,
  JsonAttribute,
  JsonAttributeValue,
  JsonMetricAggregateViewData,
  JsonExactMetric,
  JsonExactMetricSeries,
  JsonReceivedNumberDataPoint,
  JsonReceivedHistogramDataPoint,
  JsonReceivedExponentialHistogramDataPoint,
  JsonScalarAggregate,
  JsonScalarViewBucket,
  JsonAggregateBucket,
  JsonDouble,
} from '@/types/wire-types'
import type { Attribute, AttributeValue } from '@/types/api-types'
import {
  getOperatorsForFieldType,
  type FieldDefinition,
  type FieldType,
  type QueryNode,
} from '@/search/model'

type JsonRpcParamValue =
  | string
  | number
  | boolean
  | null
  | JsonQueryNode
  | SearchSort
  | readonly string[]
  | readonly number[]

type JsonRpcNamedParams = Record<string, JsonRpcParamValue | undefined>
type JsonRpcParams = JsonRpcNamedParams | readonly string[]

interface JsonRpcRequest {
  jsonrpc: '2.0'
  method: string
  params?: JsonRpcParams
  id: number
}

interface JsonRpcResponse {
  jsonrpc: '2.0'
  result?: unknown
  error?: {
    code: number
    message: string
  }
  id: number
}

/** Preserves the JSON-RPC code for error-specific UI. */
export class JsonRpcError extends Error {
  code: number
  constructor(code: number, message: string) {
    super(message)
    this.name = 'JsonRpcError'
    this.code = code
  }
}

export type SearchSort = {
  field: string
  direction: 'asc' | 'desc'
}

const ERR_CODE_METRIC_NOT_FOUND = -32003

/** An absolute query bound in Unix nanoseconds, or null when unbounded. */
export type QueryTimeBound = bigint | null

function serializeNanoseconds(bound: QueryTimeBound): string | null {
  return bound === null ? null : bound.toString()
}

// oxlint-disable-next-line anti-slop/no-unknown-parameters -- Validates the raw wire value before bigint conversion.
function bigintFromWire(value: unknown): bigint {
  // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Rejects non-string wire values before bigint conversion so rounded JSON numbers cannot be accepted as exact integers.
  if (typeof value !== 'string') {
    // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Reports the rejected wire value's runtime type in the boundary validation error.
    const received = value === null ? 'null' : typeof value
    throw new Error(
      `Invalid bigint wire value: expected string, got ${received}`
    )
  }
  return BigInt(value)
}

// oxlint-disable-next-line anti-slop/no-unknown-parameters -- Accepts null; otherwise delegates to the validating wire decoder.
function nullableBigintFromWire(value: unknown): bigint | null {
  return value === null ? null : bigintFromWire(value)
}

function doubleFromWire(value: number | string): number {
  // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Decodes the two double wire encodings: JSON numbers and hexadecimal IEEE-754 bit strings.
  if (typeof value === 'number') return value
  if (!/^0x[0-9a-f]{16}$/i.test(value)) {
    throw new Error(`Invalid double wire value: ${value}`)
  }
  const bytes = new ArrayBuffer(8)
  const view = new DataView(bytes)
  view.setBigUint64(0, BigInt(value), false)
  return view.getFloat64(0, false)
}

function nullableDoubleFromWire(value: JsonDouble | null): number | null {
  return value === null ? null : doubleFromWire(value)
}

function scalarDeltaFromWire(
  value: JsonDouble | string | null | undefined
): number | bigint | null | undefined {
  // oxlint-disable-next-line anti-slop/no-runtime-typeof -- Decodes calculated-delta wire values: numbers and nullish values pass through; strings go to the bigint or double decoder.
  if (typeof value !== 'string') return value
  return value.startsWith('0x') ? doubleFromWire(value) : bigintFromWire(value)
}

function doubleRecordFromJSON(
  values: Record<string, JsonDouble | null> | null
): Record<string, number | null> | null {
  if (values === null) return null
  return Object.fromEntries(
    Object.entries(values).map(([key, value]) => [
      key,
      nullableDoubleFromWire(value),
    ])
  )
}

function attributeValueFromJSON(value: JsonAttributeValue): AttributeValue {
  switch (value.kind) {
    case 'empty':
    case 'string':
    case 'bytes':
    case 'bool':
      return value
    case 'int64':
      return { ...value, value: bigintFromWire(value.value) }
    case 'double':
      return { ...value, value: doubleFromWire(value.value) }
    case 'array':
      return { ...value, value: value.value.map(attributeValueFromJSON) }
    case 'map':
      const encodedValuesByKey = new Map<string, Set<string>>()
      for (const entry of value.value) {
        const encodedValues =
          encodedValuesByKey.get(entry.key) ?? new Set<string>()
        encodedValues.add(JSON.stringify(entry.value))
        encodedValuesByKey.set(entry.key, encodedValues)
      }
      return {
        ...value,
        value: value.value.map(entry => ({
          key: entry.key,
          value: attributeValueFromJSON(entry.value),
        })),
        conflictingKeys: [...encodedValuesByKey].flatMap(([key, values]) =>
          values.size > 1 ? [key] : []
        ),
      }
  }
}

function attributesFromJSON(attributes: JsonAttribute[]): Attribute[] {
  const encodedValuesByKey = new Map<string, Set<string>>()
  for (const attribute of attributes) {
    const encodedValues =
      encodedValuesByKey.get(attribute.key) ?? new Set<string>()
    encodedValues.add(JSON.stringify(attribute.value))
    encodedValuesByKey.set(attribute.key, encodedValues)
  }
  return attributes.map(attribute => ({
    id: attribute.id,
    key: attribute.key,
    value: attributeValueFromJSON(attribute.value),
    hasConflict: encodedValuesByKey.get(attribute.key)!.size > 1,
  }))
}

/** Thrown when a superseded request is abandoned. */
export class RequestAbortedError extends Error {
  constructor() {
    super('Request aborted')
    this.name = 'RequestAbortedError'
  }
}

/** Returns whether a rejection represents an abandoned request. */
// oxlint-disable-next-line anti-slop/no-unknown-parameters -- Classifies arbitrary caught or rejected values by identity without assuming Error.
export function isAbortError(err: unknown): boolean {
  return (
    err instanceof RequestAbortedError ||
    (err instanceof DOMException && err.name === 'AbortError')
  )
}

// Omit unsupplied keys while preserving null and empty-array values.
function named(params: JsonRpcNamedParams): JsonRpcNamedParams {
  return Object.fromEntries(
    Object.entries(params).filter(([, value]) => value !== undefined)
  )
}

// T asserts the trusted backend wire shape at compile time; it does not validate it.
// Aborting fetch cancels the server request context and its DuckDB query.
async function callRPC<T>(
  method: string,
  params?: JsonRpcParams,
  signal?: AbortSignal
): Promise<T> {
  const request: JsonRpcRequest = {
    method,
    params,
    id: Math.floor(Math.random() * 1000000),
    jsonrpc: '2.0',
  }

  let response: Response
  try {
    response = await fetch('/rpc', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(request),
      signal,
    })
  } catch (err) {
    if (isAbortError(err)) throw new RequestAbortedError()
    throw err
  }

  if (!response.ok) {
    throw new Error(`HTTP error! status: ${response.status}`)
  }

  const data: JsonRpcResponse = await response.json()

  if (data.jsonrpc !== request.jsonrpc) {
    throw new Error(`Invalid JSON-RPC version: ${String(data.jsonrpc)}`)
  }
  if (data.id !== request.id) {
    throw new Error('JSON-RPC response ID does not match request ID')
  }

  if (data.error) {
    throw new JsonRpcError(data.error.code, data.error.message)
  }

  // SAFETY: Each successful in-process Go RPC method owns the wire type chosen
  // by its call site; service response tests pin the transformed wire shapes.
  // This assertion records that trusted boundary and performs no validation.
  return data.result as T
}

function traceSummaryFromJSON(json: JsonTraceSummary): TraceSummary {
  return {
    ...json,
    // The wire sends null for orphaned traces and for an empty root
    // service name (nullif in the projection); the domain wants
    // undefined / '' respectively.
    rootSpan: json.rootSpan
      ? {
          serviceName: json.rootSpan.serviceName ?? '',
          name: json.rootSpan.name,
        }
      : undefined,
    startTime: bigintFromWire(json.startTime),
    // durationNs arrives as a varchar-encoded int64 (ns precision
    // would otherwise be clipped by JSON's float64 numbers).
    durationNs: nullableBigintFromWire(json.durationNs),
  }
}

function traceSummariesFromJSON(json: JsonTraceSummary[]): TraceSummary[] {
  return json.map(traceSummaryFromJSON)
}

// Decodes compressed spans and shares each resolved resource and scope object.
function traceDataFromJSON(json: JsonTraceData): TraceData {
  const traceStart = bigintFromWire(json.traceStart)
  const resources = Object.fromEntries(
    Object.entries(json.resources).map(([id, resource]) => [
      id,
      { ...resource, attributes: attributesFromJSON(resource.attributes) },
    ])
  )
  const scopes = Object.fromEntries(
    Object.entries(json.scopes).map(([id, scope]) => [
      id,
      { ...scope, attributes: attributesFromJSON(scope.attributes) },
    ])
  )

  return {
    traceID: json.traceID,
    unplacedSpanCount: json.unplacedSpanCount,
    spans: json.spans.map(spanNode => {
      const { r, s, start, dur, ...rest } = spanNode.spanData
      const startTime = traceStart + BigInt(start)
      const node: TraceData['spans'][number] = {
        spanData: {
          ...rest,
          attributes: attributesFromJSON(rest.attributes),
          links: rest.links.map(link => ({
            ...link,
            attributes: attributesFromJSON(link.attributes),
          })),
          traceID: json.traceID,
          resource: resources[String(r)]!,
          scope: scopes[String(s)]!,
          startTime,
          endTime: startTime + BigInt(dur),
          events: spanNode.spanData.events.map(event => ({
            ...event,
            attributes: attributesFromJSON(event.attributes),
            timestamp: bigintFromWire(event.timestamp),
          })),
        },
        depth: spanNode.depth,
        matched: spanNode.matched,
      }
      // Keep both properties absent on every span of a healthy trace.
      if (spanNode.salvaged) {
        node.salvaged = true
        node.cyclePoint = spanNode.cyclePoint
      }
      return node
    }),
  }
}

function logSummaryFromJSON(json: JsonLogSummary): LogSummary {
  return {
    ...json,
    timestamp: bigintFromWire(json.timestamp),
  }
}

function logSummariesFromJSON(json: JsonLogSummary[]): LogSummary[] {
  return json.map(logSummaryFromJSON)
}

function traceLogSummariesFromJSON(
  json: JsonTraceLogSummary[]
): TraceLogSummary[] {
  return json.map(log => ({ ...log, timestamp: bigintFromWire(log.timestamp) }))
}

function logDataFromJSON(json: JsonLogData): LogData {
  return {
    ...json,
    body: attributeValueFromJSON(json.body),
    attributes: attributesFromJSON(json.attributes),
    resource: {
      ...json.resource,
      attributes: attributesFromJSON(json.resource.attributes),
    },
    scope: {
      ...json.scope,
      attributes: attributesFromJSON(json.scope.attributes),
    },
    timestamp: bigintFromWire(json.timestamp),
    observedTimestamp: bigintFromWire(json.observedTimestamp),
  }
}

function exemplarFromJSON(json: JsonExemplar): Exemplar {
  const base = {
    timestamp: bigintFromWire(json.timestamp),
    traceID: json.traceID,
    spanID: json.spanID,
    filteredAttributes: attributesFromJSON(json.filteredAttributes),
  }
  switch (json.valueType) {
    case 'Double':
      return {
        ...base,
        valueType: 'Double',
        doubleValue: doubleFromWire(json.doubleValue),
        intValue: null,
      }
    case 'Int':
      return {
        ...base,
        valueType: 'Int',
        doubleValue: null,
        intValue: bigintFromWire(json.intValue),
      }
    case 'Empty':
      return {
        ...base,
        valueType: 'Empty',
        doubleValue: null,
        intValue: null,
      }
  }
}

function dataPointFromJSON(json: JsonDataPoint): DataPoint {
  const base = {
    id: json.id,
    timestamp: bigintFromWire(json.timestamp),
    timestampMs: json.timestampMs,
    startTime: bigintFromWire(json.startTime),
    flags: json.flags,
    exemplars: json.exemplars.map(exemplarFromJSON),
    exemplarCount: json.exemplarCount,
  }
  switch (json.metricType) {
    case 'Gauge':
      return {
        ...base,
        metricType: 'Gauge',
        doubleValue: nullableDoubleFromWire(json.doubleValue),
        intValue: json.intValue === null ? null : bigintFromWire(json.intValue),
        valueType: json.valueType,
      }
    case 'Sum':
      return {
        ...base,
        metricType: 'Sum',
        doubleValue: nullableDoubleFromWire(json.doubleValue),
        intValue: json.intValue === null ? null : bigintFromWire(json.intValue),
        valueType: json.valueType,
        isMonotonic: json.isMonotonic,
        aggregationTemporalityCode: json.aggregationTemporalityCode,
        aggregationTemporality: json.aggregationTemporality,
        delta: scalarDeltaFromWire(json.delta),
        isReset: json.isReset,
      }
    case 'Histogram':
      return {
        ...base,
        metricType: 'Histogram',
        count: bigintFromWire(json.count),
        sum: nullableDoubleFromWire(json.sum),
        min: nullableDoubleFromWire(json.min),
        max: nullableDoubleFromWire(json.max),
        bucketCounts: json.bucketCounts.map(bigintFromWire),
        explicitBounds: json.explicitBounds.map(doubleFromWire),
        quantiles: doubleRecordFromJSON(json.quantiles),
        aggregationTemporalityCode: json.aggregationTemporalityCode,
        aggregationTemporality: json.aggregationTemporality,
      }
    case 'ExponentialHistogram':
      return {
        ...base,
        metricType: 'ExponentialHistogram',
        count: bigintFromWire(json.count),
        sum: nullableDoubleFromWire(json.sum),
        min: nullableDoubleFromWire(json.min),
        max: nullableDoubleFromWire(json.max),
        scale: json.scale,
        zeroCount: bigintFromWire(json.zeroCount),
        zeroThreshold: doubleFromWire(json.zeroThreshold),
        positiveBucketOffset: json.positiveBucketOffset,
        positiveBucketCounts: json.positiveBucketCounts.map(bigintFromWire),
        negativeBucketOffset: json.negativeBucketOffset,
        negativeBucketCounts: json.negativeBucketCounts.map(bigintFromWire),
        quantiles: doubleRecordFromJSON(json.quantiles),
        aggregationTemporalityCode: json.aggregationTemporalityCode,
        aggregationTemporality: json.aggregationTemporality,
      }
  }
}

function timeseriesFromJSON(
  json: JsonMetricSeriesViewData
): MetricSeriesViewData {
  return {
    seriesRef: json.seriesRef,
    attributes: attributesFromJSON(json.attributes),
    resource: {
      ...json.resource,
      attributes: attributesFromJSON(json.resource.attributes),
    },
    datapoints: json.datapoints.map(dataPointFromJSON),
    stats: json.stats
      ? {
          ...json.stats,
          min: doubleFromWire(json.stats.min),
          max: doubleFromWire(json.stats.max),
          sum: doubleFromWire(json.stats.sum),
          avg: doubleFromWire(json.stats.avg),
        }
      : null,
    datapointCount: json.datapointCount ?? 0,
    lastSeenNs: nullableBigintFromWire(json.lastSeenNs ?? null),
    views: json.views ? scalarViewBucketsFromJSON(json.views) : null,
    rateStats: json.rateStats
      ? {
          min: doubleFromWire(json.rateStats.min),
          max: doubleFromWire(json.rateStats.max),
          avg: doubleFromWire(json.rateStats.avg),
        }
      : null,
    sparkline:
      json.sparkline?.map(p => ({
        ...p,
        timestamp: bigintFromWire(p.timestamp),
        value: doubleFromWire(p.value),
      })) ?? null,
  }
}

/** Decodes nanosecond bucket starts and scalar values at the wire boundary. */
function scalarViewBucketsFromJSON(
  json: JsonScalarViewBucket[]
): ScalarViewBucket[] {
  return json.map(b => ({
    ...b,
    bucketStart: bigintFromWire(b.bucketStart),
    sum: nullableDoubleFromWire(b.sum),
    avg: nullableDoubleFromWire(b.avg),
    rate: nullableDoubleFromWire(b.rate),
    slope: nullableDoubleFromWire(b.slope),
  }))
}

function aggregateBucketsFromJSON(
  json: JsonAggregateBucket[] | null
): MetricAggregateViewData['aggregate'] {
  if (!json) return null
  return json.map(bucket => {
    const { sum, min, max, explicitBounds, zeroThreshold, quantiles, ...rest } =
      bucket
    const decoded: AggregateBucket = {
      ...rest,
      sum: nullableDoubleFromWire(sum),
      quantiles: doubleRecordFromJSON(quantiles),
    }
    if (min !== undefined) decoded.min = doubleFromWire(min)
    if (max !== undefined) decoded.max = doubleFromWire(max)
    if (explicitBounds !== undefined) {
      decoded.explicitBounds = explicitBounds.map(doubleFromWire)
    }
    if (zeroThreshold !== undefined) {
      decoded.zeroThreshold = doubleFromWire(zeroThreshold)
    }
    return decoded
  })
}

function scalarAggregateFromJSON(
  json: JsonScalarAggregate | null
): ScalarAggregate | null {
  if (!json) return null
  return {
    selected: scalarViewBucketsFromJSON(json.selected),
    all: scalarViewBucketsFromJSON(json.all),
  }
}

function metricViewDataFromJSON(json: JsonMetricViewData): MetricViewData {
  const {
    aggregate: _aggregate,
    scalarAggregate: _scalarAggregate,
    ...rest
  } = json
  return {
    ...rest,
    metadata: attributesFromJSON(json.metadata),
    resource: {
      ...json.resource,
      attributes: attributesFromJSON(json.resource.attributes),
    },
    scope: {
      ...json.scope,
      attributes: attributesFromJSON(json.scope.attributes),
    },
    timeseries: json.timeseries.map(timeseriesFromJSON),
    boundsMismatch: json.boundsMismatch ?? null,
    lastSeenNs: nullableBigintFromWire(json.lastSeenNs ?? null),
    window: {
      requested: {
        startNs: nullableBigintFromWire(json.window.requested.startNs),
        endNs: nullableBigintFromWire(json.window.requested.endNs),
      },
      effective: {
        startNs: nullableBigintFromWire(json.window.effective.startNs),
        endNs: nullableBigintFromWire(json.window.effective.endNs),
      },
    },
  }
}

function exactMetricIdentityFromJSON<
  T extends JsonExactMetric | JsonExactMetricSeries,
>(json: T) {
  return {
    ...json,
    metadata: attributesFromJSON(json.metadata),
    resource: {
      ...json.resource,
      attributes: attributesFromJSON(json.resource.attributes),
    },
    scope: {
      ...json.scope,
      attributes: attributesFromJSON(json.scope.attributes),
    },
  }
}

function exactMetricFromJSON(json: JsonExactMetric): ExactMetric {
  return {
    ...exactMetricIdentityFromJSON(json),
    series: json.series.map(series => ({
      ...series,
      attributes: attributesFromJSON(series.attributes),
      datapointCount: bigintFromWire(series.datapointCount),
      firstDatapointTimestamp: nullableBigintFromWire(
        series.firstDatapointTimestamp
      ),
      lastDatapointTimestamp: nullableBigintFromWire(
        series.lastDatapointTimestamp
      ),
    })),
  }
}

function receivedNumberDataPointFromJSON(
  json: JsonReceivedNumberDataPoint
): ReceivedNumberDataPoint {
  const base = {
    datapointID: json.datapointID,
    timestamp: bigintFromWire(json.timestamp),
    startTime: bigintFromWire(json.startTime),
    flags: json.flags,
    exemplars: json.exemplars.map(exemplarFromJSON),
  }
  switch (json.valueType) {
    case 'Int':
      return {
        ...base,
        valueType: 'Int',
        intValue: bigintFromWire(json.intValue),
        doubleValue: null,
      }
    case 'Double':
      return {
        ...base,
        valueType: 'Double',
        intValue: null,
        doubleValue: doubleFromWire(json.doubleValue),
      }
    case 'Empty':
      return {
        ...base,
        valueType: 'Empty',
        intValue: null,
        doubleValue: null,
      }
  }
}

function receivedHistogramDataPointFromJSON(
  json: JsonReceivedHistogramDataPoint
): ReceivedHistogramDataPoint {
  const decoded: ReceivedHistogramDataPoint = {
    datapointID: json.datapointID,
    timestamp: bigintFromWire(json.timestamp),
    startTime: bigintFromWire(json.startTime),
    flags: json.flags,
    exemplars: json.exemplars.map(exemplarFromJSON),
    count: bigintFromWire(json.count),
    bucketCounts: json.bucketCounts.map(bigintFromWire),
    explicitBounds: json.explicitBounds.map(doubleFromWire),
  }
  if (json.sum !== undefined) decoded.sum = doubleFromWire(json.sum)
  if (json.min !== undefined) decoded.min = doubleFromWire(json.min)
  if (json.max !== undefined) decoded.max = doubleFromWire(json.max)
  return decoded
}

function receivedExponentialHistogramDataPointFromJSON(
  json: JsonReceivedExponentialHistogramDataPoint
): ReceivedExponentialHistogramDataPoint {
  const decoded: ReceivedExponentialHistogramDataPoint = {
    datapointID: json.datapointID,
    timestamp: bigintFromWire(json.timestamp),
    startTime: bigintFromWire(json.startTime),
    flags: json.flags,
    exemplars: json.exemplars.map(exemplarFromJSON),
    count: bigintFromWire(json.count),
    scale: json.scale,
    zeroCount: bigintFromWire(json.zeroCount),
    zeroThreshold: doubleFromWire(json.zeroThreshold),
    positive: {
      offset: json.positive.offset,
      bucketCounts: json.positive.bucketCounts.map(bigintFromWire),
    },
    negative: {
      offset: json.negative.offset,
      bucketCounts: json.negative.bucketCounts.map(bigintFromWire),
    },
  }
  if (json.sum !== undefined) decoded.sum = doubleFromWire(json.sum)
  if (json.min !== undefined) decoded.min = doubleFromWire(json.min)
  if (json.max !== undefined) decoded.max = doubleFromWire(json.max)
  return decoded
}

function receivedMetricDatapointsFromJSON(json: JsonExactMetricSeries) {
  let datapoints: ExactMetricSeries['datapoints']
  switch (json.metricType) {
    case 'Gauge':
    case 'Sum':
      datapoints = json.datapoints.map(point =>
        receivedNumberDataPointFromJSON(point as JsonReceivedNumberDataPoint)
      )
      break
    case 'Histogram':
      datapoints = json.datapoints.map(point =>
        receivedHistogramDataPointFromJSON(
          point as JsonReceivedHistogramDataPoint
        )
      )
      break
    case 'ExponentialHistogram':
      datapoints = json.datapoints.map(point =>
        receivedExponentialHistogramDataPointFromJSON(
          point as JsonReceivedExponentialHistogramDataPoint
        )
      )
      break
  }
  return datapoints
}

function exactMetricSeriesFromJSON(
  json: JsonExactMetricSeries
): ExactMetricSeries {
  return {
    ...exactMetricIdentityFromJSON(json),
    attributes: attributesFromJSON(json.attributes),
    datapoints: receivedMetricDatapointsFromJSON(json),
  }
}

function metricSummaryFromJSON(json: JsonMetricSummary): MetricSummary {
  return {
    ...json,
    description: json.description ?? '',
    lastValue: nullableDoubleFromWire(json.lastValue),
    lastSeen: bigintFromWire(json.lastSeen),
  }
}

function metricSummariesFromJSON(json: JsonMetricSummary[]): MetricSummary[] {
  return json.map(metricSummaryFromJSON)
}

function statsFromJSON(json: JsonStats): Stats {
  return {
    traces: {
      ...json.traces,
      lastReceived: nullableBigintFromWire(json.traces.lastReceived),
    },
    logs: {
      ...json.logs,
      lastReceived: nullableBigintFromWire(json.logs.lastReceived),
    },
    metrics: {
      ...json.metrics,
      lastReceived: nullableBigintFromWire(json.metrics.lastReceived),
    },
    rejections: (json.rejections ?? []).map(r => ({
      ...r,
      samples: r.samples ?? [],
      firstSeen: nullableBigintFromWire(r.firstSeen),
      lastSeen: nullableBigintFromWire(r.lastSeen),
    })),
  }
}

export let telemetryAPI = {
  // Value-first discovery: given text the user can see, which attribute keys
  // hold it. Cross-signal by nature -- the dictionary it reads is shared by
  // traces, logs and metrics -- so unlike getXAttributes it takes no signal and
  // no time range.
  // Distinct values of one completable column, most frequent first. Fetched
  // once per field per completion session with a generous limit; the editor
  // filters the list client-side per keystroke, so this does not round-trip
  // while typing. The server allowlists which fields answer.
  getFieldValueCompletions: async (
    signal: string,
    field: string,
    term: string,
    limit: number
  ): Promise<string[]> => {
    const rawData = await callRPC<string[]>(
      'getFieldValueCompletions',
      named({ signal, field, term, limit })
    )
    return Array.isArray(rawData) ? rawData : []
  },

  searchAttributeMatches: async (
    term: string
  ): Promise<JsonAttributeMatch[]> => {
    if (!term.trim()) return []
    const rawData = await callRPC<JsonAttributeMatch[]>(
      'searchAttributeMatches',
      named({ term })
    )
    if (!Array.isArray(rawData)) {
      console.warn('searchAttributeMatches: Expected array, got:', rawData)
      return []
    }
    return rawData
  },

  getTraceAttributeDefinitions: async (): Promise<FieldDefinition[]> => {
    const rawData = await callRPC<JsonAttributeDefinition[]>(
      'getTraceAttributeDefinitions'
    )

    if (!Array.isArray(rawData)) {
      console.warn(
        'getTraceAttributeDefinitions: Expected array, got:',
        rawData
      )
      return []
    }

    const converted = convertAttributesToFieldDefinitions(rawData)
    return converted
  },

  getTraceAttributeDefinitionsByTraceID: async (
    traceID: string
  ): Promise<FieldDefinition[]> => {
    const rawData = await callRPC<JsonAttributeDefinition[]>(
      'getTraceAttributeDefinitionsByTraceID',
      named({ traceID })
    )
    if (!Array.isArray(rawData)) {
      console.warn(
        'getTraceAttributeDefinitionsByTraceID: Expected array, got:',
        rawData
      )
      return []
    }
    return convertAttributesToFieldDefinitions(rawData)
  },

  getLogAttributeDefinitions: async (): Promise<FieldDefinition[]> => {
    const rawData = await callRPC<JsonAttributeDefinition[]>(
      'getLogAttributeDefinitions'
    )

    if (!Array.isArray(rawData)) {
      console.warn('getLogAttributeDefinitions: Expected array, got:', rawData)
      return []
    }

    return convertAttributesToFieldDefinitions(rawData)
  },

  searchTraceSummaries: async (
    startTime: QueryTimeBound,
    endTime: QueryTimeBound,
    queryTree?: QueryNode,
    limit?: number,
    sort?: SearchSort
  ): Promise<TraceSummary[]> => {
    const startTimeNs = serializeNanoseconds(startTime)
    const endTimeNs = serializeNanoseconds(endTime)

    const rawData = await callRPC<JsonTraceSummary[]>(
      'searchTraceSummaries',
      named({
        startTime: startTimeNs,
        endTime: endTimeNs,
        query: queryTree && convertQueryTreeForBackend(queryTree),
        limit,
        sort,
      })
    )
    return traceSummariesFromJSON(rawData)
  },

  getTraceView: async (
    traceID: string,
    queryTree?: QueryNode,
    signal?: AbortSignal
  ): Promise<TraceData> => {
    const rawData = await callRPC<JsonTraceData>(
      'getTraceView',
      named({
        traceID,
        query: queryTree && convertQueryTreeForBackend(queryTree),
      }),
      signal
    )
    return traceDataFromJSON(rawData)
  },

  getTraceLogSummaries: async (
    traceID: string,
    signal?: AbortSignal
  ): Promise<TraceLogSummary[]> => {
    const rawData = await callRPC<JsonTraceLogSummary[]>(
      'getTraceLogSummaries',
      named({ traceID }),
      signal
    )
    return traceLogSummariesFromJSON(rawData)
  },

  clearTraces: () => callRPC<string>('clearTraces', undefined),
  deleteTraces: (traceIDs: string[]) =>
    callRPC<JsonDeleteResult>('deleteSpansByTraceID', traceIDs),

  searchLogSummaries: async (
    startTime: QueryTimeBound,
    endTime: QueryTimeBound,
    queryTree?: QueryNode,
    limit?: number,
    sort?: SearchSort
  ): Promise<LogSummary[]> => {
    const startTimeNs = serializeNanoseconds(startTime)
    const endTimeNs = serializeNanoseconds(endTime)
    const rawData = await callRPC<JsonLogSummary[]>(
      'searchLogSummaries',
      named({
        startTime: startTimeNs,
        endTime: endTimeNs,
        query: queryTree && convertQueryTreeForBackend(queryTree),
        limit,
        sort,
      })
    )
    return logSummariesFromJSON(rawData)
  },

  getLog: async (logRef: string): Promise<LogData> => {
    const rawData = await callRPC<JsonLogData>('getLog', named({ logRef }))
    return logDataFromJSON(rawData)
  },

  deleteLogsByRefs: (logRef: string) =>
    callRPC<JsonDeleteResult>('deleteLogsByRefs', [logRef]),
  clearLogs: () => callRPC<string>('clearLogs', undefined),

  searchMetricSummaries: async (
    startTime: QueryTimeBound,
    endTime: QueryTimeBound,
    queryTree?: QueryNode,
    limit?: number,
    sort?: SearchSort
  ): Promise<MetricSummary[]> => {
    const startTimeNs = serializeNanoseconds(startTime)
    const endTimeNs = serializeNanoseconds(endTime)
    const rawData = await callRPC<JsonMetricSummary[]>(
      'searchMetricSummaries',
      named({
        startTime: startTimeNs,
        endTime: endTimeNs,
        query: queryTree && convertQueryTreeForBackend(queryTree),
        limit,
        sort,
      })
    )
    return metricSummariesFromJSON(rawData)
  },

  getMetric: async (metricRef: string): Promise<ExactMetric | null> => {
    try {
      const rawData = await callRPC<JsonExactMetric>(
        'getMetric',
        named({ metricRef })
      )
      return exactMetricFromJSON(rawData)
    } catch (error) {
      if (
        error instanceof JsonRpcError &&
        error.code === ERR_CODE_METRIC_NOT_FOUND
      ) {
        return null
      }
      throw error
    }
  },

  getMetricSeries: async (
    metricRef: string,
    seriesRef: string,
    startTime: QueryTimeBound,
    endTime: QueryTimeBound
  ): Promise<ExactMetricSeries | null> => {
    try {
      const rawData = await callRPC<JsonExactMetricSeries>(
        'getMetricSeries',
        named({
          metricRef,
          seriesRef,
          startTime: serializeNanoseconds(startTime),
          endTime: serializeNanoseconds(endTime),
        })
      )
      return exactMetricSeriesFromJSON(rawData)
    } catch (error) {
      if (
        error instanceof JsonRpcError &&
        error.code === ERR_CODE_METRIC_NOT_FOUND
      ) {
        return null
      }
      throw error
    }
  },

  getMetricView: async (
    metricRef: string,
    startTime: QueryTimeBound,
    endTime: QueryTimeBound,
    /** Time buckets for scalar reduction. Omit for every datapoint. */
    targetBuckets?: number,
    /** Restrict the response to these series. Omit for all series. */
    seriesRefs?: string[],
    /** Quantiles to compute per histogram datapoint, keyed by the quantile in
     *  the response. Omit to skip the work. */
    quantiles?: readonly number[],
    /** The viewer's UTC offset in nanoseconds, so bucket boundaries fall where
     *  the reader's calendar puts them. Omit for UTC. */
    tzOffsetNs?: number,
    /** Resolution for Sum, Average and Rate views. Omit for none. */
    viewBuckets?: number,
    /** Sparkline buckets, each contributing its minimum and maximum. */
    sparklineBuckets?: number,
    /** Which series are checked for the scalar Selected pool. */
    selectedSeriesRefs?: string[],
    /** The IANA zone bucket boundaries should follow. */
    tzName?: string,
    /** Series that include datapoints. Omit to use `datapointSeriesLimit`; an
     *  empty array requests none. */
    datapointSeriesRefs?: string[],
    /** Number of leading series that include datapoints when refs are omitted. */
    datapointSeriesLimit?: number
  ): Promise<MetricViewData | null> => {
    const startTimeNs = serializeNanoseconds(startTime)
    const endTimeNs = serializeNanoseconds(endTime)
    // Not-found arrives as a JSON-RPC error (one wire convention across all
    // signals); translate it to null here so callers keep a simple contract.
    try {
      const rawData = await callRPC<JsonMetricViewData>(
        'getMetricView',
        named({
          metricRef,
          startTime: startTimeNs,
          endTime: endTimeNs,
          targetBuckets,
          // null and [] are different requests: null (or absent) means every
          // series, [] means none. Only undefined is dropped.
          seriesRefs,
          quantiles,
          tzOffsetNs,
          viewBuckets,
          sparklineBuckets,
          selectedSeriesRefs,
          tzName,
          datapointSeriesRefs,
          datapointSeriesLimit,
        })
      )
      return metricViewDataFromJSON(rawData)
    } catch (error) {
      if (
        error instanceof JsonRpcError &&
        error.code === ERR_CODE_METRIC_NOT_FOUND
      ) {
        return null
      }
      throw error
    }
  },

  /** Fetches cross-series values that depend on the legend selection. */
  getMetricAggregateView: async (
    metricRef: string,
    startTime: QueryTimeBound,
    endTime: QueryTimeBound,
    targetBuckets: number,
    /** Series included in a histogram merge. Null keeps every scalar series. */
    seriesRefs: string[] | null,
    quantiles: readonly number[],
    tzOffsetNs: number,
    viewBuckets = 0,
    /** Series included in the scalar Selected pool without narrowing All. */
    selectedSeriesRefs?: string[],
    /** Bucket time zone. It must match the per-series view. */
    tzName?: string
  ): Promise<MetricAggregateViewData | null> => {
    const startTimeNs = serializeNanoseconds(startTime)
    const endTimeNs = serializeNanoseconds(endTime)
    try {
      const raw = await callRPC<JsonMetricAggregateViewData | null>(
        'getMetricAggregateView',
        named({
          metricRef,
          startTime: startTimeNs,
          endTime: endTimeNs,
          targetBuckets,
          seriesRefs,
          quantiles,
          tzOffsetNs,
          viewBuckets,
          selectedSeriesRefs: selectedSeriesRefs ?? null,
          tzName,
        })
      )
      if (!raw) return null
      return {
        aggregate: aggregateBucketsFromJSON(raw.aggregate),
        scalarAggregate: scalarAggregateFromJSON(raw.scalarAggregate),
      }
    } catch (error) {
      if (
        error instanceof JsonRpcError &&
        error.code === ERR_CODE_METRIC_NOT_FOUND
      ) {
        return null
      }
      throw error
    }
  },

  getMetricAttributeDefinitions: async (): Promise<FieldDefinition[]> => {
    const rawData = await callRPC<JsonAttributeDefinition[]>(
      'getMetricAttributeDefinitions'
    )

    if (!Array.isArray(rawData)) {
      console.warn(
        'getMetricAttributeDefinitions: Expected array, got:',
        rawData
      )
      return []
    }

    return convertAttributesToFieldDefinitions(rawData)
  },

  deleteMetric: (metricRef: string) =>
    callRPC<string>('deleteMetric', named({ metricRef })),
  clearMetrics: () => callRPC<string>('clearMetrics', undefined),

  // Stats methods
  getStats: async (): Promise<Stats> => {
    const rawData = await callRPC<JsonStats>('getStats')
    return statsFromJSON(rawData)
  },

  getTraceSpanCount: async (traceID: string): Promise<number> => {
    return await callRPC<number>('getTraceSpanCount', named({ traceID }))
  },
}

// Helper function to convert frontend query tree to minimal backend format
function convertQueryTreeForBackend(queryTree: QueryNode): JsonQueryNode {
  if (queryTree.type === 'condition') {
    return {
      id: queryTree.id,
      type: 'condition',
      query: {
        field: {
          ...(queryTree.query.field.searchScope !== 'global' && {
            name: queryTree.query.field.name,
            type: queryTree.query.field.type,
          }),
          searchScope: queryTree.query.field.searchScope,
          ...(queryTree.query.field.searchScope === 'attribute' && {
            attributeScope: queryTree.query.field.attributeScope,
          }),
        },
        fieldOperator: queryTree.query.operator.symbol,
        value: queryTree.query.value,
      },
    }
  } else {
    return {
      id: queryTree.id,
      type: 'group',
      group: {
        logicalOperator: queryTree.group.operator,
        children: queryTree.group.children.map(convertQueryTreeForBackend),
      },
    }
  }
}

// Frontend field names retain the established scalar spellings.
function fieldTypeFromWire(type: JsonAttributeType): FieldType {
  switch (type) {
    case 'bool':
      return 'boolean'
    case 'double':
      return 'float64'
    default:
      return type
  }
}

// Helper function to convert backend attribute data to FieldDefinition objects
function convertAttributesToFieldDefinitions(
  attributes: JsonAttributeDefinition[]
): FieldDefinition[] {
  const definitions: FieldDefinition[] = []
  for (const attr of attributes) {
    if (!attr?.name || !attr.type || !attr.attributeScope) continue
    const type = fieldTypeFromWire(attr.type)
    definitions.push({
      name: attr.name,
      type,
      searchScope: 'attribute',
      attributeScope: attr.attributeScope,
      operators: getOperatorsForFieldType(type),
    })
  }
  return definitions
}
