// Telemetry Service - JSON-RPC API client for OpenTelemetry Desktop Viewer

import type {
  TraceData,
  TraceSummary,
  LogData,
  MetricData,
  MetricSummary,
  SparklinePoint,
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
      throw new Error(`JSON-RPC Error: ${data.error.message}`)
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

function logsFromJSON(json: any): LogData[] {
  return json.map((log: any) => ({
    ...log,
    timestamp: BigInt(log.timestamp),
    observedTimestamp: BigInt(log.observedTimestamp),
  }))
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

function metricDataFromJSON(json: any): MetricData {
  return {
    ...json,
    datapoints: json.datapoints?.map(dataPointFromJSON) ?? [],
    received: BigInt(json.received),
  }
}

function metricsFromJSON(json: any): MetricData[] {
  return json.map(metricDataFromJSON)
}

function sparklinePointFromJSON(json: any): SparklinePoint {
  return {
    timestamp: BigInt(json.timestamp),
    value: json.value,
  }
}

function metricSummaryFromJSON(json: any): MetricSummary {
  return {
    ...json,
    received: BigInt(json.received),
    sparkline: json.sparkline?.map(sparklinePointFromJSON) ?? null,
    sparkbar: json.sparkbar ?? null,
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
  searchLogs: async (
    startTime: number,
    endTime: number,
    queryTree?: QueryNode
  ): Promise<LogData[]> => {
    const startTimeNs = toNanoseconds(startTime)
    const endTimeNs = toNanoseconds(endTime)
    const params = queryTree
      ? [startTimeNs, endTimeNs, convertQueryTreeForBackend(queryTree)]
      : [startTimeNs, endTimeNs]
    const rawData = await callRPC('searchLogs', params)
    return logsFromJSON(rawData)
  },

  getLogByID: (logID: string) => callRPC('getLogByID', [logID]),
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
    name: string,
    unit: string,
    metricType: string,
    aggregationTemporality: string,
    isMonotonic: string,
    scopeName: string,
    scopeVersion: string,
    serviceName: string,
    startTime: number,
    endTime: number
  ): Promise<MetricData | null> => {
    const startTimeNs = toNanoseconds(startTime)
    const endTimeNs = toNanoseconds(endTime)
    const rawData = await callRPC('getMetric', [
      name,
      unit,
      metricType,
      aggregationTemporality,
      isMonotonic,
      scopeName,
      scopeVersion,
      serviceName,
      startTimeNs,
      endTimeNs,
    ])
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
  // Mode controls cross-stream merging:
  //   - per-stream: one point per (bucket, attribute set)
  //   - aggregated: one point per bucket with all streams merged (Histogram
  //     requires uniform explicit_bounds within each bucket; mismatches
  //     surface as JSON-RPC error code -32010).
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

  deleteMetrics: (metricIDs: string[]) =>
    callRPC('deleteMetricByID', metricIDs),

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
