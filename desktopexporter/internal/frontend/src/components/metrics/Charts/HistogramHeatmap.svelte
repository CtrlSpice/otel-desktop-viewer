<script lang="ts">
  import {
    Chart,
    Cell,
    Axis,
    Highlight,
    Layer,
    Rect,
    Tooltip,
  } from 'layerchart'
  import { scaleBand, scaleThreshold } from 'd3-scale'
  import type { HistogramSlicePoint } from '@/components/metrics/utils/histogram-aggregation'
  import { computeHeatmapColorScale } from '@/components/metrics/utils/heatmap-color-scale'
  import {
    computeHeatmapLayout,
    computeHeatmapPlotHeight,
  } from '@/components/metrics/utils/heatmap-layout'
  import { expBuckets } from '@/components/metrics/utils/histogram-quantile'
  import { themeSignal } from '@/state/theme.svelte'
  import MetricChartEmpty from '@/components/metrics/Charts/MetricChartEmpty.svelte'
  import ChartSelectionLegend from '@/components/metrics/Charts/ChartSelectionLegend.svelte'
  import { histogramColumnSelectionLegendRows } from '@/components/metrics/utils/heatmap-column-selection'
  import ChartTimeRangeHeader from '@/components/metrics/Charts/ChartTimeRangeHeader.svelte'
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import {
    axisBucketBounds,
    axisTime,
    chartPadding,
    DEFAULT_METRIC_CHART_HEIGHT,
  } from '@/components/metrics/Charts/MetricChartPlot.svelte'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatDateTime } from '@/utils/time'

  const timeContext = getTimeContext()
  const ctx = getMetricViewContext()

  // `time` is the raw bucket-start timestamp in milliseconds. We
  // intentionally do NOT pre-format it here -- a formatted string
  // would silently collapse two datapoints sharing the same wall-clock
  // time-of-day on different days into a single column. Keeping the
  // band scale's domain numeric is the cheap fix; the axis formatter
  // and tooltip render the human label at display time only.
  type HeatmapDatum = {
    time: number
    bucket: string
    /** Numeric sort key for the y band scale (low → high). */
    bucketOrder: number
    count: number
  }

  // The fetch was lifted up to MetricViewContext so the Heatmap and
  // Aggregated tabs can share one bucket-series request. This component is
  // now purely a renderer: parent supplies `points`, and the parent owns
  // loading / error / temporality-callout states.
  //
  // selectedTimestamp + onSelect: click toggles column selection on the
  // heatmap tab (see MetricViewContext.onHeatmapSelect).
  type Props = {
    points: HistogramSlicePoint[]
    height?: number
    timeRange?: { startMs: number; endMs: number } | null
    /** Click handler. Receives the bucket-start timestamp in ms. */
    onSelect?: (timestampMs: number) => void
    /** When set, the matching column gets a highlight. ms timestamp. */
    selectedTimestamp?: number | null
    /** Bottom inset inside the LayerChart plot (room for x-axis labels). */
    plotPaddingBottom?: number
    /** Metric unit for bucket-bound y-axis labelling (e.g. "ms"). */
    unit?: string
  }

  let {
    points,
    height = DEFAULT_METRIC_CHART_HEIGHT,
    timeRange = null,
    onSelect,
    selectedTimestamp = null,
    plotPaddingBottom = chartPadding.bottom,
    unit = '',
  }: Props = $props()

  let plotAreaHeight = $state(0)
  let plotBoxHeight = $derived(plotAreaHeight > 0 ? plotAreaHeight : height)

  function tsToMs(ts: bigint): number {
    return Number(ts / 1_000_000n)
  }

  // Tooltip header uses the project-standard datetime formatter at
  // millisecond resolution. Includes the timezone suffix so the user
  // can always tell whether they're looking at local or UTC.
  function formatTooltipTime(ms: number): string {
    return formatDateTime(ms, timeContext.timezone, 'milliseconds')
  }

  function formatBound(v: number): string {
    if (v === 0) return '0'
    if (Math.abs(v) >= 1000) return v.toExponential(1)
    if (Math.abs(v) < 0.01) return v.toExponential(1)
    return v.toPrecision(3)
  }

  let heatmapData = $derived.by((): HeatmapDatum[] => {
    if (points.length === 0) return []
    const first = points[0]
    if (first.kind === 'histogram') {
      return buildHistogramData(points)
    }
    return buildExpHistogramData(points)
  })

  function buildHistogramData(pts: HistogramSlicePoint[]): HeatmapDatum[] {
    const data: HeatmapDatum[] = []
    for (const pt of pts) {
      if (pt.kind !== 'histogram') continue
      const time = tsToMs(pt.timestamp)
      const bounds = pt.bounds
      const counts = pt.counts
      for (let i = 0; i < counts.length; i++) {
        // Skip empty buckets so "no data here" shows the chart background
        // instead of the ramp's lowest swatch. Without this, count=0
        // cells map to the same colour as count=1 cells (both fall in
        // the first quantize band), and the user can't tell "this bucket
        // never received a sample" from "this bucket got a single hit".
        // ExpHistogram already does this implicitly by only emitting
        // buckets inside its offset range; this brings Histogram in line.
        if (counts[i] === 0) continue
        let label: string
        let bucketOrder: number
        if (i === 0) {
          label = bounds.length > 0 ? `≤${formatBound(bounds[0])}` : '0'
          bucketOrder = bounds.length > 0 ? bounds[0]! : 0
        } else if (i < bounds.length) {
          bucketOrder = (bounds[i - 1]! + bounds[i]!) / 2
          label = formatBound(bucketOrder)
        } else {
          bucketOrder =
            bounds.length > 0 ? bounds[bounds.length - 1]! : 0
          label =
            bounds.length > 0
              ? `≥${formatBound(bounds[bounds.length - 1])}`
              : '0'
        }
        data.push({ time, bucket: label, bucketOrder, count: counts[i] })
      }
    }
    return data
  }

  function buildExpHistogramData(pts: HistogramSlicePoint[]): HeatmapDatum[] {
    const data: HeatmapDatum[] = []
    for (const pt of pts) {
      if (pt.kind !== 'expHistogram') continue
      const time = tsToMs(pt.timestamp)
      const buckets = expBuckets(
        pt.scale,
        pt.negativeOffset,
        pt.negativeCounts,
        pt.zeroCount,
        pt.positiveOffset,
        pt.positiveCounts
      )
      for (const bucket of buckets) {
        if (bucket.cnt === 0) continue
        const bucketOrder =
          bucket.lo === bucket.hi ? bucket.lo : (bucket.lo + bucket.hi) / 2
        const label =
          bucket.lo === bucket.hi && bucket.lo === 0
            ? '0'
            : formatBound(bucketOrder)
        data.push({
          time,
          bucket: label,
          bucketOrder,
          count: bucket.cnt,
        })
      }
    }
    return data
  }

  // Ordered domain arrays for band scales — preserve time ordering and
  // bucket ordering (by numeric value, ascending bottom-to-top).
  // timeDomain is the de-duplicated list of bucket-start timestamps in
  // their first-seen order. Backend already returns bucket rows in
  // time order, so first-seen == sorted-ascending without an extra
  // sort. Using a Set (not Map + indexOf) so dedup is O(n), not O(n^2).
  let timeDomain = $derived.by(() => {
    const seen = new Set<number>()
    const ordered: number[] = []
    for (const d of heatmapData) {
      if (!seen.has(d.time)) {
        seen.add(d.time)
        ordered.push(d.time)
      }
    }
    return ordered
  })

  let bucketDomain = $derived.by(() => {
    const orderByLabel = new Map<string, number>()
    for (const d of heatmapData) {
      const prev = orderByLabel.get(d.bucket)
      if (prev === undefined || d.bucketOrder < prev) {
        orderByLabel.set(d.bucket, d.bucketOrder)
      }
    }
    return [...orderByLabel.entries()]
      .sort((a, b) => a[1] - b[1])
      .map(([label]) => label)
      .reverse()
  })

  // Anchor cDomain at 0 (not min) so blank-ish cells visually correspond to
  // "nothing happened" rather than "the lowest observed count" -- otherwise
  // an all-medium map and an all-low map look the same.
  let maxCount = $derived.by(() => {
    let m = 0
    for (const d of heatmapData) if (d.count > m) m = d.count
    return m
  })

  // Adaptive step count over the **non-zero** distinct values. 0 isn't part
  // of the active ramp -- it's its own swatch (base-200, matches the chart
  // surface), and the heatmap colour scale (scaleThreshold below) maps any
  // value < 1 to that swatch. So a chart that's all zeros and a single
  // positive value still gets a sensible 1-step ramp.
  let distinctNonZeroCounts = $derived.by(() => {
    const seen = new Set<number>()
    for (const d of heatmapData) if (d.count > 0) seen.add(d.count)
    return seen.size
  })

  let colorScale = $derived.by(() =>
    computeHeatmapColorScale({
      maxCount,
      distinctNonZeroCount: distinctNonZeroCounts,
      theme: themeSignal.value,
    })
  )

  let cellColorThresholds = $derived(colorScale.thresholds)
  let cellColorRange = $derived(colorScale.range)

  let visibleBucketTicks = $derived.by(() => {
    const n = bucketDomain.length
    if (n <= 8) return bucketDomain
    const step = Math.ceil(n / 7)
    return bucketDomain.filter((_, i) => i % step === 0)
  })

  let visibleTimeTicks = $derived.by(() => {
    const n = timeDomain.length
    if (n <= 8) return timeDomain
    const step = Math.ceil(n / 7)
    return timeDomain.filter((_, i) => i % step === 0)
  })

  // --- Cell sizing ---
  //
  // Fluid columns: fill available width when sparse, scale down to 8px min,
  // then scroll horizontally when even 8px columns overflow.

  let heatmapPlotPadding = $derived({
    top: chartPadding.top,
    left: chartPadding.left,
    right: chartPadding.right,
    bottom: plotPaddingBottom,
  })

  let PLOT_INSET_X = $derived(
    heatmapPlotPadding.left + heatmapPlotPadding.right
  )
  let PLOT_INSET_Y = $derived(
    heatmapPlotPadding.top + heatmapPlotPadding.bottom
  )

  let containerWidth = $state(0)

  /** Scroll viewport width — measured on the plot area. */
  let plotContainerWidth = $derived(Math.max(containerWidth, 0))

  let maxPlotHeight = $derived(
    Math.max(0, plotBoxHeight - PLOT_INSET_Y)
  )

  let baseLayout = $derived.by(() =>
    computeHeatmapLayout({
      containerWidth: Math.max(plotContainerWidth, 1),
      plotInsetX: PLOT_INSET_X,
      columnCount: timeDomain.length,
    })
  )

  let plotHeight = $derived.by(() =>
    computeHeatmapPlotHeight({ maxPlotHeight })
  )

  let heatmapLayout = $derived(baseLayout)

  let chartRenderHeight = $derived(plotBoxHeight)

  let scrollChartWidth = $derived(heatmapLayout.chartWidth)
  let columnPitch = $derived(heatmapLayout.columnPitch)
  let plotWidth = $derived(heatmapLayout.plotWidth)
  let heatmapScrolls = $derived(
    containerWidth > 0 && scrollChartWidth > plotContainerWidth
  )

  let xBandScale = $derived(scaleBand().paddingOuter(0).padding(0))

  let yBandScale = $derived(scaleBand().paddingOuter(0).padding(0))

  function handleHeatmapClick(event: MouseEvent) {
    if (!onSelect || timeDomain.length === 0 || columnPitch <= 0) return
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
    const plotX = event.clientX - rect.left - heatmapPlotPadding.left
    if (plotX < 0 || plotX > plotWidth) return
    const idx = Math.floor(plotX / columnPitch)
    if (idx < 0 || idx >= timeDomain.length) return
    onSelect(timeDomain[idx])
  }

  let selectedColumnX = $derived.by(() => {
    if (selectedTimestamp === null || selectedTimestamp === undefined)
      return null
    const idx = timeDomain.indexOf(selectedTimestamp)
    if (idx < 0) return null
    return idx * columnPitch
  })

  let columnSelectionTimestamp = $derived.by((): string => {
    const sel = ctx.heatmapColumnSelection
    if (!sel) return ''
    return formatDateTime(sel.timestampMs, timeContext.timezone, 'milliseconds')
  })

  let columnSelectionRowColumns = $derived.by(() => {
    const sel = ctx.heatmapColumnSelection
    if (!sel) return []
    return histogramColumnSelectionLegendRows(sel, unit)
  })

  let hasColumnSelectionSummary = $derived(
    columnSelectionRowColumns.some(column => column.length > 0)
  )
</script>

{#if heatmapData.length === 0}
  <MetricChartEmpty {height} message="No bucket data in range" />
{:else}
  <div class="metric-heatmap-chart" style:height="{height}px">
    {#if timeRange || onSelect}
      <div class="metric-heatmap-chart__header">
        {#if timeRange}
          <ChartTimeRangeHeader
            startMs={timeRange.startMs}
            endMs={timeRange.endMs}
            variant="legend"
          />
        {/if}
        {#if onSelect}
          <div class="metric-heatmap-chart__selection-legend">
            {#if hasColumnSelectionSummary}
              <ChartSelectionLegend
                variant="columns"
                timestamp={columnSelectionTimestamp}
                rowColumns={columnSelectionRowColumns}
              />
            {/if}
          </div>
        {/if}
      </div>
    {/if}
    <div
      class="metric-heatmap-chart__plot"
      bind:clientWidth={containerWidth}
      bind:clientHeight={plotAreaHeight}
    >
        <div
          class="heatmap-scroll"
          class:heatmap-scroll--active={heatmapScrolls}
        >
          <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
          <div
            class="heatmap-wrapper"
            class:heatmap-wrapper--clickable={!!onSelect}
            style:width="{scrollChartWidth}px"
            style:height="{chartRenderHeight}px"
            onclick={handleHeatmapClick}
            onkeydown={e => {
              if (onSelect && (e.key === 'Enter' || e.key === ' ')) {
                e.preventDefault()
              }
            }}
            role={onSelect ? 'button' : undefined}
            tabindex={onSelect ? 0 : undefined}
          >
            <Chart
              data={heatmapData}
              x="time"
              xScale={xBandScale}
              xDomain={timeDomain}
              y="bucket"
              yScale={yBandScale}
              yDomain={bucketDomain}
              c="count"
              cScale={scaleThreshold()}
              cDomain={cellColorThresholds}
              cRange={cellColorRange}
              width={scrollChartWidth}
              height={chartRenderHeight}
              padding={heatmapPlotPadding}
              tooltipContext={{ mode: 'band' }}
            >
              <Layer>
                <Axis
                  placement="bottom"
                  {...axisTime(timeContext.timezone)}
                  ticks={visibleTimeTicks}
                />
                <Axis
                  placement="left"
                  {...axisBucketBounds(unit)}
                  ticks={visibleBucketTicks}
                />
                <Cell x="time" y="bucket" fill="count" />
                {#if selectedColumnX !== null}
                  <Rect
                    x={selectedColumnX}
                    y={0}
                    width={columnPitch}
                    height={maxPlotHeight}
                    class="heatmap-selection"
                  />
                {/if}
                <Highlight area={{ class: 'heatmap-hover-column' }} axis="x" />
              </Layer>
              <Tooltip.Root>
                {#snippet children({ data }: { data: HeatmapDatum })}
                  <Tooltip.Header class="text-center"
                    >{formatTooltipTime(data.time)}</Tooltip.Header
                  >
                  <Tooltip.List>
                    <Tooltip.Item label="bucket" value={data.bucket} />
                    <Tooltip.Separator />
                    <Tooltip.Item
                      label="count"
                      value={data.count}
                      format="integer"
                    />
                  </Tooltip.List>
                {/snippet}
              </Tooltip.Root>
            </Chart>
          </div>
        </div>
    </div>
  </div>
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .metric-heatmap-chart {
    @apply flex min-h-0 w-full min-w-0 flex-col;
  }

  .metric-heatmap-chart__header {
    @apply flex shrink-0 items-start justify-between gap-2 px-1 pb-1 pt-0.5;
  }

  .metric-heatmap-chart__header :global(.chart-time-range-legend__prefix) {
    color: var(--color-subtle);
  }

  .metric-heatmap-chart__header :global(.chart-time-range-legend__value) {
    @apply text-base-content;
  }

  .metric-heatmap-chart__selection-legend {
    /* Reserve stats card height so the plot does not shift on select. */
    @apply ml-auto shrink-0;
    min-height: 4rem;
    pointer-events: none;
  }

  .metric-heatmap-chart__selection-legend :global(.chart-selection-legend--columns) {
    width: max-content;
    min-width: 0;
    max-width: none;
  }

  .metric-heatmap-chart__selection-legend :global(.chart-selection-legend__columns) {
    display: flex;
    flex-wrap: nowrap;
    align-items: flex-start;
    gap: 0;
  }

  .metric-heatmap-chart__selection-legend
    :global(.chart-selection-legend__column + .chart-selection-legend__column) {
    border-left: 1px solid
      color-mix(in oklab, var(--color-base-300) 70%, transparent);
    margin-left: 0.55rem;
    padding-left: 0.55rem;
  }

  .metric-heatmap-chart__selection-legend :global(.chart-selection-legend__rows) {
    grid-template-columns: auto auto;
    column-gap: 0.35rem;
    row-gap: 0.12rem;
    min-width: 0;
  }

  .metric-heatmap-chart__selection-legend :global(.chart-selection-legend__dot) {
    display: none;
  }

  .metric-heatmap-chart__selection-legend :global(.chart-selection-legend__label) {
    color: var(--color-subtle);
  }

  .metric-heatmap-chart__selection-legend :global(.chart-selection-legend__label::after) {
    content: ':';
  }

  .metric-heatmap-chart__selection-legend :global(.chart-selection-legend__value) {
    @apply text-base-content;
  }

  .metric-heatmap-chart__plot {
    @apply relative min-h-0 min-w-0 flex-1 overflow-hidden;
  }

  .heatmap-scroll {
    @apply h-full min-w-0 overflow-x-hidden overflow-y-hidden;
  }

  .heatmap-scroll--active {
    @apply overflow-x-auto;
  }

  .heatmap-wrapper :global(.lc-rect) {
    stroke: none;
    shape-rendering: crispEdges;
  }

  /* Cursor affordance + light keyboard-focus ring when the heatmap is
     interactive. Only applied when onSelect is wired so non-interactive
     surfaces (Gauge/Sum, where the heatmap isn't even rendered, but
     defensive) don't lie about being clickable. */
  .heatmap-wrapper--clickable {
    @apply cursor-pointer;
  }
  .heatmap-wrapper--clickable:focus-visible {
    @apply outline outline-2 outline-offset-2 outline-primary/60;
  }

  /* Full-column hover band (Highlight axis="x"). */
  .heatmap-wrapper :global(.heatmap-hover-column) {
    --fill-color: color-mix(in oklab, var(--color-primary, #eb6f92) 14%, transparent);
    pointer-events: none;
  }

  /* Persistent selection ring drawn over the active column. Stroke-only
     (no fill) so the underlying Cell colour stays readable -- the user
     still wants to see the count distribution in the column they're
     inspecting. Rosé Pine "love" reads well on both light and dark. */
  .heatmap-wrapper :global(.heatmap-selection) {
    fill: none;
    stroke: var(--color-primary, #eb6f92);
    stroke-width: 2;
    pointer-events: none;
  }
</style>
