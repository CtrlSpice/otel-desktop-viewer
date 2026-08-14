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
import type {
  JsonAttributeDefinition,
  JsonDataPoint,
  JsonDeleteResult,
  JsonExemplar,
  JsonLogData,
  JsonLogSummary,
  JsonMetricData,
  JsonMetricSummary,
  JsonMetricTimeseries,
  JsonStats,
  JsonTraceData,
  JsonTraceSummary,
  JsonAttributeType,
  JsonQueryNode,
  JsonAttributeMatch,
} from '@/types/wire-types'
import { parseBigInt, parseNullableBigInt } from '@/utils/bigint'
import type { QueryNode } from '@/components/shared/Search/queryTree'
import type { FieldDefinition, FieldType } from '@/constants/fields'
import { getOperatorsForFieldType } from '@/constants/operators'

// JSON-RPC Client
interface JsonRpcRequest {
  jsonrpc: '2.0'
  method: string
  params?: unknown
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

// Custom error codes minted by the backend (internal/server/errors.go).
const ERR_CODE_METRIC_NOT_FOUND = -32003

// Helper function to convert milliseconds to nanoseconds
function toNanoseconds(milliseconds: number): string {
  return milliseconds === 0 ? '0' : milliseconds.toString() + '000000'
}

/** Thrown when a request is abandoned. Callers that supersede their own
 *  requests should swallow this rather than surfacing it as an error -- the
 *  result was discarded on purpose. */
export class RequestAbortedError extends Error {
  constructor() {
    super('Request aborted')
    this.name = 'RequestAbortedError'
  }
}

/** True when a rejection is just an abandoned request. */
export function isAbortError(err: unknown): boolean {
  return (
    err instanceof RequestAbortedError ||
    (err instanceof DOMException && err.name === 'AbortError')
  )
}

// Generic JSON-RPC transport. T is a compile-time assertion of the wire
// shape (see wire-types.ts), not runtime validation -- the backend is
// trusted to serve what its projections declare.
//
// `signal` is what makes server-side cancellation reachable. The backend
// already tears a running DuckDB query down when the request context is
// cancelled, but without a signal here the fetch survives navigation, so the
// query runs to completion holding the store's read lock for a result nobody
// will read. Aborting the fetch closes the connection, which cancels
// request.Context(), which interrupts the query.
async function callRPC<T>(
  method: string,
  params?: unknown,
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

  if (data.error) {
    throw new JsonRpcError(data.error.code, data.error.message)
  }

  return data.result as T
}

// Data Transformation Functions

// Helper functions to deserialize timestamps
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
    startTime: parseBigInt(json.startTime),
    // durationNs arrives as a varchar-encoded int64 (ns precision
    // would otherwise be clipped by JSON's float64 numbers).
    durationNs: parseNullableBigInt(json.durationNs),
  }
}

function traceSummariesFromJSON(json: JsonTraceSummary[]): TraceSummary[] {
  return json.map(traceSummaryFromJSON)
}

// Rehydrates the compressed searchSpans wire shape into the SpanData the views
// expect: references resolved against the response's resource and scope maps,
// times reconstructed from the trace baseline, traceID reattached.
//
// Every compression the wire format applies is undone here, at one boundary, so
// no view knows the transport changed. That was worth doing deliberately -- the
// waterfall, the detail panel and the search results all read SpanData, and
// pushing `r`/`s` lookups into each of them would have spread the wire format
// across the app for no benefit.
//
// Resolved resources and scopes are *shared*, not copied: all 4,891 spans of a
// trace point at the same 23 resource objects. They are read-only downstream,
// and copying them per span would rebuild client-side exactly the duplication
// the wire format just removed.
function traceDataFromJSON(json: JsonTraceData): TraceData {
  const traceStart = parseBigInt(json.traceStart)

  return {
    traceID: json.traceID,
    // events is coalesced to [] server-side and matched is always
    // emitted (literal true when no search criteria), so no fallbacks;
    // links rides the spanData spread untouched.
    spans: json.spans.map(spanNode => {
      const { r, s, start, dur, ...rest } = spanNode.spanData
      const startTime = traceStart + BigInt(start)
      return {
        spanData: {
          ...rest,
          traceID: json.traceID,
          resource: json.resources[String(r)],
          scope: json.scopes[String(s)],
          startTime,
          endTime: startTime + BigInt(dur),
          events: spanNode.spanData.events.map(event => ({
            ...event,
            timestamp: parseBigInt(event.timestamp),
          })),
        },
        depth: spanNode.depth,
        matched: spanNode.matched,
      }
    }),
  }
}

// Summary projection returned by searchLogs. Lightweight -- only
// promote the one bigint field (timestamp); the rest are plain
// primitives on the wire.
function logSummaryFromJSON(json: JsonLogSummary): LogSummary {
  return {
    ...json,
    timestamp: parseBigInt(json.timestamp),
  }
}

function logSummariesFromJSON(json: JsonLogSummary[]): LogSummary[] {
  return json.map(logSummaryFromJSON)
}

// Full log row returned by getLog(id). Promotes both timestamp
// columns; everything else matches the wire shape.
function logDataFromJSON(json: JsonLogData): LogData {
  return {
    ...json,
    timestamp: parseBigInt(json.timestamp),
    observedTimestamp: parseBigInt(json.observedTimestamp),
  }
}

function exemplarFromJSON(json: JsonExemplar): Exemplar {
  return {
    ...json,
    timestamp: parseBigInt(json.timestamp),
  }
}

function dataPointFromJSON(json: JsonDataPoint): DataPoint {
  return {
    ...json,
    timestamp: parseBigInt(json.timestamp),
    startTime: parseBigInt(json.startTime),
    exemplars: json.exemplars.map(exemplarFromJSON),
  }
}

// MetricTimeseries owns the attribute set for its group (lifted out
// of the per-dp objects on the wire). The reviver itself has no
// BigInts to promote at the timeseries level -- attributesKey is a
// plain string and attributes are already plain string/string/string
// trios -- so it's just a recursive call into the timeseries'
// datapoints to revive their timestamp BigInts.
function timeseriesFromJSON(json: JsonMetricTimeseries): MetricTimeseries {
  return {
    attributesKey: json.attributesKey,
    attributes: json.attributes,
    resource: json.resource,
    datapoints: json.datapoints.map(dataPointFromJSON),
    stats: json.stats ?? null,
  }
}

function metricDataFromJSON(json: JsonMetricData): MetricData {
  return {
    ...json,
    timeseries: json.timeseries.map(timeseriesFromJSON),
  }
}

function metricSummaryFromJSON(json: JsonMetricSummary): MetricSummary {
  return {
    ...json,
    description: json.description ?? '',
    lastSeen: parseBigInt(json.lastSeen),
  }
}

function metricSummariesFromJSON(json: JsonMetricSummary[]): MetricSummary[] {
  return json.map(metricSummaryFromJSON)
}

function statsFromJSON(json: JsonStats): Stats {
  return {
    traces: {
      ...json.traces,
      lastReceived: parseNullableBigInt(json.traces.lastReceived),
    },
    logs: {
      ...json.logs,
      lastReceived: parseNullableBigInt(json.logs.lastReceived),
    },
    metrics: {
      ...json.metrics,
      lastReceived: parseNullableBigInt(json.metrics.lastReceived),
    },
  }
}

// API Methods

// Export typed methods for each RPC call with built-in conversion
export let telemetryAPI = {
  // Value-first discovery: given text the user can see, which attribute keys
  // hold it. Cross-signal by nature -- the dictionary it reads is shared by
  // traces, logs and metrics -- so unlike getXAttributes it takes no signal and
  // no time range.
  searchAttributes: async (term: string): Promise<JsonAttributeMatch[]> => {
    if (!term.trim()) return []
    const rawData = await callRPC<JsonAttributeMatch[]>('searchAttributes', [
      term,
    ])
    if (!Array.isArray(rawData)) {
      console.warn('searchAttributes: Expected array, got:', typeof rawData)
      return []
    }
    return rawData
  },

  // Trace methods
  getTraceAttributes: async (
    startTime: number,
    endTime: number
  ): Promise<FieldDefinition[]> => {
    const startTimeNs = toNanoseconds(startTime)
    const endTimeNs = toNanoseconds(endTime)
    const params = [startTimeNs, endTimeNs]
    const rawData = await callRPC<JsonAttributeDefinition[]>(
      'getTraceAttributes',
      params
    )

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
    const rawData = await callRPC<JsonAttributeDefinition[]>(
      'getAttributesByTraceID',
      [traceID]
    )
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
    const rawData = await callRPC<JsonAttributeDefinition[]>(
      'getLogAttributes',
      params
    )

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
    const rawData = await callRPC<JsonTraceSummary[]>('searchTraces', params)
    return traceSummariesFromJSON(rawData)
  },

  // signal is plumbed here first because searchSpans is the heaviest query
  // and the one most often abandoned -- clicking through traces supersedes it
  // repeatedly.
  searchSpans: async (
    traceID: string,
    queryTree?: QueryNode,
    signal?: AbortSignal
  ): Promise<TraceData> => {
    const params = queryTree
      ? [traceID, convertQueryTreeForBackend(queryTree)]
      : [traceID]
    const rawData = await callRPC<JsonTraceData>('searchSpans', params, signal)
    return traceDataFromJSON(rawData)
  },

  clearTraces: () => callRPC<string>('clearTraces', undefined),
  deleteTraces: (traceIDs: string[]) =>
    callRPC<JsonDeleteResult>('deleteSpansByTraceID', traceIDs),

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
    const rawData = await callRPC<JsonLogSummary[]>('searchLogs', params)
    return logSummariesFromJSON(rawData)
  },

  getLog: async (logID: string): Promise<LogData> => {
    const rawData = await callRPC<JsonLogData>('getLog', [logID])
    return logDataFromJSON(rawData)
  },

  deleteLogByID: (logId: string) =>
    callRPC<JsonDeleteResult>('deleteLogByID', [logId]),
  clearLogs: () => callRPC<string>('clearLogs', undefined),

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
    const rawData = await callRPC<JsonMetricSummary[]>(
      'searchMetricSummaries',
      params
    )
    return metricSummariesFromJSON(rawData)
  },

  getMetric: async (
    streamId: string,
    startTime: number,
    endTime: number,
    /** How many time buckets to reduce the window to. Omit for every
     *  datapoint. The store keeps up to four points per bucket -- first, last,
     *  smallest, largest -- so the drawn line is the same as it would be with
     *  every point, and it ignores this entirely for histograms, which need
     *  merging rather than sampling. */
    targetBuckets?: number
  ): Promise<MetricData | null> => {
    const startTimeNs = toNanoseconds(startTime)
    const endTimeNs = toNanoseconds(endTime)
    // Not-found arrives as a JSON-RPC error (one wire convention across all
    // signals); translate it to null here so callers keep a simple contract.
    try {
      const rawData = await callRPC<JsonMetricData>('getMetric', [
        streamId,
        startTimeNs,
        endTimeNs,
        ...(targetBuckets ? [String(targetBuckets)] : []),
      ])
      return metricDataFromJSON(rawData)
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

  getMetricAttributes: async (
    startTime: number,
    endTime: number
  ): Promise<FieldDefinition[]> => {
    const startTimeNs = toNanoseconds(startTime)
    const endTimeNs = toNanoseconds(endTime)
    const params = [startTimeNs, endTimeNs]
    const rawData = await callRPC<JsonAttributeDefinition[]>(
      'getMetricAttributes',
      params
    )

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

  // Takes a bare stream id, not an array: metrics address a stream by a single
  // uuid everywhere (see getMetric), unlike deleteLogByID / deleteTraces.
  deleteMetricStream: (streamID: string) =>
    callRPC<string>('deleteMetricStream', [streamID]),
  clearMetrics: () => callRPC<string>('clearMetrics', undefined),

  // Stats methods
  getStats: async (): Promise<Stats> => {
    const rawData = await callRPC<JsonStats>('getStats')
    return statsFromJSON(rawData)
  },

  getTraceSpanCount: async (traceID: string): Promise<number> => {
    return await callRPC<number>('getTraceSpanCount', [traceID])
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

// DuckDB's attr_type enum spells scalar booleans 'bool'; the frontend's
// FieldType uses 'boolean' (matching the enum's own 'boolean[]' arrays).
// Translate at the boundary -- without this, boolean attributes fall
// through getOperatorsForFieldType's default branch and lose the
// boolean operator set.
function fieldTypeFromWire(type: JsonAttributeType): FieldType {
  return type === 'bool' ? 'boolean' : type
}

// Helper function to convert backend attribute data to FieldDefinition objects
function convertAttributesToFieldDefinitions(
  attributes: JsonAttributeDefinition[]
): FieldDefinition[] {
  return attributes
    .filter(attr => attr && attr.name && attr.type && attr.attributeScope) // Filter out invalid entries
    .map(attr => {
      const type = fieldTypeFromWire(attr.type)
      return {
        name: attr.name,
        type,
        searchScope: 'attribute' as const,
        attributeScope: attr.attributeScope,
        operators: getOperatorsForFieldType(type),
      }
    })
}
