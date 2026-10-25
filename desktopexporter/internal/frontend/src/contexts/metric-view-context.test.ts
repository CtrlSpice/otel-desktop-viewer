// @vitest-environment jsdom
import { describe, expect, it, vi } from 'vitest'
import { tick } from 'svelte'
import { fireEvent, screen, waitFor, within } from '@testing-library/svelte'
import { navigateCurrentRoute, readRoute, withQueryPatch } from '@/route'
import {
  aggregateToSlices,
  type MetricViewContext,
} from '@/contexts/metric-view-context.svelte'
import type {
  MetricViewData,
  ResourceData,
  ScalarAggregate,
  ScopeData,
  SumDataPoint,
} from '@/types/api-types'
import MetricViewProbe from '@/test/MetricViewProbe.svelte'
import { renderWithContexts, setTestUrl } from '@/test/render-helpers'
import { metricViewStorageKey } from '@/components/metrics/utils/metric-timeseries-visible'
import { detailPanel } from '@/state/detail-panel.svelte'
import { getSeriesPalette } from '@/themes'
import { graphColorOverrides } from '@/state/graph-color-overrides.svelte'
import { GRAPH_SWATCHES } from '@/themes/graph-inks'

const BASE_TIMESTAMP_MS = 1_700_000_000_000

it('retains saved overrides and datapoint selection across seasonal changes', async () => {
  const previousTheme = document.documentElement.getAttribute('data-theme')
  const scope = 'metric:ink-switch-test'
  setTestUrl('/metrics/ink-switch-test')
  let ctx!: MetricViewContext
  const manual = GRAPH_SWATCHES[100]!
  const view = renderWithContexts(MetricViewProbe, {
    metric: { ...makeCumulativeSumMetric(), metricRef: 'ink-switch-test' },
    oncontext: context => (ctx = context),
  })
  try {
    await tick()
    const original = new Map(ctx.timeseriesColorByKey)
    graphColorOverrides.set(scope, 'route=/a', manual.id)
    ctx.onChartPointClick('route=/a', 'dp-a1')
    await tick()
    const url = window.location.href
    const visible = [...ctx.visibleSeries]
    document.documentElement.setAttribute('data-theme', 'madoromi-fuyu-hiru')
    await tick()
    expect(ctx.timeseriesColorByKey.get('route=/a')).toBe(manual.color)
    expect(ctx.timeseriesColorByKey.get('route=/b')).toBe(
      original.get('route=/b')
    )
    expect(ctx.selectedDatapointID).toBe('dp-a1')
    expect([...ctx.visibleSeries]).toEqual(visible)
    expect(window.location.href).toBe(url)
    graphColorOverrides.set(scope, 'route=/a', null)
    await tick()
    expect(ctx.timeseriesColorByKey).toEqual(original)
  } finally {
    view.unmount()
    graphColorOverrides.set(scope, 'route=/a', null)
    if (previousTheme)
      document.documentElement.setAttribute('data-theme', previousTheme)
    else document.documentElement.removeAttribute('data-theme')
  }
})

it('keeps chart selection and native disclosures independent', async () => {
  setTestUrl('/metrics/m1')
  let context!: MetricViewContext
  const scroll = vi.fn()
  const original = Element.prototype.scrollIntoView
  Element.prototype.scrollIntoView = scroll
  try {
    const metric = makeCumulativeSumMetric()
    const view = renderWithContexts(MetricViewProbe, {
      metric,
      showDetail: true,
      oncontext: ctx => (context = ctx),
    })
    const series = screen
      .getByText('Series', { exact: true })
      .closest('details')!
    const fields = screen
      .getByText('Metric', { exact: true })
      .closest('details')!
    expect(series.open).toBe(true)
    expect(fields.open).toBe(true)
    expect(
      series.compareDocumentPosition(fields) & Node.DOCUMENT_POSITION_FOLLOWING
    ).not.toBe(0)
    expect(
      screen.getByRole('tablist', { name: 'Detail view' })
    ).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: 'Properties' })).toHaveAttribute(
      'aria-selected',
      'true'
    )
    const metadata = screen.getByRole('table', { name: 'Metric fields' })
    for (const [key, value] of [
      ['name', 'test-scope'],
      ['version', '1.0.0'],
    ]) {
      const row = within(metadata)
        .getByRole('button', { name: `Copy ${key} (scope)` })
        .closest('tr')!
      expect(row.querySelector('.detail-property__key')).toHaveTextContent(key!)
      expect(row.querySelector('.detail-property__origin')).toHaveTextContent(
        'scope'
      )
      expect(row.querySelector('.detail-property__type')).toHaveTextContent(
        'string'
      )
      expect(row.querySelector('.detail-property__body')).toHaveTextContent(
        value!
      )
    }
    expect(fields.querySelectorAll('details')).toHaveLength(0)
    for (let selection = 0; selection < 2; selection++) {
      detailPanel.setOpen(false)
      context.onChartPointClick('route=/a', 'dp-a1')
      await tick()
      await tick()
      expect(detailPanel.open).toBe(true)
      expect(series.open).toBe(true)
      expect(fields.open).toBe(true)
      expect(document.querySelector('[data-dp-id="dp-a1"]')).toBeVisible()
      expect(context.expandedTimeseries.has('route=/a')).toBe(true)
    }
    await waitFor(() =>
      expect(scroll).toHaveBeenCalledWith({ block: 'nearest' })
    )
    await fireEvent.click(series.querySelector('summary')!)
    expect(series.open).toBe(true)
    await fireEvent.click(fields.querySelector('summary')!)
    detailPanel.setOpen(false)
    await tick()
    detailPanel.setOpen(true)
    await tick()
    expect(series.open).toBe(true)
    expect(fields.open).toBe(false)
    detailPanel.setOpen(false)
    await view.rerender({
      componentProps: { metric: { ...metric }, showDetail: true },
    })
    await tick()
    expect(detailPanel.open).toBe(false)
  } finally {
    Element.prototype.scrollIntoView = original
  }
})

it('keeps series attributes and datapoints in one table without nested disclosures', async () => {
  setTestUrl('/metrics/m1')
  const metric = makeCumulativeSumMetric()
  const view = renderWithContexts(MetricViewProbe, {
    metric,
    showTimeseries: true,
    oncontext: ctx => ctx.expandedTimeseries.add('route=/a'),
  })
  await tick()
  const series = view.container.querySelector<HTMLTableRowElement>(
    '[data-series-detail="route=/a"]'
  )!
  const table = within(series).getByRole('table', { name: 'Datapoints' })
  expect(series.querySelectorAll('table')).toHaveLength(1)
  expect(series.querySelectorAll('details')).toHaveLength(0)
  const attribute = table.querySelector('[data-field="route"]')!
  expect(attribute.querySelector('.detail-property__key')).toHaveTextContent(
    'route'
  )
  expect(attribute.querySelector('.detail-property__type')).toHaveTextContent(
    'string'
  )
  expect(table).toHaveTextContent('/a')
  expect(table).toHaveTextContent('Time')
  expect(table).toHaveTextContent('Value')
  expect(series.querySelector('.field-group__caret')).toBeNull()
  expect(
    view.container.querySelector('[data-series-key="route=/a"]')
  ).toHaveClass('item-state')
  expect(series).not.toHaveAttribute('hidden')
  expect(screen.queryByText('Resource attributes')).toBeNull()

  await view.rerender({
    componentProps: { metric: { ...metric, id: 'm2' }, showTimeseries: true },
  })
  await tick()
  expect(series.querySelectorAll('table')).toHaveLength(1)
})

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
    aggregationTemporalityCode: 2,
    aggregationTemporality: 'Cumulative',
  }
}

/**
 * Cumulative monotonic Sum with two timeseries: 'rate' is an explicit option,
 * and the second series unlocks 'sum' / 'avg'. Raw remains the default.
 */
function makeCumulativeSumMetric(): MetricViewData {
  return {
    metricRef: 'm1',
    name: 'http.server.requests',
    description: 'Total inbound requests',
    metadata: [],
    unit: '1',
    metricType: 'Sum',
    aggregationTemporalityCode: 2,
    aggregationTemporality: 'Cumulative',
    isMonotonic: true,
    resourceDroppedAttributesCount: 0,
    resourceSchemaUrl: '',
    resource: EMPTY_RESOURCE,
    scopeName: EMPTY_SCOPE.name,
    scopeVersion: EMPTY_SCOPE.version,
    scopeSchemaUrl: '',
    scopeDroppedAttributesCount: 0,
    scope: EMPTY_SCOPE,
    datapointCount: 0,
    // The window as the caller asked for it: this fixture's assertions are
    // about aggregation views, not about the axis.
    window: {
      requested: { startNs: null, endNs: null },
      effective: {
        startNs: BigInt(BASE_TIMESTAMP_MS) * 1_000_000n,
        endNs: BigInt(BASE_TIMESTAMP_MS + 60_000) * 1_000_000n,
      },
    },
    lastSeenNs: BigInt(BASE_TIMESTAMP_MS + 60_000) * 1_000_000n,
    // No merge was refused; this fixture's histograms-that-aren't have no
    // bounds to disagree about.
    boundsMismatch: null,
    timeseries: [
      {
        seriesRef: 'route=/a',
        resource: { attributes: [], droppedAttributesCount: 0 },
        attributes: [{ key: 'route', value: { kind: 'string', value: '/a' } }],
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
        seriesRef: 'route=/b',
        resource: { attributes: [], droppedAttributesCount: 0 },
        attributes: [{ key: 'route', value: { kind: 'string', value: '/b' } }],
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
  metric?: MetricViewData
  seriesDatapoints?: Readonly<Record<string, SumDataPoint[]>>
  scalarAggregate?: ScalarAggregate
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
    scalarAggregate: options.scalarAggregate,
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

it('treats unknown received temporalities as unsafe for derived modes', () => {
  const metric = makeCumulativeSumMetric()
  metric.aggregationTemporalityCode = 99
  metric.aggregationTemporality = 'Cumulative'

  const ctx = renderProbe('/metrics/m1', { metric })

  expect(ctx.isUnsafeTemporality).toBe(true)
  expect(ctx.temporalityCode).toBe(99)
  expect(ctx.availableAggregationViews).toEqual(['raw'])
})

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

describe('metric effective window', () => {
  it('uses concrete effective bounds for the chart domain', () => {
    renderProbe('/metrics/m1')
    expect(screen.getByTestId('chart-time-start')).toHaveTextContent(
      String(BASE_TIMESTAMP_MS)
    )
    expect(screen.getByTestId('chart-time-end')).toHaveTextContent(
      String(BASE_TIMESTAMP_MS + 60_000)
    )
  })

  it('leaves the chart domain empty when effective bounds are unresolved', () => {
    const metric = makeCumulativeSumMetric()
    metric.window.effective = { startNs: null, endNs: null }
    renderProbe('/metrics/m1', { metric })
    expect(screen.getByTestId('chart-time-start')).toHaveTextContent('')
    expect(screen.getByTestId('chart-time-end')).toHaveTextContent('')
  })
})

/** Simulate external browser navigation without adding a history entry. */
function externalNavigationTo(url: string): void {
  window.history.replaceState(null, '', url)
  window.dispatchEvent(new PopStateEvent('popstate'))
}

describe('metric view context aggregation URL sync', () => {
  it('remembers histogram mode across both histogram kinds and intervening gauges without overriding links', () => {
    const storageKey = 'metrics:histogram-view'
    const previous = localStorage.getItem(storageKey)
    localStorage.removeItem(storageKey)
    try {
      const visits = [
        {
          type: 'Histogram',
          query: '',
          expected: 'heatmap',
          choose: 'histogram',
        },
        { type: 'ExponentialHistogram', query: '', expected: 'histogram' },
        { type: 'Gauge', query: '', expected: null },
        { type: 'Histogram', query: '', expected: 'histogram' },
        {
          type: 'ExponentialHistogram',
          query: '?htab=heatmap',
          expected: 'heatmap',
        },
        { type: 'Histogram', query: '', expected: 'histogram' },
      ] as const
      for (const [index, visit] of visits.entries()) {
        const metric: MetricViewData = {
          ...makeCumulativeSumMetric(),
          metricRef: `shared-view-${index}`,
          metricType: visit.type,
          timeseries: [],
          datapointCount: 0,
        }
        setTestUrl(`/metrics/${metric.metricRef}${visit.query}`)
        let context: MetricViewContext | undefined
        const view = renderWithContexts(MetricViewProbe, {
          metric,
          oncontext: (ctx: MetricViewContext) => (context = ctx),
        })
        if (!context)
          throw new Error('probe did not report a metric view context')
        if (visit.expected)
          expect(context.activeHistogramTab).toBe(visit.expected)
        else expect(context.aggregationView).toBe('raw')
        if ('choose' in visit) context.setActiveHistogramTab(visit.choose)
        view.unmount()
      }
    } finally {
      if (previous === null) localStorage.removeItem(storageKey)
      else localStorage.setItem(storageKey, previous)
    }
  })

  it('offers rate for a cumulative monotonic sum with two series', () => {
    renderProbe('/metrics/m1')
    expect(reportedAvailableAggregationViews()).toEqual([
      'raw',
      'sum',
      'avg',
      'rate',
    ])
  })

  it('uses Raw when the URL carries no aggregation or saved preference', () => {
    renderProbe('/metrics/m1')
    expect(reportedAggregationView()).toBe('raw')
  })

  it('shows a single counter reading in Raw rather than deriving an empty rate', () => {
    const metric = makeCumulativeSumMetric()
    const series = metric.timeseries[0]!
    series.datapoints = [series.datapoints[0]!]
    series.datapointCount = 1
    metric.timeseries = [series]
    metric.datapointCount = 1
    const ctx = renderProbe('/metrics/m1', { metric })
    expect(reportedAggregationView()).toBe('raw')
    expect(ctx.transformedGaugeSumChartTimeseries[0]!.points).toHaveLength(1)
    expect(ctx.transformedGaugeSumChartTimeseries[0]!.points[0]!.value).toBe(10)
    expect(reportedAvailableAggregationViews()).toEqual(['raw', 'rate'])
  })

  it.each([
    ['/metrics/m1', 'rate'],
    ['/metrics/m1?agg=bogus', 'rate'],
    ['/metrics/m1?agg=raw', 'raw'],
    ['/metrics/m1?agg=sum', 'sum'],
    ['/metrics/m1?agg=rate', 'rate'],
  ])(
    'respects an applicable saved preference or explicit URL at %s',
    (url, expected) => {
      const key = metricViewStorageKey('m1')
      const preference = JSON.stringify({ aggregationView: 'rate' })
      localStorage.setItem(key, preference)
      try {
        renderProbe(url)
        expect(reportedAggregationView()).toBe(expected)
        expect(localStorage.getItem(key)).toBe(preference)
      } finally {
        localStorage.removeItem(key)
      }
    }
  )

  it('applies an aggregation deep link on load', () => {
    renderProbe('/metrics/m1?agg=sum')
    expect(reportedAggregationView()).toBe('sum')
  })

  it('falls back to Raw when the URL aggregation is invalid', () => {
    renderProbe('/metrics/m1?agg=bogus')
    expect(reportedAggregationView()).toBe('raw')
  })

  it('keeps Raw when the browser goes back to a URL without an aggregation', async () => {
    renderProbe('/metrics/m1?agg=sum')
    expect(reportedAggregationView()).toBe('sum')

    externalNavigationTo('/metrics/m1')
    await tick()

    expect(reportedAggregationView()).toBe('raw')
  })

  it('ignores an aggregation dropped from the URL behind its back', async () => {
    renderProbe('/metrics/m1?agg=rate')

    window.history.replaceState(null, '', '/metrics/m1')
    await tick()

    expect(reportedAggregationView()).toBe('rate')
  })

  it('keeps Raw when an unrelated query param is written', async () => {
    renderProbe('/metrics/m1')
    expect(reportedAggregationView()).toBe('raw')

    navigateCurrentRoute(
      withQueryPatch(readRoute().query, { start: '1000', end: '2000' }),
      'replace'
    )
    await tick()

    expect(window.location.search).toContain('start=1000')
    expect(reportedAggregationView()).toBe('raw')
  })

  it('writes its own aggregation choice to the URL', async () => {
    const ctx = renderProbe('/metrics/m1')

    ctx.setAggregationView('sum')
    await tick()

    expect(window.location.search).toBe('?agg=sum')
    expect(reportedAggregationView()).toBe('sum')
  })

  it('updates the aggregation URL when preference storage rejects the save', async () => {
    const ctx = renderProbe('/metrics/m1')
    const setItem = vi
      .spyOn(Storage.prototype, 'setItem')
      .mockImplementation(() => {
        throw new Error('quota exceeded')
      })

    try {
      ctx.setAggregationView('sum')
      await tick()

      expect(window.location.search).toBe('?agg=sum')
      expect(reportedAggregationView()).toBe('sum')
    } finally {
      setItem.mockRestore()
    }
  })

  it('returns to Raw when the browser goes back past its own write', async () => {
    const ctx = renderProbe('/metrics/m1')

    ctx.setAggregationView('sum')
    await tick()
    expect(reportedAggregationView()).toBe('sum')

    externalNavigationTo('/metrics/m1')
    await tick()

    expect(reportedAggregationView()).toBe('raw')
  })

  it('spells out raw rather than omitting it', async () => {
    const ctx = renderProbe('/metrics/m1')

    ctx.setAggregationView('raw')
    await tick()

    expect(window.location.search).toBe('?agg=raw')
  })
})

describe('metric view context visibility seeding', () => {
  it("seeds the set with the scalar metric's own keys, unclobbered", () => {
    const ctx = renderProbe('/metrics/m1')
    expect(ctx.isHistogramKind).toBe(false)
    expect([...ctx.visibleSeries].sort()).toEqual(['route=/a', 'route=/b'])
  })

  it('keeps the aggregate producer order through the chart series', async () => {
    const metric = makeCumulativeSumMetric()
    metric.timeseries.push({
      ...metric.timeseries[1]!,
      seriesRef: 'route=/c',
      attributes: [{ key: 'route', value: { kind: 'string', value: '/c' } }],
    })
    const buckets = [
      {
        bucketStart: BigInt(BASE_TIMESTAMP_MS) * 1_000_000n,
        sampleCount: 2,
        sum: 14,
        avg: 7,
        rate: null,
        slope: null,
        hasReset: false,
      },
    ]
    const scalarAggregate: ScalarAggregate = {
      selected: buckets,
      all: buckets,
    }
    const ctx = renderProbe('/metrics/m1?agg=sum', {
      metric,
      scalarAggregate,
    })

    ctx.visibleSeries.delete('route=/c')
    ctx.setShowAllSeriesAggregate(true)
    await tick()

    expect(
      ctx.transformedGaugeSumChartTimeseries.map(series => series.key)
    ).toEqual(['route=/a', 'route=/b', '__agg:selected__', '__agg:all__'])
  })

  it('replaces the last full scalar selection for a linked series without saving it', async () => {
    const metric = makeCumulativeSumMetric()
    const template = metric.timeseries[0]!
    metric.timeseries = Array.from({ length: 23 }, (_, index) => ({
      ...template,
      seriesRef: `series-${String(index).padStart(3, '0')}`,
    }))
    const persistedKeys = metric.timeseries
      .slice(0, 22)
      .map(series => series.seriesRef)
    const storageKey = metricViewStorageKey(metric.metricRef)
    const persisted = JSON.stringify({ visibleKeys: persistedKeys })
    localStorage.setItem(storageKey, persisted)

    try {
      const ctx = renderProbe('/metrics/m1?agg=raw', { metric })
      const colorsBeforeReveal = new Map(ctx.timeseriesColorByKey)
      externalNavigationTo('/metrics/m1?agg=raw&series=series-022')
      await tick()

      const visible = [...ctx.visibleSeries]
      expect(visible).toEqual([...persistedKeys.slice(0, -1), 'series-022'])
      expect(visible).toHaveLength(22)
      expect(ctx.timeseriesColorByKey.get('series-021')).toBeUndefined()
      expect(ctx.timeseriesColorByKey.get('series-022')).not.toBe(
        colorsBeforeReveal.get('series-021')
      )
      for (const key of persistedKeys.slice(0, -1)) {
        expect(ctx.timeseriesColorByKey.get(key)).toBe(
          colorsBeforeReveal.get(key)
        )
      }
      expect(
        ctx.transformedGaugeSumChartTimeseries.map(series => series.key)
      ).toContain('series-022')
      expect(localStorage.getItem(storageKey)).toBe(persisted)
    } finally {
      localStorage.removeItem(storageKey)
    }
  })

  it('keeps visible key slots and colour identities across theme changes', async () => {
    const previousTheme = document.documentElement.getAttribute('data-theme')
    document.documentElement.setAttribute('data-theme', 'madoromi-aki-yoru')
    try {
      const ctx = renderProbe('/metrics/m1')
      const before = new Map(ctx.timeseriesColorByKey)
      document.documentElement.setAttribute('data-theme', 'madoromi-fuyu-hiru')
      await tick()
      await tick()

      expect([...ctx.timeseriesColorByKey.keys()]).toEqual([...before.keys()])
      const previous = getSeriesPalette('madoromi-aki-yoru')
      const next = getSeriesPalette('madoromi-fuyu-hiru')
      for (const [key, color] of before) {
        expect(ctx.timeseriesColorByKey.get(key)).toBe(
          next[previous.indexOf(color)]
        )
      }
    } finally {
      if (previousTheme === null)
        document.documentElement.removeAttribute('data-theme')
      else document.documentElement.setAttribute('data-theme', previousTheme)
    }
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

  it('pins an exact Rate bucket directly without inventing a raw datapoint or URL id', async () => {
    const { metric, selectedBucketStart } = makeRateSelectionMetric()
    const ctx = renderProbe('/metrics/m1?agg=rate', { metric })
    ctx.onRateChartPointClick('route=/a', selectedBucketStart)
    await tick()
    expect(ctx.highlightedTimestamp).toBe(selectedBucketStart)
    expect(ctx.selectedSeriesKey).toBe('route=/a')
    expect(ctx.selectedRateSlope).toBe(6)
    expect(ctx.selectedDatapointID).toBeNull()
    expect(ctx.selectionSource).toBe('chart')
    expect(new URLSearchParams(window.location.search).has('dp')).toBe(false)
    ctx.onRateChartPointClick('route=/a', selectedBucketStart + 1n)
    await tick()
    expect(ctx.highlightedTimestamp).toBe(selectedBucketStart)
    ctx.clearChartSelection()
    await tick()
    expect(ctx.highlightedTimestamp).toBeNull()
  })

  it('keeps raw detail selection underneath a Rate pick and lets the list take over again', async () => {
    const { metric, later, selectedBucketStart } = makeRateSelectionMetric()
    const ctx = renderProbe('/metrics/m1?agg=rate', { metric })
    ctx.onDatapointClick(later)
    await tick()
    const before = window.location.search
    const earlierBucket = selectedBucketStart - 300n
    ctx.onRateChartPointClick('route=/a', earlierBucket)
    await tick()
    expect(ctx.highlightedTimestamp).toBe(earlierBucket)
    expect(ctx.selectedDatapointID).toBe(later.id)
    expect(window.location.search).toBe(before)
    ctx.clearChartSelection()
    await tick()
    expect(ctx.highlightedTimestamp).toBe(selectedBucketStart)
    expect(ctx.selectionSource).toBe('detail')
    ctx.onRateChartPointClick('route=/a', earlierBucket)
    ctx.onDatapointClick(later)
    await tick()
    expect(ctx.highlightedTimestamp).toBe(selectedBucketStart)
    expect(ctx.selectedDatapointID).toBe(later.id)
  })

  it('rejects unknown or aggregate Rate buckets and clears a pick when its series is hidden', async () => {
    const { metric, selectedBucketStart } = makeRateSelectionMetric()
    const ctx = renderProbe('/metrics/m1?agg=rate', { metric })
    ctx.onRateChartPointClick('__agg:selected__', selectedBucketStart)
    ctx.onRateChartPointClick('missing', selectedBucketStart)
    await tick()
    expect(ctx.highlightedTimestamp).toBeNull()
    ctx.onRateChartPointClick('route=/a', selectedBucketStart)
    await tick()
    expect(ctx.highlightedTimestamp).toBe(selectedBucketStart)
    ctx.visibleSeries.delete('route=/a')
    await tick()
    expect(ctx.highlightedTimestamp).toBeNull()
    ctx.setAggregationView('raw')
    ctx.onRateChartPointClick('route=/a', selectedBucketStart)
    await tick()
    expect(ctx.highlightedTimestamp).toBeNull()
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

  it('reopens details when selecting the same chart datapoint again', async () => {
    const ctx = renderProbe('/metrics/m1')
    for (let selection = 0; selection < 2; selection++) {
      detailPanel.setOpen(false)
      ctx.onChartPointClick('route=/a', 'dp-a1')
      await tick()
      expect(detailPanel.open).toBe(true)
      expect(reportedSelectedDatapointID()).toBe('dp-a1')
    }
  })

  it('does not select a neighboring datapoint for an unknown source id', () => {
    const ctx = renderProbe('/metrics/m1')
    detailPanel.setOpen(false)

    ctx.onChartPointClick('route=/a', 'missing-datapoint')

    expect(reportedSelectedDatapointID()).toBe('')
    expect(detailPanel.open).toBe(false)
  })

  it('does not change selection or URL for an aggregate chart point', async () => {
    const ctx = renderProbe('/metrics/m1')

    ctx.onChartPointClick('route=/a', 'dp-a1')
    await tick()
    const selectionUrl = window.location.href
    detailPanel.setOpen(false)

    ctx.onChartPointClick('__agg:selected__', 'dp-a1')
    await tick()

    expect(reportedSelectedDatapointID()).toBe('dp-a1')
    expect(window.location.href).toBe(selectionUrl)
    expect(detailPanel.open).toBe(false)
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
  it('keeps an unavailable merged sum distinct from zero', () => {
    const [unknown, zero] = aggregateToSlices([
      {
        timestamp: String(BigInt(BASE_TIMESTAMP_MS) * 1_000_000n),
        startTime: String(BigInt(BASE_TIMESTAMP_MS) * 1_000_000n),
        count: 1,
        sum: null,
        min: 0,
        max: 1,
        bucketCounts: [1],
        explicitBounds: [],
        quantiles: null,
      },
      {
        timestamp: String(BigInt(BASE_TIMESTAMP_MS + 1) * 1_000_000n),
        startTime: String(BigInt(BASE_TIMESTAMP_MS + 1) * 1_000_000n),
        count: 1,
        sum: 0,
        min: 0,
        max: 1,
        bucketCounts: [1],
        explicitBounds: [],
        quantiles: null,
      },
    ])

    expect(unknown!.totals.sum).toBeUndefined()
    expect(zero!.totals.sum).toBe(0)
  })

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
