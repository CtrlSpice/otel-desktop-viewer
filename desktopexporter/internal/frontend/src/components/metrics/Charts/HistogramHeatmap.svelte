<script lang="ts">
  import ChartOverlayLabel from './ChartOverlayLabel.svelte'
  import MetricAxes from './MetricAxes.svelte'
  import { createOverlayEmphasis } from '@/components/metrics/utils/overlay-emphasis.svelte'

  import ChartSticky from '@/components/metrics/Charts/ChartSticky.svelte'
  import HistogramContributionPattern from './HistogramContributionPattern.svelte'
  import type { ChartStickySize } from '@/components/metrics/utils/chart-sticky-position'
  import { heatmapCardPosition } from '@/components/metrics/utils/heatmap-card-position'
  import {
    Chart,
    Axis,
    Highlight,
    Layer,
    Rect,
    Tooltip,
    type ChartState,
  } from 'layerchart'
  import { scaleBand, scaleThreshold, type ScaleBand } from 'd3-scale'
  import { bisectRight } from 'd3-array'
  import ChartKeyboardSurface from '@/components/metrics/Charts/ChartKeyboardSurface.svelte'
  import { chartScrollDiscovery } from '@/components/metrics/utils/chart-scroll-discovery'
  import {
    QUANTILE_LABELS_DESCENDING,
    type HistogramSlicePoint,
  } from '@/components/metrics/utils/histogram-aggregation'
  import {
    formatMetricValue,
    formatMetricCount,
    metricNumberPrecision,
  } from '@/components/metrics/utils/format-metric-value'
  import {
    formatHistogramBound,
    formatHistogramRange,
    exponentialZeroBucketIdentity,
    exponentialZeroBucketLabel,
  } from '@/components/metrics/utils/histogram-bucket'
  import { computeHeatmapColorScale } from '@/components/metrics/utils/heatmap-color-scale'
  import { themeSignal } from '@/state/theme.svelte'
  import { countPalette } from '@/state/count-palette.svelte'
  import {
    heatmapQuantilePaths,
    histogramQuantileColor,
  } from '@/components/metrics/utils/heatmap-quantiles'
  import {
    buildHeatmapBlobs,
    heatmapBlobDetail,
    heatmapBlobPath,
    heatmapColourContours,
  } from '@/components/metrics/utils/heatmap-blobs'
  import {
    computeHeatmapLayout,
    computeHeatmapPlotHeight,
  } from '@/components/metrics/utils/heatmap-layout'
  import {
    buildHistogramHeatmapData,
    type HeatmapDatum,
  } from '@/components/metrics/utils/histogram-heatmap-data'
  import MetricChartEmpty from '@/components/metrics/Charts/MetricChartEmpty.svelte'
  import ChartSelectionLegend from '@/components/metrics/Charts/ChartSelectionLegend.svelte'
  import { histogramColumnSelectionLegendRows } from '@/components/metrics/utils/heatmap-column-selection'
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
  const emphasis = createOverlayEmphasis()
  const ctx = getMetricViewContext()
  const blobClipPrefix = $props.id()
  const quantileChannelID = `${blobClipPrefix}-quantile-channels`
  const contributionPatternID = `${blobClipPrefix}-contribution-hatch`

  // The context owns fetching and state; this component only renders points.
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
  let chartContext = $state<
    ChartState<HeatmapDatum, ScaleBand<string>, ScaleBand<string>> | undefined
  >(undefined)
  let hoveredColumnKey = $derived(chartContext?.tooltip.data?.columnKey ?? null)
  let hoveredBucketKey = $derived(chartContext?.tooltip.data?.bucketKey ?? null)
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

  let contributionCells = $derived(
    buildHistogramHeatmapData(
      ctx.histogramContribution?.point ? [ctx.histogramContribution.point] : []
    ).cells
  )
  let contributionByBucket = $derived(
    new Map(contributionCells.map(cell => [cell.bucketKey, cell.count]))
  )
  let contributionTimestamp = $derived(
    ctx.histogramContribution?.point?.timestamp
  )
  function contributionCount(cell: HeatmapDatum): number {
    return cell.timestampNs === contributionTimestamp
      ? (contributionByBucket.get(cell.bucketKey) ?? 0)
      : 0
  }

  // Exact domains are O(columns + rows), independent of conceptual cells.
  let timeDomain = $derived(heatmapColumns.map(column => column.key))

  let columnByKey = $derived(
    new Map(heatmapColumns.map(column => [column.key, column]))
  )
  let pointByColumnKey = $derived(
    new Map(heatmapColumns.map((column, index) => [column.key, points[index]!]))
  )

  let bucketDomain = $derived(heatmapModel.rows.map(row => row.key))

  let bucketLabelByKey = $derived(
    new Map(heatmapModel.rows.map(row => [row.key, row.label]))
  )

  function formatBucketHover(cell: HeatmapDatum): string {
    const index = heatmapModel.rows.findIndex(row => row.key === cell.bucketKey)
    const row = heatmapModel.rows[index]
    const point = pointByColumnKey.get(cell.columnKey)
    if (!row || !point) return cell.bucketLabel
    const digits = metricNumberPrecision(
      [
        row.lo,
        row.hi,
        ...heatmapModel.rows
          .slice(Math.max(0, index - 1), index + 2)
          .map(row => row.order),
      ],
      6
    )
    if (point.kind === 'expHistogram') {
      return row.key === exponentialZeroBucketIdentity(point.zeroThreshold)
        ? exponentialZeroBucketLabel(point.zeroThreshold, digits)
        : formatHistogramBound(row.order, digits)
    }
    if (!Number.isFinite(row.lo) && !Number.isFinite(row.hi))
      return 'all values'
    if (!Number.isFinite(row.lo))
      return `≤${formatHistogramBound(row.hi, digits)}`
    if (!Number.isFinite(row.hi))
      return `>${formatHistogramBound(row.lo, digits)}`
    return formatHistogramRange(row.lo, row.hi, digits)
  }

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
      palette: countPalette.value,
    })
  )

  let cellColorThresholds = $derived(colorScale.thresholds)
  // Resting cells retain the original palette; interactions lift their own colour.
  let cellColorRange = $derived(
    colorScale.range.map(
      colour =>
        `color-mix(in oklab, ${colour} var(--heatmap-cell-colour-weight, 100%), var(--color-base-content))`
    )
  )

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
  let selectedCellKey = $state<{ columnKey: string; bucketKey: string } | null>(
    null
  )
  let selectedCell = $derived.by(() => {
    if (!selectedCellKey || selectedCellKey.columnKey !== selectedColumnKey)
      return null
    const rowIndex = bucketDomain.indexOf(selectedCellKey.bucketKey)
    if (rowIndex < 0) return null
    return {
      ...selectedCellKey,
      rowIndex,
      bucketLabel:
        bucketLabelByKey.get(selectedCellKey.bucketKey) ??
        selectedCellKey.bucketKey,
      count:
        heatmapCountByCell
          .get(selectedCellKey.columnKey)
          ?.get(selectedCellKey.bucketKey) ?? 0,
    }
  })
  $effect(() => {
    if (selectedCellKey && selectedCellKey.columnKey !== selectedColumnKey)
      selectedCellKey = null
  })

  function selectCell(columnKey: string, bucketKey: string) {
    const column = columnByKey.get(columnKey)
    if (!column || !onSelect) return
    const sameCell =
      selectedColumnKey === columnKey && selectedCell?.bucketKey === bucketKey
    selectedCellKey = sameCell ? null : { columnKey, bucketKey }
    // Picking another row in the pinned column changes the cell, not the pin.
    if (selectedColumnKey !== columnKey || sameCell)
      onSelect(column.timestampNs)
  }
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

  function dismissSelection() {
    selectedCellKey = null
    onClearSelection?.()
    document
      .getElementById('metric-heatmap-keyboard-surface')
      ?.focus({ preventScroll: true })
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
    const estimates = pointByColumnKey.get(column.key)?.quantiles
    const quantileReadout = QUANTILE_LABELS_DESCENDING.filter(({ key }) =>
      ctx.activeQuantileOverlays.has(key)
    )
      .flatMap(({ key, label }) => {
        const value = estimates?.[key]
        return value != null && Number.isFinite(value)
          ? [`${label} estimate ${formatMetricValue(value)}${unitSuffix}.`]
          : []
      })
      .join(' ')
    return (
      `Column ${cursor.columnIndex + 1} of ${timeDomain.length}, ${formatTooltipTime(column.timestampMs)}. ` +
      `Row ${cursor.rowIndex + 1} of ${bucketDomain.length}, bucket ${bucketWithUnit}. ` +
      `Count ${count}. ${quantileReadout}`
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
      selectCell(column.key, String(cursor.rowKey))
      return
    }

    if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      onClearSelection?.()
    }
  }

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
  let keyboardRowHeight = $derived(
    bucketDomain.length > 0 ? maxPlotHeight / bucketDomain.length : 0
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
  let quantilePaths = $derived(
    heatmapQuantilePaths(
      points,
      heatmapModel,
      columnPitch,
      maxPlotHeight,
      ctx.activeQuantileOverlays
    )
  )
  let plotWidth = $derived(heatmapLayout.plotWidth)
  let quantileLabels = $derived.by(() => {
    const labels = quantilePaths
      .map(line => ({ ...line, labelY: line.end.y }))
      .sort((a, b) => a.labelY - b.labelY)
    const gap = Math.min(
      26,
      Math.max(0, maxPlotHeight - 24) / Math.max(1, labels.length - 1)
    )
    for (let index = 0; index < labels.length; index++) {
      labels[index]!.labelY = Math.max(
        labels[index]!.labelY,
        index === 0 ? 12 : labels[index - 1]!.labelY + gap
      )
    }
    for (let index = labels.length - 1; index >= 0; index--) {
      labels[index]!.labelY = Math.min(
        labels[index]!.labelY,
        index === labels.length - 1
          ? maxPlotHeight - 12
          : labels[index + 1]!.labelY - gap
      )
    }
    return labels
  })
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

  function handleHeatmapClick(
    event: MouseEvent & { currentTarget: HTMLDivElement }
  ) {
    if (!onSelect || timeDomain.length === 0 || columnPitch <= 0) return
    const rect = event.currentTarget.getBoundingClientRect()
    const plotX = event.clientX - rect.left - heatmapPlotPadding.left
    const plotY = event.clientY - rect.top - heatmapPlotPadding.top
    if (plotX < 0 || plotX > plotWidth) return
    if (plotY < 0 || plotY >= maxPlotHeight || keyboardRowHeight <= 0) return
    const idx = Math.floor(plotX / columnPitch)
    if (idx < 0 || idx >= timeDomain.length) return
    const column = columnByKey.get(timeDomain[idx]!)
    if (!column) return
    const bucketKey = bucketDomain[Math.floor(plotY / keyboardRowHeight)]
    if (bucketKey !== undefined) selectCell(column.key, bucketKey)
  }

  let selectedColumnX = $derived.by(() => {
    if (selectedColumnKey === null) return null
    const idx = timeDomain.indexOf(selectedColumnKey)
    if (idx < 0) return null
    return idx * columnPitch
  })

  let scrollLeft = $state(0)
  let selectionAnchor = $derived(
    selectedColumnX === null
      ? null
      : {
          x:
            heatmapPlotPadding.left +
            selectedColumnX +
            columnPitch / 2 -
            scrollLeft,
          y:
            heatmapPlotPadding.top +
            (selectedCell
              ? (selectedCell.rowIndex + 0.5) * keyboardRowHeight
              : maxPlotHeight / 2),
        }
  )

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

  let blobDetail = $derived(heatmapBlobDetail(columnPitch, keyboardRowHeight))
  let blobs = $derived(
    buildHeatmapBlobs(heatmapModel, cellColorThresholds, blobDetail.joinColours)
  )
  let colourContours = $derived(
    heatmapColourContours(
      heatmapModel,
      cellColorThresholds,
      columnPitch,
      keyboardRowHeight
    )
  )
  let contributionRegions = $derived(
    buildHeatmapBlobs(
      {
        ...heatmapModel,
        cells: heatmapData.filter(cell => contributionCount(cell) > 0),
      },
      cellColorThresholds
    ).map((blob, index) => ({
      key: blob.key,
      patternID: `${contributionPatternID}-${index}`,
      color: cellColorRange[blob.band],
      path: heatmapBlobPath(blob, columnPitch, keyboardRowHeight, {
        columns: timeDomain.length,
        rows: bucketDomain.length,
      }),
    }))
  )
  // Geometry depends on the grid and viewport, not pointer/selection state.
  let renderedBlobs = $derived.by(() => {
    const columns = new Map(timeDomain.map((key, index) => [key, index]))
    const rows = new Map(bucketDomain.map((key, index) => [key, index]))
    return blobs.map((blob, index) => ({
      key: blob.key,
      band: blob.band,
      clipID: `${blobClipPrefix}-blob-${index}`,
      path: heatmapBlobPath(blob, columnPitch, keyboardRowHeight, {
        columns: timeDomain.length,
        rows: bucketDomain.length,
      }),
      cells: blob.cells.map(cell => ({
        datum: cell,
        x: columns.get(cell.columnKey)! * columnPitch,
        y: rows.get(cell.bucketKey)! * keyboardRowHeight,
      })),
    }))
  })
  let cardObstacles = $derived(
    renderedBlobs.flatMap(blob =>
      blob.cells.map(cell => ({
        left: heatmapPlotPadding.left + cell.x - scrollLeft,
        top: heatmapPlotPadding.top + cell.y,
        width: columnPitch,
        height: keyboardRowHeight,
      }))
    )
  )
  let occupiedBlobPath = $derived(renderedBlobs.map(blob => blob.path).join(''))

  function positionSelectionCard(size: ChartStickySize) {
    return heatmapCardPosition({
      ...size,
      columnLeft: heatmapPlotPadding.left + (selectedColumnX ?? 0) - scrollLeft,
      columnWidth: columnPitch,
      anchorY: selectedCell ? selectionAnchor?.y : undefined,
      plot: {
        left: heatmapPlotPadding.left,
        top: heatmapPlotPadding.top,
        width: Math.max(0, plotContainerWidth - PLOT_INSET_X),
        height: maxPlotHeight,
      },
      cells: cardObstacles,
    })
  }
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

  function formatTimeTick(key: string): string {
    const column = columnByKey.get(key)
    return column ? axisTime(timeContext.tz).format(column.timestampMs) : ''
  }

  function formatBucketTick(key: string): string {
    return bucketLabelByKey.get(key) ?? key
  }
  let chartAxes = $derived({
    xAxis: {
      ...axisTime(timeContext.tz),
      tickSpacing: 80,
      format: formatTimeTick,
    },
    yAxis: {
      ...axisBucketBounds(unit),
      tickSpacing: 50,
      format: formatBucketTick,
    },
  })
</script>

{#if timeDomain.length === 0 || bucketDomain.length === 0}
  <MetricChartEmpty {height} message="No bucket data in range" />
{:else}
  <div class="metric-heatmap-chart" style:height="{height}px">
    <div
      class="metric-heatmap-chart__plot chart-keyboard-surface-host"
      bind:clientWidth={containerWidth}
      bind:clientHeight={plotAreaHeight}
    >
      <div
        class="heatmap-scroll"
        class:heatmap-scroll--active={heatmapScrolls}
        bind:this={heatmapScrollElement}
        use:chartScrollDiscovery={{
          visit: ctx.metric?.metricRef,
          overflows:
            plotContainerWidth > 0 && scrollChartWidth - plotContainerWidth > 1,
        }}
        onscroll={event => (scrollLeft = event.currentTarget.scrollLeft)}
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
            bind:context={chartContext}
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
              <Axis placement="bottom" {...chartAxes.xAxis} />
              <Axis placement="left" {...chartAxes.yAxis} />
              <defs>
                <mask
                  id={quantileChannelID}
                  maskUnits="userSpaceOnUse"
                  x={0}
                  y={0}
                  width={plotWidth}
                  height={maxPlotHeight}
                  style="mask-type: luminance"
                >
                  <rect
                    x={0}
                    y={0}
                    width={plotWidth}
                    height={maxPlotHeight}
                    fill="white"
                  />
                  {#each quantilePaths as quantile (quantile.key)}
                    <path
                      d={quantile.path}
                      class="heatmap-quantile-channel"
                      class:chart-overlay--emphasized={emphasis.has(
                        quantile.key
                      )}
                      fill="none"
                      stroke="black"
                      stroke-linecap="round"
                      stroke-linejoin="round"
                    />
                  {/each}
                </mask>
              </defs>
              <g
                class="heatmap-blobs"
                class:heatmap-blobs--joined={blobDetail.joinColours}
                mask={`url(#${quantileChannelID})`}
              >
                {#each renderedBlobs as blob (blob.key)}
                  <defs>
                    <clipPath id={blob.clipID} clipPathUnits="userSpaceOnUse">
                      <path d={blob.path} clip-rule="evenodd" />
                    </clipPath>
                    {#if blobDetail.joinColours}
                      <mask
                        id={`${blob.clipID}-outline`}
                        maskUnits="userSpaceOnUse"
                        x="0"
                        y="0"
                        width={timeDomain.length * columnPitch}
                        height={bucketDomain.length * keyboardRowHeight}
                        style="mask-type: alpha"
                      >
                        <path
                          d={blob.path}
                          fill="none"
                          stroke="white"
                          stroke-width={blobDetail.liner * 2}
                          stroke-linejoin="round"
                        />
                      </mask>
                    {/if}
                  </defs>
                  <g class="heatmap-blob" clip-path={`url(#${blob.clipID})`}>
                    {#if !blobDetail.joinColours}<path
                        class="heatmap-blob-fill"
                        d={blob.path}
                        fill={cellColorRange[blob.band]}
                        fill-rule="evenodd"
                      />{/if}
                    {#each blob.cells as cell (cell.datum)}
                      <rect
                        x={cell.x}
                        y={cell.y}
                        width={columnPitch}
                        height={keyboardRowHeight}
                        fill={cellColorRange[
                          bisectRight(cellColorThresholds, cell.datum.count)
                        ]}
                        class="heatmap-cell"
                        class:heatmap-cell--hovered={cell.datum.columnKey ===
                          hoveredColumnKey &&
                          cell.datum.bucketKey === hoveredBucketKey}
                        class:heatmap-cell--selected={cell.datum.columnKey ===
                          selectedColumnKey &&
                          (!selectedCell ||
                            cell.datum.bucketKey === selectedCell.bucketKey)}
                        data-contribution-count={contributionCount(
                          cell.datum
                        ) || undefined}
                      />
                    {/each}
                    {#if blobDetail.joinColours}
                      <g
                        class="heatmap-joined-outline"
                        mask={`url(#${blob.clipID}-outline)`}
                        aria-hidden="true"
                      >
                        {#each blob.cells as cell (cell.datum)}
                          <rect
                            x={cell.x}
                            y={cell.y}
                            width={columnPitch}
                            height={keyboardRowHeight}
                            fill={cellColorRange[
                              bisectRight(cellColorThresholds, cell.datum.count)
                            ]}
                          />
                        {/each}
                      </g>
                    {:else}<path
                        class="heatmap-blob-outline"
                        d={blob.path}
                        style:--blob-color={cellColorRange[blob.band]}
                        style:--blob-outline-width={blobDetail.liner * 2}
                        aria-hidden="true"
                      />{/if}
                  </g>
                {/each}
                <g class="heatmap-colour-corners" aria-hidden="true">
                  {#each colourContours.corners as corner (corner.key)}
                    <path
                      d={corner.path}
                      fill={cellColorRange[corner.band]}
                      class:heatmap-colour-corner--hovered={corner.datum.columnKey ===
                        hoveredColumnKey &&
                        corner.datum.bucketKey === hoveredBucketKey}
                      class:heatmap-colour-corner--selected={corner.datum.columnKey ===
                        selectedColumnKey &&
                        (!selectedCell ||
                          corner.datum.bucketKey === selectedCell.bucketKey)}
                    />
                  {/each}
                </g>
                {#if blobDetail.joinColours}
                  <!-- Keep each liner inside its colour region and the existing
                       occupied silhouette, including its empty holes. -->
                  <defs>
                    <clipPath
                      id={`${blobClipPrefix}-occupied`}
                      clipPathUnits="userSpaceOnUse"
                    >
                      <path d={occupiedBlobPath} clip-rule="evenodd" />
                    </clipPath>
                    {#each colourContours.regions as region (region.band)}
                      <clipPath
                        id={`${blobClipPrefix}-colour-${region.band}`}
                        clipPathUnits="userSpaceOnUse"
                      >
                        <path d={region.path} clip-rule="evenodd" />
                      </clipPath>
                    {/each}
                  </defs>
                  <g
                    class="heatmap-colour-outlines"
                    clip-path={`url(#${blobClipPrefix}-occupied)`}
                    aria-hidden="true"
                  >
                    {#each colourContours.regions as region (region.band)}
                      <path
                        class="heatmap-blob-outline"
                        d={region.path}
                        clip-path={`url(#${blobClipPrefix}-colour-${region.band})`}
                        style:--blob-color={cellColorRange[region.band]}
                        style:--blob-outline-width={blobDetail.liner * 2}
                      />
                    {/each}
                  </g>
                {/if}
              </g>
              <Highlight area={{ class: 'heatmap-hover-column' }} axis="x" />
              {#if selectedColumnX !== null}
                <Rect
                  x={selectedColumnX}
                  y={0}
                  width={columnPitch}
                  height={maxPlotHeight}
                  class="heatmap-selection"
                />
              {/if}
              <g class="heatmap-contributions" aria-hidden="true">
                {#each contributionRegions as region (region.key)}
                  <HistogramContributionPattern
                    id={region.patternID}
                    color={region.color}
                  />
                  <defs>
                    <clipPath
                      id={`${region.patternID}-clip`}
                      clipPathUnits="userSpaceOnUse"
                    >
                      <path d={region.path} />
                    </clipPath>
                  </defs>
                  <g clip-path={`url(#${region.patternID}-clip)`}>
                    <path d={region.path} fill={region.color} />
                    <path
                      class="heatmap-contribution-texture"
                      d={region.path}
                      fill={`url(#${region.patternID})`}
                      style:--contribution-color={region.color}
                      style:--contribution-outline-width={blobDetail.liner * 2}
                    />
                  </g>
                {/each}
              </g>
              <g class="heatmap-quantile-overlays" aria-hidden="true">
                {#each quantilePaths as quantile (quantile.key)}
                  <g
                    class:chart-overlay--emphasized={emphasis.has(quantile.key)}
                  >
                    <path d={quantile.path} class="quantile-line-backing" />
                    <path
                      d={quantile.path}
                      class="heatmap-quantile-line"
                      style:--quantile-color={histogramQuantileColor(
                        quantile.key
                      )}
                      stroke-dasharray={quantile.dash}
                      data-quantile={quantile.key}
                    >
                      <title
                        >{quantile.label}: bucket-derived estimate of the
                        combined checked series</title
                      >
                    </path>
                  </g>
                {/each}
              </g>
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
            <Tooltip.Root class="chart-hover-card">
              {#snippet children({ data }: { data: HeatmapDatum })}
                <Tooltip.Header
                  >{formatTooltipTime(data.timestampMs)}</Tooltip.Header
                >
                <dl class="heatmap-hover-cell">
                  <dt>bucket</dt>
                  <dd>
                    {formatBucketHover(data)}{data.bucketLabel !==
                      'all values' &&
                    unit.trim() &&
                    unit.trim() !== '1'
                      ? ` ${unit.trim()}`
                      : ''}
                  </dd>
                  <dt>count</dt>
                  <dd>{formatMetricCount(data.count)}</dd>
                  {#if ctx.histogramContribution}
                    <dt>selected contribution</dt>
                    <dd>
                      {formatMetricCount(contributionCount(data))} of {formatMetricCount(
                        data.count
                      )}
                    </dd>
                  {/if}
                </dl>
              {/snippet}
            </Tooltip.Root>
          </Chart>
          <MetricAxes padding={heatmapPlotPadding} />
          {#each quantileLabels as quantile (quantile.key)}
            <div
              class="heatmap-quantile-label chart-selection-legend chart-selection-legend--stat"
              style:left="{heatmapPlotPadding.left + quantile.end.x + 8}px"
              style:top="{heatmapPlotPadding.top + quantile.labelY}px"
            >
              <span
                class="chart-selection-legend__dot"
                style:--color={histogramQuantileColor(quantile.key)}
              ></span>
              <ChartOverlayLabel
                label={quantile.label}
                active={emphasis.has(quantile.key)}
                onclick={() => emphasis.toggle(quantile.key)}
              />
            </div>
          {/each}
        </div>
      </div>
      {#if onSelect && hasColumnSelectionSummary}
        <ChartSticky
          anchor={selectionAnchor}
          positioner={positionSelectionCard}
        >
          <div class="metric-heatmap-chart__selection-legend">
            <ChartSelectionLegend
              variant="columns"
              timestamp={columnSelectionTimestamp}
              rowColumns={columnSelectionRowColumns}
              onDismiss={onClearSelection ? dismissSelection : undefined}
            >
              {#snippet leading()}
                {#if selectedCell}
                  <div class="heatmap-selected-cell">
                    <dl>
                      <div>
                        <dt>bucket</dt>
                        <dd>
                          {selectedCell.bucketLabel}{selectedCell.bucketLabel !==
                            'all values' &&
                          unit.trim() &&
                          unit.trim() !== '1'
                            ? ` ${unit.trim()}`
                            : ''}
                        </dd>
                      </div>
                      <div>
                        <dt>count</dt>
                        <dd>{formatMetricCount(selectedCell.count)}</dd>
                      </div>
                    </dl>
                  </div>
                {/if}
                <div class="heatmap-selection-heading">Column summary</div>
              {/snippet}
            </ChartSelectionLegend>
          </div>
        </ChartSticky>
      {/if}
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

  .heatmap-contributions {
    pointer-events: none;
  }

  .heatmap-contribution-texture {
    stroke: color-mix(
      in oklab,
      var(--contribution-color) 85%,
      var(--color-base-content)
    );
    stroke-width: var(--contribution-outline-width);
    stroke-linejoin: round;
  }

  .heatmap-quantile-label {
    @apply pointer-events-none absolute z-[2] inline-flex items-center gap-1.5;
    transform: translateY(-50%);
  }

  .heatmap-hover-cell {
    @apply m-0 grid items-baseline gap-x-1.5 whitespace-nowrap;
    grid-template-columns: auto auto auto auto;
  }

  .heatmap-hover-cell dt {
    color: var(--color-subtle);
  }

  .heatmap-hover-cell dt:not(:first-child) {
    margin-left: 0.5rem;
  }

  .heatmap-hover-cell dd {
    @apply m-0 font-medium tabular-nums text-base-content;
  }

  .metric-heatmap-chart__selection-legend {
    @apply min-w-0 max-w-full;
  }

  .heatmap-selection-heading {
    @apply mb-1 font-semibold text-base-content;
  }

  .heatmap-selected-cell {
    @apply mb-2 border-b border-base-300 pb-2;
  }

  .heatmap-selected-cell dl {
    @apply flex flex-wrap gap-x-4 gap-y-1;
  }

  .heatmap-selected-cell dl > div {
    @apply flex gap-2;
  }

  .heatmap-selected-cell dt {
    color: var(--color-subtle);
  }

  .heatmap-selected-cell dd {
    @apply m-0 text-base-content;
  }

  .metric-heatmap-chart__selection-legend
    :global(.chart-selection-legend--columns) {
    width: max-content;
    min-width: 0;
    max-width: 100%;
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

  .heatmap-wrapper {
    position: relative;
  }

  .heatmap-wrapper :global(.heatmap-cell) {
    stroke: none;
    fill-opacity: 0;
    shape-rendering: crispEdges;
  }

  .heatmap-wrapper :global(.heatmap-blobs--joined .heatmap-cell) {
    fill-opacity: 1;
  }

  .heatmap-wrapper :global(.heatmap-blob-fill),
  .heatmap-wrapper :global(.heatmap-colour-corners) {
    stroke: none;
    pointer-events: none;
  }

  .heatmap-wrapper :global(.heatmap-blob-outline) {
    fill: none;
    stroke: color-mix(
      in oklab,
      var(--blob-color) 85%,
      var(--color-base-content)
    );
    /* Each region's clip retains only the inner half of its outline. */
    stroke-width: var(--blob-outline-width);
    stroke-linejoin: round;
    pointer-events: none;
  }

  .heatmap-wrapper :global(.heatmap-joined-outline) {
    --heatmap-cell-colour-weight: 85%;
    stroke: none;
    pointer-events: none;
    shape-rendering: crispEdges;
  }

  .heatmap-wrapper :global(.heatmap-cell--hovered),
  .heatmap-wrapper :global(.heatmap-colour-corner--hovered) {
    --heatmap-cell-colour-weight: 88%;
    fill-opacity: 1;
  }

  .heatmap-wrapper :global(.heatmap-cell--selected),
  .heatmap-wrapper :global(.heatmap-colour-corner--selected) {
    --heatmap-cell-colour-weight: 84%;
    fill-opacity: 1;
  }

  /* Pointer affordance applies only when column selection is wired. */
  .heatmap-wrapper--clickable {
    @apply cursor-pointer;
  }
</style>
