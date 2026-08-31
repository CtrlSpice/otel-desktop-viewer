// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { tick } from 'svelte'
import { screen } from '@testing-library/svelte'
import { navigateCurrentRoute, readRoute, withQueryPatch } from '@/route'
import {
  aggregateToSlices,
  type MetricViewContext,
} from '@/contexts/metric-view-context.svelte'
import type {
  MetricData,
  ResourceData,
  ScopeData,
  SumDataPoint,
} from '@/types/api-types'
import MetricViewProbe from '@/test/MetricViewProbe.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'

const BASE_TIMESTAMP_MS = 1_700_000_000_000

const EMPTY_RESOURCE: ResourceData = {
  attributes: [],
  droppedAttributesCount: 0,
}

const EMPTY_SCOPE: ScopeData = {
  name: 'test-scope',
  version: '1.0.0',
  attributes: [],
  droppedAttributesCount: 0,
}

function makeSumDatapoint(
  id: string,
  offsetMs: number,
  value: number
): SumDataPoint {
  const timestamp = BigInt(BASE_TIMESTAMP_MS + offsetMs) * 1_000_000n
  return makeSumDatapointAt(id, timestamp, value)
}

function makeSumDatapointAt(
  id: string,
  timestamp: bigint,
  value: number
): SumDataPoint {
  return {
    id,
    timestamp,
    timestampMs: Number(timestamp / 1_000_000n),
    startTime: BigInt(BASE_TIMESTAMP_MS) * 1_000_000n,
    flags: 0,
    exemplars: [],
    metricType: 'Sum',
    doubleValue: value,
    intValue: null,
    valueType: 'double',
    isMonotonic: true,
    aggregationTemporality: 'Cumulative',
  }
}

/**
 * Cumulative monotonic Sum with two timeseries: 'rate' is allowed (and is the
 * smart default), and the second series unlocks 'sum' / 'avg' so tests have an
 * allowed value that is NOT the default to deep-link to.
 */
function makeCumulativeSumMetric(): MetricData {
  return {
    id: 'm1',
    name: 'http.server.requests',
    description: 'Total inbound requests',
    metadata: [],
    unit: '1',
    metricType: 'Sum',
    aggregationTemporality: 'Cumulative',
    isMonotonic: true,
    resourceDroppedAttributesCount: 0,
    resource: EMPTY_RESOURCE,
    scopeName: EMPTY_SCOPE.name,
    scopeVersion: EMPTY_SCOPE.version,
    scopeDroppedAttributesCount: 0,
    scope: EMPTY_SCOPE,
    datapointCount: 0,
    // The window as the caller asked for it: this fixture's assertions are
    // about aggregation views, not about the axis.
    window: { fittedToData: false, startNs: null, endNs: null },
    lastSeenNs: BigInt(BASE_TIMESTAMP_MS + 60_000) * 1_000_000n,
    // No merge was refused; this fixture's histograms-that-aren't have no
    // bounds to disagree about.
    boundsMismatch: null,
    timeseries: [
      {
        attributesKey: 'route=/a',
        resource: { attributes: [], droppedAttributesCount: 0 },
        attributes: [{ key: 'route', value: '/a', type: 'string' }],
        datapoints: [
          makeSumDatapoint('dp-a1', 0, 10),
          makeSumDatapoint('dp-a2', 60_000, 25),
        ],
        stats: null,
        // The store's view buckets; this fixture is about aggregation defaults,
        // not the views themselves.
        views: null,
        // Likewise the row sparkline, which the store reduces separately.
        sparkline: null,
        // And the drawn rate line's extremes, which need view buckets.
        rateStats: null,
        // What the window holds for this series, as opposed to what the
        // response carried: equal here, since this fixture narrows nothing.
        datapointCount: 2,
        lastSeenNs: BigInt(BASE_TIMESTAMP_MS + 60_000) * 1_000_000n,
      },
      {
        attributesKey: 'route=/b',
        resource: { attributes: [], droppedAttributesCount: 0 },
        attributes: [{ key: 'route', value: '/b', type: 'string' }],
        datapoints: [
          makeSumDatapoint('dp-b1', 0, 4),
          makeSumDatapoint('dp-b2', 60_000, 9),
        ],
        stats: null,
        // The store's view buckets; this fixture is about aggregation defaults,
        // not the views themselves.
        views: null,
        // Likewise the row sparkline, which the store reduces separately.
        sparkline: null,
        // And the drawn rate line's extremes, which need view buckets.
        rateStats: null,
        // What the window holds for this series, as opposed to what the
        // response carried: equal here, since this fixture narrows nothing.
        datapointCount: 2,
        lastSeenNs: BigInt(BASE_TIMESTAMP_MS + 60_000) * 1_000_000n,
      },
    ],
  }
}

function makeRateSelectionMetric() {
  const metric = makeCumulativeSumMetric()
  const base = BigInt(BASE_TIMESTAMP_MS) * 1_000_000n
  const first = makeSumDatapointAt('rate-first', base + 100n, 1)
  const middle = makeSumDatapointAt('rate-middle', base + 400n, 5)
  const later = makeSumDatapointAt('rate-later', base + 900n, 9)
  const series = metric.timeseries[0]!
  series.datapoints = [later, middle, first]
  series.datapointCount = 3
  series.lastSeenNs = later.timestamp
  series.views = [
    {
      bucketStart: base,
      sampleCount: 1,
      sum: 1,
      avg: 1,
      rate: null,
      slope: null,
      hasReset: false,
    },
    {
      bucketStart: base + 300n,
      sampleCount: 1,
      sum: 5,
      avg: 5,
      rate: 2,
      slope: null,
      hasReset: false,
    },
    {
      bucketStart: base + 600n,
      sampleCount: 1,
      sum: 9,
      avg: 9,
      rate: 4,
      slope: 6,
      hasReset: false,
    },
  ]
  series.rateStats = { min: 2, max: 4, avg: 3 }
  metric.lastSeenNs = later.timestamp
  return { metric, later, selectedBucketStart: base + 600n }
}

type ProbeOptions = {
  metric?: MetricData
  seriesDatapoints?: Readonly<Record<string, SumDataPoint[]>>
}

function renderProbeView(url: string, options: ProbeOptions = {}) {
  setTestUrl(url)
  const metric = options.metric ?? makeCumulativeSumMetric()
  let captured: MetricViewContext | undefined
  const oncontext = (ctx: MetricViewContext) => {
    captured = ctx
  }
  const view = renderWithContexts(MetricViewProbe, {
    metric,
    seriesDatapoints: options.seriesDatapoints,
    oncontext,
  })
  if (!captured) throw new Error('probe did not report a metric view context')
  return {
    context: captured,
    setSeriesDatapoints: (
      seriesDatapoints: Readonly<Record<string, SumDataPoint[]>>
    ) =>
      view.rerender({
        componentProps: { metric, seriesDatapoints, oncontext },
      }),
  }
}

function renderProbe(
  url: string,
  options: ProbeOptions = {}
): MetricViewContext {
  return renderProbeView(url, options).context
}

function reportedAggregationView(): string {
  return screen.getByTestId('aggregation-view').textContent?.trim() ?? ''
}

function reportedSelectedDatapointID(): string {
  return screen.getByTestId('selected-datapoint-id').textContent?.trim() ?? ''
}

function reportedResolvedDatapointID(): string {
  return (
    screen.getByTestId('selected-datapoint-resolved-id').textContent?.trim() ??
    ''
  )
}

function reportedSelectedSeriesKey(): string {
  return screen.getByTestId('selected-series-key').textContent?.trim() ?? ''
}

function reportedAvailableAggregationViews(): string[] {
  const text =
    screen.getByTestId('available-aggregation-views').textContent?.trim() ?? ''
  return text ? text.split(',') : []
}

/**
 * Browser back/forward as the context sees it: the URL changes behind its
 * back, then a popstate announces it. jsdom's own `history.back()` cannot
 * reach a URL that was never pushed, which is exactly the #235 case.
 */
function externalNavigationTo(url: string): void {
  window.history.replaceState(null, '', url)
  window.dispatchEvent(new PopStateEvent('popstate'))
}

describe('metric view context aggregation URL sync', () => {
  it('offers rate for a cumulative monotonic sum with two series', () => {
    renderProbe('/metrics/m1')
    expect(reportedAvailableAggregationViews()).toEqual([
      'raw',
      'sum',
      'avg',
      'rate',
    ])
  })

  it('uses the smart default when the URL carries no aggregation', () => {
    renderProbe('/metrics/m1')
    expect(reportedAggregationView()).toBe('rate')
  })

  it('applies an aggregation deep link on load', () => {
    renderProbe('/metrics/m1?agg=sum')
    expect(reportedAggregationView()).toBe('sum')
  })

  it('falls back to the smart default when the URL aggregation is invalid', () => {
    renderProbe('/metrics/m1?agg=bogus')
    expect(reportedAggregationView()).toBe('rate')
  })

  // Issue #235: an absent `agg` means nobody wrote one, not 'raw'. Going back
  // to a URL that predates any aggregation write keeps the smart default --
  // the state that URL was actually showing -- instead of resetting.
  it('keeps the smart default when the browser goes back to a URL without an aggregation', async () => {
    renderProbe('/metrics/m1?agg=sum')
    expect(reportedAggregationView()).toBe('sum')

    externalNavigationTo('/metrics/m1')
    await tick()

    expect(reportedAggregationView()).toBe('rate')
  })

  it('ignores an aggregation dropped from the URL behind its back', async () => {
    renderProbe('/metrics/m1?agg=rate')

    window.history.replaceState(null, '', '/metrics/m1')
    await tick()

    expect(reportedAggregationView()).toBe('rate')
  })

  it('keeps the smart default when an unrelated query param is written', async () => {
    renderProbe('/metrics/m1')
    expect(reportedAggregationView()).toBe('rate')

    navigateCurrentRoute(
      withQueryPatch(readRoute().query, { start: '1000', end: '2000' }),
      'replace'
    )
    await tick()

    expect(window.location.search).toContain('start=1000')
    expect(reportedAggregationView()).toBe('rate')
  })

  it('writes its own aggregation choice to the URL', async () => {
    const ctx = renderProbe('/metrics/m1')

    ctx.setAggregationView('sum')
    await tick()

    expect(window.location.search).toBe('?agg=sum')
    expect(reportedAggregationView()).toBe('sum')
  })

  it('returns to the smart default when the browser goes back past its own write', async () => {
    const ctx = renderProbe('/metrics/m1')

    ctx.setAggregationView('sum')
    await tick()
    expect(reportedAggregationView()).toBe('sum')

    externalNavigationTo('/metrics/m1')
    await tick()

    expect(reportedAggregationView()).toBe('rate')
  })

  // The reachable form of #235: the datapoint write pushes a history entry
  // carrying the aggregation, so Back lands on the param-free URL that came
  // before it. That URL was showing the smart default, not raw.
  it('spells out raw rather than omitting it', async () => {
    const ctx = renderProbe('/metrics/m1')

    ctx.setAggregationView('raw')
    await tick()

    expect(window.location.search).toBe('?agg=raw')
  })
})

describe('metric view context visibility seeding', () => {
  // One visible-series set for every metric shape. It was two -- a metric
  // seeding the wrong shape's box once left a scalar carrying a frozen
  // ten-key histogram set, which the aggregate fetch sent as the store's
  // narrowing parameter and drew an "All" line over ten of its series. With
  // one box that class is structurally gone, and the hazard inverts: seeding
  // used to write both boxes back to back, so if that pattern survived the
  // merge, the histogram branch's empty seed would land second and clobber
  // the scalar's. A scalar seeing its own keys is therefore also the proof
  // that exactly one seed was written.
  it("seeds the set with the scalar metric's own keys, unclobbered", () => {
    const ctx = renderProbe('/metrics/m1')
    expect(ctx.isHistogramKind).toBe(false)
    expect([...ctx.visibleSeries].sort()).toEqual(['route=/a', 'route=/b'])
  })
})

describe('metric view context datapoint URL sync', () => {
  it('selects the datapoint named in the URL on load', () => {
    renderProbe('/metrics/m1?dp=dp-b2')
    expect(reportedSelectedDatapointID()).toBe('dp-b2')
  })

  it('ignores a datapoint id the metric does not have', () => {
    renderProbe('/metrics/m1?dp=dp-nope')
    expect(reportedSelectedDatapointID()).toBe('')
  })

  it('does not guess a series for a cold raw-only URL without series', async () => {
    const raw = makeSumDatapoint('dp-raw-b', 30_000, 7)
    const view = renderProbeView('/metrics/m1?dp=dp-raw-b')

    expect(reportedSelectedDatapointID()).toBe('')
    expect(view.context.expandedTimeseries.size).toBe(0)

    await view.setSeriesDatapoints({ 'route=/b': [raw] })
    await tick()

    expect(reportedSelectedDatapointID()).toBe('')
    expect(reportedResolvedDatapointID()).toBe('')
  })

  it('clears the selection when the browser goes back to a URL without dp', async () => {
    renderProbe('/metrics/m1?dp=dp-b2')
    expect(reportedSelectedDatapointID()).toBe('dp-b2')

    externalNavigationTo('/metrics/m1')
    await tick()

    expect(reportedSelectedDatapointID()).toBe('')
  })

  it('resolves a selected datapoint from the unreduced series cache', async () => {
    const raw = makeSumDatapoint('dp-raw-b', 30_000, 7)
    const ctx = renderProbe('/metrics/m1', {
      seriesDatapoints: { 'route=/b': [raw] },
    })

    ctx.onDatapointClick(raw)
    await tick()

    expect(reportedSelectedDatapointID()).toBe('dp-raw-b')
    expect(reportedResolvedDatapointID()).toBe('dp-raw-b')
    expect(reportedSelectedSeriesKey()).toBe('route=/b')
    expect(new URLSearchParams(window.location.search).get('dp')).toBe(
      'dp-raw-b'
    )
    expect(new URLSearchParams(window.location.search).get('series')).toBe(
      'route=/b'
    )
  })

  it('resolves a cold-load raw datapoint only after its series loads', async () => {
    const raw = makeSumDatapoint('dp-raw-b', 30_000, 7)
    const view = renderProbeView('/metrics/m1?dp=dp-raw-b&series=route%3D%2Fb')

    expect(reportedSelectedDatapointID()).toBe('')
    expect(reportedResolvedDatapointID()).toBe('')
    expect(reportedSelectedSeriesKey()).toBe('route=/b')
    expect(view.context.expandedTimeseries.has('route=/b')).toBe(true)
    expect(new URLSearchParams(window.location.search).get('dp')).toBe(
      'dp-raw-b'
    )

    await view.setSeriesDatapoints({ 'route=/b': [raw] })
    await tick()

    expect(reportedSelectedDatapointID()).toBe('dp-raw-b')
    expect(reportedResolvedDatapointID()).toBe('dp-raw-b')
    expect(reportedSelectedSeriesKey()).toBe('route=/b')
    expect(view.context.selectionSource).toBe('detail')
  })

  it('resolves a raw-only datapoint after Back triggers its lazy fetch', async () => {
    const raw = makeSumDatapoint('dp-raw-b', 30_000, 7)
    const view = renderProbeView('/metrics/m1')

    externalNavigationTo('/metrics/m1?dp=dp-raw-b&series=route%3D%2Fb')
    await tick()

    expect(reportedSelectedDatapointID()).toBe('')
    expect(view.context.expandedTimeseries.has('route=/b')).toBe(true)

    await view.setSeriesDatapoints({ 'route=/b': [raw] })
    await tick()

    expect(reportedSelectedDatapointID()).toBe('dp-raw-b')
    expect(reportedResolvedDatapointID()).toBe('dp-raw-b')
    expect(view.context.selectionSource).toBe('detail')
  })

  it('does not resurrect a rejected pending datapoint without new navigation', async () => {
    const raw = makeSumDatapoint('dp-raw-b', 30_000, 7)
    const view = renderProbeView('/metrics/m1?dp=dp-raw-b&series=route%3D%2Fb')

    await view.setSeriesDatapoints({ 'route=/b': [] })
    await tick()
    expect(reportedSelectedDatapointID()).toBe('')

    await view.setSeriesDatapoints({ 'route=/b': [raw] })
    navigateCurrentRoute(
      withQueryPatch(readRoute().query, { start: '1000' }),
      'replace'
    )
    await tick()

    expect(reportedSelectedDatapointID()).toBe('')
    expect(reportedResolvedDatapointID()).toBe('')

    externalNavigationTo('/metrics/m1?series=route%3D%2Fb')
    await tick()
    externalNavigationTo('/metrics/m1?dp=dp-raw-b&series=route%3D%2Fb')
    await tick()

    expect(reportedSelectedDatapointID()).toBe('dp-raw-b')
  })

  it('rejects a URL datapoint owned by a different series', () => {
    renderProbe('/metrics/m1?dp=dp-a1&series=route%3D%2Fb')

    expect(reportedSelectedDatapointID()).toBe('')
    expect(reportedSelectedSeriesKey()).toBe('route=/b')
  })

  it('keeps reduced chart and raw detail datapoints selectable together', async () => {
    const raw = makeSumDatapoint('dp-raw-b', 30_000, 7)
    const ctx = renderProbe('/metrics/m1', {
      seriesDatapoints: { 'route=/b': [raw] },
    })

    ctx.onChartPointClick('route=/b', 'dp-b2')
    await tick()
    expect(reportedSelectedDatapointID()).toBe('dp-b2')
    expect(reportedResolvedDatapointID()).toBe('dp-b2')
    expect(ctx.selectionSource).toBe('chart')

    ctx.onDatapointClick(raw)
    await tick()
    expect(reportedSelectedDatapointID()).toBe('dp-raw-b')
    expect(reportedResolvedDatapointID()).toBe('dp-raw-b')
    expect(reportedSelectedSeriesKey()).toBe('route=/b')
    expect(ctx.selectionSource).toBe('detail')
  })

  it('maps a reduced raw selection onto its exact synthetic rate bucket', async () => {
    const { metric, later, selectedBucketStart } = makeRateSelectionMetric()
    const ctx = renderProbe('/metrics/m1?agg=rate', { metric })

    ctx.onDatapointClick(later)
    await tick()

    expect(ctx.highlightedTimestamp).toBe(selectedBucketStart)
    expect(ctx.selectedRateSlope).toBe(6)
    const selectedLine = ctx.transformedGaugeSumChartTimeseries.find(
      series => series.key === 'route=/a'
    )
    expect(
      selectedLine?.points.find(
        point => point.timestampNs === selectedBucketStart
      )?.value
    ).toBe(4)
  })

  it('does not map a raw-only same-time selection onto a reduced rate bucket', async () => {
    const { metric, later } = makeRateSelectionMetric()
    const rawOnly = makeSumDatapointAt('rate-raw-only', later.timestamp, 99)
    const ctx = renderProbe('/metrics/m1?agg=rate', {
      metric,
      seriesDatapoints: { 'route=/a': [rawOnly] },
    })

    ctx.onDatapointClick(rawOnly)
    await tick()

    expect(ctx.selectedDatapointID).toBe(rawOnly.id)
    expect(ctx.selectedSeriesKey).toBe('route=/a')
    expect(ctx.highlightedTimestamp).toBeNull()
    expect(ctx.selectedRateSlope).toBeUndefined()
  })

  it('lets an exact chart selection supersede a pending URL datapoint', async () => {
    const raw = makeSumDatapoint('dp-raw-b', 30_000, 7)
    const view = renderProbeView('/metrics/m1?dp=dp-raw-b&series=route%3D%2Fb')

    view.context.onChartPointClick('route=/a', 'dp-a1')
    await view.setSeriesDatapoints({ 'route=/b': [raw] })
    await tick()

    expect(reportedSelectedDatapointID()).toBe('dp-a1')
    expect(reportedResolvedDatapointID()).toBe('dp-a1')
    expect(reportedSelectedSeriesKey()).toBe('route=/a')
    expect(view.context.selectionSource).toBe('chart')
  })

  it('reuses the raw selection index across repeated lookups and URL sync', async () => {
    const raw = Array.from({ length: 1_000 }, (_, index) =>
      makeSumDatapoint(`dp-raw-${index}`, index, index)
    )
    let iterations = 0
    const iterator = raw[Symbol.iterator].bind(raw)
    Object.defineProperty(raw, Symbol.iterator, {
      value: () => {
        iterations++
        return iterator()
      },
    })
    const ctx = renderProbe('/metrics/m1', {
      seriesDatapoints: { 'route=/b': raw },
    })
    const indexedAt = iterations

    ctx.onDatapointClick(raw[999]!)
    await tick()
    void ctx.selectedDatapoint
    void ctx.selectedSeriesKey
    void ctx.selectedDatapoint

    expect(iterations).toBe(indexedAt)
  })

  it('clears a chart-owned datapoint selection and its URL state', async () => {
    const ctx = renderProbe('/metrics/m1')

    ctx.onChartPointClick('route=/a', 'dp-a1')
    await tick()
    expect(reportedSelectedDatapointID()).toBe('dp-a1')

    ctx.clearChartSelection()
    await tick()

    expect(reportedSelectedDatapointID()).toBe('')
    expect(new URLSearchParams(window.location.search).get('dp')).toBeNull()
  })

  it('does not select a neighboring datapoint for an unknown source id', () => {
    const ctx = renderProbe('/metrics/m1')

    ctx.onChartPointClick('route=/a', 'missing-datapoint')

    expect(reportedSelectedDatapointID()).toBe('')
  })

  it('does not clear a raw datapoint selected from the detail pane', async () => {
    const datapoint = makeSumDatapoint('dp-raw-b', 30_000, 7)
    const ctx = renderProbe('/metrics/m1', {
      seriesDatapoints: { 'route=/b': [datapoint] },
    })

    ctx.onDatapointClick(datapoint)
    await tick()
    ctx.clearChartSelection()
    await tick()

    expect(reportedSelectedDatapointID()).toBe(datapoint.id)
    expect(reportedResolvedDatapointID()).toBe(datapoint.id)
    expect(new URLSearchParams(window.location.search).get('dp')).toBe(
      datapoint.id
    )
  })
})

describe('metric view context histogram aggregate decoding', () => {
  it('keeps an explicit empty-bound catch-all as an explicit histogram', () => {
    const [slice] = aggregateToSlices([
      {
        timestamp: String(BigInt(BASE_TIMESTAMP_MS) * 1_000_000n),
        startTime: String(BigInt(BASE_TIMESTAMP_MS) * 1_000_000n),
        count: 7,
        sum: 0,
        min: 0,
        max: 0,
        bucketCounts: [7],
        explicitBounds: [],
        quantiles: null,
      },
    ])

    expect(slice).toMatchObject({
      kind: 'histogram',
      bounds: [],
      counts: [7],
    })
  })
})
