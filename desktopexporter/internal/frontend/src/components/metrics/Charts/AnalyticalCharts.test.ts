// @vitest-environment jsdom
import { fireEvent, render, screen } from '@testing-library/svelte'
import { tick, type Component } from 'svelte'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import HistogramChart from './HistogramChart.svelte'
import { chartPadding } from './MetricChartPlot.svelte'
import HistogramHeatmap from './HistogramHeatmap.svelte'
import MetricTimeSeriesChart from './MetricTimeSeriesChart.svelte'
import MetricChartHarness from '@/test/MetricChartHarness.svelte'
import type { HistogramSlicePoint } from '@/components/metrics/utils/histogram-aggregation'
import { histogramDatapointToChartDatapoint } from '@/components/metrics/utils/histogram-aggregation'
import type { ChartTimeseries } from '@/types/metric-chart-types'
import type {
  ExponentialHistogramDataPoint,
  GaugeDataPoint,
  HistogramDataPoint,
  MetricViewData,
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

function queryHTMLElement(
  selector: string,
  root: ParentNode = document
): HTMLElement {
  const element = root.querySelector(selector)
  if (!(element instanceof HTMLElement)) {
    throw new Error(`Expected HTMLElement matching ${selector}`)
  }
  return element
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
    count: bucketCounts.reduce((sum, count) => sum + BigInt(count), 0n),
    sum: 0,
    min: 0,
    max: 0,
    bucketCounts: bucketCounts.map(BigInt),
    explicitBounds,
    aggregationTemporalityCode: 1,
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
    count: 7n,
    sum: 0,
    min: -zeroThreshold,
    max: zeroThreshold,
    scale: 1,
    zeroCount: 7n,
    zeroThreshold,
    positiveBucketOffset: 0,
    positiveBucketCounts: [],
    negativeBucketOffset: 0,
    negativeBucketCounts: [],
    aggregationTemporalityCode: 1,
    aggregationTemporality: 'Delta',
    quantiles: null,
  }
}

function metricWithDatapoints(
  metricType: 'Gauge' | 'Histogram',
  datapoints: GaugeDataPoint[] | HistogramDataPoint[]
): MetricViewData {
  return {
    metricRef: 'metric-1',
    name: 'test.metric',
    description: 'test metric',
    metadata: [],
    unit: 'ms',
    metricType,
    aggregationTemporality: metricType === 'Gauge' ? null : 'Delta',
    aggregationTemporalityCode: metricType === 'Gauge' ? null : 1,
    isMonotonic: null,
    resourceDroppedAttributesCount: 0,
    resourceSchemaUrl: '',
    resource: EMPTY_RESOURCE,
    scopeName: EMPTY_SCOPE.name,
    scopeVersion: EMPTY_SCOPE.version,
    scopeSchemaUrl: '',
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
        seriesRef: 'series-a',
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
  metric: MetricViewData,
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
  it.each([
    { values: [2, 8], ruleCount: 0 },
    { values: [-2, 8], ruleCount: 1 },
    { values: [-8, -2], ruleCount: 1 },
  ])(
    'keeps only zero rules that do not duplicate the rounded bottom axis: $values',
    async ({ values, ruleCount }) => {
      const datapoints = values.map((value, index) =>
        gaugeDatapoint(
          `point-${index}`,
          BASE_NS + BigInt(index) * 1_000_000_000n,
          value
        )
      )
      renderChart(
        MetricTimeSeriesChart,
        {
          timeseries: [
            {
              key: 'series-a',
              label: 'Series A',
              points: datapoints.map(point => ({
                date: new Date(point.timestampMs),
                value: point.doubleValue,
                timestampNs: point.timestamp,
              })),
            },
          ],
          colorByKey: new Map([['series-a', '#123456']]),
        },
        metricWithDatapoints('Gauge', datapoints)
      )
      await tick()
      expect(document.querySelectorAll('.lc-rule-y-line')).toHaveLength(
        ruleCount
      )
      const zeroLabels = [
        ...document.querySelectorAll('.lc-axis-tick-label'),
      ].filter(label => label.textContent === '0')
      expect(zeroLabels).toHaveLength(ruleCount)
      expect(document.querySelector('.metric-axes')).toBeInTheDocument()
    }
  )

  it('draws aggregate buckets as step-after geometry while preserving raw sample interpolation', async () => {
    const datapoints = [2, 8, 4].map((value, index) =>
      gaugeDatapoint(
        `point-${index}`,
        BASE_NS + BigInt(index) * 1_000_000_000n,
        value
      )
    )
    const keys = [
      'series-a',
      '__agg:selected__',
      '__agg:all__',
      '__agg:total__',
    ]
    renderChart(
      MetricTimeSeriesChart,
      {
        timeseries: keys.map(key => ({
          key,
          label: key,
          points: datapoints.map(point => ({
            date: new Date(point.timestampMs),
            value: point.doubleValue,
            timestampNs: point.timestamp,
          })),
        })),
        colorByKey: new Map(keys.map(key => [key, '#123456'])),
      },
      metricWithDatapoints('Gauge', datapoints)
    )
    await tick()

    for (const key of keys) {
      const path = document.querySelector(`path[data-series-key="${key}"]`)!
      const d = path.getAttribute('d')!
      expect(d).toBeTruthy()
      if (key === 'series-a') {
        const samples = [...d.matchAll(/[ML](-?[\d.]+),(-?[\d.]+)/g)]
        expect(samples).toHaveLength(3)
        expect(samples[1]![1]).not.toBe(samples[0]![1])
        expect(samples[1]![2]).not.toBe(samples[0]![2])
        continue
      }
      expect(path).toHaveAttribute('stroke-dasharray', '6 4')
      expect(d).not.toMatch(/[CQ]/)
      const vertices = [...d.matchAll(/[ML](-?[\d.]+),(-?[\d.]+)/g)].map(
        match => [Number(match[1]), Number(match[2])]
      )
      expect(vertices).toHaveLength(5)
      for (let i = 1; i < vertices.length; i++) {
        const previous = vertices[i - 1]!
        const current = vertices[i]!
        // Hold the previous bucket value until the next bucket's X, then jump.
        expect(current[i % 2 === 1 ? 1 : 0]).toBe(previous[i % 2 === 1 ? 1 : 0])
        expect(current[i % 2 === 1 ? 0 : 1]).not.toBe(
          previous[i % 2 === 1 ? 0 : 1]
        )
      }
    }
  })

  it.each(['raw', 'rate'] as const)(
    'uses the selected series colour for min/max dots in %s, but the primary tone for derived annotations',
    async aggregationView => {
      const first = gaugeDatapoint('first', BASE_NS, 2)
      const second = gaugeDatapoint('second', BASE_NS + 1_000_000_000n, 4)
      const metric = metricWithDatapoints('Gauge', [first, second])
      const props = {
        timeseries: ['series-a', 'series-b'].map(key => ({
          key,
          label: key,
          points: [
            {
              date: new Date(BASE_MS),
              timestampNs: first.timestamp,
              sourceDatapointID: first.id,
              value: 2,
            },
            {
              date: new Date(BASE_MS + 1000),
              timestampNs: second.timestamp,
              sourceDatapointID: second.id,
              value: 4,
              slope: 2,
            },
          ],
        })),
        colorByKey: new Map([
          ['series-a', '#123456'],
          ['series-b', '#abcdef'],
        ]),
        highlightedTimestamp: second.timestamp,
        highlightedPointID: second.id,
        selectedSeriesKey: 'series-a',
        aggregationView,
        seriesStats: new Map(
          ['series-a', 'series-b'].map(key => [key, { min: 2, max: 4, avg: 3 }])
        ),
      }
      const view = renderChart(MetricTimeSeriesChart, props, metric)
      function expectMarkerColours(seriesColour: string) {
        const markers = [...document.querySelectorAll('.series-stat-marker')]
        for (const kind of ['min', 'max']) {
          const marker = markers.find(marker =>
            marker.querySelector('title')?.textContent?.startsWith(`${kind} `)
          )
          expect(marker?.querySelector('.series-stat-dot')).toHaveAttribute(
            'fill',
            seriesColour
          )
        }
        const derived = markers.find(marker =>
          marker
            .querySelector('title')
            ?.textContent?.startsWith(
              aggregationView === 'rate' ? 'rate slope ' : 'avg '
            )
        )
        if (aggregationView === 'rate') {
          expect(derived?.querySelector('.series-stat-dot')).toHaveAttribute(
            'fill',
            'var(--color-primary)'
          )
          expect(document.querySelector('.series-average-marker')).toBeNull()
        } else {
          expect(derived?.querySelector('.series-stat-dot')).toBeNull()
          const averageLabel = document.querySelector(
            '.series-stat-tooltip--right'
          )
          expect(
            averageLabel?.querySelector('.chart-selection-legend__label')
          ).toHaveTextContent('avg')
          expect(averageLabel).toHaveAttribute('title', 'avg 3')
          const line = derived?.querySelector('.series-average-line')
          expect(line).not.toBeNull()
          expect(line?.getAttribute('y1')).toBe(line?.getAttribute('y2'))
          expect(line?.getAttribute('x1')).not.toBe(line?.getAttribute('x2'))
          expect(
            document.querySelectorAll('.series-stat-tooltip')
          ).toHaveLength(3)
        }
      }
      expectMarkerColours('#123456')
      await view.rerender({
        component: MetricTimeSeriesChart,
        componentProps: { ...props, selectedSeriesKey: 'series-b' },
        metric,
      })
      expectMarkerColours('#abcdef')
    }
  )

  it('shows window statistics without a point selection and requires a series choice when several are visible', async () => {
    const datapoints = [2, 4].map((value, index) =>
      gaugeDatapoint(
        `point-${index}`,
        BASE_NS + BigInt(index) * 1_000_000_000n,
        value
      )
    )
    const metric = metricWithDatapoints('Gauge', datapoints)
    const series = {
      key: 'series-a',
      label: 'Series A',
      points: datapoints.map(point => ({
        date: new Date(point.timestampMs),
        value: point.doubleValue,
        timestampNs: point.timestamp,
      })),
    }
    const props = {
      timeseries: [series],
      colorByKey: new Map([
        ['series-a', '#123456'],
        ['series-b', '#abcdef'],
      ]),
      seriesStats: new Map([['series-a', { min: 2, max: 4, avg: 3 }]]),
    }
    const view = renderChart(MetricTimeSeriesChart, props, metric)
    await tick()
    expect(document.querySelectorAll('.series-stat-tooltip')).toHaveLength(3)
    await view.rerender({
      component: MetricTimeSeriesChart,
      componentProps: { ...props, showStatOverlays: false },
      metric,
    })
    expect(document.querySelectorAll('.series-stat-tooltip')).toHaveLength(0)
    const multiple = {
      ...props,
      timeseries: [series, { ...series, key: 'series-b' }],
    }
    await view.rerender({
      component: MetricTimeSeriesChart,
      componentProps: multiple,
      metric,
    })
    expect(document.querySelectorAll('.series-stat-tooltip')).toHaveLength(0)
    await view.rerender({
      component: MetricTimeSeriesChart,
      componentProps: { ...multiple, selectedSeriesKey: 'series-a' },
      metric,
    })
    expect(document.querySelectorAll('.series-stat-tooltip')).toHaveLength(3)
  })

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
      document.querySelector('.series-stat-tooltip--right')
    ).toHaveTextContent('slope')
    expect(
      [...document.querySelectorAll('.series-stat-tooltip')].some(tooltip =>
        tooltip.textContent?.includes('slope')
      )
    ).toBe(true)
    expect(
      [...document.querySelectorAll('.series-stat-marker title')].some(title =>
        title.textContent?.startsWith('rate slope')
      )
    ).toBe(true)
    expect(
      document.querySelector('.selection-overlay .highlight-rule')
    ).toBeNull()
    expect(document.querySelector('.series-stat-hitbox')).toBeNull()
    expect(
      document
        .querySelector(
          '.series-stat-tooltip--right .chart-selection-legend__dot'
        )
        ?.getAttribute('style')
    ).toContain('--color: var(--color-primary)')
  })

  it('activates exact Rate buckets with keyboard without fabricating raw identities', async () => {
    const onRateChartPointClick = vi.fn()
    const onChartPointClick = vi.fn()
    const metric = metricWithDatapoints('Gauge', [
      gaugeDatapoint('source', BASE_NS, 10),
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
                timestampNs: BASE_NS + 300n,
                value: 2,
              },
              {
                date: new Date(BASE_MS),
                timestampNs: BASE_NS + 600n,
                value: 4,
                slope: 6,
              },
            ],
          },
        ],
        aggregationView: 'rate',
        onRateChartPointClick,
        onChartPointClick,
        colorByKey: new Map([['series-a', '#123456']]),
      },
      metric
    )
    const surface = screen.getByRole('application', {
      name: 'Metric time series chart',
    })
    await fireEvent.focus(surface)
    await fireEvent.keyDown(surface, { key: 'Home' })
    await fireEvent.keyDown(surface, { key: 'ArrowRight' })
    await fireEvent.keyDown(surface, { key: 'Enter' })
    expect(onRateChartPointClick).toHaveBeenCalledWith(
      'series-a',
      BASE_NS + 600n
    )
    expect(onChartPointClick).not.toHaveBeenCalled()
    expect(readout()).not.toContain('Read-only synthetic point')
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

  it('keeps selected and all aggregate summaries in chart-series order', () => {
    const metric = metricWithDatapoints('Gauge', [
      gaugeDatapoint('source', BASE_NS, 10),
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
                timestampNs: BASE_NS,
                sourceDatapointID: 'source',
              },
            ],
          },
          {
            key: '__agg:all__',
            label: 'All',
            points: [
              { date: new Date(BASE_MS), value: 14, timestampNs: BASE_NS },
            ],
          },
          {
            key: '__agg:selected__',
            label: 'Selected',
            points: [
              { date: new Date(BASE_MS), value: 12, timestampNs: BASE_NS },
            ],
          },
        ],
        colorByKey: new Map([
          ['series-a', '#123456'],
          ['__agg:selected__', '#234567'],
          ['__agg:all__', '#345678'],
        ]),
        highlightedTimestamp: BASE_NS,
        highlightedPointID: 'source',
        selectedSeriesKey: 'series-a',
      },
      metric
    )

    expect(
      [...document.querySelectorAll('.chart-aggregate-summary__label')].map(
        label => label.textContent
      )
    ).toEqual(['all series:', 'selected series:'])
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

    expect(document.querySelector('.highlight-rule')).toBeNull()
    expect(document.querySelector('.selection-dot')).toBeNull()
    expect(
      document.querySelector('.metric-time-series-chart__selection-legend')
    ).toBeNull()
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
      {
        datapoint: histogramDatapointToChartDatapoint(duplicateBounds),
      },
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
      { datapoint: histogramDatapointToChartDatapoint(catchAll) },
      metricWithDatapoints('Histogram', [catchAll])
    )
    const catchAllSurface = screen.getByRole('application', {
      name: 'Histogram distribution chart',
    })
    expect(catchAllSurface.getAttribute('aria-keyshortcuts')).toContain('Enter')
    await fireEvent.focus(catchAllSurface)
    expect(readout()).toContain('all values')
  })

  it('syncs a pointer-pinned bucket and does not auto-scroll it before focus', async () => {
    clientWidthSpy!.mockReturnValue(630)
    const bounds = Array.from({ length: 20 }, (_, index) => index)
    const datapoint = histogramDatapoint(
      'scrolling-histogram',
      bounds,
      Array.from({ length: 21 }, (_, index) => index + 1)
    )
    const metric = metricWithDatapoints('Histogram', [datapoint])
    renderChart(
      HistogramChart,
      {
        datapoint: histogramDatapointToChartDatapoint(datapoint),
      },
      metric
    )
    const surface = screen.getByRole('application', {
      name: 'Histogram distribution chart',
    })
    const scroller = queryHTMLElement('.histogram-chart-scroll')
    const wrapper = queryHTMLElement('.histogram-chart-wrapper')
    const root = queryHTMLElement('.lc-root-container', wrapper)
    root.getBoundingClientRect = () => new DOMRect(0, 0, 630, 320)
    Object.defineProperty(scroller, 'clientWidth', {
      configurable: true,
      value: 80,
    })
    scroller.scrollLeft = 17

    const selectedBar = wrapper.querySelectorAll('.histogram-bar__fill')[19]!
    const clientX =
      chartPadding.left +
      Number(selectedBar.getAttribute('x')) +
      Number(selectedBar.getAttribute('width')) / 2
    const clientY =
      chartPadding.top +
      Number(selectedBar.getAttribute('y')) +
      Number(selectedBar.getAttribute('height')) / 2
    await fireEvent.click(wrapper, { clientX, clientY })
    await tick()
    expect(scroller.scrollLeft).toBe(17)
    expect(readout()).toContain('Bucket 20 of 21, 18 – 19. Count 20.')

    await fireEvent.focus(surface)
    await tick()
    expect(readout()).toContain('Bucket 20 of 21, 18 – 19. Count 20.')
    expect(scroller.scrollLeft).toBeGreaterThan(17)
  })

  it('describes exponential zero buckets as threshold ranges or exact zero', async () => {
    const metric = metricWithDatapoints('Histogram', [
      histogramDatapoint('context', [], [7]),
    ])
    const threshold = exponentialHistogramDatapoint('threshold', 0.001)
    const { unmount } = renderChart(
      HistogramChart,
      { datapoint: histogramDatapointToChartDatapoint(threshold) },
      metric
    )
    const rangedSurface = screen.getByRole('application', {
      name: 'Histogram distribution chart',
    })

    await fireEvent.focus(rangedSurface)
    expect(readout()).toContain('[-0.001, +0.001]')
    unmount()

    const exact = exponentialHistogramDatapoint('exact-zero', 0)
    renderChart(
      HistogramChart,
      { datapoint: histogramDatapointToChartDatapoint(exact) },
      metric
    )
    const exactSurface = screen.getByRole('application', {
      name: 'Histogram distribution chart',
    })
    await fireEvent.focus(exactSurface)
    expect(readout()).toContain('= 0')
    expect(readout()).not.toContain('[-')
  })

  it('clears a chart-owned histogram cell without erasing a coexisting detail datapoint', async () => {
    const datapoint = histogramDatapoint('raw-histogram', [], [7])
    const metric = metricWithDatapoints('Histogram', [datapoint])
    let context: MetricViewContext | undefined
    renderChart(
      HistogramChart,
      { datapoint: histogramDatapointToChartDatapoint(datapoint) },
      metric,
      next => (context = next)
    )
    if (!context) throw new Error('metric context was not captured')
    context.onDatapointClick(datapoint)
    expect(context.activeHistogramTab).toBe('heatmap')
    // A detail selection cannot substitute its raw snapshot for the window summary.
    expect(context.histogramChartDatapoint).toBeUndefined()
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
      name: 'Histogram distribution chart',
    })
    await fireEvent.focus(surface)
    await fireEvent.keyDown(surface, { key: 'Escape' })
    await tick()

    expect(context.selectedDatapointID).toBe(datapoint.id)
    expect(context.heatmapColumnStartNs).toBeNull()
    expect(context.selectionSource).toBe('detail')
  })
})

describe('histogram source contributions', () => {
  it.each(['heatmap', 'histogram'] as const)(
    'highlights activity on the existing %s and clears on repeat selection',
    async chart => {
      const source = histogramDatapoint('source', [1, 2], [40, 30, 20])
      const aggregate = histogramDatapoint('aggregate', [1, 2], [10, 12, 8])
      aggregate.quantiles = { '0.5': 1.5 }
      const metric = metricWithDatapoints('Histogram', [source])
      const point: HistogramSlicePoint = {
        kind: 'histogram',
        timestamp: BASE_NS,
        seriesRef: '',
        bounds: [1, 2],
        counts: [2, 0, 3],
        quantiles: { '0.5': 1.5 },
        totals: { count: 5, sum: undefined, min: undefined, max: undefined },
      }
      let ctx!: MetricViewContext
      render(MetricChartHarness, {
        props: {
          component: chart === 'heatmap' ? HistogramHeatmap : HistogramChart,
          componentProps:
            chart === 'heatmap'
              ? {
                  points: [
                    {
                      ...point,
                      counts: [10, 12, 8],
                      totals: { ...point.totals, count: 30 },
                    },
                  ],
                }
              : { datapoint: histogramDatapointToChartDatapoint(aggregate) },
          metric,
          contribution: { datapointID: source.id, point },
          oncontext: context => {
            ctx = context
          },
        },
      })
      ctx.onDatapointClick(source)
      await tick()
      expect(
        [...document.querySelectorAll('[data-contribution-count]')].map(e =>
          e.getAttribute('data-contribution-count')
        )
      ).toEqual(['2', '3'])
      expect(
        screen.queryByRole('status', { name: 'Datapoint contribution' })
      ).toBeNull()
      const contributionLayer = document.querySelector(
        chart === 'heatmap'
          ? '.heatmap-contributions'
          : '.histogram-contributions'
      )!
      const quantile = document.querySelector(
        chart === 'heatmap' ? '.heatmap-quantile-line' : '.quantile-marker'
      )!
      expect(quantile).not.toBeNull()
      // Heatmap quantiles cross filled cells and must paint above contribution
      // textures (also covered by the browser intersection test). Histogram
      // contributions retain their existing layer above vertical quantile guides.
      expect(
        quantile.compareDocumentPosition(contributionLayer) &
          (chart === 'heatmap'
            ? Node.DOCUMENT_POSITION_PRECEDING
            : Node.DOCUMENT_POSITION_FOLLOWING)
      ).not.toBe(0)
      const textures = contributionLayer.querySelectorAll('[fill^="url(#"]')
      expect(textures.length).toBeGreaterThan(0)
      for (const texture of textures) {
        const id = texture.getAttribute('fill')!.slice(5, -1)
        expect(document.getElementById(id)?.tagName.toLowerCase()).toBe(
          'pattern'
        )
      }
      expect(ctx.selectedDatapoint).toMatchObject({ count: 90n })
      ctx.onDatapointClick(source)
      await tick()
      expect(
        document.querySelectorAll('[data-contribution-count]')
      ).toHaveLength(0)
      expect(screen.queryByText('Selected contribution:')).toBeNull()
      expect(ctx.selectedDatapointID).toBeNull()
    }
  )

  it('does not display stale or unchecked source contributions', async () => {
    const source = histogramDatapoint('source', [], [5])
    const other = histogramDatapoint('other', [], [8])
    let ctx!: MetricViewContext
    render(MetricChartHarness, {
      props: {
        component: HistogramHeatmap,
        componentProps: { points: [] },
        metric: metricWithDatapoints('Histogram', [source, other]),
        contribution: { datapointID: source.id, point: null },
        oncontext: context => {
          ctx = context
        },
      },
    })
    ctx.onDatapointClick(source)
    expect(ctx.histogramContribution?.point).toBeNull()
    ctx.onDatapointClick(other)
    expect(ctx.histogramContribution).toBeNull()
    ctx.onDatapointClick(source)
    ctx.toggleTimeseriesVisible('series-a', false)
    expect(ctx.histogramContribution).toBeNull()
  })
})

describe('HistogramHeatmap keyboard model', () => {
  it('formats explicit string-domain ticks as time and bucket labels', async () => {
    localStorage.setItem('time-tz', 'UTC')
    const point: HistogramSlicePoint = {
      kind: 'histogram',
      timestamp: BASE_NS,
      seriesRef: '',
      bounds: [1, 2],
      counts: [1, 2, 3],
      totals: { count: 6, sum: 0, min: 0, max: 2 },
    }
    const metric = metricWithDatapoints('Histogram', [
      histogramDatapoint('labels', [1, 2], [1, 2, 3]),
    ])
    renderChart(HistogramHeatmap, { points: [point] }, metric)
    await tick()

    const labels = Array.from(
      document.querySelectorAll('.lc-axis-tick-label'),
      label => label.textContent
    )
    expect(labels).toEqual(
      expect.arrayContaining(['22:13:20', '>2', '(1, 2]', '≤1'])
    )
  })

  it('keeps formatted row labels and sub-millisecond columns distinct, syncs selection on focus, and scrolls only while focused', async () => {
    const columns: HistogramSlicePoint[] = Array.from(
      { length: 24 },
      (_, index) => ({
        kind: 'histogram' as const,
        timestamp:
          BASE_NS +
          BigInt(index) * 1_000_000n +
          BigInt(index === 1 ? 900 : 100),
        seriesRef: '',
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
    const scroller = queryHTMLElement('.heatmap-scroll')
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
        seriesRef: '',
        bounds: [1.001, 1.002, 1.003],
        counts: [5, 0, 0, 0],
        totals: { count: 5, sum: 0, min: 0, max: 1 },
      },
      {
        kind: 'histogram',
        timestamp: BASE_NS + 900n,
        seriesRef: '',
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
    expect(readout()).toContain('Row 1 of 4, bucket >1.003. Count 0.')
    await fireEvent.keyDown(surface, { key: 'ArrowDown' })
    expect(readout()).toContain('Row 2 of 4, bucket (1.002, 1.003]. Count 0.')
    await fireEvent.keyDown(surface, { key: 'ArrowDown' })
    expect(readout()).toContain('Row 3 of 4, bucket (1.001, 1.002]. Count 0.')
    await fireEvent.keyDown(surface, { key: 'ArrowDown' })
    expect(readout()).toContain('Row 4 of 4, bucket ≤1.001. Count 5.')
    await fireEvent.keyDown(surface, { key: 'ArrowUp' })
    await fireEvent.keyDown(surface, { key: 'ArrowRight' })
    expect(readout()).toContain('Row 3 of 4, bucket (1.001, 1.002]. Count 0.')
    await fireEvent.keyDown(surface, { key: 'Home' })
    expect(readout()).toContain('Column 1 of 2')
    await fireEvent.keyDown(surface, { key: 'End' })
    expect(readout()).toContain('Column 2 of 2')
    await fireEvent.keyDown(surface, { key: 'ArrowUp' })
    await fireEvent.keyDown(surface, { key: 'ArrowUp' })
    expect(readout()).toContain('Row 1 of 4, bucket >1.003. Count 7.')
    await fireEvent.keyDown(surface, { key: 'Enter' })
    expect(onSelect).toHaveBeenLastCalledWith(points[1]!.timestamp)
  })

  it('keeps all-zero histogram structure keyboard navigable', async () => {
    const point: HistogramSlicePoint = {
      kind: 'histogram',
      timestamp: BASE_NS,
      seriesRef: '',
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
      seriesRef: '',
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
