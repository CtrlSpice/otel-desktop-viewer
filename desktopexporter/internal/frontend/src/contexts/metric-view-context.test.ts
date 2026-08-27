// @vitest-environment jsdom
import { describe, expect, it } from 'vitest'
import { tick } from 'svelte'
import { screen } from '@testing-library/svelte'
import { navigateCurrentRoute, readRoute, withQueryPatch } from '@/route'
import type { MetricViewContext } from '@/contexts/metric-view-context.svelte'
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

function renderProbe(url: string): MetricViewContext {
  setTestUrl(url)
  let captured: MetricViewContext | undefined
  renderWithContexts(MetricViewProbe, {
    metric: makeCumulativeSumMetric(),
    oncontext: (ctx: MetricViewContext) => {
      captured = ctx
    },
  })
  if (!captured) throw new Error('probe did not report a metric view context')
  return captured
}

function reportedAggregationView(): string {
  return screen.getByTestId('aggregation-view').textContent?.trim() ?? ''
}

function reportedSelectedDatapointID(): string {
  return screen.getByTestId('selected-datapoint-id').textContent?.trim() ?? ''
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

  it('clears the selection when the browser goes back to a URL without dp', async () => {
    renderProbe('/metrics/m1?dp=dp-b2')
    expect(reportedSelectedDatapointID()).toBe('dp-b2')

    externalNavigationTo('/metrics/m1')
    await tick()

    expect(reportedSelectedDatapointID()).toBe('')
  })
})
