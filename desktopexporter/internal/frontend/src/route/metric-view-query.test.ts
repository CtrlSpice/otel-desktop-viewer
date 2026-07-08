import { describe, expect, it } from 'vitest'
import {
  mergeRouteQueryWithMetricView,
  metricViewQueryToParams,
  parseMetricViewQuery,
  type MetricViewQuery,
} from './metric-view-query'

const timeseriesCtx = {
  isHistogramKind: false,
  allowedAggs: ['raw', 'sum', 'avg', 'rate'],
  datapointIds: new Set(['dp-1', 'dp-2']),
}

const histogramCtx = {
  isHistogramKind: true,
  allowedAggs: ['raw'],
  datapointIds: new Set(['dp-h1']),
}

describe('parseMetricViewQuery', () => {
  it('parses timeseries params and strips histogram keys', () => {
    const q = parseMetricViewQuery(
      { agg: 'rate', dp: 'dp-1', htab: 'heatmap', hscope: 'bucket' },
      timeseriesCtx
    )
    expect(q).toEqual({
      kind: 'timeseries',
      agg: 'rate',
      dp: 'dp-1',
    })
  })

  it('parses histogram params and ignores agg', () => {
    const q = parseMetricViewQuery(
      { agg: 'rate', htab: 'quantiles', hscope: 'window', dp: 'dp-h1' },
      histogramCtx
    )
    expect(q).toEqual({
      kind: 'histogram',
      htab: 'quantiles',
      hscope: 'window',
      dp: 'dp-h1',
    })
  })

  it('rejects disallowed agg on timeseries', () => {
    const q = parseMetricViewQuery({ agg: 'not-a-view' }, timeseriesCtx)
    expect(q).toEqual({ kind: 'timeseries', agg: null, dp: null })
  })

  it('rejects unknown datapoint id', () => {
    const q = parseMetricViewQuery({ dp: 'missing' }, timeseriesCtx)
    expect(q).toEqual({ kind: 'timeseries', agg: null, dp: null })
  })

  it('defaults invalid histogram enums', () => {
    const q = parseMetricViewQuery({ htab: 'nope', hscope: 'nope' }, histogramCtx)
    expect(q).toEqual({
      kind: 'histogram',
      htab: 'heatmap',
      hscope: 'window',
      dp: null,
    })
  })
})

describe('metricViewQueryToParams', () => {
  it('serializes timeseries without cross-kind keys', () => {
    const q: MetricViewQuery = { kind: 'timeseries', agg: 'avg', dp: 'dp-1' }
    expect(metricViewQueryToParams(q)).toEqual({ agg: 'avg', dp: 'dp-1' })
  })

  it('serializes histogram without agg', () => {
    const q: MetricViewQuery = {
      kind: 'histogram',
      htab: 'histogram',
      hscope: 'bucket',
      dp: null,
    }
    expect(metricViewQueryToParams(q)).toEqual({
      htab: 'histogram',
      hscope: 'bucket',
    })
  })

  it('round-trips through mergeRouteQueryWithMetricView', () => {
    const routeQuery = {
      start: '1',
      end: '2',
      agg: 'junk',
      htab: 'junk',
      tz: 'UTC',
    }
    const view: MetricViewQuery = {
      kind: 'timeseries',
      agg: 'sum',
      dp: 'dp-2',
    }
    const merged = mergeRouteQueryWithMetricView(routeQuery, view)
    expect(merged).toEqual({
      start: '1',
      end: '2',
      tz: 'UTC',
      agg: 'sum',
      dp: 'dp-2',
    })
    expect(parseMetricViewQuery(merged, timeseriesCtx)).toEqual(view)
  })
})
