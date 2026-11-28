import type {
  TraceData,
  TraceWaterfallData,
  TraceColumn,
  TraceColumnPreview,
  SpanData,
  TraceSummary,
  SpanTiming,
} from '@/types/api-types'
import type { JsonDeleteResult } from '@/types/wire-types'
import type { FieldDefinition, QueryNode } from '@/search/model'
import { attributeDefinitions } from '../arrow/arrow-search'
import { rowViews } from '../arrow/arrow-values'
import {
  traceData,
  traceWaterfallData,
  traceColumnPreviews,
} from '../arrow/arrow-traces'
import { traceSummary } from '../arrow/arrow-summaries'
import {
  serializeNanoseconds,
  named,
  callRPC,
  callArrow,
  convertQueryTreeForBackend,
  RequestAbortedError,
  type SearchSort,
  type QueryTimeBound,
} from '../telemetry-request'

export const traceAPI = {
  getTraceAttributeDefinitions: async (
    signal?: AbortSignal
  ): Promise<FieldDefinition[]> => {
    return attributeDefinitions(
      await callArrow('getTraceAttributeDefinitions', named({}), signal)
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
          return `('${uuid}'::uuid, '0x${spanID}'::ubigint)`
        })
        .join(', ')
      const result = await callRPC<{
        rows: [string, string, string, string | null, string, string, number][]
        truncated: boolean
      }>(
        'query',
        named({
          sql: `with requested(trace_id, span_id) as (values ${values})
select trace_id_wire(s.trace_id), span_id_wire(s.span_id), s.name,
s.service_name, s.start_time::varchar, s.end_time::varchar, s.status_code
from spans s join requested r on s.trace_id = r.trace_id and s.span_id = r.span_id
limit ${batch.length}`,
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
    signal?: AbortSignal,
    onTrace?: (
      result:
        | { status: 'ready'; trace: TraceData }
        | { status: 'failed'; traceID: string }
    ) => void
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
    if (signal?.aborted) throw new RequestAbortedError()
    const results = await Promise.allSettled(
      entries.map(([traceID, selection]) => {
        const load = async () => {
          let ids = selection
          if (ids === null) {
            const uuid = `${traceID.slice(0, 8)}-${traceID.slice(8, 12)}-${traceID.slice(12, 16)}-${traceID.slice(16, 20)}-${traceID.slice(20)}`
            const selection = await callRPC<{
              rows: [string][]
              truncated: boolean
            }>(
              'query',
              named({
                sql: `select span_id_wire(span_id) from spans where trace_id = '${uuid}'::uuid order by start_time, span_id limit 257`,
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
        }
        return load().then(
          trace => {
            if (!signal?.aborted) onTrace?.({ status: 'ready', trace })
            return trace
          },
          error => {
            if (!signal?.aborted) onTrace?.({ status: 'failed', traceID })
            throw error
          }
        )
      })
    )
    if (signal?.aborted) throw new RequestAbortedError()
    results.forEach((result, index) => {
      if (result.status === 'fulfilled') traces.push(result.value)
      else unavailableTraceIDs.push(entries[index]![0])
    })
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

  clearTraces: () => callRPC<string>('clearTraces', undefined),
  deleteTraces: (traceIDs: string[]) =>
    callRPC<JsonDeleteResult>('deleteSpansByTraceID', traceIDs),
}
