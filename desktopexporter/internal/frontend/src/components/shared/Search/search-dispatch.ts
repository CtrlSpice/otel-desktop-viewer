import type { QueryNode } from './queryTree'
import type { SearchSort, telemetryAPI } from '@/services/telemetry-service'
import type { SearchResultEvent } from '@/types/api-types'

export type SearchSignal = SearchResultEvent['signal']

export interface SearchContext<Signal extends SearchSignal = SearchSignal> {
  signal: Signal
  startTime: number | null
  endTime: number | null
}

type SearchEventBySignal = {
  [Signal in SearchSignal]: Extract<SearchResultEvent, { signal: Signal }>
}

type SearchResultsBySignal = {
  [Signal in SearchSignal]: SearchEventBySignal[Signal]['results']
}

type SearchAPI = Pick<
  typeof telemetryAPI,
  'searchTraces' | 'searchLogs' | 'searchMetricSummaries'
>

type SearchFactory<Signal extends SearchSignal> = (
  ctx: SearchContext<Signal>,
  queryTree?: QueryNode,
  limit?: number,
  sort?: SearchSort
) => () => Promise<SearchResultsBySignal[Signal]>

export type SearchDispatch = {
  [Signal in SearchSignal]: SearchFactory<Signal>
}

export type SearchEventFactory = (
  updateSeq: number
) => Promise<SearchResultEvent>

export function createSearchDispatch(api: SearchAPI) {
  return {
    traces:
      (ctx, queryTree = undefined, limit = undefined, sort = undefined) =>
      () =>
        api.searchTraces(ctx.startTime, ctx.endTime, queryTree, limit, sort),
    logs:
      (ctx, queryTree = undefined, limit = undefined, sort = undefined) =>
      () =>
        api.searchLogs(ctx.startTime, ctx.endTime, queryTree, limit, sort),
    metrics:
      (ctx, queryTree = undefined, limit = undefined, sort = undefined) =>
      () =>
        api.searchMetricSummaries(
          ctx.startTime,
          ctx.endTime,
          queryTree,
          limit,
          sort
        ),
  } satisfies SearchDispatch
}

export function buildSearchEventFactory(
  dispatch: SearchDispatch,
  ctx: SearchContext,
  queryTree?: QueryNode,
  limit?: number,
  sort?: SearchSort
): SearchEventFactory | null {
  switch (ctx.signal) {
    case 'traces': {
      const search = dispatch.traces(
        { ...ctx, signal: 'traces' },
        queryTree,
        limit,
        sort
      )
      return async updateSeq => ({
        signal: 'traces',
        results: await search(),
        queryTree,
        updateSeq,
      })
    }
    case 'logs': {
      const search = dispatch.logs(
        { ...ctx, signal: 'logs' },
        queryTree,
        limit,
        sort
      )
      return async updateSeq => ({
        signal: 'logs',
        results: await search(),
        queryTree,
        updateSeq,
      })
    }
    case 'metrics': {
      const search = dispatch.metrics(
        { ...ctx, signal: 'metrics' },
        queryTree,
        limit,
        sort
      )
      return async updateSeq => ({
        signal: 'metrics',
        results: await search(),
        queryTree,
        updateSeq,
      })
    }
    default:
      return null
  }
}
