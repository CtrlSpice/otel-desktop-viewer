import type { QueryNode, SearchSignal } from '@/search/model'
import { telemetryAPI, type SearchSort } from '@/services/telemetry-service'
import type { SearchResultEvent } from '@/types/api-types'

export interface SearchContext {
  signal: SearchSignal
  startTime: bigint | null
  endTime: bigint | null
}

export async function runSearch(
  ctx: SearchContext,
  updateSeq: number,
  queryTree?: QueryNode,
  limit?: number,
  sort?: SearchSort
): Promise<SearchResultEvent> {
  switch (ctx.signal) {
    case 'traces':
      return {
        signal: 'traces',
        results: await telemetryAPI.searchTraceSummaries(
          ctx.startTime,
          ctx.endTime,
          queryTree,
          limit,
          sort
        ),
        queryTree,
        updateSeq,
      }
    case 'logs':
      return {
        signal: 'logs',
        results: await telemetryAPI.searchLogSummaries(
          ctx.startTime,
          ctx.endTime,
          queryTree,
          limit,
          sort
        ),
        queryTree,
        updateSeq,
      }
    case 'metrics':
      return {
        signal: 'metrics',
        results: await telemetryAPI.searchMetricSummaries(
          ctx.startTime,
          ctx.endTime,
          queryTree,
          limit,
          sort
        ),
        queryTree,
        updateSeq,
      }
  }
}
