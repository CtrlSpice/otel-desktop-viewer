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
  import ChartKeyboardSurface from '@/components/metrics/Charts/ChartKeyboardSurface.svelte'
  import type { HistogramSlicePoint } from '@/components/metrics/utils/histogram-aggregation'
  import { computeHeatmapColorScale } from '@/components/metrics/utils/heatmap-color-scale'
  import {
    computeHeatmapLayout,
    computeHeatmapPlotHeight,
  } from '@/components/metrics/utils/heatmap-layout'
  import {
    buildHistogramHeatmapData,
    type HeatmapDatum,
  } from '@/components/metrics/utils/histogram-heatmap-data'
  import { themeSignal } from '@/state/theme.svelte'
  import MetricChartEmpty from '@/components/metrics/Charts/MetricChartEmpty.svelte'
  import ChartSelectionLegend from '@/components/metrics/Charts/ChartSelectionLegend.svelte'
  import { histogramColumnSelectionLegendRows } from '@/components/metrics/utils/heatmap-column-selection'
  import ChartTimeRangeHeader from '@/components/metrics/Charts/ChartTimeRangeHeader.svelte'
  import {
    axisBucketBounds,
    axisTime,
    chartPadding,
    DEFAULT_METRIC_CHART_HEIGHT,
  } from '@/components/metrics/Charts/MetricChartPlot.svelte'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import { formatDateTime } from '@/utils/time'
  import {
    gridCursorCommandForKey,
    isChartActivationKey,
  } from '@/components/metrics/utils/chart-keyboard-cursor'
  import { createGridChartKeyboardCursor } from '@/components/metrics/utils/chart-keyboard-state.svelte'

  const timeContext = getTimeContext()
  const ctx = getMetricViewContext()

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
    /** Click handler. Receives the exact bucket-start timestamp. */
    onSelect?: (timestampNs: bigint) => void
    /** Exact bucket-start timestamp for the highlighted column. */
    selectedTimestamp?: bigint | null
    /** Bottom inset inside the LayerChart plot (room for x-axis labels). */
    plotPaddingBottom?: number
    /** Metric unit for bucket-bound y-axis labelling (e.g. "ms"). */
    unit?: string
    onClearSelection?: () => void
  }

  let {
    points,
    height = DEFAULT_METRIC_CHART_HEIGHT,
    timeRange = null,
    onSelect,
    selectedTimestamp = null,
    plotPaddingBottom = chartPadding.bottom,
    unit = '',
    onClearSelection,
  }: Props = $props()

  let plotAreaHeight = $state(0)
  let plotBoxHeight = $derived(plotAreaHeight > 0 ? plotAreaHeight : height)

  // Tooltip header uses the project-standard datetime formatter at
  // millisecond resolution. Includes the timezone suffix so the user
  // can always tell whether they're looking at local or UTC.
  function formatTooltipTime(ms: number): string {
    return formatDateTime(ms, timeContext.tz, 'milliseconds')
  }

  // Rendering and count lookup retain nonzero cells only. The separate column
  // and row arrays describe missing intersections as logical zero cells.
  let heatmapModel = $derived(buildHistogramHeatmapData(points))
  let heatmapColumns = $derived(heatmapModel.columns)
  let heatmapData = $derived(heatmapModel.cells)

  // Exact domains are O(columns + rows), independent of conceptual cells.
  let timeDomain = $derived(heatmapColumns.map(column => column.key))

  let columnByKey = $derived(
    new Map(heatmapColumns.map(column => [column.key, column]))
  )

  let bucketDomain = $derived(heatmapModel.rows.map(row => row.key))

  let bucketLabelByKey = $derived(
    new Map(heatmapModel.rows.map(row => [row.key, row.label]))
  )

  // Anchor cDomain at 0 (not min) so blank-ish cells visually correspond to
  // "nothing happened" rather than "the lowest observed count" -- otherwise
  // an all-medium map and an all-low map look the same.
  let maxCount = $derived(heatmapModel.maxCount)

  // Adaptive step count over the **non-zero** distinct values. 0 isn't part
  // of the active ramp -- it's its own swatch (base-200, matches the chart
  // surface), and the heatmap colour scale (scaleThreshold below) maps any
  // value < 1 to that swatch. So a chart that's all zeros and a single
  // positive value still gets a sensible 1-step ramp.
  let distinctNonZeroCounts = $derived(heatmapModel.distinctNonZeroCount)

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

  let heatmapCountByCell = $derived(heatmapModel.countByColumn)

  let selectedColumnKey = $derived.by(() => {
    if (selectedTimestamp === null || selectedTimestamp === undefined) {
      return null
    }
    return (
      heatmapColumns.find(column => column.timestampNs === selectedTimestamp)
        ?.key ?? null
    )
  })
  const keyboardNavigation = createGridChartKeyboardCursor(
    () => timeDomain,
    () => bucketDomain,
    () => selectedColumnKey
  )
  let activeKeyboardCursor = $derived(keyboardNavigation.current)
  let keyboardFocused = $derived(keyboardNavigation.focused)

  function handleKeyboardFocusChange(focused: boolean) {
    keyboardNavigation.setFocused(focused)
  }

  let keyboardReadout = $derived.by((): string => {
    const cursor = activeKeyboardCursor
    if (!cursor) return ''
    const column = columnByKey.get(String(cursor.columnKey))
    const bucketKey = String(cursor.rowKey)
    if (!column) return ''
    const bucket = bucketLabelByKey.get(bucketKey) ?? bucketKey
    const count = heatmapCountByCell.get(column.key)?.get(bucketKey) ?? 0
    const unitSuffix =
      unit.trim() && unit.trim() !== '1' ? ` ${unit.trim()}` : ''
    const bucketWithUnit =
      bucket === 'all values' ? bucket : `${bucket}${unitSuffix}`
    return (
      `Column ${cursor.columnIndex + 1} of ${timeDomain.length}, ${formatTooltipTime(column.timestampMs)}. ` +
      `Row ${cursor.rowIndex + 1} of ${bucketDomain.length}, bucket ${bucketWithUnit}. ` +
      `Count ${count}.`
    )
  })

  function handleChartKeyboard(event: KeyboardEvent) {
    const command = gridCursorCommandForKey(event)
    if (command) {
      event.preventDefault()
      event.stopPropagation()
      keyboardNavigation.move(command)
      return
    }

    if (isChartActivationKey(event.key)) {
      const cursor = activeKeyboardCursor
      if (!cursor || !onSelect) return
      const column = columnByKey.get(String(cursor.columnKey))
      if (!column) return
      event.preventDefault()
      event.stopPropagation()
      onSelect(column.timestampNs)
      return
    }

    if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      onClearSelection?.()
    }
  }

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

  let maxPlotHeight = $derived(Math.max(0, plotBoxHeight - PLOT_INSET_Y))

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

  let heatmapScrollElement = $state<HTMLElement | undefined>(undefined)

  $effect(() => {
    if (!keyboardFocused) return
    const viewport = heatmapScrollElement
    const cursor = activeKeyboardCursor
    if (!viewport || !cursor || columnPitch <= 0) return
    const left = heatmapPlotPadding.left + cursor.columnIndex * columnPitch
    const right = left + columnPitch
    if (left < viewport.scrollLeft) {
      viewport.scrollLeft = Math.max(0, left - heatmapPlotPadding.left)
    } else if (right > viewport.scrollLeft + viewport.clientWidth) {
      viewport.scrollLeft =
        right - viewport.clientWidth + heatmapPlotPadding.right
    }
  })

  function handleHeatmapClick(event: MouseEvent) {
    if (!onSelect || timeDomain.length === 0 || columnPitch <= 0) return
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
    const plotX = event.clientX - rect.left - heatmapPlotPadding.left
    if (plotX < 0 || plotX > plotWidth) return
    const idx = Math.floor(plotX / columnPitch)
    if (idx < 0 || idx >= timeDomain.length) return
    const column = columnByKey.get(timeDomain[idx]!)
    if (!column) return
    onSelect(column.timestampNs)
  }

  let selectedColumnX = $derived.by(() => {
    if (selectedColumnKey === null) return null
    const idx = timeDomain.indexOf(selectedColumnKey)
    if (idx < 0) return null
    return idx * columnPitch
  })

  let columnSelectionTimestamp = $derived.by((): string => {
    const sel = ctx.heatmapColumnSelection
    if (!sel) return ''
    return formatDateTime(sel.timestampMs, timeContext.tz, 'milliseconds')
  })

  let columnSelectionRowColumns = $derived.by(() => {
    const sel = ctx.heatmapColumnSelection
    if (!sel) return []
    return histogramColumnSelectionLegendRows(sel, unit)
  })

  let hasColumnSelectionSummary = $derived(
    columnSelectionRowColumns.some(column => column.length > 0)
  )

  let keyboardRowHeight = $derived(
    bucketDomain.length > 0 ? maxPlotHeight / bucketDomain.length : 0
  )
  let keyboardShortcuts = $derived([
    'ArrowLeft',
    'ArrowRight',
    'ArrowUp',
    'ArrowDown',
    'Home',
    'End',
    'Control+Home',
    'Control+End',
    'Meta+Home',
    'Meta+End',
    ...(onSelect ? ['Enter', 'Space'] : []),
    'Escape',
  ])
  let keyboardInstructions = $derived(
    `Use Left and Right to inspect time columns and Up and Down to inspect bucket rows. Home and End move to row boundaries; Control or Command plus Home or End move to grid boundaries.${onSelect ? ' Enter or Space selects the current time column.' : ''} Escape clears chart selection.`
  )

  function formatTimeTick(key: unknown): string {
    const column = columnByKey.get(String(key))
    return column ? axisTime(timeContext.tz).format(column.timestampMs) : ''
  }

  function formatBucketTick(key: unknown): string {
    return bucketLabelByKey.get(String(key)) ?? String(key)
  }
</script>

{#if timeDomain.length === 0 || bucketDomain.length === 0}
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
            fitToData={ctx.histogramAxisFitToData}
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
      class="metric-heatmap-chart__plot chart-keyboard-surface-host"
      bind:clientWidth={containerWidth}
      bind:clientHeight={plotAreaHeight}
    >
      <div
        class="heatmap-scroll"
        class:heatmap-scroll--active={heatmapScrolls}
        bind:this={heatmapScrollElement}
      >
        <!-- Pointer coordinates stay on the scroll-sized chart target; the
             named outer surface owns all equivalent keyboard handling. -->
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="heatmap-wrapper"
          class:heatmap-wrapper--clickable={!!onSelect}
          style:width="{scrollChartWidth}px"
          style:height="{chartRenderHeight}px"
          onclick={handleHeatmapClick}
        >
          <Chart
            data={heatmapData}
            x="columnKey"
            xScale={xBandScale}
            xDomain={timeDomain}
            y="bucketKey"
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
                {...axisTime(timeContext.tz)}
                ticks={visibleTimeTicks}
                format={formatTimeTick}
              />
              <Axis
                placement="left"
                {...axisBucketBounds(unit)}
                ticks={visibleBucketTicks}
                format={formatBucketTick}
              />
              <Cell x="columnKey" y="bucketKey" fill="count" />
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
              {#if keyboardFocused && activeKeyboardCursor && keyboardRowHeight > 0}
                <Rect
                  x={activeKeyboardCursor.columnIndex * columnPitch}
                  y={activeKeyboardCursor.rowIndex * keyboardRowHeight}
                  width={columnPitch}
                  height={keyboardRowHeight}
                  class="chart-keyboard-cursor-cell"
                />
              {/if}
            </Layer>
            <Tooltip.Root>
              {#snippet children({ data }: { data: HeatmapDatum })}
                <Tooltip.Header class="text-center"
                  >{formatTooltipTime(data.timestampMs)}</Tooltip.Header
                >
                <Tooltip.List>
                  <Tooltip.Item label="bucket" value={data.bucketLabel} />
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
      <ChartKeyboardSurface
        id="metric-heatmap-keyboard-surface"
        label="Histogram heatmap"
        roleDescription="interactive heatmap"
        instructions={keyboardInstructions}
        readout={keyboardReadout}
        shortcuts={keyboardShortcuts}
        onKeydown={handleChartKeyboard}
        onFocusChange={handleKeyboardFocusChange}
      />
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

  .metric-heatmap-chart__selection-legend
    :global(.chart-selection-legend--columns) {
    width: max-content;
    min-width: 0;
    max-width: none;
  }

  .metric-heatmap-chart__selection-legend
    :global(.chart-selection-legend__columns) {
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

  .metric-heatmap-chart__selection-legend
    :global(.chart-selection-legend__rows) {
    grid-template-columns: auto auto;
    column-gap: 0.35rem;
    row-gap: 0.12rem;
    min-width: 0;
  }

  .metric-heatmap-chart__selection-legend
    :global(.chart-selection-legend__dot) {
    display: none;
  }

  .metric-heatmap-chart__selection-legend
    :global(.chart-selection-legend__label) {
    color: var(--color-subtle);
  }

  .metric-heatmap-chart__selection-legend
    :global(.chart-selection-legend__label::after) {
    content: ':';
  }

  .metric-heatmap-chart__selection-legend
    :global(.chart-selection-legend__value) {
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

  .heatmap-wrapper
    :global(.lc-rect:not(.heatmap-selection):not(.chart-keyboard-cursor-cell)) {
    stroke: none;
    shape-rendering: crispEdges;
  }

  /* Pointer affordance applies only when column selection is wired. */
  .heatmap-wrapper--clickable {
    @apply cursor-pointer;
  }

  /* Full-column hover band (Highlight axis="x"). */
  .heatmap-wrapper :global(.heatmap-hover-column) {
    --fill-color: color-mix(
      in oklab,
      var(--color-primary, #eb6f92) 14%,
      transparent
    );
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
