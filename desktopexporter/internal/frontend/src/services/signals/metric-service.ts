import type {
  MetricViewData,
  MetricSummary,
  MetricAggregateViewData,
  ExactMetric,
  ExactMetricSeries,
} from '@/types/api-types'
import type { FieldDefinition, QueryNode } from '@/search/model'
import { attributeDefinitions } from '../arrow/arrow-search'
import { rowViews } from '../arrow/arrow-values'
import {
  exactMetric,
  exactMetricSeries,
  metricSeriesOrder,
  metricView,
  metricAggregateView,
} from '../arrow/arrow-metrics'
import { metricSummary } from '../arrow/arrow-summaries'
import { JsonRpcError } from '../service-error'
import {
  serializeNanoseconds,
  named,
  callRPC,
  callArrow,
  convertQueryTreeForBackend,
  type SearchSort,
  type QueryTimeBound,
} from '../telemetry-request'

const ERR_CODE_METRIC_NOT_FOUND = -32003

export const metricAPI = {
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

  getMetricAttributeDefinitions: async (
    signal?: AbortSignal
  ): Promise<FieldDefinition[]> => {
    return attributeDefinitions(
      await callArrow('getMetricAttributeDefinitions', named({}), signal)
    )
  },

  deleteMetric: (metricRef: string) =>
    callRPC<string>('deleteMetric', named({ metricRef })),
  clearMetrics: () => callRPC<string>('clearMetrics', undefined),
}
