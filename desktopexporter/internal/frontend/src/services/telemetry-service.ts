// Telemetry Service - JSON-RPC API client for OpenTelemetry Desktop Viewer

import type {
  TraceData,
  TraceSummary,
  LogData,
  LogSummary,
  MetricData,
  MetricTimeseries,
  MetricSummary,
  Stats,
  Exemplar,
  DataPoint,
  QuantileSeriesMode,
  QuantileSeriesPoint,
  BucketSeriesMode,
  BucketSeriesPoint,
} from '@/types/api-types'
import type { QueryNode } from '@/components/SignalToolbar/search/queryTree'
import type {
  AttributeScope,
  FieldDefinition,
  FieldType,
} from '@/constants/fields'
import { getOperatorsForFieldType } from '@/constants/operators'

// JSON-RPC Client
interface JsonRpcRequest {
  jsonrpc: '2.0'
  method: string
  params?: any
  id: number
}

interface JsonRpcResponse {
  jsonrpc: '2.0'
  result?: any
  error?: {
    code: number
    message: string
  }
  id: number
}

// Error subclass that preserves the JSON-RPC error code so callers can
// pattern-match on it (e.g. detect ErrCodeUnspecifiedTemporality and render
// the FunError callout instead of a generic failure UI).
export class JsonRpcError extends Error {
  code: number
  constructor(code: number, message: string) {
    super(message)
    this.name = 'JsonRpcError'
    this.code = code
  }
}

// Server error codes that callers care about. Mirrors
// desktopexporter/internal/server/errors.go. Keep this list small -- only
// codes the frontend pattern-matches on belong here.
export const ErrCodeUnspecifiedTemporality = -32013
export const ErrCodeHistogramBoundsMismatch = -32010

// Helper function to convert milliseconds to nanoseconds
function toNanoseconds(milliseconds: number): string {
  return milliseconds === 0 ? '0' : milliseconds.toString() + '000000'
}

// Helper function to make typed RPC calls
async function callRPC(method: string, params?: any): Promise<any> {
  const request: JsonRpcRequest = {
    method,
    params,
    id: Math.floor(Math.random() * 1000000),
    jsonrpc: '2.0',
  }

  try {
    const response = await fetch('/rpc', {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
      },
      body: JSON.stringify(request),
    })

    if (!response.ok) {
      throw new Error(`HTTP error! status: ${response.status}`)
    }

    const data: JsonRpcResponse = await response.json()

    if (data.error) {
      throw new JsonRpcError(data.error.code, data.error.message)
    }

    return data.result
  } catch (error) {
    throw error
  }
}

// Data Transformation Functions

// Helper functions to deserialize timestamps
function traceSummaryFromJSON(json: any): TraceSummary {
  return {
    ...json,
    rootSpan: json.rootSpan
      ? {
          ...json.rootSpan,
          startTime: BigInt(json.rootSpan.startTime),
          endTime: BigInt(json.rootSpan.endTime),
        }
      : undefined,
    startTime: BigInt(json.startTime),
    // durationNs arrives as a varchar-encoded int64 (ns precision
    // would otherwise be clipped by JSON's float64 numbers).
    durationNs: parseNullableBigInt(json.durationNs),
  }
}

function traceSummariesFromJSON(json: any): TraceSummary[] {
  return json.map(traceSummaryFromJSON)
}

function traceDataFromJSON(json: any): TraceData {
  return {
    ...json,
    spans: json.spans.map((spanNode: any) => ({
      spanData: {
        ...spanNode.spanData,
        startTime: BigInt(spanNode.spanData.startTime),
        endTime: BigInt(spanNode.spanData.endTime),
        events: spanNode.spanData.events
          ? spanNode.spanData.events.map((event: any) => ({
              ...event,
              timestamp:
                event.timestamp && event.timestamp !== undefined
                  ? BigInt(event.timestamp)
                  : undefined,
            }))
          : [],
        links: spanNode.spanData.links || [],
      },
      depth: spanNode.depth,
      matched: spanNode.matched ?? true,
    })),
  }
}

// Summary projection returned by searchLogs. Lightweight -- only
// promote the one bigint field (timestamp); the rest are plain
// primitives on the wire.
function logSummaryFromJSON(json: any): LogSummary {
  return {
    ...json,
    timestamp: BigInt(json.timestamp),
  }
}

function logSummariesFromJSON(json: any): LogSummary[] {
  return json.map(logSummaryFromJSON)
}

// Full log row returned by getLog(id). Promotes both timestamp
// columns; everything else matches the wire shape.
function logDataFromJSON(json: any): LogData {
  return {
    ...json,
    timestamp: BigInt(json.timestamp),
    observedTimestamp: BigInt(json.observedTimestamp),
  }
}

function exemplarFromJSON(json: any): Exemplar {
  return {
    ...json,
    timestamp: BigInt(json.timestamp),
  }
}

function dataPointFromJSON(json: any): DataPoint {
  return {
    ...json,
    timestamp: BigInt(json.timestamp),
    startTime: BigInt(json.startTime),
    exemplars: json.exemplars?.map(exemplarFromJSON) ?? [],
  }
}

// MetricTimeseries owns the attribute set for its group (lifted out
// of the per-dp objects on the wire). The reviver itself has no
// BigInts to promote at the timeseries level -- attributesKey is a
// plain string and attributes are already plain string/string/string
// trios -- so it's just a recursive call into the timeseries'
// datapoints to revive their timestamp BigInts.
function timeseriesFromJSON(json: any): MetricTimeseries {
  return {
    attributesKey: json.attributesKey ?? '',
    attributes: json.attributes ?? [],
    datapoints: json.datapoints?.map(dataPointFromJSON) ?? [],
  }
}

function metricDataFromJSON(json: any): MetricData {
  return {
    ...json,
    timeseries: json.timeseries?.map(timeseriesFromJSON) ?? [],
  }
}

function metricsFromJSON(json: any): MetricData[] {
  return json.map(metricDataFromJSON)
}

function metricSummaryFromJSON(json: any): MetricSummary {
  return {
    ...json,
    description: json.description ?? '',
    serviceName: json.serviceName ?? '',
    lastSeen: BigInt(json.lastSeen),
  }
}

function metricSummariesFromJSON(json: any): MetricSummary[] {
  return json.map(metricSummaryFromJSON)
}

function parseNullableBigInt(value: unknown): bigint | null {
  if (value === null || value === undefined) return null
  if (typeof value === 'bigint') return value
  if (typeof value === 'string' || typeof value === 'number')
    return BigInt(value)
  throw new Error(`Invalid bigint value: ${String(value)}`)
}

function statsFromJSON(json: any): Stats {
  return {
    traces: {
      ...json.traces,
      lastReceived: parseNullableBigInt(json.traces?.lastReceived),
    },
    logs: {
      ...json.logs,
      lastReceived: parseNullableBigInt(json.logs?.lastReceived),
    },
    metrics: {
      ...json.metrics,
      lastReceived: parseNullableBigInt(json.metrics?.lastReceived),
    },
  }
}

// API Methods

// Export typed methods for each RPC call with built-in conversion
export let telemetryAPI = {
  // Trace methods
  getTraceAttributes: async (
    startTime: number,
    endTime: number
  ): Promise<FieldDefinition[]> => {
    const startTimeNs = toNanoseconds(startTime)
    const endTimeNs = toNanoseconds(endTime)
    const params = [startTimeNs, endTimeNs]
    const rawData = await callRPC('getTraceAttributes', params)

    // Validate that we received an array
    if (!Array.isArray(rawData)) {
      console.warn(
        'getTraceAttributes: Expected array, got:',
        typeof rawData,
        rawData
      )
      return []
    }

    // Convert backend attribute data to FieldDefinition objects
    const converted = convertAttributesToFieldDefinitions(rawData)
    return converted
  },

  getAttributesByTraceID: async (
    traceID: string
  ): Promise<FieldDefinition[]> => {
    const rawData = await callRPC('getAttributesByTraceID', [traceID])
    if (!Array.isArray(rawData)) {
      console.warn(
        'getAttributesByTraceID: Expected array, got:',
        typeof rawData,
        rawData
      )
      return []
    }
    return convertAttributesToFieldDefinitions(rawData)
  },

  getLogAttributes: async (
    startTime: number,
    endTime: number
  ): Promise<FieldDefinition[]> => {
    const startTimeNs = toNanoseconds(startTime)
    const endTimeNs = toNanoseconds(endTime)
    const params = [startTimeNs, endTimeNs]
    const rawData = await callRPC('getLogAttributes', params)

    if (!Array.isArray(rawData)) {
      console.warn(
        'getLogAttributes: Expected array, got:',
        typeof rawData,
        rawData
      )
      return []
    }

    return convertAttributesToFieldDefinitions(rawData)
  },

  searchTraces: async (
    startTime: number,
    endTime: number,
    queryTree?: QueryNode
  ): Promise<TraceSummary[]> => {
    const startTimeNs = toNanoseconds(startTime)
    const endTimeNs = toNanoseconds(endTime)

    const params = queryTree
      ? [startTimeNs, endTimeNs, convertQueryTreeForBackend(queryTree)]
      : [startTimeNs, endTimeNs]
    const rawData = await callRPC('searchTraces', params)
    return traceSummariesFromJSON(rawData)
  },

  searchSpans: async (
    traceID: string,
    queryTree?: QueryNode
  ): Promise<TraceData> => {
    const params = queryTree
      ? [traceID, convertQueryTreeForBackend(queryTree)]
      : [traceID]
    const rawData = await callRPC('searchSpans', params)
    return traceDataFromJSON(rawData)
  },

  clearTraces: () => callRPC('clearTraces', undefined),
  deleteTraces: (traceIDs: string[]) =>
    callRPC('deleteSpansByTraceID', traceIDs),

  // Log methods
  //
  // searchLogs returns LogSummary[] -- a card-shaped projection
  // without bodies/attributes/etc. Use getLog(id) to fetch the
  // full LogData for one row when the detail pane opens.
  searchLogs: async (
    startTime: number,
    endTime: number,
    queryTree?: QueryNode
  ): Promise<LogSummary[]> => {
    const startTimeNs = toNanoseconds(startTime)
    const endTimeNs = toNanoseconds(endTime)
    const params = queryTree
      ? [startTimeNs, endTimeNs, convertQueryTreeForBackend(queryTree)]
      : [startTimeNs, endTimeNs]
    const rawData = await callRPC('searchLogs', params)
    return logSummariesFromJSON(rawData)
  },

  getLog: async (logID: string): Promise<LogData> => {
    const rawData = await callRPC('getLog', [logID])
    return logDataFromJSON(rawData)
  },

  getLogsByTraceID: (traceID: string) => callRPC('getLogsByTraceID', [traceID]),
  deleteLogByID: (logId: string) => callRPC('deleteLogByID', [logId]),
  clearLogs: () => callRPC('clearLogs', undefined),

  // Metric methods
  getMetrics: async (
    startTime: number,
    endTime: number,
    queryTree?: QueryNode
  ): Promise<MetricData[]> => {
    const startTimeNs = toNanoseconds(startTime)
    const endTimeNs = toNanoseconds(endTime)
    const params = queryTree
      ? [startTimeNs, endTimeNs, convertQueryTreeForBackend(queryTree)]
      : [startTimeNs, endTimeNs]
    const rawData = await callRPC('getMetrics', params)
    return metricsFromJSON(rawData)
  },

  searchMetricSummaries: async (
    startTime: number,
    endTime: number
  ): Promise<MetricSummary[]> => {
    const startTimeNs = toNanoseconds(startTime)
    const endTimeNs = toNanoseconds(endTime)
    const rawData = await callRPC('searchMetricSummaries', [
      startTimeNs,
      endTimeNs,
    ])
    return metricSummariesFromJSON(rawData)
  },

  getMetric: async (
    streamId: string,
    startTime: number,
    endTime: number
  ): Promise<MetricData | null> => {
    const startTimeNs = toNanoseconds(startTime)
    const endTimeNs = toNanoseconds(endTime)
    const rawData = await callRPC('getMetric', [streamId, startTimeNs, endTimeNs])
    if (rawData === null || rawData === 'null') return null
    return metricDataFromJSON(rawData)
  },

  getMetricAttributes: async (
    startTime: number,
    endTime: number
  ): Promise<FieldDefinition[]> => {
    const startTimeNs = toNanoseconds(startTime)
    const endTimeNs = toNanoseconds(endTime)
    const params = [startTimeNs, endTimeNs]
    const rawData = await callRPC('getMetricAttributes', params)

    if (!Array.isArray(rawData)) {
      console.warn(
        'getMetricAttributes: Expected array, got:',
        typeof rawData,
        rawData
      )
      return []
    }

    return convertAttributesToFieldDefinitions(rawData)
  },

  // Returns a map of quantile -> interpolated value for a single histogram or
  // exponential-histogram datapoint. Keys come back as the quantile formatted
  // by Go's strconv.FormatFloat with -1 precision (e.g. "0.5", "0.95"). A
  // value of null means the macro declined to interpolate (empty buckets or
  // total count of zero) -- callers should render it as an em-dash or similar.
  getDatapointQuantiles: async (
    datapointID: string,
    quantiles: number[]
  ): Promise<Record<string, number | null>> => {
    const rawData = await callRPC('getDatapointQuantiles', [
      datapointID,
      quantiles,
    ])
    if (rawData === null || typeof rawData !== 'object') {
      console.warn(
        'getDatapointQuantiles: Expected object, got:',
        typeof rawData,
        rawData
      )
      return {}
    }
    return rawData as Record<string, number | null>
  },

  // Returns one quantile sample per adaptive time bucket for a histogram
  // or exponential-histogram metric. The backend computes bucket width as
  // greatest(1ms, (endTs - startTs) / maxPoints), so callers should pass
  // maxPoints = chart pixel width. Time params are accepted in ms (matching
  // searchMetrics et al) and converted to ns strings on the wire to dodge
  // float64 precision loss for large epoch-ns values.
  //
  // Mode controls cross-timeseries merging:
  //   - per-attribute: one point per (bucket, attribute set)
  //   - merged: one point per bucket with all timeseries merged
  //     (Histogram requires uniform explicit_bounds within each bucket;
  //     mismatches surface as JSON-RPC error code -32010).
  getMetricQuantileSeries: async (
    metricID: string,
    quantiles: number[],
    mode: QuantileSeriesMode,
    startTime: number,
    endTime: number,
    maxPoints: number
  ): Promise<QuantileSeriesPoint[]> => {
    const startTsNs = toNanoseconds(startTime)
    const endTsNs = toNanoseconds(endTime)
    const rawData = await callRPC('getMetricQuantileSeries', [
      metricID,
      quantiles,
      mode,
      startTsNs,
      endTsNs,
      maxPoints,
    ])
    if (!Array.isArray(rawData)) {
      console.warn(
        'getMetricQuantileSeries: Expected array, got:',
        typeof rawData,
        rawData
      )
      return []
    }
    // bucket_start arrives as a JSON number from Go. Funnel it through
    // BigInt so callers always see the same nanosecond shape as DataPoint.
    return rawData.map((pt: any) => ({
      ...pt,
      timestamp: BigInt(pt.timestamp),
      attributes: pt.attributes ?? [],
    })) as QuantileSeriesPoint[]
  },

  // Returns raw bucket vectors per adaptive time bucket for a histogram or
  // exponential-histogram metric. Same bucketing and temporality semantics as
  // getMetricQuantileSeries, but no quantile computation -- callers receive
  // the merged distribution data directly for heatmap rendering.
  getMetricBucketSeries: async (
    metricID: string,
    mode: BucketSeriesMode,
    startTime: number,
    endTime: number,
    maxPoints: number
  ): Promise<BucketSeriesPoint[]> => {
    const startTsNs = toNanoseconds(startTime)
    const endTsNs = toNanoseconds(endTime)
    const rawData = await callRPC('getMetricBucketSeries', [
      metricID,
      mode,
      startTsNs,
      endTsNs,
      maxPoints,
    ])
    if (!Array.isArray(rawData)) {
      console.warn(
        'getMetricBucketSeries: Expected array, got:',
        typeof rawData,
        rawData
      )
      return []
    }
    return rawData.map((pt: any) => ({
      ...pt,
      timestamp: BigInt(pt.timestamp),
      attributes: pt.attributes ?? [],
    })) as BucketSeriesPoint[]
  },

  // Returns one set of quantile values for a histogram/exp-histogram metric
  // computed across the entire [startTime, endTime) window with all
  // per-attribute timeseries merged. Same return shape as
  // getDatapointQuantiles ("0.5" -> value), so HistogramChart can consume
  // both. Time params are in ms and converted to ns strings on the wire
  // (large epoch-ns values won't survive float64).
  //
  // Errors mirror getMetricQuantileSeries(mode='merged'): unspecified
  // temporality (-32011), bounds mismatch (-32010), unsupported metric type.
  // The Aggregated tab's bar chart relies on getMetricBucketSeries succeeding
  // first; if THAT errors, the chart never renders and this RPC is moot.
  getMetricMergedQuantiles: async (
    metricID: string,
    quantiles: number[],
    startTime: number,
    endTime: number
  ): Promise<Record<string, number | null>> => {
    const startTsNs = toNanoseconds(startTime)
    const endTsNs = toNanoseconds(endTime)
    const rawData = await callRPC('getMetricMergedQuantiles', [
      metricID,
      quantiles,
      startTsNs,
      endTsNs,
    ])
    if (rawData === null || typeof rawData !== 'object') {
      console.warn(
        'getMetricMergedQuantiles: Expected object, got:',
        typeof rawData,
        rawData
      )
      return {}
    }
    return rawData as Record<string, number | null>
  },

  clearMetrics: () => callRPC('clearMetrics', undefined),

  // Stats methods
  getStats: async (): Promise<Stats> => {
    const rawData = await callRPC('getStats')
    return statsFromJSON(rawData)
  },

  getTraceSpanCount: async (traceID: string): Promise<number> => {
    return await callRPC('getTraceSpanCount', [traceID])
  },
}

// Helper function to convert frontend query tree to minimal backend format
function convertQueryTreeForBackend(queryTree: QueryNode): any {
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

// Helper function to convert backend attribute data to FieldDefinition objects
function convertAttributesToFieldDefinitions(
  attributes: { name: string; type: string; attributeScope: string }[]
): FieldDefinition[] {
  return attributes
    .filter(attr => attr && attr.name && attr.type && attr.attributeScope) // Filter out invalid entries
    .map(attr => ({
      name: attr.name,
      type: attr.type as FieldType,
      searchScope: 'attribute' as const,
      attributeScope: attr.attributeScope as AttributeScope,
      operators: getOperatorsForFieldType(attr.type as FieldType),
    }))
}
