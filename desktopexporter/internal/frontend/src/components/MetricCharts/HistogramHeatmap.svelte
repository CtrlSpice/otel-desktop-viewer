<script lang="ts">
  import {
    Chart,
    Cell,
    Axis,
    Highlight,
    Layer,
    Legend,
    Rect,
    Tooltip,
  } from 'layerchart'
  import { scaleBand, scaleOrdinal, scaleThreshold } from 'd3-scale'
  import type {
    BucketSeriesPoint,
    HistogramBucketPoint,
    ExpHistogramBucketPoint,
  } from '@/types/api-types'
  import {
    adaptiveStepCount,
    legendBinEdges,
  } from '@/utils/heatmap-palette'
  import { heatmapSwatches } from '@/utils/chart-palette'
  import { themeSignal } from '@/utils/theme-signal.svelte'
  import MetricChartEmpty from '@/components/MetricChartEmpty.svelte'
  import {
    axisCount,
    axisTime,
    chartPadding,
    DEFAULT_METRIC_CHART_HEIGHT,
  } from '@/components/MetricChartPlot.svelte'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatDateTime } from '@/utils/time'

  const timeContext = getTimeContext()

  // `time` is the raw bucket-start timestamp in milliseconds. We
  // intentionally do NOT pre-format it here -- a formatted string
  // would silently collapse two datapoints sharing the same wall-clock
  // time-of-day on different days into a single column. Keeping the
  // band scale's domain numeric is the cheap fix; the axis formatter
  // and tooltip render the human label at display time only.
  type HeatmapDatum = {
    time: number
    bucket: string
    count: number
  }

  // The fetch was lifted up to MetricViewContext so the Heatmap and
  // Aggregated tabs can share one bucket-series request. This component is
  // now purely a renderer: parent supplies `points` (plus the window range
  // so the axis-tier picker still works), and the parent owns loading /
  // error / temporality-callout states.
  //
  // selectedTimestamp + onSelect implement the heatmap-click ->
  // snapshot-tab interaction: clicking a Cell calls onSelect with that
  // column's timestamp; the parent resolves it to a datapoint id and
  // switches tabs. selectedTimestamp drives a column highlight so the
  // user can scan-locate which column corresponds to the active snapshot.
  type Props = {
    points: BucketSeriesPoint[]
    /** Query window in ms (same range passed to getMetricBucketSeries). */
    windowStartMs: number
    windowEndMs: number
    height?: number
    /** Click handler. Receives the bucket-start timestamp in ms. */
    onSelect?: (timestampMs: number) => void
    /** When set, the matching column gets a highlight. ms timestamp. */
    selectedTimestamp?: number | null
  }

  let {
    points,
    windowStartMs,
    windowEndMs,
    height = DEFAULT_METRIC_CHART_HEIGHT,
    onSelect,
    selectedTimestamp = null,
  }: Props = $props()

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
      return buildHistogramData(points as HistogramBucketPoint[])
    }
    return buildExpHistogramData(points as ExpHistogramBucketPoint[])
  })

  function buildHistogramData(pts: HistogramBucketPoint[]): HeatmapDatum[] {
    const data: HeatmapDatum[] = []
    for (const pt of pts) {
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
        if (i === 0) {
          label = bounds.length > 0 ? `≤${formatBound(bounds[0])}` : '0'
        } else if (i < bounds.length) {
          label = formatBound((bounds[i - 1] + bounds[i]) / 2)
        } else {
          label =
            bounds.length > 0
              ? `≥${formatBound(bounds[bounds.length - 1])}`
              : '0'
        }
        data.push({ time, bucket: label, count: counts[i] })
      }
    }
    return data
  }

  function buildExpHistogramData(
    pts: ExpHistogramBucketPoint[]
  ): HeatmapDatum[] {
    const data: HeatmapDatum[] = []
    for (const pt of pts) {
      const time = tsToMs(pt.timestamp)
      const base = Math.pow(2, Math.pow(2, -pt.scale))

      if (pt.zeroCount > 0) {
        data.push({ time, bucket: '0', count: pt.zeroCount })
      }

      for (let i = 0; i < pt.positiveCounts.length; i++) {
        const idx = pt.positiveOffset + i
        const mid = (Math.pow(base, idx) + Math.pow(base, idx + 1)) / 2
        data.push({
          time,
          bucket: formatBound(mid),
          count: pt.positiveCounts[i],
        })
      }

      for (let i = 0; i < pt.negativeCounts.length; i++) {
        const idx = pt.negativeOffset + i
        const mid = (-Math.pow(base, idx + 1) + -Math.pow(base, idx)) / 2
        data.push({
          time,
          bucket: formatBound(mid),
          count: pt.negativeCounts[i],
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
    const seen = new Set<string>()
    const ordered: string[] = []
    for (const d of heatmapData) {
      if (!seen.has(d.bucket)) {
        seen.add(d.bucket)
        ordered.push(d.bucket)
      }
    }
    return ordered.reverse()
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

  let swatchSteps = $derived(adaptiveStepCount(distinctNonZeroCounts))

  // Active-range swatches only -- 0 is the "empty" swatch, prepended below.
  let swatches = $derived.by(() => {
    return heatmapSwatches(swatchSteps, themeSignal.value)
  })

  // Threshold breakpoints over (0, maxCount]. scaleThreshold returns
  // range[0] for any input < domain[0], so domain[0] = 1 means
  // "count === 0 -> empty swatch" and the rest of the range covers
  // positive counts uniformly.
  let cellColorThresholds = $derived.by(() => {
    if (swatchSteps <= 0 || maxCount <= 0) return [1]
    const out: number[] = new Array(swatchSteps)
    out[0] = 1
    for (let i = 1; i < swatchSteps; i++) {
      out[i] = (i * maxCount) / swatchSteps
    }
    return out
  })

  // Empty swatch (matches chart surface) + active ramp; length = thresholds + 1.
  let cellColorRange = $derived(['var(--color-base-200)', ...swatches])

  // Legend labels mirror the threshold structure: '0' first, then '(prev - next]'
  // for each active swatch. legendBinEdges divides [0, max] into swatchSteps slots;
  // we treat the first slot as (0 - max/N], etc.
  let legendLabels = $derived.by(() => {
    const edges = legendBinEdges(maxCount, swatchSteps)
    const labels: string[] = ['0']
    for (let i = 0; i < swatches.length; i++) {
      const lo = i === 0 ? 0 : Math.round(edges[i])
      const hi = Math.round(edges[i + 1])
      const close = i === swatches.length - 1 ? ']' : ')'
      labels.push(`(${lo}\u2009–\u2009${hi}${close}`)
    }
    return labels
  })

  // Legend skips the '0' swatch entirely -- empty cells already read as
  // "nothing happened" because they match the chart surface, so the legend
  // only documents the active (positive-count) buckets.
  let legendScale = $derived(
    scaleOrdinal<string, string>()
      .domain(legendLabels.slice(1))
      .range(cellColorRange.slice(1))
  )

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
  // Time columns: fixed TIME_COLUMN_WIDTH; scroll horizontally when wide.
  // Bucket rows: share the pane height evenly (ExpHist compresses, no
  // vertical scroll).

  /** Fixed time-column width in the plot area (px). */
  const TIME_COLUMN_WIDTH = 16
  const PLOT_INSET_X = chartPadding.left + chartPadding.right
  const PLOT_INSET_Y = chartPadding.top + chartPadding.bottom

  // clientWidth of the outer measurement wrapper. Bound below; starts at
  // 0 before mount.
  let containerWidth = $state(0)

  let plotHeightAvailable = $derived(
    Math.max(0, height - PLOT_INSET_Y)
  )

  let chartWidth = $derived.by(() => {
    if (timeDomain.length === 0) return Math.max(containerWidth, 1)
    return TIME_COLUMN_WIDTH * timeDomain.length + PLOT_INSET_X
  })

  let chartHeight = $derived(height)

  let effectiveCellWidth = TIME_COLUMN_WIDTH

  function handleHeatmapClick(event: MouseEvent) {
    if (!onSelect || timeDomain.length === 0 || effectiveCellWidth <= 0) return
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
    const plotX = event.clientX - rect.left - chartPadding.left
    const plotW = TIME_COLUMN_WIDTH * timeDomain.length
    if (plotX < 0 || plotX > plotW) return
    const idx = Math.floor(plotX / effectiveCellWidth)
    if (idx < 0 || idx >= timeDomain.length) return
    onSelect(timeDomain[idx])
  }

  // Persistent column highlight for the active selection. Resolved to
  // the band's pixel x by the band's own index (same math as the click
  // router, run in reverse). Rendered as a <Rect> in the SVG layer with
  // a translucent fill + thin border so it reads as "this column is
  // selected" without obscuring the cells underneath.
  let selectedColumnX = $derived.by(() => {
    if (selectedTimestamp === null || selectedTimestamp === undefined)
      return null
    const idx = timeDomain.indexOf(selectedTimestamp)
    if (idx < 0) return null
    return chartPadding.left + idx * effectiveCellWidth
  })
</script>

{#if heatmapData.length === 0}
  <MetricChartEmpty {height} message="No bucket data in range" />
{:else}
  <div class="heatmap-chart metric-chart-view">
    <!-- Outer wrapper measures width; height is fixed to the chart pane.
         Horizontal scroll only when there are many time columns. -->
    <div
      class="heatmap-measure"
      style:height="{height}px"
      bind:clientWidth={containerWidth}
    >
      <div class="heatmap-scroll" style:height="{height}px">
        <!-- onclick lives on the wrapper (not on each Cell) because Cell
           iterates internally and doesn't surface the bound datum on the
           click event. Resolving from offsetX -> band index keeps the
           per-cell binding implicit, and we don't have to fight the
           chart context boundary. role="button" + tabindex make it
           keyboard/AT reachable; the listener is no-op when no onSelect
           is passed (Gauge/Sum case). svelte-ignore is needed because
           the compiler can't statically prove role is set in the
           tabindex=0 branch (we wire both via the same onSelect prop). -->
        <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
        <div
          class="heatmap-wrapper"
          class:heatmap-wrapper--clickable={!!onSelect}
          style:width="{chartWidth}px"
          style:height="{chartHeight}px"
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
            xScale={scaleBand().padding(0)}
            xDomain={timeDomain}
            y="bucket"
            yScale={scaleBand().padding(0)}
            yDomain={bucketDomain}
            c="count"
            cScale={scaleThreshold()}
            cDomain={cellColorThresholds}
            cRange={cellColorRange}
            width={chartWidth}
            height={chartHeight}
            padding={chartPadding}
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
                {...axisCount()}
                ticks={visibleBucketTicks}
              />
              <Cell x="time" y="bucket" fill="count" />
              {#if selectedColumnX !== null}
                <!-- Persistent selection ring around the active column.
                   Rendered before <Highlight> so the hover highlight
                   still wins visually when the user is mousing over a
                   different column. Pixel mode + plot-area coords. -->
                <Rect
                  x={selectedColumnX}
                  y={chartPadding.top}
                  width={effectiveCellWidth}
                  height={plotHeightAvailable}
                  class="heatmap-selection"
                />
              {/if}
              <Highlight area />
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
            <Legend
              scale={legendScale}
              placement="top-left"
              variant="swatches"
              classes={{
                root: 'heatmap-legend px-2 rounded-full',
                title: 'text-xs',
                label: 'text-xs text-rp-subtle',
                tick: 'stroke-base-200',
              }}
            />
          </Chart>
        </div>
      </div>
    </div>
  </div>
{/if}

<style lang="postcss">
  @reference "../../app.css";

  .heatmap-chart {
    @apply w-full;
  }

  .heatmap-measure {
    @apply w-full overflow-hidden;
  }

  /* Tall bucket grids stay clipped to the pane; only pan sideways in time. */
  .heatmap-scroll {
    @apply w-full overflow-x-auto overflow-y-hidden;
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
