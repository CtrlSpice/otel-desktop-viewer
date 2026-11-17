import { describe, expect, it } from 'vitest'
import {
  mergeRouteQueryWithMetricView,
  metricViewQueriesEqual,
  metricViewQueryToParams,
  parseMetricViewQuery,
  parseMetricVisibleSeries,
  metricVisibleSeriesToParam,
  type MetricViewParseContext,
  type MetricViewQuery,
} from './metric-view-query'

const timeseriesCtx = {
  isHistogramKind: false,
  allowedAggs: ['raw', 'sum', 'avg', 'rate'],
  datapointIDs: new Set(['dp-1', 'dp-2']),
  seriesKeys: new Set(['series-1', 'series-2']),
} satisfies MetricViewParseContext

const histogramCtx = {
  isHistogramKind: true,
  allowedAggs: ['raw'],
  datapointIDs: new Set(['dp-h1']),
  seriesKeys: new Set(['series-h1']),
} satisfies MetricViewParseContext

describe('visible series', () => {
  it('distinguishes defaults from an explicitly empty chart', () => {
    expect(parseMetricVisibleSeries({})).toBeNull()
    for (const visible of ['none', '', ' , ']) {
      expect(parseMetricVisibleSeries({ visible })).toEqual([])
    }
    expect(metricVisibleSeriesToParam([])).toBe('none')
  })

  it('canonicalizes the set and retains missing IDs independently of inspection', () => {
    const query = parseMetricViewQuery(
      {
        visible: ' series-2,missing,series-2, ',
        series: 'series-1',
        dp: 'dp-1',
      },
      timeseriesCtx
    )
    expect(query.visible).toEqual(['missing', 'series-2'])
    expect(query.series).toBe('series-1')
    expect(query.dp).toBe('dp-1')
    expect(metricViewQueryToParams(query).visible).toBe('missing,series-2')
    expect(
      metricViewQueriesEqual(query, {
        ...query,
        visible: ['series-2', 'missing'],
      })
    ).toBe(true)
    expect(
      metricViewQueriesEqual(query, { ...query, visible: ['series-2'] })
    ).toBe(false)
    expect(
      metricViewQueriesEqual(
        { ...query, visible: null },
        { ...query, visible: [] }
      )
    ).toBe(false)
  })

  it('round-trips an uncapped histogram set through URL encoding', () => {
    const keys = Array.from(
      { length: 1000 },
      (_, index) =>
        `00000000-0000-4000-8000-${index.toString(16).padStart(12, '0')}`
    )
    const query = parseMetricViewQuery(
      { visible: keys.join(',') },
      histogramCtx
    )
    const encoded = new URLSearchParams(metricViewQueryToParams(query))
    expect(parseMetricVisibleSeries(Object.fromEntries(encoded))).toEqual(keys)
  })

  it('writes current histogram fields and removes obsolete scope from old links', () => {
    const route = {
      htab: 'quantiles',
      hscope: 'bucket',
      visible: 'none',
      time: 'all',
      q: 'service.name = "checkout"',
    }
    const query = parseMetricViewQuery(route, histogramCtx)
    expect(mergeRouteQueryWithMetricView(route, query)).toEqual({
      htab: 'heatmap',
      visible: 'none',
      time: 'all',
      q: route.q,
    })
  })
})

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
      visible: null,
    })
  })

  it('parses histogram params and ignores agg', () => {
    const q = parseMetricViewQuery(
      { agg: 'rate', htab: 'quantiles', hscope: 'window', dp: 'dp-h1' },
      histogramCtx
    )
    expect(q).toEqual({
      kind: 'histogram',
      htab: 'heatmap',
      dp: 'dp-h1',
      series: null,
      visible: null,
    })
  })

  it('opens old snapshot links in whole-window mode while keeping the selection', () => {
    expect(
      parseMetricViewQuery(
        {
          htab: 'histogram',
          hscope: 'bucket',
          dp: 'dp-h1',
          series: 'series-h1',
        },
        histogramCtx
      )
    ).toEqual({
      kind: 'histogram',
      htab: 'histogram',
      dp: 'dp-h1',
      series: 'series-h1',
      visible: null,
    })
  })

  it('rejects disallowed agg on timeseries', () => {
    const q = parseMetricViewQuery({ agg: 'not-a-view' }, timeseriesCtx)
    expect(q).toEqual({
      kind: 'timeseries',
      agg: null,
      dp: null,
      series: null,
      visible: null,
    })
  })

  it('rejects unknown datapoint id', () => {
    const q = parseMetricViewQuery({ dp: 'missing' }, timeseriesCtx)
    expect(q).toEqual({
      kind: 'timeseries',
      agg: null,
      dp: null,
      series: null,
      visible: null,
    })
  })

  it('defaults invalid histogram enums', () => {
    const q = parseMetricViewQuery(
      { htab: 'nope', hscope: 'nope' },
      histogramCtx
    )
    expect(q).toEqual({
      kind: 'histogram',
      htab: 'heatmap',
      dp: null,
      series: null,
      visible: null,
    })
  })
})

describe('metricViewQueriesEqual', () => {
  const timeseries: MetricViewQuery = {
    kind: 'timeseries',
    agg: 'rate',
    dp: 'dp-1',
    series: null,
    visible: null,
  }
  const histogram: MetricViewQuery = {
    kind: 'histogram',
    htab: 'heatmap',
    dp: 'dp-h1',
    series: null,
    visible: null,
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

  it('rejects differing histogram tabs', () => {
    expect(
      metricViewQueriesEqual(histogram, { ...histogram, htab: 'histogram' })
    ).toBe(false)
  })

  it('is insensitive to property order', () => {
    // SAFETY: This fixture lists every required timeseries variant field with valid literal values.
    const reordered = {
      dp: 'dp-1',
      series: null,
      visible: null,
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
      visible: null,
    }
    expect(metricViewQueryToParams(q)).toEqual({ agg: 'avg', dp: 'dp-1' })
  })

  it('serializes histogram without agg', () => {
    const q: MetricViewQuery = {
      kind: 'histogram',
      htab: 'histogram',
      dp: null,
      series: null,
      visible: null,
    }
    expect(metricViewQueryToParams(q)).toEqual({
      htab: 'histogram',
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
      visible: null,
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
// degrades to no selection. A series id is stable within the database,
// originating resource attributes, and datapoint labels: the same series has
// the same id across restarts and re-ingests, so it still resolves long after
// the point it was captured with has been pruned.
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
        visible: null,
      })
    ).toEqual({ series: 'series-1' })

    expect(
      metricViewQueryToParams({
        kind: 'timeseries',
        agg: null,
        dp: null,
        series: null,
        visible: null,
      })
    ).toEqual({})
  })

  it('participates in equality, so a series change is an external navigation', () => {
    const base = {
      kind: 'timeseries',
      agg: null,
      dp: null,
      visible: null,
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
      visible: null,
    }
    const merged = mergeRouteQueryWithMetricView({ start: '1' }, view)
    expect(merged.series).toBe('series-2')
    expect(parseMetricViewQuery(merged, timeseriesCtx)).toEqual(view)
  })
})
