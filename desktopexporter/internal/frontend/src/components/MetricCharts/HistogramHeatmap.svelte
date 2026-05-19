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
  import { scaleBand, scaleOrdinal, scaleQuantize } from 'd3-scale'
  import type {
    BucketSeriesPoint,
    HistogramBucketPoint,
    ExpHistogramBucketPoint,
  } from '@/types/api-types'
  import {
    adaptiveStepCount,
    getHeatmapSwatches,
    legendBinEdges,
  } from '@/utils/heatmap-palette'
  import { themeSignal } from '@/utils/theme-signal.svelte'
  import MetricChartEmpty from '@/components/MetricChartEmpty.svelte'
  import {
    axisCount,
    axisTime,
    chartPadding,
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
    height = 300,
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

  let countDomain = $derived([0, maxCount] as [number, number])

  // Adaptive step count: more distinct cell values -> more swatches, capped
  // at 9. Sparse data with only a handful of distinct values gets a smaller
  // ramp so the legend has fewer indistinguishable bins.
  let distinctCounts = $derived.by(() => {
    const seen = new Set<number>()
    for (const d of heatmapData) seen.add(d.count)
    return seen.size
  })

  let swatchSteps = $derived(adaptiveStepCount(distinctCounts))

  // Recompute swatches when step count or theme changes (moon vs dawn
  // use different hand-tuned ramps in heatmap-palette.ts).
  let swatches = $derived.by(() => {
    return getHeatmapSwatches(swatchSteps, themeSignal.value)
  })

  let legendLabels = $derived.by(() => {
    const edges = legendBinEdges(maxCount, swatchSteps)
    return swatches.map((_, i) => {
      const lo = Math.round(edges[i])
      const hi = Math.round(edges[i + 1])
      const close = i === swatches.length - 1 ? ']' : ')'
      return `[${lo}\u2009–\u2009${hi}${close}`
    })
  })

  let legendScale = $derived(
    scaleOrdinal<string, string>().domain(legendLabels).range(swatches)
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

  // --- Cell aspect clamping ---
  //
  // Without intervention scaleBand divides the available pixels evenly,
  // which gives stripey wide cells for sparse time data and hairline cells
  // for dense data. We aim for roughly square cells, clamped to a
  // [0.5x, 2x] aspect. When the natural chart size exceeds the container,
  // the wrapper scrolls (horizontally for too-wide, vertically for
  // too-many-buckets) instead of stretching/squishing.

  // Floor under cell height so a 200-bucket ExpHist doesn't smush rows
  // into 1px slivers. Used both to clamp aspect and to decide whether the
  // chart needs to grow taller than its container (and scroll).
  const MIN_CELL_HEIGHT = 6

  // clientWidth of the outer measurement wrapper. Bound below; starts at
  // 0 before mount.
  let containerWidth = $state(0)

  let plotWidth = $derived(Math.max(0, containerWidth))
  let plotHeightContainer = $derived(Math.max(0, height))

  // Natural cell height in the container: the plot area divided by bucket
  // count, but never below MIN_CELL_HEIGHT. When MIN_CELL_HEIGHT is the
  // binding constraint the plot has to grow taller than the container,
  // and the wrapper scrolls.
  let cellHeight = $derived.by(() => {
    if (bucketDomain.length === 0) return MIN_CELL_HEIGHT
    return Math.max(MIN_CELL_HEIGHT, plotHeightContainer / bucketDomain.length)
  })

  // Target cell width: clamp to [0.5x, 2x] cell height. Anything wider
  // than 2x looks like a stripe; anything narrower than 0.5x looks like a
  // hairline.
  let targetCellWidth = $derived.by(() => {
    if (timeDomain.length === 0) return cellHeight
    const natural = plotWidth / timeDomain.length
    const minW = 0.5 * cellHeight
    const maxW = 2 * cellHeight
    return Math.max(minW, Math.min(maxW, natural))
  })

  let chartWidth = $derived.by(() => {
    if (timeDomain.length === 0 || containerWidth === 0) return containerWidth
    const naturalPlot = targetCellWidth * timeDomain.length
    return Math.max(containerWidth, naturalPlot)
  })

  let chartHeight = $derived.by(() => {
    if (bucketDomain.length === 0) return height
    const naturalPlot = cellHeight * bucketDomain.length
    return Math.max(height, naturalPlot)
  })

  let plotChartWidth = $derived(Math.max(0, chartWidth))

  // Effective cell width as the band scale would resolve it. timeDomain
  // length is the divisor: same math layerchart does internally for
  // scaleBand. Avoids reaching into the chart context just to read
  // bandwidth() (which would mean nesting another component).
  let effectiveCellWidth = $derived.by(() => {
    if (timeDomain.length === 0) return 0
    return plotChartWidth / timeDomain.length
  })

  function handleHeatmapClick(event: MouseEvent) {
    if (!onSelect || timeDomain.length === 0 || effectiveCellWidth <= 0) return
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
    const plotX = event.clientX - rect.left
    if (plotX < 0 || plotX > plotChartWidth) return
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
    return idx * effectiveCellWidth
  })
</script>

{#if heatmapData.length === 0}
  <MetricChartEmpty {height} message="No bucket data in range" />
{:else}
  <div class="heatmap-chart metric-chart-view">
    <!-- Outer wrapper measures container width via bind:clientWidth so the
         cell-aspect math has a real number to work with. Inner scroll
         wrapper is fixed to the requested height; if the chart grows past
         the container in either direction (too many time bins -> wider,
         too many buckets -> taller), this is what scrolls. -->
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
            cScale={scaleQuantize()}
            cDomain={countDomain}
            cRange={swatches}
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
                  y={0}
                  width={effectiveCellWidth}
                  height={chartHeight}
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
              placement="bottom"
              variant="swatches"
              classes={{
                root: 'px-2 rounded-full',
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
    @apply w-full;
  }

  /* Scrolls in both axes when the chart grows past its container.
     Horizontal scroll = too many time bins (clamped at minCellWidth).
     Vertical scroll = too many buckets (clamped at minCellHeight).
     Just the heatmap scrolls -- the surrounding detail panel stays
     reachable so the metadata table never disappears off the bottom. */
  .heatmap-scroll {
    @apply w-full overflow-auto;
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
