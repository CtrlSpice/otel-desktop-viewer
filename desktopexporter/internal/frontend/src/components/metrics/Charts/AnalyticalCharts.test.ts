// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte'
import { tick, type Component } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import HistogramChart from './HistogramChart.svelte'
import HistogramHeatmap from './HistogramHeatmap.svelte'
import MetricQuantileAreaChart from './MetricQuantileAreaChart.svelte'
import MetricTimeSeriesChart from './MetricTimeSeriesChart.svelte'
import MetricChartHarness from '@/test/MetricChartHarness.svelte'
import type { HistogramSlicePoint } from '@/components/metrics/utils/histogram-aggregation'
import { quantileSeriesKey } from '@/components/metrics/utils/histogram-aggregation'
import type { ChartTimeseries } from '@/types/metric-chart-types'
import type {
  ExponentialHistogramDataPoint,
  GaugeDataPoint,
  HistogramDataPoint,
  MetricData,
  ResourceData,
  ScopeData,
} from '@/types/api-types'
import type { MetricViewContext } from '@/contexts/metric-view-context.svelte'

const BASE_MS = 1_700_000_000_000
const BASE_NS = BigInt(BASE_MS) * 1_000_000n
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

function gaugeDatapoint(id: string, timestamp: bigint, value: number) {
  return {
    id,
    timestamp,
    timestampMs: Number(timestamp / 1_000_000n),
    startTime: timestamp,
    flags: 0,
    exemplars: [],
    metricType: 'Gauge',
    doubleValue: value,
    intValue: null,
    valueType: 'double',
  } satisfies GaugeDataPoint
}

function histogramDatapoint(
  id: string,
  explicitBounds: number[],
  bucketCounts: number[]
): HistogramDataPoint {
  return {
    id,
    timestamp: BASE_NS,
    timestampMs: BASE_MS,
    startTime: BASE_NS,
    flags: 0,
    exemplars: [],
    metricType: 'Histogram',
    count: bucketCounts.reduce((sum, count) => sum + count, 0),
    sum: 0,
    min: 0,
    max: 0,
    bucketCounts,
    explicitBounds,
    aggregationTemporality: 'Delta',
    quantiles: null,
  }
}

function exponentialHistogramDatapoint(
  id: string,
  zeroThreshold: number
): ExponentialHistogramDataPoint {
  return {
    id,
    timestamp: BASE_NS,
    timestampMs: BASE_MS,
    startTime: BASE_NS,
    flags: 0,
    exemplars: [],
    metricType: 'ExponentialHistogram',
    count: 7,
    sum: 0,
    min: -zeroThreshold,
    max: zeroThreshold,
    scale: 1,
    zeroCount: 7,
    zeroThreshold,
    positiveBucketOffset: 0,
    positiveBucketCounts: [],
    negativeBucketOffset: 0,
    negativeBucketCounts: [],
    aggregationTemporality: 'Delta',
    quantiles: null,
  }
}

function metricWithDatapoints(
  metricType: 'Gauge' | 'Histogram',
  datapoints: GaugeDataPoint[] | HistogramDataPoint[]
): MetricData {
  return {
    id: 'metric-1',
    name: 'test.metric',
    description: 'test metric',
    metadata: [],
    unit: 'ms',
    metricType,
    aggregationTemporality: metricType === 'Gauge' ? null : 'Delta',
    isMonotonic: null,
    resourceDroppedAttributesCount: 0,
    resource: EMPTY_RESOURCE,
    scopeName: EMPTY_SCOPE.name,
    scopeVersion: EMPTY_SCOPE.version,
    scopeDroppedAttributesCount: 0,
    scope: EMPTY_SCOPE,
    datapointCount: datapoints.length,
    window: {
      requested: { startNs: null, endNs: null },
      effective: { startNs: BASE_NS, endNs: BASE_NS + 60_000_000_000n },
    },
    lastSeenNs: datapoints[0]?.timestamp ?? null,
    boundsMismatch: null,
    timeseries: [
      {
        attributesKey: 'series-a',
        resource: EMPTY_RESOURCE,
        attributes: [],
        datapoints,
        stats: null,
        views: null,
        sparkline: null,
        rateStats: null,
        datapointCount: datapoints.length,
        lastSeenNs: datapoints[0]?.timestamp ?? null,
      },
    ],
  }
}

function renderChart(
  component: Component<any>,
  componentProps: Record<string, unknown>,
  metric: MetricData,
  oncontext?: (context: MetricViewContext) => void
) {
  return render(MetricChartHarness, {
    props: { component, componentProps, metric, oncontext },
  })
}

function readout(): string {
  return (
    document
      .querySelector('.chart-keyboard-readout')
      ?.textContent?.replace(/\s+/g, ' ') ?? ''
  ).trim()
}

let clientWidthSpy: ReturnType<typeof vi.spyOn> | undefined
let clientHeightSpy: ReturnType<typeof vi.spyOn> | undefined

beforeEach(() => {
  window.history.replaceState(null, '', '/metrics/metric-1')
  clientWidthSpy = vi
    .spyOn(HTMLElement.prototype, 'clientWidth', 'get')
    .mockReturnValue(240)
  clientHeightSpy = vi
    .spyOn(HTMLElement.prototype, 'clientHeight', 'get')
    .mockReturnValue(320)
})

afterEach(() => {
  clientWidthSpy?.mockRestore()
  clientHeightSpy?.mockRestore()
})

describe('MetricTimeSeriesChart keyboard model', () => {
  it('distinguishes sub-millisecond source points, follows external selection on focus, and activates the exact id', async () => {
    const first = gaugeDatapoint('first', BASE_NS + 100n, 10)
    const second = gaugeDatapoint('second', BASE_NS + 900n, 20)
    const metric = metricWithDatapoints('Gauge', [second, first])
    const timeseries: ChartTimeseries[] = [
      {
        key: 'series-a',
        label: 'series A',
        points: [
          {
            date: new Date(BASE_MS),
            value: 10,
            timestampNs: first.timestamp,
            sourceDatapointID: first.id,
          },
          {
            date: new Date(BASE_MS),
            value: 20,
            timestampNs: second.timestamp,
            sourceDatapointID: second.id,
          },
        ],
      },
    ]
    const onChartPointClick = vi.fn()
    const props = {
      timeseries,
      colorByKey: new Map([['series-a', '#123456']]),
      highlightedTimestamp: second.timestamp,
      highlightedPointID: second.id,
      selectedSeriesKey: 'series-a',
      onChartPointClick,
    }
    const { rerender } = renderChart(MetricTimeSeriesChart, props, metric)
    const surface = screen.getByRole('application', {
      name: 'Metric time series chart',
    })

    expect(readout()).toContain('Point 2 of 2')
    expect(
      document.querySelector('.metric-time-series-chart__selection-legend')
    ).toHaveTextContent('20')
    expect(document.querySelector('.chart-keyboard-cursor')).toBeNull()
    await fireEvent.focus(surface)
    await tick()
    expect(document.querySelector('.chart-keyboard-cursor')).not.toBeNull()
    await fireEvent.keyDown(surface, { key: 'Enter' })
    expect(onChartPointClick).toHaveBeenLastCalledWith('series-a', 'second')

    await rerender({
      component: MetricTimeSeriesChart,
      componentProps: {
        ...props,
        highlightedTimestamp: first.timestamp,
        highlightedPointID: first.id,
      },
      metric,
    })
    await tick()
    expect(readout()).toContain('Point 1 of 2')

    await fireEvent.keyDown(surface, { key: 'ArrowRight' })
    await fireEvent.keyDown(surface, { key: 'Enter' })
    expect(onChartPointClick).toHaveBeenLastCalledWith('series-a', 'second')

    await rerender({
      component: MetricTimeSeriesChart,
      componentProps: {
        ...props,
        highlightedTimestamp: null,
        highlightedPointID: null,
      },
      metric,
    })
    await fireEvent.blur(surface)
    await fireEvent.focus(surface)
    expect(readout()).toContain('Point 2 of 2')
    await fireEvent.blur(surface)
    await tick()
    expect(document.querySelector('.chart-keyboard-cursor')).toBeNull()
  })

  it('resolves a raw selection to its exact same-ms synthetic rate bucket', () => {
    const firstBucketNs = BASE_NS + 300n
    const selectedBucketNs = BASE_NS + 600n
    const metric = metricWithDatapoints('Gauge', [
      gaugeDatapoint('raw-later', BASE_NS + 900n, 9),
    ])
    renderChart(
      MetricTimeSeriesChart,
      {
        timeseries: [
          {
            key: 'series-a',
            label: 'series A',
            points: [
              {
                date: new Date(BASE_MS),
                value: 2,
                timestampNs: firstBucketNs,
                slope: null,
              },
              {
                date: new Date(BASE_MS),
                value: 4,
                timestampNs: selectedBucketNs,
                slope: 6,
              },
            ],
          },
        ],
        colorByKey: new Map([['series-a', '#123456']]),
        highlightedTimestamp: selectedBucketNs,
        highlightedPointID: 'raw-later',
        selectedSeriesKey: 'series-a',
        aggregationView: 'rate',
        unit: 'requests',
        selectedRateSlope: 6,
        seriesStats: new Map([['series-a', { min: 2, max: 4, avg: 3 }]]),
      },
      metric
    )

    const surface = screen.getByRole('application', {
      name: 'Metric time series chart',
    })
    expect(readout()).toContain('Point 2 of 2')
    expect(readout()).toContain('Value 4 requests/s')
    expect(readout()).toContain('Read-only synthetic point')
    expect(surface.getAttribute('aria-keyshortcuts')).not.toContain('Enter')
    expect(document.querySelector('.selection-dot--selected')).not.toBeNull()
    expect(
      document.querySelector('.metric-time-series-chart__selection-legend')
    ).toHaveTextContent('4')
    expect(
      [...document.querySelectorAll('.series-stat-marker title')].some(title =>
        title.textContent?.startsWith('rate slope')
      )
    ).toBe(true)
  })

  it('does not advertise or perform activation for a synthetic point', async () => {
    const metric = metricWithDatapoints('Gauge', [
      gaugeDatapoint('source', BASE_NS, 10),
    ])
    const onChartPointClick = vi.fn()
    renderChart(
      MetricTimeSeriesChart,
      {
        timeseries: [
          {
            key: 'series-a',
            label: 'synthetic series',
            points: [
              {
                date: new Date(BASE_MS),
                value: 10,
                timestampNs: BASE_NS,
              },
            ],
          },
        ],
        colorByKey: new Map([['series-a', '#123456']]),
        onChartPointClick,
      },
      metric
    )
    const surface = screen.getByRole('application', {
      name: 'Metric time series chart',
    })

    expect(surface.getAttribute('aria-keyshortcuts')).not.toContain('Enter')
    await fireEvent.focus(surface)
    expect(readout()).toContain('Read-only synthetic point')
    await fireEvent.keyDown(surface, { key: 'Enter' })
    expect(onChartPointClick).not.toHaveBeenCalled()
  })

  it('omits a persistent dot when a raw-only selection is absent from the chart projection', () => {
    const rawOnlyTimestamp = BASE_NS + 900n
    const metric = metricWithDatapoints('Gauge', [
      gaugeDatapoint('projected', BASE_NS + 100n, 10),
    ])
    renderChart(
      MetricTimeSeriesChart,
      {
        timeseries: [
          {
            key: 'series-a',
            label: 'series A',
            points: [
              {
                date: new Date(BASE_MS),
                value: 10,
                timestampNs: BASE_NS + 100n,
                sourceDatapointID: 'projected',
              },
            ],
          },
        ],
        colorByKey: new Map([['series-a', '#123456']]),
        highlightedTimestamp: rawOnlyTimestamp,
        highlightedPointID: 'raw-only',
        selectedSeriesKey: 'series-a',
      },
      metric
    )

    expect(document.querySelector('.highlight-rule')).not.toBeNull()
    expect(document.querySelector('.selection-dot')).toBeNull()
    expect(
      document.querySelector('.metric-time-series-chart__selection-legend')
    ).toBeNull()
  })
})

describe('MetricQuantileAreaChart keyboard model', () => {
  it('keeps same-millisecond quantile columns distinct and activates exact nanoseconds', async () => {
    const firstNs = BASE_NS + 100n
    const secondNs = BASE_NS + 900n
    const metric = metricWithDatapoints('Histogram', [
      histogramDatapoint('hist', [], [2]),
    ])
    const timeseries: ChartTimeseries[] = [
      {
        key: quantileSeriesKey('series-a', '0.95'),
        label: 'series A · p95',
        points: [
          { date: new Date(BASE_MS), value: 1, timestampNs: firstNs },
          { date: new Date(BASE_MS), value: 2, timestampNs: secondNs },
        ],
      },
    ]
    const onChartPointClick = vi.fn()
    let context: MetricViewContext | undefined
    renderChart(
      MetricQuantileAreaChart,
      {
        timeseries,
        activeQuantileKeys: ['0.95'],
        colorByKey: new Map([['series-a', '#123456']]),
        onChartPointClick,
      },
      metric,
      next => (context = next)
    )
    if (!context) throw new Error('metric context was not captured')
    context.onQuantileChartPointClick('series-a', secondNs, '0.95')
    await tick()
    const secondSelectionY = document
      .querySelector('.metric-quantile-area-chart .selection-dot')
      ?.getAttribute('cy')
    const surface = screen.getByRole('application', {
      name: 'Histogram quantile chart',
    })

    expect(document.querySelector('.chart-keyboard-cursor')).toBeNull()
    await fireEvent.focus(surface)
    await tick()
    expect(document.querySelector('.chart-keyboard-cursor')).not.toBeNull()
    expect(readout()).toContain('Point 2 of 2')
    await fireEvent.keyDown(surface, { key: 'Enter' })
    expect(onChartPointClick).toHaveBeenCalledWith('series-a', secondNs, '0.95')

    context.onQuantileChartPointClick('series-a', firstNs, '0.95')
    await tick()
    expect(readout()).toContain('Point 1 of 2')
    const firstSelectionY = document
      .querySelector('.metric-quantile-area-chart .selection-dot')
      ?.getAttribute('cy')
    expect(secondSelectionY).toBeTruthy()
    expect(firstSelectionY).toBeTruthy()
    expect(firstSelectionY).not.toBe(secondSelectionY)
    await fireEvent.blur(surface)
    await tick()
    expect(document.querySelector('.chart-keyboard-cursor')).toBeNull()
  })
})

describe('HistogramChart keyboard model', () => {
  it('keeps duplicate display ranges distinct and describes an empty-bound catch-all', async () => {
    const duplicateBounds = histogramDatapoint(
      'duplicate-bounds',
      [1, 1, 1],
      [1, 2, 3, 4]
    )
    const metric = metricWithDatapoints('Histogram', [duplicateBounds])
    const { unmount } = renderChart(
      HistogramChart,
      { datapoint: duplicateBounds, enableValueBucketPin: true },
      metric
    )
    const surface = screen.getByRole('application', {
      name: 'Histogram distribution chart',
    })

    expect(surface).toHaveAttribute(
      'aria-keyshortcuts',
      'ArrowLeft ArrowRight Home End Enter Space Escape'
    )
    expect(document.querySelector('.chart-keyboard-cursor-band')).toBeNull()
    await fireEvent.focus(surface)
    await tick()
    expect(document.querySelector('.chart-keyboard-cursor-band')).not.toBeNull()
    await fireEvent.keyDown(surface, { key: 'End' })
    expect(readout()).toContain('Bucket 4 of 4')
    await fireEvent.keyDown(surface, { key: 'ArrowLeft' })
    expect(readout()).toContain('Bucket 3 of 4')
    await fireEvent.keyDown(surface, { key: 'Enter' })
    expect(
      document.querySelector('.metric-histogram-bar-chart__value-pin-legend')
    ).toHaveTextContent(/count\s*3/)
    unmount()

    const catchAll = histogramDatapoint('catch-all', [], [7])
    renderChart(
      HistogramChart,
      { datapoint: catchAll },
      metricWithDatapoints('Histogram', [catchAll])
    )
    const snapshot = screen.getByRole('application', {
      name: 'Histogram snapshot distribution chart',
    })
    expect(snapshot.getAttribute('aria-keyshortcuts')).not.toContain('Enter')
    await fireEvent.focus(snapshot)
    expect(readout()).toContain('all values')
  })

  it('syncs a pointer-pinned bucket and does not auto-scroll it before focus', async () => {
    const bounds = Array.from({ length: 20 }, (_, index) => index)
    const datapoint = histogramDatapoint(
      'scrolling-histogram',
      bounds,
      Array.from({ length: 21 }, (_, index) => index + 1)
    )
    const metric = metricWithDatapoints('Histogram', [datapoint])
    renderChart(
      HistogramChart,
      { datapoint, enableValueBucketPin: true },
      metric
    )
    const surface = screen.getByRole('application', {
      name: 'Histogram distribution chart',
    })
    const scroller = document.querySelector(
      '.histogram-chart-scroll'
    ) as HTMLElement
    const wrapper = document.querySelector(
      '.histogram-chart-wrapper'
    ) as HTMLElement
    const root = wrapper.querySelector('.lc-root-container') as HTMLElement
    root.getBoundingClientRect = () =>
      ({
        x: 0,
        y: 0,
        left: 0,
        top: 0,
        right: 630,
        bottom: 320,
        width: 630,
        height: 320,
        toJSON: () => ({}),
      }) as DOMRect
    Object.defineProperty(scroller, 'clientWidth', {
      configurable: true,
      value: 80,
    })
    scroller.scrollLeft = 17

    await fireEvent.click(wrapper, { clientX: 160, clientY: 100 })
    await tick()
    expect(scroller.scrollLeft).toBe(17)
    expect(readout()).toMatch(/Bucket (19|20|21) of 21/)

    await fireEvent.focus(surface)
    await tick()
    expect(scroller.scrollLeft).toBeGreaterThan(17)
  })

  it('describes exponential zero buckets as threshold ranges or exact zero', async () => {
    const metric = metricWithDatapoints('Histogram', [
      histogramDatapoint('context', [], [7]),
    ])
    const threshold = exponentialHistogramDatapoint('threshold', 0.001)
    const { unmount } = renderChart(
      HistogramChart,
      { datapoint: threshold },
      metric
    )
    const rangedSurface = screen.getByRole('application', {
      name: 'Histogram snapshot distribution chart',
    })

    await fireEvent.focus(rangedSurface)
    expect(readout()).toContain('[-1.0e-3, +1.0e-3]')
    unmount()

    const exact = exponentialHistogramDatapoint('exact-zero', 0)
    renderChart(HistogramChart, { datapoint: exact }, metric)
    const exactSurface = screen.getByRole('application', {
      name: 'Histogram snapshot distribution chart',
    })
    await fireEvent.focus(exactSurface)
    expect(readout()).toContain('= 0')
    expect(readout()).not.toContain('[-')
  })

  it('clears a chart-owned histogram cell without erasing a coexisting detail datapoint', async () => {
    const datapoint = histogramDatapoint('raw-histogram', [], [7])
    const metric = metricWithDatapoints('Histogram', [datapoint])
    let context: MetricViewContext | undefined
    renderChart(HistogramChart, { datapoint }, metric, next => (context = next))
    if (!context) throw new Error('metric context was not captured')
    context.onDatapointClick(datapoint)
    context.onHeatmapSelect(BASE_NS + 500n)
    expect(context.selectionSource).toBe('chart')
    context.onHeatmapSelect(BASE_NS + 500n)
    expect(context.selectionSource).toBe('detail')
    expect(context.selectedDatapointID).toBe(datapoint.id)
    context.onQuantileChartPointClick('series-a', BASE_NS + 600n, '0.95')
    expect(context.selectionSource).toBe('chart')
    context.onQuantileChartPointClick('series-a', BASE_NS + 600n, '0.95')
    expect(context.selectionSource).toBe('detail')
    expect(context.selectedDatapointID).toBe(datapoint.id)
    context.onHeatmapSelect(BASE_NS + 500n)
    await tick()

    const surface = screen.getByRole('application', {
      name: 'Histogram snapshot distribution chart',
    })
    await fireEvent.focus(surface)
    await fireEvent.keyDown(surface, { key: 'Escape' })
    await tick()

    expect(context.selectedDatapointID).toBe(datapoint.id)
    expect(context.heatmapColumnStartNs).toBeNull()
    expect(context.selectionSource).toBe('detail')
  })
})

describe('HistogramHeatmap keyboard model', () => {
  it('keeps formatted row labels and sub-millisecond columns distinct, syncs selection on focus, and scrolls only while focused', async () => {
    const columns: HistogramSlicePoint[] = Array.from(
      { length: 24 },
      (_, index) => ({
        kind: 'histogram' as const,
        timestamp:
          BASE_NS +
          BigInt(index) * 1_000_000n +
          BigInt(index === 1 ? 900 : 100),
        attributesKey: '',
        bounds: [1.001, 1.002, 1.003],
        counts: [index, index + 1, index + 2, index + 3],
        totals: { count: 1, sum: 0, min: 0, max: 1 },
      })
    )
    // The first two timestamps deliberately share one Date millisecond.
    columns[1]!.timestamp = BASE_NS + 900n
    const onSelect = vi.fn()
    const props = {
      points: columns,
      selectedTimestamp: columns[0]!.timestamp,
      onSelect,
      height: 320,
    }
    const metric = metricWithDatapoints('Histogram', [
      histogramDatapoint('hist', [1.001, 1.002, 1.003], [1, 2, 3, 4]),
    ])
    const { rerender } = renderChart(HistogramHeatmap, props, metric)
    const surface = screen.getByRole('application', {
      name: 'Histogram heatmap',
    })
    const scroller = document.querySelector('.heatmap-scroll') as HTMLElement
    scroller.scrollLeft = 17
    expect(document.querySelector('.chart-keyboard-cursor-cell')).toBeNull()

    await rerender({
      component: HistogramHeatmap,
      componentProps: {
        ...props,
        selectedTimestamp: columns[columns.length - 1]!.timestamp,
      },
      metric,
    })
    await tick()
    expect(scroller.scrollLeft).toBe(17)
    expect(readout()).toContain('Column 24 of 24')

    await fireEvent.focus(surface)
    await tick()
    expect(document.querySelector('.chart-keyboard-cursor-cell')).not.toBeNull()
    expect(scroller.scrollLeft).toBeGreaterThan(17)
    await fireEvent.keyDown(surface, { key: 'Home', ctrlKey: true })
    await fireEvent.keyDown(surface, { key: 'ArrowRight' })
    expect(readout()).toContain('Column 2 of 24')
    await fireEvent.keyDown(surface, { key: 'ArrowDown' })
    expect(readout()).toContain('Row 2 of 4')
    await fireEvent.keyDown(surface, { key: 'Enter' })
    expect(onSelect).toHaveBeenLastCalledWith(BASE_NS + 900n)
    await fireEvent.blur(surface)
    await tick()
    expect(document.querySelector('.chart-keyboard-cursor-cell')).toBeNull()
  })

  it('navigates sparse missing intersections as logical zero cells', async () => {
    const points: HistogramSlicePoint[] = [
      {
        kind: 'histogram',
        timestamp: BASE_NS + 100n,
        attributesKey: '',
        bounds: [1.001, 1.002, 1.003],
        counts: [5, 0, 0, 0],
        totals: { count: 5, sum: 0, min: 0, max: 1 },
      },
      {
        kind: 'histogram',
        timestamp: BASE_NS + 900n,
        attributesKey: '',
        bounds: [1.001, 1.002, 1.003],
        counts: [0, 0, 0, 7],
        totals: { count: 7, sum: 0, min: 1, max: 2 },
      },
    ]
    const onSelect = vi.fn()
    const metric = metricWithDatapoints('Histogram', [
      histogramDatapoint('sparse', [1.001, 1.002, 1.003], [5, 0, 0, 0]),
    ])
    renderChart(
      HistogramHeatmap,
      {
        points,
        selectedTimestamp: points[0]!.timestamp,
        onSelect,
      },
      metric
    )
    const surface = screen.getByRole('application', {
      name: 'Histogram heatmap',
    })

    await fireEvent.focus(surface)
    expect(readout()).toContain('Row 1 of 4, bucket >1.00. Count 0.')
    await fireEvent.keyDown(surface, { key: 'ArrowDown' })
    expect(readout()).toContain('Row 2 of 4, bucket (1.00, 1.00]. Count 0.')
    await fireEvent.keyDown(surface, { key: 'ArrowDown' })
    expect(readout()).toContain('Row 3 of 4, bucket (1.00, 1.00]. Count 0.')
    await fireEvent.keyDown(surface, { key: 'ArrowRight' })
    await fireEvent.keyDown(surface, { key: 'Home' })
    expect(readout()).toContain('Column 1 of 2')
    await fireEvent.keyDown(surface, { key: 'End' })
    expect(readout()).toContain('Column 2 of 2')
    await fireEvent.keyDown(surface, { key: 'Enter' })
    expect(onSelect).toHaveBeenLastCalledWith(points[1]!.timestamp)
  })

  it('keeps all-zero histogram structure keyboard navigable', async () => {
    const point: HistogramSlicePoint = {
      kind: 'histogram',
      timestamp: BASE_NS,
      attributesKey: '',
      bounds: [1, 2],
      counts: [0, 0, 0],
      totals: { count: 0, sum: 0, min: 0, max: 0 },
    }
    const metric = metricWithDatapoints('Histogram', [
      histogramDatapoint('all-zero', [1, 2], [0, 0, 0]),
    ])
    renderChart(HistogramHeatmap, { points: [point] }, metric)
    const surface = screen.getByRole('application', {
      name: 'Histogram heatmap',
    })

    expect(screen.queryByText('No bucket data in range')).toBeNull()
    await fireEvent.focus(surface)
    expect(readout()).toContain('Column 1 of 1')
    expect(readout()).toContain('Count 0')
  })

  it('describes an explicit histogram without bounds as one catch-all row', async () => {
    const point: HistogramSlicePoint = {
      kind: 'histogram',
      timestamp: BASE_NS,
      attributesKey: '',
      bounds: [],
      counts: [9],
      totals: { count: 9, sum: 0, min: 0, max: 0 },
    }
    const metric = metricWithDatapoints('Histogram', [
      histogramDatapoint('catch-all', [], [9]),
    ])
    renderChart(HistogramHeatmap, { points: [point] }, metric)
    const surface = screen.getByRole('application', {
      name: 'Histogram heatmap',
    })

    await fireEvent.focus(surface)
    expect(readout()).toContain('Row 1 of 1, bucket all values')
  })
})
