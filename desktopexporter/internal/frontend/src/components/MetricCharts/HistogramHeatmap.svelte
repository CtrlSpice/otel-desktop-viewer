<script lang="ts">
  import { Chart, Cell, Axis, Highlight, Layer, Rect, Tooltip } from 'layerchart'
  import { scaleBand, scaleQuantize } from 'd3-scale'
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
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatDateTime, type Timezone } from '@/utils/time'

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
    /** Window in ms; used only to pick the axis-tier (seconds vs date),
     * not for fetching. Same window the parent passed to
     * getMetricBucketSeries. */
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

  // --- Time formatting -------------------------------------------------
  //
  // Two surfaces care about timestamps:
  //
  //   * Axis labels: short strings, lots of them, no room for the
  //     full date+timezone-suffix that the project's standard
  //     formatDateTime returns. We pick a tier of precision based on
  //     the chart's TIME RANGE, so a 30-second heatmap shows seconds,
  //     a 7-day heatmap shows just dates, and the in-between cases
  //     get exactly the precision they need to disambiguate samples
  //     from each other. Crucially, ranges that span midnight (or
  //     longer) include the date in every label, which is the actual
  //     fix for the "two days, both labelled 14:25:00" footgun the
  //     numeric-keying change exposed.
  //
  //   * Tooltip: only ever one shown at a time, the user is asking
  //     for precision, so we delegate to the project-standard
  //     formatDateTime with millisecond resolution and timezone tag.
  //     Stays consistent with how trace/log details show timestamps.
  //
  // The axis helper uses Intl.DateTimeFormat.formatToParts() so we
  // can assemble the string from raw parts (no locale-dependent
  // commas, no "May 9, 14:25"). 'en-US' is the locale only because
  // it gives the part shapes we want; the timezone is the user's
  // choice (timeContext.timezone), so 'UTC' selection swaps the
  // numbers correctly. If we ever want to honour the user's locale
  // for month names, swap 'en-US' for navigator.language here.

  type AxisTier = 'seconds' | 'minutes' | 'datetime' | 'date'
  const HOUR_MS = 60 * 60 * 1000
  const DAY_MS = 24 * HOUR_MS
  const WEEK_MS = 7 * DAY_MS

  function pickAxisTier(spanMs: number): AxisTier {
    if (spanMs < HOUR_MS) return 'seconds'
    if (spanMs < DAY_MS) return 'minutes'
    if (spanMs < WEEK_MS) return 'datetime'
    return 'date'
  }

  function intlOptionsFor(
    tier: AxisTier,
    timezone: Timezone
  ): Intl.DateTimeFormatOptions {
    const base: Intl.DateTimeFormatOptions = { hour12: false }
    if (timezone === 'UTC') base.timeZone = 'UTC'
    switch (tier) {
      case 'seconds':
        return { ...base, hour: '2-digit', minute: '2-digit', second: '2-digit' }
      case 'minutes':
        return { ...base, hour: '2-digit', minute: '2-digit' }
      case 'datetime':
        return {
          ...base,
          month: 'short',
          day: 'numeric',
          hour: '2-digit',
          minute: '2-digit',
        }
      case 'date':
        return { ...base, month: 'short', day: 'numeric' }
    }
  }

  function formatAxisTime(
    ms: number,
    timezone: Timezone,
    tier: AxisTier
  ): string {
    const fmt = new Intl.DateTimeFormat('en-US', intlOptionsFor(tier, timezone))
    const parts = fmt.formatToParts(new Date(ms))
    const grab = (t: Intl.DateTimeFormatPartTypes) =>
      parts.find(p => p.type === t)?.value ?? ''
    const hh = grab('hour')
    const mm = grab('minute')
    const ss = grab('second')
    const mon = grab('month')
    const day = grab('day')
    switch (tier) {
      case 'seconds':
        return `${hh}:${mm}:${ss}`
      case 'minutes':
        return `${hh}:${mm}`
      case 'datetime':
        return `${mon} ${day} ${hh}:${mm}`
      case 'date':
        return `${mon} ${day}`
    }
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
          label = bounds.length > 0 ? `≥${formatBound(bounds[bounds.length - 1])}` : '0'
        }
        data.push({ time, bucket: label, count: counts[i] })
      }
    }
    return data
  }

  function buildExpHistogramData(pts: ExpHistogramBucketPoint[]): HeatmapDatum[] {
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
        data.push({ time, bucket: formatBound(mid), count: pt.positiveCounts[i] })
      }

      for (let i = 0; i < pt.negativeCounts.length; i++) {
        const idx = pt.negativeOffset + i
        const mid = (-Math.pow(base, idx + 1) + -Math.pow(base, idx)) / 2
        data.push({ time, bucket: formatBound(mid), count: pt.negativeCounts[i] })
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

  let legendEdges = $derived(legendBinEdges(maxCount, swatchSteps))

  function formatLegendEdge(v: number): string {
    if (v >= 1000) return v.toExponential(1)
    if (Number.isInteger(v)) return v.toString()
    return v.toFixed(1)
  }

  // X-axis labelling strategy:
  //   * One TICK per data column (full ruler -- every datapoint the
  //     backend returned gets its own tick on the bottom edge so the
  //     user can see "yes, my data has N samples").
  //   * Labels are thinned so they don't overlap. Density is driven by
  //     chart width: each rotated label needs ~PIXELS_PER_LABEL of
  //     horizontal room (we rotate 315 degrees, see <Axis> below).
  //     Number labelable = max(2, floor(width / PIXELS_PER_LABEL)).
  //     We then walk timeDomain and pick `labelable` indices spaced
  //     evenly via Math.round(i * (n-1) / (labelable-1)) so the first
  //     and last samples are ALWAYS labelled and the middle ones land
  //     at uniform positions.
  //
  // Y-axis stays on the older mod-N ticks-and-labels-together approach
  // -- bucket edges are numeric values, not times, and changing both
  // axes in one pass risked too many moving parts. Defer.
  const PIXELS_PER_LABEL = 60

  let labelableTimestamps = $derived.by(() => {
    const n = timeDomain.length
    if (n === 0) return new Set<number>()
    const budget = chartWidth > 0
      ? Math.max(2, Math.floor(chartWidth / PIXELS_PER_LABEL))
      : 8
    if (n <= budget) return new Set(timeDomain)
    const set = new Set<number>()
    for (let i = 0; i < budget; i++) {
      const idx = Math.round((i * (n - 1)) / (budget - 1))
      set.add(timeDomain[idx])
    }
    return set
  })

  // Tier picked once per render based on the requested heatmap window
  // (NOT the data span -- a 7-day window with one cluster of data
  // still wants date-aware labels because the user is asking about
  // 7 days). Reactive via $derived so changing the time picker
  // re-tiers labels immediately.
  let axisTier = $derived(pickAxisTier(windowEndMs - windowStartMs))

  // Tick label formatter for layerchart's <Axis format={...}>. Receives
  // each tick's domain value (a timestamp number, since we changed
  // HeatmapDatum.time to number above). Returns the formatted clock
  // string for "labelable" timestamps and an empty string otherwise --
  // the tick mark itself still renders, only the text is suppressed.
  function formatTimeTick(value: unknown): string {
    if (typeof value !== 'number') return ''
    if (!labelableTimestamps.has(value)) return ''
    return formatAxisTime(value, timeContext.timezone, axisTier)
  }

  let visibleBucketTicks = $derived.by(() => {
    const n = bucketDomain.length
    if (n <= 8) return bucketDomain
    const step = Math.ceil(n / 7)
    return bucketDomain.filter((_, i) => i % step === 0)
  })

  // --- Cell aspect clamping ---
  //
  // Without intervention scaleBand divides the available pixels evenly,
  // which gives stripey wide cells for sparse time data and hairline cells
  // for dense data. We aim for roughly square cells, clamped to a
  // [0.5x, 2x] aspect. When the natural chart size exceeds the container,
  // the wrapper scrolls (horizontally for too-wide, vertically for
  // too-many-buckets) instead of stretching/squishing.

  // Chart padding values must match the <Chart padding={...} /> below so
  // that "plot area = container minus padding" math stays honest.
  const PAD_TOP = 8
  const PAD_RIGHT = 8
  const PAD_BOTTOM = 48
  const PAD_LEFT = 56

  // Floor under cell height so a 200-bucket ExpHist doesn't smush rows
  // into 1px slivers. Used both to clamp aspect and to decide whether the
  // chart needs to grow taller than its container (and scroll).
  const MIN_CELL_HEIGHT = 6

  // clientWidth of the outer measurement wrapper. Bound below; starts at
  // 0 before mount.
  let containerWidth = $state(0)

  // Plot area dimensions (excluding axis padding) computed from the
  // measured container width and the prop-driven container height.
  let plotWidth = $derived(Math.max(0, containerWidth - PAD_LEFT - PAD_RIGHT))
  let plotHeightContainer = $derived(Math.max(0, height - PAD_TOP - PAD_BOTTOM))

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

  // Total chart dimensions including padding. If the natural plot width
  // exceeds container plot width, chartWidth will exceed containerWidth
  // and the wrapper scrolls horizontally. Same for chartHeight.
  let chartWidth = $derived.by(() => {
    if (timeDomain.length === 0 || containerWidth === 0) return containerWidth
    const naturalPlot = targetCellWidth * timeDomain.length
    return Math.max(containerWidth, naturalPlot + PAD_LEFT + PAD_RIGHT)
  })

  let chartHeight = $derived.by(() => {
    if (bucketDomain.length === 0) return height
    const naturalPlot = cellHeight * bucketDomain.length
    return Math.max(height, naturalPlot + PAD_TOP + PAD_BOTTOM)
  })

  // Plot area width inside the chart (excluding axis padding). Used both
  // by the click-to-select handler and the selection-highlight rect.
  let plotChartWidth = $derived(Math.max(0, chartWidth - PAD_LEFT - PAD_RIGHT))

  // Effective cell width as the band scale would resolve it. timeDomain
  // length is the divisor: same math layerchart does internally for
  // scaleBand. Avoids reaching into the chart context just to read
  // bandwidth() (which would mean nesting another component).
  let effectiveCellWidth = $derived.by(() => {
    if (timeDomain.length === 0) return 0
    return plotChartWidth / timeDomain.length
  })

  // Click router: figure out which time-band the user hit and forward
  // that timestamp to the parent's onSelect handler. The math mirrors
  // layerchart's band scale (which is why we need exact PAD_LEFT and
  // plotChartWidth -- any drift between this and the chart's internal
  // sizing puts the click index off by one). currentTarget is the
  // wrapper div; offsetX is relative to its content box, which starts
  // at the chart's (0,0) since the chart fills the wrapper.
  function handleHeatmapClick(event: MouseEvent) {
    if (!onSelect || timeDomain.length === 0 || effectiveCellWidth <= 0) return
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
    const localX = event.clientX - rect.left
    const plotX = localX - PAD_LEFT
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
    if (selectedTimestamp === null || selectedTimestamp === undefined) return null
    const idx = timeDomain.indexOf(selectedTimestamp)
    if (idx < 0) return null
    return idx * effectiveCellWidth
  })
</script>

{#if heatmapData.length === 0}
  <div
    class="flex items-center justify-center text-base-content/40 text-sm"
    style:height="{height}px"
  >
    No bucket data in range
  </div>
{:else}
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
        onkeydown={(e) => {
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
          padding={{ top: PAD_TOP, right: PAD_RIGHT, bottom: PAD_BOTTOM, left: PAD_LEFT }}
          width={chartWidth}
          height={chartHeight}
          tooltipContext={{ mode: 'band' }}
        >
          <Layer>
            <!-- Full-density tick ruler: one tick per data column. The
                 `format` callback decides which ticks ALSO get a text
                 label (returning '' suppresses just the text). -->
            <Axis
              placement="bottom"
              rule
              ticks={timeDomain}
              format={formatTimeTick}
              tickLabelProps={{
                rotate: 315,
                textAnchor: 'end',
                verticalAnchor: 'middle',
                dy: 8,
              }}
            />
            <Axis placement="left" rule ticks={visibleBucketTicks} />
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
                height={chartHeight - PAD_TOP - PAD_BOTTOM}
                class="heatmap-selection"
              />
            {/if}
            <Highlight area />
          </Layer>
          <Tooltip.Root>
            {#snippet children({ data }: { data: HeatmapDatum })}
              <Tooltip.Header class="text-center">{formatTooltipTime(data.time)}</Tooltip.Header>
              <Tooltip.List>
                <Tooltip.Item label="bucket" value={data.bucket} />
                <Tooltip.Separator />
                <Tooltip.Item label="count" value={data.count} format="integer" />
              </Tooltip.List>
            {/snippet}
          </Tooltip.Root>
        </Chart>
      </div>
    </div>
  </div>

  <!-- Legend strip: same swatch array the heatmap uses, plus bin edge
       labels so a colour can be translated back to a count range. Without
       this the heatmap is just "warm = more", which is fine for ambient
       awareness but useless for actual analysis. -->
  <div class="heatmap-legend" aria-label="Heatmap legend">
    <span class="heatmap-legend__edge tabular-nums">{formatLegendEdge(legendEdges[0])}</span>
    <div class="heatmap-legend__swatches">
      {#each swatches as swatch, i (i)}
        <span
          class="heatmap-legend__swatch"
          style:background-color={swatch}
          title={`${formatLegendEdge(legendEdges[i])} – ${formatLegendEdge(legendEdges[i + 1])}`}
        ></span>
      {/each}
    </div>
    <span class="heatmap-legend__edge tabular-nums">
      {formatLegendEdge(legendEdges[legendEdges.length - 1])}
    </span>
  </div>
{/if}

<style lang="postcss">
  @reference "../../app.css";

  .heatmap-measure {
    @apply w-full;
  }

  /* Scrolls in both axes when the chart grows past its container.
     Horizontal scroll = too many time bins (clamped at minCellWidth).
     Vertical scroll = too many buckets (clamped at minCellHeight).
     Just the heatmap scrolls -- the surrounding detail panel stays
     reachable so the metadata table never disappears off the bottom. */
  .heatmap-scroll {
    @apply w-full overflow-auto rounded-lg;
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

  .heatmap-legend {
    @apply mt-2 flex items-center justify-center gap-2 px-2 text-[0.65rem] text-base-content/55;
  }

  .heatmap-legend__edge {
    @apply shrink-0;
  }

  .heatmap-legend__swatches {
    @apply flex h-2 max-w-[16rem] flex-1 overflow-hidden rounded-full;
  }

  .heatmap-legend__swatch {
    @apply h-full flex-1;
  }
</style>
