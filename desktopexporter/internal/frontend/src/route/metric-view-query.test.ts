import { describe, expect, it } from 'vitest'
import {
  mergeRouteQueryWithMetricView,
  metricViewQueriesEqual,
  metricViewQueryToParams,
  parseMetricViewQuery,
  type MetricViewQuery,
} from './metric-view-query'

const timeseriesCtx = {
  isHistogramKind: false,
  allowedAggs: ['raw', 'sum', 'avg', 'rate'],
  datapointIDs: new Set(['dp-1', 'dp-2']),
  seriesKeys: new Set(['series-1', 'series-2']),
  quantileKeys: new Set(['0.5', '0.95', '0.99']),
}

const histogramCtx = {
  isHistogramKind: true,
  allowedAggs: ['raw'],
  datapointIDs: new Set(['dp-h1']),
  seriesKeys: new Set(['series-h1']),
  quantileKeys: new Set(['0.5', '0.95', '0.99']),
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
      series: null,
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
      series: null,
    })
  })

  it('rejects disallowed agg on timeseries', () => {
    const q = parseMetricViewQuery({ agg: 'not-a-view' }, timeseriesCtx)
    expect(q).toEqual({ kind: 'timeseries', agg: null, dp: null, series: null })
  })

  it('rejects unknown datapoint id', () => {
    const q = parseMetricViewQuery({ dp: 'missing' }, timeseriesCtx)
    expect(q).toEqual({ kind: 'timeseries', agg: null, dp: null, series: null })
  })

  it('defaults invalid histogram enums', () => {
    const q = parseMetricViewQuery(
      { htab: 'nope', hscope: 'nope' },
      histogramCtx
    )
    expect(q).toEqual({
      kind: 'histogram',
      htab: 'heatmap',
      hscope: 'window',
      dp: null,
      series: null,
    })
  })
})

describe('metricViewQueriesEqual', () => {
  const timeseries: MetricViewQuery = {
    kind: 'timeseries',
    agg: 'rate',
    dp: 'dp-1',
    series: null,
  }
  const histogram: MetricViewQuery = {
    kind: 'histogram',
    htab: 'quantiles',
    hscope: 'bucket',
    dp: 'dp-h1',
    series: null,
  }

  it('matches identical timeseries queries', () => {
    expect(metricViewQueriesEqual(timeseries, { ...timeseries })).toBe(true)
  })

  it('matches identical histogram queries', () => {
    expect(metricViewQueriesEqual(histogram, { ...histogram })).toBe(true)
  })

  it('rejects cross-kind comparison', () => {
    expect(metricViewQueriesEqual(timeseries, histogram)).toBe(false)
  })

  it('rejects differing dp', () => {
    expect(
      metricViewQueriesEqual(timeseries, { ...timeseries, dp: 'dp-2' })
    ).toBe(false)
  })

  it('rejects differing agg', () => {
    expect(
      metricViewQueriesEqual(timeseries, { ...timeseries, agg: null })
    ).toBe(false)
  })

  it('rejects differing histogram tab or scope', () => {
    expect(
      metricViewQueriesEqual(histogram, { ...histogram, htab: 'heatmap' })
    ).toBe(false)
    expect(
      metricViewQueriesEqual(histogram, { ...histogram, hscope: 'window' })
    ).toBe(false)
  })

  it('is insensitive to property order', () => {
    const reordered = {
      dp: 'dp-1',
      series: null,
      agg: 'rate',
      kind: 'timeseries',
    } as MetricViewQuery
    expect(metricViewQueriesEqual(timeseries, reordered)).toBe(true)
  })
})

describe('metricViewQueryToParams', () => {
  it('serializes timeseries without cross-kind keys', () => {
    const q: MetricViewQuery = {
      kind: 'timeseries',
      agg: 'avg',
      dp: 'dp-1',
      series: null,
    }
    expect(metricViewQueryToParams(q)).toEqual({ agg: 'avg', dp: 'dp-1' })
  })

  it('serializes histogram without agg', () => {
    const q: MetricViewQuery = {
      kind: 'histogram',
      htab: 'histogram',
      hscope: 'bucket',
      dp: null,
      series: null,
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
      series: null,
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

// The `series` param exists because `dp` cannot be durable.
//
// A datapoint id is minted per row and deleted by retention, so a metric link
// more than a retention window old names a point that no longer exists and
// degrades to no selection. A series id is content-derived from
// (stream, resource, labels): the same series has the same id across restarts
// and re-ingests, so it still resolves long after the point it was captured
// with has been pruned.
describe('series param', () => {
  it('parses a known series id', () => {
    const q = parseMetricViewQuery({ series: 'series-1' }, timeseriesCtx)
    expect(q.series).toBe('series-1')
  })

  it('survives when the datapoint it was captured with is gone', () => {
    // The shape of an aged link: the series still exists, the point does not.
    const q = parseMetricViewQuery(
      { series: 'series-1', dp: 'dp-pruned-by-retention' },
      timeseriesCtx
    )
    expect(q.dp).toBeNull()
    expect(q.series).toBe('series-1')
  })

  it('drops a series that no longer exists', () => {
    // A series genuinely disappears when its labels stop being reported, and a
    // link naming one should fall back to no selection rather than dangle.
    const q = parseMetricViewQuery({ series: 'series-gone' }, timeseriesCtx)
    expect(q.series).toBeNull()
  })

  it('applies to histogram metrics too', () => {
    const q = parseMetricViewQuery({ series: 'series-h1' }, histogramCtx)
    expect(q.series).toBe('series-h1')
  })

  it('is serialized when set and omitted when not', () => {
    expect(
      metricViewQueryToParams({
        kind: 'timeseries',
        agg: null,
        dp: null,
        series: 'series-1',
      })
    ).toEqual({ series: 'series-1' })

    expect(
      metricViewQueryToParams({
        kind: 'timeseries',
        agg: null,
        dp: null,
        series: null,
      })
    ).toEqual({})
  })

  it('participates in equality, so a series change is an external navigation', () => {
    const base = {
      kind: 'timeseries',
      agg: null,
      dp: null,
    } as const
    expect(
      metricViewQueriesEqual(
        { ...base, series: 'series-1' },
        { ...base, series: 'series-2' }
      )
    ).toBe(false)
    expect(
      metricViewQueriesEqual(
        { ...base, series: 'series-1' },
        { ...base, series: 'series-1' }
      )
    ).toBe(true)
  })

  it('round-trips through the route query', () => {
    const view: MetricViewQuery = {
      kind: 'timeseries',
      agg: 'sum',
      dp: 'dp-2',
      series: 'series-2',
    }
    const merged = mergeRouteQueryWithMetricView({ start: '1' }, view)
    expect(merged.series).toBe('series-2')
    expect(parseMetricViewQuery(merged, timeseriesCtx)).toEqual(view)
  })
})

describe('shareable legend state', () => {
  it('round-trips known visible series and quantile overlays', () => {
    const q = parseMetricViewQuery(
      { visible: 'series-h1,missing', quantiles: '0.99,unknown,0.5' },
      histogramCtx
    )

    expect(q).toMatchObject({
      kind: 'histogram',
      visible: ['series-h1'],
      quantiles: ['0.5', '0.99'],
    })
    expect(metricViewQueryToParams(q)).toMatchObject({
      visible: 'series-h1',
      quantiles: '0.5,0.99',
    })
  })

  it('does not turn an entirely unknown set into an explicit empty view', () => {
    const q = parseMetricViewQuery(
      { visible: 'missing', quantiles: 'unknown' },
      histogramCtx
    )

    expect(q).not.toHaveProperty('visible')
    expect(q).not.toHaveProperty('quantiles')
  })
})
