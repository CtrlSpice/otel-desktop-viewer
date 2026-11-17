import type {
  LogData,
  LogSummary,
  LogColumn,
  LogTableRow,
  LogErrorNavigation,
  TraceLogSummary,
} from '@/types/api-types'
import type { JsonDeleteResult } from '@/types/wire-types'
import type { FieldDefinition, QueryNode } from '@/search/model'
import { attributeDefinitions } from './arrow/arrow-search'
import { rowViews } from './arrow/arrow-values'
import { ArrowFrame } from './arrow/arrow-data'
import {
  logData,
  logSummary,
  logTableRow,
  traceLogSummary,
} from './arrow/arrow-summaries'
import {
  serializeNanoseconds,
  named,
  callRPC,
  callArrow,
  convertQueryTreeForBackend,
  type SearchSort,
  type QueryTimeBound,
} from './telemetry-request'

export const logAPI = {
  getLogAttributeDefinitions: async (
    signal?: AbortSignal
  ): Promise<FieldDefinition[]> => {
    return attributeDefinitions(
      await callArrow('getLogAttributeDefinitions', named({}), signal)
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
}
