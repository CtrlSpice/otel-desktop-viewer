import type {
  TraceData,
  TraceWaterfallData,
  TraceColumn,
  TraceColumnPreview,
  SpanData,
  TraceSummary,
  SpanTiming,
  LogData,
  LogSummary,
  LogColumn,
  LogTableRow,
  LogErrorNavigation,
  TraceLogSummary,
  MetricViewData,
  MetricSummary,
  Stats,
  MetricAggregateViewData,
  ExactMetric,
  ExactMetricSeries,
} from '@/types/api-types'
import type {
  JsonDeleteResult,
  JsonStats,
  JsonQueryNode,
} from '@/types/wire-types'
import type {
  DiscoveryOptions,
  DiscoveryPage,
  ServiceSummary,
  ServiceConnection,
  ServiceConnectionSpan,
  AttributeKey,
  AttributeFrequency,
  HomeCoveragePage,
} from './discovery-types'
import {
  attributeMatchesSQL,
  type AttributeMatchOptions,
  type AttributeRecordMatch,
} from './attribute-matches'
import type { FieldDefinition, QueryNode } from '@/search/model'
import {
  attributeDefinitions,
  attributeMatch,
  type AttributeMatch,
} from './arrow-search'
import { fetchArrow } from './arrow-fetch'
import { attributeValue, rowViews } from './arrow-values'
import { ArrowFrame } from './arrow-data'
import {
  traceData,
  traceWaterfallData,
  traceColumnPreviews,
} from './arrow-traces'
import {
  discoveryPage,
  serviceSummary,
  serviceConnection,
  serviceConnectionSpan,
  attributeKey,
  attributeFrequency,
} from './arrow-discovery'
import {
  exactMetric,
  exactMetricSeries,
  metricSeriesOrder,
  metricView,
  metricAggregateView,
} from './arrow-metrics'
import {
  logData,
  logSummary,
  logTableRow,
  traceLogSummary,
  traceSummary,
  metricSummary,
} from './arrow-summaries'
import { JsonRpcError } from './service-error'
export { JsonRpcError } from './service-error'

type JsonRpcParamValue =
  | string
  | number
  | boolean
  | null
  | JsonQueryNode
  | SearchSort
  | readonly (string | number)[]
  | readonly LogColumn[]
  | readonly TraceColumn[]
  | (Omit<DiscoveryOptions, 'queries'> & {
      queries?: Record<
        'traces' | 'metrics' | 'logs',
        JsonQueryNode | false | null
      >
    })

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

async function callArrow(
  method: string,
  params: JsonRpcNamedParams,
  signal?: AbortSignal
): Promise<ArrowFrame> {
  try {
    return await fetchArrow(method, JSON.stringify(params), signal)
  } catch (error) {
    if (signal?.aborted || isAbortError(error)) throw new RequestAbortedError()
    throw error
  }
}

function statsFromJSON(json: JsonStats): Stats {
  return {
    storage: json.storage,
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

async function discover<T>(
  options: DiscoveryOptions,
  view: (frame: ArrowFrame, row: number) => T,
  signal?: AbortSignal
): Promise<DiscoveryPage<T>> {
  const convert = (query: QueryNode | false | null) =>
    query ? convertQueryTreeForBackend(query) : query
  const { queries, ...plainOptions } = options
  const wireOptions = queries
    ? {
        ...plainOptions,
        queries: {
          traces: convert(queries.traces),
          metrics: convert(queries.metrics),
          logs: convert(queries.logs),
        },
      }
    : plainOptions
  return discoveryPage(
    await fetchArrow(
      'discoverTelemetry',
      JSON.stringify({ options: wireOptions }),
      signal
    ),
    view
  )
}

export let telemetryAPI = {
  discoverHomeAttributeValues: (
    options: Omit<DiscoveryOptions, 'kind' | 'signal'>,
    signal?: AbortSignal
  ): Promise<HomeCoveragePage> => {
    const convert = (query: QueryNode | false | null) =>
      query ? convertQueryTreeForBackend(query) : query
    return callRPC(
      'discoverTelemetry',
      named({
        options: {
          ...options,
          kind: 'home-values',
          queries: options.queries
            ? {
                traces: convert(options.queries.traces),
                metrics: convert(options.queries.metrics),
                logs: convert(options.queries.logs),
              }
            : undefined,
        },
      }),
      signal
    )
  },
  getHomeAttributeValue: async (valueRef: string, signal?: AbortSignal) => {
    const frame = await callArrow(
      'discoverTelemetry',
      named({
        options: {
          kind: 'home-value',
          valueRef,
        },
      }),
      signal
    )
    return attributeValue(frame.cell('value', 0))
  },
  discoverServiceConnections: (
    options: Omit<DiscoveryOptions, 'kind'>,
    signal?: AbortSignal
  ) =>
    discover<ServiceConnection>(
      { ...options, kind: 'connections' },
      serviceConnection,
      signal
    ),
  discoverServiceConnectionSpans: (
    options: Omit<DiscoveryOptions, 'kind'>,
    signal?: AbortSignal
  ) =>
    discover<ServiceConnectionSpan>(
      { ...options, kind: 'connection-spans' },
      serviceConnectionSpan,
      signal
    ),
  getAttributeRecordMatches: async (
    options: AttributeMatchOptions,
    signal?: AbortSignal
  ): Promise<AttributeRecordMatch[]> => {
    if (options.valueIdentity?.startsWith('home:')) {
      const resolved = await callRPC<{ identity: string }>(
        'discoverTelemetry',
        named({
          options: { kind: 'home-value', valueRef: options.valueIdentity },
        }),
        signal
      )
      options = { ...options, valueIdentity: resolved.identity }
    }
    const matches: AttributeRecordMatch[] = []
    let more = true
    while (more) {
      signal?.throwIfAborted()
      const result = await callRPC<{
        rows: [string, string[], number][]
        truncated: boolean
      }>(
        'query',
        named({
          sql: attributeMatchesSQL(options, matches.length),
          limit: 500,
        }),
        signal
      )
      matches.push(
        ...result.rows.map(([itemRef, spanIDs, count]) => ({
          itemRef,
          spanIDs,
          count,
        }))
      )
      more = result.truncated
    }
    return matches
  },
  discoverServices: (
    options: Omit<DiscoveryOptions, 'kind'> = {},
    signal?: AbortSignal
  ) =>
    discover<ServiceSummary>(
      { ...options, kind: 'services' },
      serviceSummary,
      signal
    ),
  discoverAttributeKeys: (
    options: Omit<DiscoveryOptions, 'kind'>,
    signal?: AbortSignal
  ) =>
    discover<AttributeKey>({ ...options, kind: 'keys' }, attributeKey, signal),
  discoverAttributeValues: (
    options: Omit<DiscoveryOptions, 'kind'>,
    signal?: AbortSignal
  ): Promise<DiscoveryPage<AttributeFrequency>> =>
    discover({ ...options, kind: 'values' }, attributeFrequency, signal),
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
    const rawData = await callArrow(
      'getFieldValueCompletions',
      named({ signal, field, term, limit })
    )
    return rowViews(rawData, (frame, row) => frame.cell('value', row).text())
  },

  searchAttributeMatches: async (
    term: string,
    options: {
      signal?: string
      owner?: string
      path?: readonly (string | number)[]
      type?: string
    } = {}
  ): Promise<AttributeMatch[]> => {
    if (!term.trim() && !options.path) return []
    const rawData = await callArrow(
      'searchAttributeMatches',
      named({ term, ...options })
    )
    return rowViews(rawData, attributeMatch)
  },

  getTraceAttributeDefinitions: async (): Promise<FieldDefinition[]> => {
    return attributeDefinitions(
      await callArrow('getTraceAttributeDefinitions', named({}))
    )
  },

  getTraceAttributeDefinitionsByTraceID: async (
    traceID: string
  ): Promise<FieldDefinition[]> => {
    const rawData = await callArrow(
      'getTraceAttributeDefinitionsByTraceID',
      named({ traceID })
    )
    return attributeDefinitions(rawData)
  },

  getLogAttributeDefinitions: async (): Promise<FieldDefinition[]> => {
    return attributeDefinitions(
      await callArrow('getLogAttributeDefinitions', named({}))
    )
  },

  searchTraceSummaries: async (
    startTime: QueryTimeBound,
    endTime: QueryTimeBound,
    queryTree?: QueryNode,
    limit?: number,
    sort?: SearchSort,
    signal?: AbortSignal
  ): Promise<TraceSummary[]> => {
    const startTimeNs = serializeNanoseconds(startTime)
    const endTimeNs = serializeNanoseconds(endTime)

    const rawData = await callArrow(
      'searchTraceSummaries',
      named({
        startTime: startTimeNs,
        endTime: endTimeNs,
        query: queryTree && convertQueryTreeForBackend(queryTree),
        limit,
        sort,
      }),
      signal
    )
    return rowViews(rawData, traceSummary)
  },

  /** Fetch only timing fields for exact span references, in bounded batches. */
  getSpanTimings: async (
    references: readonly Pick<SpanTiming, 'traceID' | 'spanID'>[],
    signal?: AbortSignal
  ): Promise<SpanTiming[]> => {
    const unique = new Map<string, { traceID: string; spanID: string }>()
    for (const reference of references) {
      if (
        !/^[a-f0-9]{32}$/i.test(reference.traceID) ||
        !/^[a-f0-9]{16}$/i.test(reference.spanID)
      ) {
        throw new Error(
          'Span timing references require a 32-hex trace ID and a 16-hex span ID.'
        )
      }
      const traceID = reference.traceID.toLowerCase()
      const spanID = reference.spanID.toLowerCase()
      unique.set(`${traceID}/${spanID}`, { traceID, spanID })
    }
    const pairs = [...unique.values()]
    const timings: SpanTiming[] = []
    for (let offset = 0; offset < pairs.length; offset += 64) {
      const batch = pairs.slice(offset, offset + 64)
      // Only validated hex identifiers enter SQL; project exact timestamps as
      // strings so JSON number parsing cannot lose nanoseconds.
      const values = batch
        .map(({ traceID, spanID }) => {
          const uuid = `${traceID.slice(0, 8)}-${traceID.slice(8, 12)}-${traceID.slice(12, 16)}-${traceID.slice(16, 20)}-${traceID.slice(20)}`
          return `('${uuid}'::UUID, '0x${spanID}'::UBIGINT)`
        })
        .join(', ')
      const result = await callRPC<{
        rows: [string, string, string, string | null, string, string, number][]
        truncated: boolean
      }>(
        'query',
        named({
          sql: `WITH requested(trace_id, span_id) AS (VALUES ${values})
SELECT trace_id_wire(s.trace_id), span_id_wire(s.span_id), s.name,
s.service_name, s.start_time::VARCHAR, s.end_time::VARCHAR, s.status_code
FROM spans s JOIN requested r ON s.trace_id = r.trace_id AND s.span_id = r.span_id
LIMIT ${batch.length}`,
          limit: batch.length,
        }),
        signal
      )
      if (result.truncated)
        throw new Error('Span timing lookup returned more rows than requested.')
      timings.push(
        ...result.rows.map(
          ([
            traceID,
            spanID,
            name,
            serviceName,
            startTime,
            endTime,
            statusCodeValue,
          ]) => ({
            traceID,
            spanID,
            name,
            serviceName: serviceName ?? '',
            startTime: BigInt(startTime),
            endTime: BigInt(endTime),
            statusCodeValue,
          })
        )
      )
    }
    return timings
  },

  /** Resolve trace-only references to a bounded span selection; retain exact selections. */
  getReferencedTraceViews: async (
    references: readonly { traceID: string; spanID?: string }[],
    signal?: AbortSignal
  ): Promise<{ traces: TraceData[]; unavailableTraceIDs: string[] }> => {
    const groups = new Map<string, Set<string> | null>()
    for (const reference of references) {
      if (
        !/^[a-f0-9]{32}$/i.test(reference.traceID) ||
        (reference.spanID !== undefined &&
          !/^[a-f0-9]{16}$/i.test(reference.spanID))
      ) {
        throw new Error(
          'Trace references require a 32-hex trace ID and a 16-hex span ID.'
        )
      }
      const traceID = reference.traceID.toLowerCase()
      if (reference.spanID === undefined) groups.set(traceID, null)
      else if (groups.get(traceID) !== null) {
        const ids = groups.get(traceID) ?? new Set<string>()
        ids.add(reference.spanID.toLowerCase())
        groups.set(traceID, ids)
      }
    }
    const traces: TraceData[] = []
    const unavailableTraceIDs: string[] = []
    const entries = [...groups]
    // Never split a trace's selection: that would lose ancestry across batches.
    // Four concurrent trace queries, each with at most 256 exact span identities.
    for (let offset = 0; offset < entries.length; offset += 4) {
      if (signal?.aborted) throw new RequestAbortedError()
      const batch = entries.slice(offset, offset + 4)
      const results = await Promise.allSettled(
        batch.map(async ([traceID, ids]) => {
          if (ids === null) {
            const uuid = `${traceID.slice(0, 8)}-${traceID.slice(8, 12)}-${traceID.slice(12, 16)}-${traceID.slice(16, 20)}-${traceID.slice(20)}`
            const selection = await callRPC<{
              rows: [string][]
              truncated: boolean
            }>(
              'query',
              named({
                sql: `SELECT span_id_wire(span_id) FROM spans WHERE trace_id = '${uuid}'::UUID ORDER BY start_time, span_id LIMIT 257`,
                limit: 257,
              }),
              signal
            )
            if (selection.truncated || selection.rows.length > 256)
              throw new Error(
                'Referenced trace exceeds the 256-span view limit.'
              )
            if (!selection.rows.length)
              return { traceID, spans: [], unplacedSpanCount: 0 }
            ids = new Set(selection.rows.map(([id]) => id))
          }
          if (ids.size > 256)
            throw new Error('Referenced trace exceeds the 256-span view limit.')
          const raw = await callArrow(
            'getTraceView',
            named({ traceID, spanIDs: [...ids] }),
            signal
          )
          return traceData(raw)
        })
      )
      if (signal?.aborted) throw new RequestAbortedError()
      results.forEach((result, index) => {
        if (result.status === 'fulfilled') traces.push(result.value)
        else unavailableTraceIDs.push(batch[index]![0])
      })
    }
    return { traces, unavailableTraceIDs }
  },

  /** Look up the associated trace without the Logs time/query filters or a waterfall fetch. */
  getTraceSummary: async (
    traceID: string,
    signal?: AbortSignal
  ): Promise<TraceSummary | null> => {
    const result = await callArrow(
      'searchTraceSummaries',
      named({
        startTime: null,
        endTime: null,
        limit: 1,
        query: {
          id: 'associated-trace',
          type: 'condition',
          query: {
            field: { owner: 'span', path: ['traceID'], type: 'string' },
            fieldOperator: '=',
            value: traceID,
          },
        },
      }),
      signal
    )
    return result.length ? traceSummary(result, 0) : null
  },

  getTraceView: async (
    traceID: string,
    queryTree?: QueryNode,
    signal?: AbortSignal,
    columns?: readonly TraceColumn[]
  ): Promise<TraceWaterfallData> => {
    const rawData = await callArrow(
      'getTraceView',
      named({
        traceID,
        query: queryTree && convertQueryTreeForBackend(queryTree),
        columns,
      }),
      signal
    )
    return traceWaterfallData(rawData)
  },

  getSpan: async (
    traceID: string,
    spanID: string,
    signal?: AbortSignal
  ): Promise<SpanData | null> => {
    const frame = await callArrow('getSpan', named({ traceID, spanID }), signal)
    return frame.length ? (traceData(frame).spans[0]?.spanData ?? null) : null
  },

  getTraceColumnPreviews: async (
    traceID: string,
    columns: readonly TraceColumn[],
    signal?: AbortSignal
  ): Promise<TraceColumnPreview[]> => {
    return traceColumnPreviews(
      await callArrow(
        'getTraceColumnPreviews',
        named({ traceID, columns }),
        signal
      )
    )
  },

  getTraceLogSummaries: async (
    traceID: string,
    signal?: AbortSignal
  ): Promise<TraceLogSummary[]> => {
    const rawData = await callArrow(
      'getTraceLogSummaries',
      named({ traceID }),
      signal
    )
    return rowViews(rawData, traceLogSummary)
  },

  /** Full logs for one exact received trace/span pair. No trace-wide association. */
  getSpanLogs: async (
    traceID: string,
    spanID: string,
    signal?: AbortSignal
  ): Promise<LogData[]> => {
    const result = await callArrow(
      'getSpanLogs',
      named({ traceID, spanID }),
      signal
    )
    return rowViews(result, logData)
  },

  clearTraces: () => callRPC<string>('clearTraces', undefined),
  deleteTraces: (traceIDs: string[]) =>
    callRPC<JsonDeleteResult>('deleteSpansByTraceID', traceIDs),

  searchLogSummaries: async (
    startTime: QueryTimeBound,
    endTime: QueryTimeBound,
    queryTree?: QueryNode,
    limit?: number,
    sort?: SearchSort,
    signal?: AbortSignal
  ): Promise<LogSummary[]> => {
    const startTimeNs = serializeNanoseconds(startTime)
    const endTimeNs = serializeNanoseconds(endTime)
    const rawData = await callArrow(
      'searchLogSummaries',
      named({
        startTime: startTimeNs,
        endTime: endTimeNs,
        query: queryTree && convertQueryTreeForBackend(queryTree),
        limit,
        sort,
      }),
      signal
    )
    return rowViews(rawData, logSummary)
  },

  searchLogPage: async (
    startTime: QueryTimeBound,
    endTime: QueryTimeBound,
    pageSize: number,
    offset: number,
    queryTree?: QueryNode,
    limit?: number,
    sort?: SearchSort,
    columns?: LogColumn[],
    signal?: AbortSignal
  ): Promise<{ logs: LogTableRow[]; total: number }> => {
    const data = await callArrow(
      'searchLogPage',
      named({
        startTime: serializeNanoseconds(startTime),
        endTime: serializeNanoseconds(endTime),
        query: queryTree && convertQueryTreeForBackend(queryTree),
        limit,
        sort,
        pageSize,
        offset,
        columns: columns?.length ? columns : undefined,
      }),
      signal
    )
    return {
      logs: rowViews(
        new ArrowFrame(data.cell('logs', 0).list()),
        (frame, row) => logTableRow(frame, row, columns ?? [])
      ),
      total: data.cell('total', 0).number(),
    }
  },

  getLogErrorNavigation: async (
    startTime: QueryTimeBound,
    endTime: QueryTimeBound,
    selectedLogRef: string | null,
    queryTree?: QueryNode,
    limit?: number,
    sort?: SearchSort,
    signal?: AbortSignal
  ): Promise<LogErrorNavigation> =>
    callRPC<LogErrorNavigation>(
      'getLogErrorNavigation',
      named({
        startTime: serializeNanoseconds(startTime),
        endTime: serializeNanoseconds(endTime),
        selectedLogRef,
        query: queryTree && convertQueryTreeForBackend(queryTree),
        limit,
        sort,
      }),
      signal
    ),

  getLog: async (logRef: string, signal?: AbortSignal): Promise<LogData> => {
    const rawData = await callArrow('getLog', named({ logRef }), signal)
    return logData(rawData, 0)
  },

  deleteLogsByRefs: (...logRefs: string[]) =>
    callRPC<JsonDeleteResult>('deleteLogsByRefs', logRefs),
  clearLogs: () => callRPC<string>('clearLogs', undefined),

  searchMetricSummaries: async (
    startTime: QueryTimeBound,
    endTime: QueryTimeBound,
    queryTree?: QueryNode,
    limit?: number,
    sort?: SearchSort,
    signal?: AbortSignal
  ): Promise<MetricSummary[]> => {
    const startTimeNs = serializeNanoseconds(startTime)
    const endTimeNs = serializeNanoseconds(endTime)
    const rawData = await callArrow(
      'searchMetricSummaries',
      named({
        startTime: startTimeNs,
        endTime: endTimeNs,
        query: queryTree && convertQueryTreeForBackend(queryTree),
        limit,
        sort,
      }),
      signal
    )
    return rowViews(rawData, metricSummary)
  },

  getMetric: async (
    metricRef: string,
    signal?: AbortSignal
  ): Promise<ExactMetric | null> => {
    try {
      const rawData = await callArrow('getMetric', named({ metricRef }), signal)
      return exactMetric(rawData)
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

  getMetricSeriesOrder: async (
    metricRef: string,
    startTime: QueryTimeBound,
    endTime: QueryTimeBound,
    seriesRefs: string[],
    key: string,
    direction: 'asc' | 'desc',
    signal?: AbortSignal
  ): Promise<string[]> =>
    metricSeriesOrder(
      await callArrow(
        'getMetricSeriesOrder',
        named({
          metricRef,
          startTime: serializeNanoseconds(startTime),
          endTime: serializeNanoseconds(endTime),
          seriesRefs,
          key,
          direction,
        }),
        signal
      )
    ),

  getMetricSeries: async (
    metricRef: string,
    seriesRef: string,
    startTime: QueryTimeBound,
    endTime: QueryTimeBound,
    includeDatapoints?: boolean,
    signal?: AbortSignal
  ): Promise<ExactMetricSeries | null> => {
    try {
      const rawData = await callArrow(
        'getMetricSeries',
        named({
          metricRef,
          seriesRef,
          includeDatapoints,
          startTime: serializeNanoseconds(startTime),
          endTime: serializeNanoseconds(endTime),
        }),
        signal
      )
      return exactMetricSeries(rawData)
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
    datapointSeriesLimit?: number,
    signal?: AbortSignal
  ): Promise<MetricViewData | null> => {
    const startTimeNs = serializeNanoseconds(startTime)
    const endTimeNs = serializeNanoseconds(endTime)
    // Keep the established not-found code and nullable consumer contract.
    try {
      const rawData = await callArrow(
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
        }),
        signal
      )
      return metricView(rawData)
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
    tzName?: string,
    /** Highlight this source's interval activity inside the aggregate. */
    datapointRef?: string,
    signal?: AbortSignal
  ): Promise<MetricAggregateViewData | null> => {
    const startTimeNs = serializeNanoseconds(startTime)
    const endTimeNs = serializeNanoseconds(endTime)
    try {
      const raw = await callArrow(
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
          datapointRef,
        }),
        signal
      )
      return raw.length ? metricAggregateView(raw) : null
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
    return attributeDefinitions(
      await callArrow('getMetricAttributeDefinitions', named({}))
    )
  },

  deleteMetric: (metricRef: string) =>
    callRPC<string>('deleteMetric', named({ metricRef })),
  clearMetrics: () => callRPC<string>('clearMetrics', undefined),

  // Stats methods
  getImportConfig: async (
    signal?: AbortSignal
  ): Promise<{ otlpHttpPort: number; otlpGrpcPort?: number }> => {
    const config = await callRPC<{
      otlpHttpPort: number
      otlpGrpcPort?: number
    }>('getImportConfig', undefined, signal)
    if (
      !Number.isInteger(config?.otlpHttpPort) ||
      config.otlpHttpPort < 1 ||
      config.otlpHttpPort > 65535
    ) {
      throw new Error('The viewer returned an invalid OTLP HTTP port')
    }
    if (
      config.otlpGrpcPort !== undefined &&
      (!Number.isInteger(config.otlpGrpcPort) ||
        config.otlpGrpcPort < 1 ||
        config.otlpGrpcPort > 65535)
    ) {
      throw new Error('The viewer returned an invalid OTLP gRPC port')
    }
    return config
  },

  getStats: async (): Promise<Stats> => {
    const rawData = await callRPC<JsonStats>('getStats')
    return statsFromJSON(rawData)
  },

  getTraceSpanCount: async (traceID: string): Promise<number> => {
    return await callRPC<number>('getTraceSpanCount', named({ traceID }))
  },
}

// Helper function to convert frontend query tree to minimal backend format
export function convertQueryTreeForBackend(
  queryTree: QueryNode
): JsonQueryNode {
  if (queryTree.type === 'text') {
    return {
      id: queryTree.id,
      type: 'text',
      query: {
        fieldOperator: queryTree.query.operator.symbol,
        value: queryTree.query.value,
      },
    }
  }
  if (queryTree.type === 'condition') {
    return {
      id: queryTree.id,
      type: 'condition',
      query: {
        field: {
          owner: queryTree.query.field.owner,
          path: queryTree.query.field.path,
          type: queryTree.query.field.type,
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
