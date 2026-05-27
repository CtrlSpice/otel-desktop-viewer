<script lang="ts">
  import {
    quantileMergedSelectionLegendRows,
  } from '@/components/metrics/utils/heatmap-column-selection'
  import { formatMetricValue } from '@/components/metrics/utils/format-metric-value'
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import {
    LineChart,
    Line,
    Spline,
    Tooltip,
    type ChartState,
  } from 'layerchart'
  import { bisector } from 'd3-array'
  import { curveStepAfter } from 'd3-shape'
  import { scaleTime } from 'd3-scale'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatDateTime } from '@/utils/time'
  import MetricChartEmpty from '@/components/metrics/Charts/MetricChartEmpty.svelte'
  import ChartSelectionLegend, {
    type SelectionLegendRow,
  } from '@/components/metrics/Charts/ChartSelectionLegend.svelte'
  import ChartTimeRangeHeader from '@/components/metrics/Charts/ChartTimeRangeHeader.svelte'
  import MetricChartPlot, {
    axisTime,
    axisValue,
    chartPadding,
    DEFAULT_METRIC_CHART_HEIGHT,
  } from '@/components/metrics/Charts/MetricChartPlot.svelte'
  import { chartNeutral } from '@/utils/chart-palette'
  import {
    parseQuantileSeriesKey,
    QUANTILE_LABELS,
    quantileLabelForKey,
  } from '@/components/metrics/utils/histogram-aggregation'
  import type { ChartPoint, ChartTimeseries } from '@/types/metric-chart-types'

  type QuantileLineMeta = {
    seriesKey: string
    quantileKey: string
  }

  type QuantileSeriesGroup = {
    seriesKey: string
    color: string
    lines: { quantileKey: string; points: ChartPoint[] }[]
  }

  type Props = {
    timeseries: ChartTimeseries[]
    activeQuantileKeys: readonly string[]
    height?: number
    unit?: string
    colorByKey: ReadonlyMap<string, string>
    timeRange?: { startMs: number; endMs: number } | null
    emptyMessage?: string
    onChartPointClick?: (
      seriesKey: string,
      clickedAt: Date,
      quantileKey: string | null
    ) => void
  }

  let {
    timeseries,
    activeQuantileKeys,
    height = DEFAULT_METRIC_CHART_HEIGHT,
    unit = '',
    colorByKey,
    timeRange = null,
    emptyMessage = 'No quantile data in range',
    onChartPointClick,
  }: Props = $props()

  const timeContext = getTimeContext()
  const ctx = getMetricViewContext()

  let plotAreaHeight = $state(0)
  let lineChartContext = $state<ChartState<ChartPoint> | undefined>(undefined)

  let orderedQuantileKeys = $derived(
    QUANTILE_LABELS.map(q => q.key).filter(k => activeQuantileKeys.includes(k))
  )

  let lineMetaByKey = $derived.by(() => {
    const map = new Map<string, QuantileLineMeta>()
    for (const ts of timeseries) {
      const parsed = parseQuantileSeriesKey(ts.key)
      if (!parsed) continue
      map.set(ts.key, parsed)
    }
    return map
  })

  let quantileGroups = $derived.by((): QuantileSeriesGroup[] => {
    const bySeries = new Map<string, Map<string, ChartPoint[]>>()
    for (const ts of timeseries) {
      const parsed = parseQuantileSeriesKey(ts.key)
      if (!parsed) continue
      let qmap = bySeries.get(parsed.seriesKey)
      if (!qmap) {
        qmap = new Map()
        bySeries.set(parsed.seriesKey, qmap)
      }
      qmap.set(parsed.quantileKey, ts.points)
    }

    return [...bySeries.entries()]
      .map(([seriesKey, qmap]) => ({
        seriesKey,
        color: colorByKey.get(seriesKey) ?? chartNeutral(),
        lines: orderedQuantileKeys
          .filter(k => qmap.has(k))
          .map(k => ({ quantileKey: k, points: qmap.get(k)! })),
      }))
      .sort((a, b) => a.seriesKey.localeCompare(b.seriesKey))
  })

  let chartSeries = $derived.by(() =>
    timeseries.map(ts => {
      const parsed = parseQuantileSeriesKey(ts.key)
      const seriesKey = parsed?.seriesKey ?? ts.key
      return {
        key: ts.key,
        label: ts.label,
        data: ts.points,
        color: colorByKey.get(seriesKey) ?? chartNeutral(),
        props: {
          curve: curveStepAfter,
          fill: 'none',
          'stroke-width': 2,
        },
      }
    })
  )

  let visiblePointCount = $derived.by(() => {
    let n = 0
    for (const ts of timeseries) n += ts.points.length
    return n
  })

  let yAxisLabel = $derived(unit.trim() || 'value')

  let selectedDate = $derived.by((): Date | null => {
    const ts = ctx.heatmapSelectedTimestamp
    if (ts === null || ts === undefined) return null
    return new Date(ts)
  })

  let selectionTimestamp = $derived.by((): string => {
    const sel = ctx.quantilePointSelection
    if (!sel) return ''
    return formatDateTime(sel.timestampMs, timeContext.timezone, 'milliseconds')
  })

  let unitSuffix = $derived(unit.trim() ? ` ${unit.trim()}` : '')

  let seriesQuantileRows = $derived.by((): SelectionLegendRow[] => {
    const sel = ctx.quantilePointSelection
    const quantileKey = ctx.selectedQuantileKey
    if (!sel || sel.series.length <= 1 || !quantileKey) return []

    const rows: SelectionLegendRow[] = []
    for (const entry of sel.series) {
      const color = colorByKey.get(entry.seriesKey) ?? chartNeutral()
      const value = entry.quantiles[quantileKey]
      rows.push({
        key: `${entry.seriesKey}:${quantileKey}`,
        color,
        label: entry.seriesKey,
        valueText:
          value === null || value === undefined
            ? '—'
            : `${formatMetricValue(value)}${unitSuffix}`,
      })
    }
    rows.sort((a, b) => a.label.localeCompare(b.label))
    return rows
  })

  let seriesSelectionTimestamp = $derived.by((): string => {
    if (seriesQuantileRows.length === 0) return ''
    const quantileKey = ctx.selectedQuantileKey
    const base = selectionTimestamp
    if (!quantileKey) return base
    return `${base} · ${quantileLabelForKey(quantileKey)}`
  })

  let mergedSelectionRows = $derived.by(() => {
    const merged = ctx.quantilePointSelection?.merged
    if (!merged) return []
    return quantileMergedSelectionLegendRows(merged, unit)
  })

  let hasSelectionSummary = $derived(
    seriesQuantileRows.length > 0 || mergedSelectionRows.length > 0
  )

  let selectionDots = $derived.by(() => {
    if (selectedDate === null) return []
    const quantileFilter = ctx.selectedQuantileKey
    const dots: {
      key: string
      color: string
      value: number
    }[] = []
    for (const group of quantileGroups) {
      for (const line of group.lines) {
        if (quantileFilter && line.quantileKey !== quantileFilter) continue
        const value = nearestValueAt(line.points, selectedDate)
        if (value === undefined || !Number.isFinite(value)) continue
        dots.push({
          key: `${group.seriesKey}:${line.quantileKey}`,
          color: group.color,
          value,
        })
      }
    }
    return dots
  })

  function nearestValueAt(
    points: readonly ChartPoint[],
    x: Date
  ): number | undefined {
    if (points.length === 0) return undefined
    const t = x.getTime()
    const i = bisector((p: ChartPoint) => p.date.getTime()).left(points, t)
    const lo = Math.max(0, i - 1)
    const hi = Math.min(points.length - 1, i)
    const dl = Math.abs(points[lo]!.date.getTime() - t)
    const dh = Math.abs(points[hi]!.date.getTime() - t)
    return (dl <= dh ? points[lo] : points[hi])!.value
  }

  function chartPointDate(data: unknown): Date | null {
    if (data == null || typeof data !== 'object') return null
    const row = data as { date?: unknown; x?: unknown; data?: ChartPoint }
    if (row.date instanceof Date) return row.date
    if (row.date != null) return new Date(row.date as string | number)
    if (row.data?.date instanceof Date) return row.data.date
    if (row.x instanceof Date) return row.x
    if (row.x != null) return new Date(row.x as string | number)
    return null
  }

  function tooltipXDate(context: {
    tooltip: { data: unknown }
    x: (d: unknown) => unknown
  }): Date | null {
    const d = context.tooltip.data
    if (d == null) return null
    const v = context.x(d)
    if (v instanceof Date) return v
    if (v != null) return new Date(v as string | number)
    return null
  }

  function dispatchPointClick(
    date: Date,
    seriesKey?: string,
    quantileKey: string | null = null
  ) {
    if (!onChartPointClick) return
    onChartPointClick(seriesKey ?? '', date, quantileKey)
  }

  function handlePointClick(
    e: MouseEvent,
    details: {
      data: { data?: ChartPoint; date?: Date }
      series: { key: string }
    }
  ) {
    const row = details.data?.data ?? details.data
    const date = row?.date ?? chartPointDate(details.data)
    if (!date) return
    e.stopPropagation()
    const meta = lineMetaByKey.get(details.series.key)
    dispatchPointClick(
      date,
      meta?.seriesKey ?? details.series.key,
      meta?.quantileKey ?? null
    )
  }

  function handleTooltipClick(e: MouseEvent, detail: { data: unknown }) {
    const date = chartPointDate(detail.data)
    if (!date) return
    dispatchPointClick(date, '', null)
  }
</script>

{#if visiblePointCount > 0}
  <div class="metric-quantile-area-chart" style:height="{height}px">
    {#if timeRange || onChartPointClick}
      <div class="metric-quantile-area-chart__header">
        {#if timeRange}
          <ChartTimeRangeHeader
            startMs={timeRange.startMs}
            endMs={timeRange.endMs}
            variant="legend"
          />
        {/if}
        {#if onChartPointClick && hasSelectionSummary}
          <div class="metric-quantile-area-chart__selection-legend">
            {#if seriesQuantileRows.length > 0}
              <ChartSelectionLegend
                timestamp={seriesSelectionTimestamp}
                rows={seriesQuantileRows}
              />
            {/if}
            {#if mergedSelectionRows.length > 0}
              <div class="metric-quantile-area-chart__merged-totals">
                <ChartSelectionLegend
                  timestamp={seriesQuantileRows.length > 0 ? '' : selectionTimestamp}
                  rows={mergedSelectionRows}
                />
              </div>
            {/if}
          </div>
        {/if}
      </div>
    {/if}
    <div
      class="metric-quantile-area-chart__plot"
      bind:clientHeight={plotAreaHeight}
    >
      <MetricChartPlot height={plotAreaHeight > 0 ? plotAreaHeight : height}>
        <LineChart
          bind:context={lineChartContext}
          x="date"
          y="value"
          xScale={scaleTime()}
          yNice
          padding={chartPadding}
          tooltipContext={{ mode: 'bisect-x' }}
          series={chartSeries}
          highlight={{ lines: true, points: true }}
          onPointClick={handlePointClick}
          onTooltipClick={handleTooltipClick}
          props={{
            xAxis: axisTime(timeContext.timezone),
            yAxis: axisValue(yAxisLabel),
          }}
        >
          {#snippet tooltip({ context }: { context: any })}
            {@const xDate = tooltipXDate(context)}
            {@const headerLabel =
              xDate != null
                ? formatDateTime(
                    xDate.getTime(),
                    timeContext.timezone,
                    'milliseconds'
                  )
                : undefined}
            <Tooltip.Root {context}>
              {#snippet children()}
                <Tooltip.Header value={headerLabel} />
                <Tooltip.List>
                  {#each quantileGroups as group (group.seriesKey)}
                    {#each group.lines as line (line.quantileKey)}
                      {@const value =
                        xDate != null
                          ? nearestValueAt(line.points, xDate)
                          : undefined}
                      {#if value !== undefined}
                        <Tooltip.Item
                          label="{group.seriesKey} · {quantileLabelForKey(line.quantileKey)}"
                          {value}
                          color={group.color}
                          format={formatMetricValue}
                          valueAlign="right"
                        />
                      {/if}
                    {/each}
                  {/each}
                </Tooltip.List>
              {/snippet}
            </Tooltip.Root>
          {/snippet}

          {#snippet marks({ context }: { context: ChartState<ChartPoint> })}
            {#each context.series.visibleSeries as s (s.key)}
              <Spline
                seriesKey={s.key}
                curve={curveStepAfter}
                stroke={s.color}
                fill="none"
                stroke-width={2}
              />
            {/each}
          {/snippet}

          {#snippet aboveMarks({ context }: { context: ChartState<ChartPoint> })}
            {#if selectedDate && context.yScale}
              {@const px = context.xScale(selectedDate)}
              {@const yTop = context.yRange[1]}
              {@const yBot = context.yRange[0]}
              <g class="selection-overlay" aria-hidden="true">
                <Line
                  x1={px}
                  x2={px}
                  y1={yTop}
                  y2={yBot}
                  class="highlight-rule"
                />
                {#each selectionDots as dot (dot.key)}
                  {@const py = context.yScale(dot.value)}
                  <circle
                    cx={px}
                    cy={py}
                    r="6"
                    fill={dot.color}
                    class="selection-dot"
                  />
                {/each}
              </g>
            {/if}
          {/snippet}
        </LineChart>
      </MetricChartPlot>
    </div>
  </div>
{:else}
  <MetricChartEmpty {height} message={emptyMessage} />
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .metric-quantile-area-chart {
    @apply flex min-h-0 w-full min-w-0 flex-col;
  }

  .metric-quantile-area-chart__header {
    @apply flex shrink-0 items-start justify-between gap-2 px-1 pb-1 pt-0.5;
  }

  .metric-quantile-area-chart__header :global(.chart-time-range-legend__prefix) {
    color: var(--color-subtle);
  }

  .metric-quantile-area-chart__header :global(.chart-time-range-legend__value) {
    @apply text-base-content;
  }

  .metric-quantile-area-chart__selection-legend {
    @apply ml-auto flex shrink-0 flex-col items-end gap-1;
    min-height: 4rem;
    pointer-events: none;
  }

  .metric-quantile-area-chart__selection-legend :global(.chart-selection-legend__label) {
    color: var(--color-subtle);
  }

  .metric-quantile-area-chart__selection-legend :global(.chart-selection-legend__label::after) {
    content: ':';
  }

  .metric-quantile-area-chart__selection-legend :global(.chart-selection-legend__value) {
    @apply text-base-content;
  }

  .metric-quantile-area-chart__merged-totals :global(.chart-selection-legend__rows) {
    grid-template-columns: 1fr;
  }

  .metric-quantile-area-chart__merged-totals :global(.chart-selection-legend__dot),
  .metric-quantile-area-chart__merged-totals :global(.chart-selection-legend__label) {
    display: none;
  }

  .metric-quantile-area-chart__merged-totals :global(.chart-selection-legend__value) {
    text-align: left;
    white-space: nowrap;
    color: var(--color-subtle);
    font-weight: 400;
  }

  .metric-quantile-area-chart__plot {
    @apply relative min-h-0 min-w-0 flex-1;
  }
</style>
