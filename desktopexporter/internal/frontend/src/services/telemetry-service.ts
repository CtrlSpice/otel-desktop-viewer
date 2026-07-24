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
} from '@/types/api-types'
import { parseBigInt, parseNullableBigInt } from '@/utils/bigint'
import type { QueryNode } from '@/components/shared/Search/queryTree'
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
// pattern-match on it to render a specific callout instead of a generic
// failure UI.
export class JsonRpcError extends Error {
  code: number
  constructor(code: number, message: string) {
    super(message)
    this.name = 'JsonRpcError'
    this.code = code
  }
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
    rootSpan: json.rootSpan ?? undefined,
    startTime: parseBigInt(json.startTime),
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
        startTime: parseBigInt(spanNode.spanData.startTime),
        endTime: parseBigInt(spanNode.spanData.endTime),
        events: spanNode.spanData.events
          ? spanNode.spanData.events.map((event: any) => ({
              ...event,
              timestamp: parseBigInt(event.timestamp),
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
    timestamp: parseBigInt(json.timestamp),
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
    timestamp: parseBigInt(json.timestamp),
    observedTimestamp: parseBigInt(json.observedTimestamp),
  }
}

function exemplarFromJSON(json: any): Exemplar {
  return {
    ...json,
    timestamp: parseBigInt(json.timestamp),
  }
}

function dataPointFromJSON(json: any): DataPoint {
  return {
    ...json,
    timestamp: parseBigInt(json.timestamp),
    startTime: parseBigInt(json.startTime),
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
    metricType: json.metricType ?? undefined,
    aggregationTemporality: json.aggregationTemporality ?? null,
    isMonotonic:
      json.isMonotonic === null || json.isMonotonic === undefined
        ? null
        : Boolean(json.isMonotonic),
    timeseries: json.timeseries?.map(timeseriesFromJSON) ?? [],
  }
}

function metricSummaryFromJSON(json: any): MetricSummary {
  return {
    ...json,
    description: json.description ?? '',
    serviceName: json.serviceName ?? '',
    seriesCount: Number(json.seriesCount ?? 0),
    dataPointCount: Number(json.dataPointCount ?? 0),
    lastSeen: parseBigInt(json.lastSeen),
  }
}

function metricSummariesFromJSON(json: any): MetricSummary[] {
  return json.map(metricSummaryFromJSON)
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
  searchMetricSummaries: async (
    startTime: number,
    endTime: number,
    queryTree?: QueryNode
  ): Promise<MetricSummary[]> => {
    const startTimeNs = toNanoseconds(startTime)
    const endTimeNs = toNanoseconds(endTime)
    const params = queryTree
      ? [startTimeNs, endTimeNs, convertQueryTreeForBackend(queryTree)]
      : [startTimeNs, endTimeNs]
    const rawData = await callRPC('searchMetricSummaries', params)
    return metricSummariesFromJSON(rawData)
  },

  getMetric: async (
    streamId: string,
    startTime: number,
    endTime: number
  ): Promise<MetricData | null> => {
    const startTimeNs = toNanoseconds(startTime)
    const endTimeNs = toNanoseconds(endTime)
    const rawData = await callRPC('getMetric', [
      streamId,
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
