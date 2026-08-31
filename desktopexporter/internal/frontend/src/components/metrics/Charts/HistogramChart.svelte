<script lang="ts">
  import { BarChart, Line, Rect, Tooltip, scaleInvert } from 'layerchart'
  import { scaleBand, scaleLinear } from 'd3-scale'
  import ChartKeyboardSurface from '@/components/metrics/Charts/ChartKeyboardSurface.svelte'
  import MetricChartEmpty from '@/components/metrics/Charts/MetricChartEmpty.svelte'
  import ChartSelectionLegend, {
    type SelectionLegendRow,
  } from '@/components/metrics/Charts/ChartSelectionLegend.svelte'
  import ChartTimeRangeHeader from '@/components/metrics/Charts/ChartTimeRangeHeader.svelte'
  import MetricChartPlot, {
    axisBuckets,
    axisCount,
    chartPadding,
    DEFAULT_METRIC_CHART_HEIGHT,
  } from '@/components/metrics/Charts/MetricChartPlot.svelte'
  import { metricTypeSeriesColor } from '@/components/metrics/utils/metric-type'
  import { expBuckets } from '@/components/metrics/utils/histogram-quantile'
  import { formatMetricValue } from '@/components/metrics/utils/format-metric-value'
  import {
    exponentialBucketIdentity,
    exponentialZeroBucketIdentity,
    exponentialZeroBucketLabel,
    formatHistogramBound,
    indexedHistogramRangeIdentity,
  } from '@/components/metrics/utils/histogram-bucket'
  import {
    isChartActivationKey,
    orderedCursorCommandForKey,
  } from '@/components/metrics/utils/chart-keyboard-cursor'
  import { createOrderedChartKeyboardCursor } from '@/components/metrics/utils/chart-keyboard-state.svelte'
  import type {
    HistogramDataPoint,
    ExponentialHistogramDataPoint,
  } from '@/types/api-types'

  // lo/hi are the numeric bucket bounds; they may be -Infinity, +Infinity, or
  // (for an exact exp-histogram zero bucket) both 0. Used to position quantile
  // markers within their bar.
  type Bucket = {
    key: string
    label: string
    count: number
    lo: number
    hi: number
    zeroThreshold?: number
  }

  type Props = {
    datapoint: HistogramDataPoint | ExponentialHistogramDataPoint
    /** Metric `unit` for axis labelling (e.g. "ms", "bytes"). Optional;
     * the x-axis title shows just "value" when unit is empty. */
    unit?: string
    height?: number
    timeRange?: { startMs: number; endMs: number } | null
    /** Formatted snapshot timestamp for snapshot-scope header. */
    selectionTimestamp?: string
    /** Whole-window mode: click a column to pin bucket summary in the header. */
    enableValueBucketPin?: boolean
    onClearSelection?: () => void
  }

  let {
    datapoint,
    unit = '',
    height = DEFAULT_METRIC_CHART_HEIGHT,
    timeRange = null,
    selectionTimestamp = '',
    enableValueBucketPin = false,
    onClearSelection,
  }: Props = $props()

  let plotAreaHeight = $state(0)
  let pinnedBucketKey = $state<string | null>(null)

  // Hardcoded for now; the plan defers configurable quantiles to a future pass.
  // Keys must match Go's strconv.FormatFloat(q, 'f', -1, 64) output.
  const QUANTILES = [0.5, 0.95, 0.99]
  const QUANTILE_LABELS: { key: string; label: string }[] = [
    { key: '0.5', label: 'p50' },
    { key: '0.95', label: 'p95' },
    { key: '0.99', label: 'p99' },
  ]

  // The store's quantiles for this datapoint. Not computed here: that walked
  // the bucket list once per quantile for numbers already in the response.
  let quantiles = $derived(
    Object.fromEntries(
      QUANTILES.map(q => [String(q), datapoint.quantiles?.[String(q)] ?? null])
    ) as Record<string, number | null>
  )

  let buckets = $derived.by((): Bucket[] => {
    if (datapoint.metricType === 'Histogram') {
      return buildHistogramBuckets(datapoint)
    }
    return buildExpHistogramBuckets(datapoint)
  })

  let bucketByKey = $derived(
    new Map(buckets.map(bucket => [bucket.key, bucket]))
  )

  let pinnedBucket = $derived.by((): Bucket | null => {
    if (!pinnedBucketKey) return null
    return bucketByKey.get(pinnedBucketKey) ?? null
  })

  let keyboardBucketKeys = $derived(buckets.map(bucket => bucket.key))
  const keyboardNavigation = createOrderedChartKeyboardCursor(
    () => keyboardBucketKeys,
    () => pinnedBucketKey
  )
  let activeKeyboardCursor = $derived(keyboardNavigation.current)
  let keyboardFocused = $derived(keyboardNavigation.focused)

  function handleKeyboardFocusChange(focused: boolean) {
    keyboardNavigation.setFocused(focused)
  }

  let keyboardBucket = $derived.by((): Bucket | null => {
    const cursor = activeKeyboardCursor
    if (!cursor) return null
    return bucketByKey.get(String(cursor.key)) ?? null
  })

  let keyboardReadout = $derived.by((): string => {
    const cursor = activeKeyboardCursor
    const bucket = keyboardBucket
    if (!cursor || !bucket) return ''
    const snapshot = selectionTimestamp
      ? `Snapshot ${selectionTimestamp}. `
      : ''
    return (
      `${snapshot}Bucket ${cursor.index + 1} of ${buckets.length}, ${formatBucketRange(bucket)}. ` +
      `Count ${bucket.count}. ${formatPct(bucket.count)} of total.`
    )
  })
  let keyboardShortcuts = $derived([
    'ArrowLeft',
    'ArrowRight',
    'Home',
    'End',
    ...(enableValueBucketPin ? ['Enter', 'Space'] : []),
    'Escape',
  ])

  let pinDatapointID = $state<string | undefined>(undefined)

  $effect(() => {
    const id = datapoint.id
    if (pinDatapointID !== undefined && pinDatapointID !== id) {
      pinnedBucketKey = null
    }
    pinDatapointID = id
  })

  $effect(() => {
    if (!enableValueBucketPin) {
      pinnedBucketKey = null
    }
  })

  function buildHistogramBuckets(dp: HistogramDataPoint): Bucket[] {
    const bounds = dp.explicitBounds
    const counts = dp.bucketCounts
    const result: Bucket[] = []
    for (let i = 0; i < counts.length; i++) {
      let label: string
      let lo: number
      let hi: number
      if (bounds.length === 0) {
        label = 'all values'
        lo = -Infinity
        hi = Infinity
      } else if (i === 0) {
        label = `(-∞, ${bounds[0]}]`
        lo = -Infinity
        hi = bounds[0]
      } else if (i < bounds.length) {
        label = `(${bounds[i - 1]}, ${bounds[i]}]`
        lo = bounds[i - 1]
        hi = bounds[i]
      } else {
        label = `(${bounds[bounds.length - 1]}, +∞)`
        lo = bounds[bounds.length - 1]
        hi = Infinity
      }
      result.push({
        key: indexedHistogramRangeIdentity(i, lo, hi),
        label,
        count: counts[i],
        lo,
        hi,
      })
    }
    return result
  }

  function buildExpHistogramBuckets(
    dp: ExponentialHistogramDataPoint
  ): Bucket[] {
    const negativeCount = dp.negativeBucketCounts.length
    return expBuckets(
      dp.scale,
      dp.negativeBucketOffset,
      dp.negativeBucketCounts,
      dp.zeroCount,
      dp.positiveBucketOffset,
      dp.positiveBucketCounts
    ).map((bucket, index) => {
      let key: string
      const zeroThreshold =
        index === negativeCount ? dp.zeroThreshold : undefined
      if (index < negativeCount) {
        const exponent = dp.negativeBucketOffset + (negativeCount - index - 1)
        key = exponentialBucketIdentity(dp.scale, 'negative', exponent)
      } else if (index === negativeCount) {
        key = exponentialZeroBucketIdentity(dp.zeroThreshold)
      } else {
        const exponent = dp.positiveBucketOffset + (index - negativeCount - 1)
        key = exponentialBucketIdentity(dp.scale, 'positive', exponent)
      }
      return {
        key,
        label:
          zeroThreshold !== undefined
            ? exponentialZeroBucketLabel(zeroThreshold)
            : `(${formatHistogramBound(bucket.lo)}, ${formatHistogramBound(bucket.hi)}]`,
        count: bucket.cnt,
        lo:
          zeroThreshold !== undefined && zeroThreshold > 0
            ? -zeroThreshold
            : bucket.lo,
        hi:
          zeroThreshold !== undefined && zeroThreshold > 0
            ? zeroThreshold
            : bucket.hi,
        zeroThreshold,
      }
    })
  }

  type QuantileMark = {
    key: string
    label: string
    value: number
    bucketIndex: number
    // Position within the bar [0, 1]. 0 = left edge, 1 = right edge.
    fraction: number
    color: string
  }

  // Per-quantile color so p50/p95/p99 are distinguishable at a glance.
  // Falls through to base-content for anything we didn't preassign.
  const QUANTILE_COLORS: Record<string, string> = {
    '0.5': 'var(--color-info)',
    '0.95': 'var(--color-warning)',
    '0.99': 'var(--color-error)',
  }

  // Locates which bucket a value lives in and where inside the bar to draw the
  // marker. Returns null if no bucket matches (shouldn't happen for valid
  // quantile output, but we play it safe).
  //
  // For unbounded edge buckets (-∞ or +∞ side) and exact-zero buckets, we
  // can't compute a meaningful linear fraction, so we center the marker.
  function findBarPosition(
    v: number
  ): { index: number; fraction: number } | null {
    for (let i = 0; i < buckets.length; i++) {
      const { lo, hi } = buckets[i]
      if (v < lo || v > hi) continue
      if (!Number.isFinite(lo) || !Number.isFinite(hi) || hi === lo) {
        return { index: i, fraction: 0.5 }
      }
      const f = (v - lo) / (hi - lo)
      return { index: i, fraction: Math.min(1, Math.max(0, f)) }
    }
    return null
  }

  let quantileMarks = $derived.by((): QuantileMark[] => {
    const marks: QuantileMark[] = []
    for (const { key, label } of QUANTILE_LABELS) {
      const v = quantiles[key]
      if (v === null || v === undefined) continue
      const pos = findBarPosition(v)
      if (!pos) continue
      marks.push({
        key,
        label,
        value: v,
        bucketIndex: pos.index,
        fraction: pos.fraction,
        color: QUANTILE_COLORS[key] ?? 'var(--color-base-content)',
      })
    }
    return marks
  })

  const QUANTILE_ORDER: Record<string, number> = {
    '0.5': 0,
    '0.95': 1,
    '0.99': 2,
  }

  let quantileLabelPlacements = $derived.by(() => {
    const ctx = chartContext
    if (!ctx || quantileMarks.length === 0) return []

    const xs = ctx.xScale
    const ys = ctx.yScale
    const bw = typeof xs.bandwidth === 'function' ? xs.bandwidth() : 0
    const plotLeft = ctx.padding.left
    const plotTop = ctx.padding.top
    const plotCeiling = ctx.yRange[1] + plotTop + 4
    const STAGGER_PX = 32
    const COLLIDE_PX = 88

    type Draft = {
      key: string
      statLabel: string
      valueText: string
      color: string
      title: string
      left: number
      top: number
      sortOrder: number
    }

    const drafts: Draft[] = quantileMarks.map(m => {
      const bucket = buckets[m.bucketIndex]
      const x0 = xs(bucket.key)
      const px = (x0 ?? 0) + bw * m.fraction
      const valueText = formatQuantileValue(m.value)
      return {
        key: m.key,
        statLabel: m.label,
        valueText,
        color: m.color,
        title: `${m.label} ${valueText}`,
        left: px + plotLeft,
        top: ys(bucket.count) + plotTop,
        sortOrder: QUANTILE_ORDER[m.key] ?? 0,
      }
    })

    drafts.sort((a, b) => a.left - b.left || a.sortOrder - b.sortOrder)

    for (let i = 0; i < drafts.length; i++) {
      let top = drafts[i].top
      for (let j = 0; j < i; j++) {
        if (drafts[i].left - drafts[j].left < COLLIDE_PX) {
          top = Math.min(top, drafts[j].top - STAGGER_PX)
        }
      }
      drafts[i].top = Math.max(top, plotCeiling)
    }

    return drafts.map(({ sortOrder: _sortOrder, ...mark }) => mark)
  })

  function formatQuantileValue(v: number): string {
    const formatted = formatMetricValue(v)
    const u = unit.trim()
    return u ? `${formatted} ${u}` : formatted
  }

  function bucketLabelForKey(key: unknown): string {
    return bucketByKey.get(String(key))?.label ?? String(key)
  }

  // Bar colour mirrors the metric-type badge so a glance at the chart
  // matches the colour you saw on the card. Histogram = warning, ExpHist
  // = secondary.
  let barColor = $derived(metricTypeSeriesColor(datapoint.metricType))

  // --- Bar width clamping ---
  //
  // BarChart with no width hint stretches bars to fill the container; for
  // ExpHistograms with 30+ buckets that yields hairline bars, and for
  // 3-bucket Histograms it yields chonky stripes. Anchor each bar's slot
  // to ~30px and let the chart grow past its parent if needed (with the
  // wrapper scrolling horizontally) so density stays consistent across
  // metric shapes.
  const BAR_SLOT_WIDTH = 30
  const MIN_CHART_WIDTH = 280

  let parentWidth = $state(0)
  let chartContext = $state<any>(undefined)

  let chartRenderHeight = $derived(plotAreaHeight > 0 ? plotAreaHeight : height)

  let chartWidth = $derived.by(() => {
    const natural = buckets.length * BAR_SLOT_WIDTH
    const lower = Math.max(MIN_CHART_WIDTH, natural)
    if (parentWidth <= 0) return lower
    return Math.max(parentWidth, natural)
  })

  let histogramScroll = $state<HTMLElement | undefined>(undefined)

  function togglePinnedBucket(bucket: Bucket) {
    if (!enableValueBucketPin) return
    pinnedBucketKey = pinnedBucketKey === bucket.key ? null : bucket.key
  }

  function handleChartKeyboard(event: KeyboardEvent) {
    const command = orderedCursorCommandForKey(event)
    if (command) {
      event.preventDefault()
      event.stopPropagation()
      keyboardNavigation.move(command)
      return
    }

    if (isChartActivationKey(event.key)) {
      if (!enableValueBucketPin || !keyboardBucket) return
      event.preventDefault()
      event.stopPropagation()
      togglePinnedBucket(keyboardBucket)
      return
    }

    if (event.key === 'Escape') {
      event.preventDefault()
      event.stopPropagation()
      pinnedBucketKey = null
      onClearSelection?.()
    }
  }

  $effect(() => {
    if (!keyboardFocused) return
    const viewport = histogramScroll
    const bucket = keyboardBucket
    const context = chartContext
    if (!viewport || !bucket || !context) return
    const x = context.xScale(bucket.key)
    if (x == null) return
    const width = context.xScale.bandwidth?.() ?? 0
    const left = context.padding.left + x
    const right = left + width
    if (left < viewport.scrollLeft) {
      viewport.scrollLeft = Math.max(0, left - context.padding.left)
    } else if (right > viewport.scrollLeft + viewport.clientWidth) {
      viewport.scrollLeft = right - viewport.clientWidth + context.padding.right
    }
  })

  function handlePlotClick(event: MouseEvent) {
    if (!enableValueBucketPin || !chartContext || buckets.length === 0) return
    const root = (event.currentTarget as HTMLElement).querySelector(
      '.lc-root-container'
    )
    if (!root) return
    const rect = root.getBoundingClientRect()
    const pointX = event.clientX - rect.left
    const pointY = event.clientY - rect.top
    const { padding, xScale, xRange, yRange } = chartContext

    // Same plot coords as layerchart tooltip band mode (TooltipContext bisect-band).
    const plotX = pointX - padding.left
    const plotY = pointY - padding.top
    const xMin = Math.min(xRange[0], xRange[1])
    const xMax = Math.max(xRange[0], xRange[1])
    const yMin = Math.min(yRange[0], yRange[1])
    const yMax = Math.max(yRange[0], yRange[1])
    if (plotX < xMin || plotX > xMax || plotY < yMin || plotY > yMax) return

    const key = scaleInvert(xScale, plotX)
    if (key == null) return
    const bucket = bucketByKey.get(String(key))
    if (!bucket) return
    togglePinnedBucket(bucket)
  }

  // --- Tooltip helpers ---

  function formatBucketRange(b: Bucket): string {
    const { lo, hi } = b
    const u = unit ? ` ${unit}` : ''
    if (b.zeroThreshold !== undefined) {
      return b.zeroThreshold > 0
        ? `${exponentialZeroBucketLabel(b.zeroThreshold)}${u}`
        : `= 0${u}`
    }
    if (!Number.isFinite(lo) && !Number.isFinite(hi)) return 'all values'
    if (!Number.isFinite(lo)) return `≤ ${formatNumber(hi)}${u}`
    if (!Number.isFinite(hi)) return `> ${formatNumber(lo)}${u}`
    if (lo === hi) return `= ${formatNumber(lo)}${u}`
    return `${formatNumber(lo)} – ${formatNumber(hi)}${u}`
  }

  function formatNumber(v: number): string {
    if (!Number.isFinite(v)) return v > 0 ? '∞' : '-∞'
    if (Number.isInteger(v)) return v.toString()
    const abs = Math.abs(v)
    if (abs >= 10000 || (abs > 0 && abs < 0.01)) return v.toExponential(2)
    return v.toPrecision(4).replace(/\.?0+$/, '')
  }

  // Denominator is the sum of the bars actually drawn, not datapoint.count.
  // For ExponentialHistogram we currently render zero + positive buckets
  // only -- a non-zero negativeCount would make stats.count larger than
  // what the chart shows and "% of total" would never reach 100%, even
  // when one rendered bar holds every visible observation. Summing the
  // visible buckets keeps the percentages internally consistent with the
  // chart in front of the user.
  let visibleTotal = $derived.by(() => {
    let t = 0
    for (const b of buckets) t += b.count
    return t
  })

  function formatPct(count: number): string {
    if (!visibleTotal) return '—'
    const pct = (count / visibleTotal) * 100
    if (pct >= 10) return `${pct.toFixed(1)}%`
    return `${pct.toFixed(2)}%`
  }

  let valuePinLegendRows = $derived.by((): SelectionLegendRow[] => {
    if (!pinnedBucket) return []
    return [
      {
        key: 'count',
        color: 'var(--color-base-content)',
        label: 'count',
        valueText: String(pinnedBucket.count),
      },
      {
        key: 'share',
        color: 'var(--color-base-content)',
        label: 'of total',
        valueText: formatPct(pinnedBucket.count),
      },
    ]
  })
</script>

{#if buckets.length > 0}
  <div
    class="metric-histogram-bar-chart"
    class:metric-histogram-bar-chart--value-pin={enableValueBucketPin}
    style:height="{height}px"
  >
    {#if timeRange || selectionTimestamp || (enableValueBucketPin && pinnedBucket)}
      <div class="metric-histogram-bar-chart__header">
        {#if timeRange}
          <ChartTimeRangeHeader
            startMs={timeRange.startMs}
            endMs={timeRange.endMs}
            variant="legend"
          />
        {/if}
        <div class="metric-histogram-bar-chart__header-end">
          {#if selectionTimestamp}
            <ChartSelectionLegend timestamp={selectionTimestamp} rows={[]} />
          {/if}
          {#if enableValueBucketPin && pinnedBucket}
            <div class="metric-histogram-bar-chart__value-pin-legend">
              <ChartSelectionLegend
                timestamp={formatBucketRange(pinnedBucket)}
                rows={valuePinLegendRows}
              />
            </div>
          {/if}
        </div>
      </div>
    {/if}
    <div
      class="metric-histogram-bar-chart__plot chart-keyboard-surface-host"
      bind:clientWidth={parentWidth}
      bind:clientHeight={plotAreaHeight}
    >
      <div class="histogram-chart-scroll" bind:this={histogramScroll}>
        <!-- Pointer coordinates stay on the chart-sized hit target; the
             named outer surface owns all equivalent keyboard handling. -->
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="histogram-chart-wrapper"
          class:histogram-chart-wrapper--clickable={enableValueBucketPin}
          style:width="{chartWidth}px"
          style:height="{chartRenderHeight}px"
          onclick={handlePlotClick}
        >
          <MetricChartPlot
            class="histogram-chart"
            height={chartRenderHeight}
            width={chartWidth}
          >
            <BarChart
              bind:context={chartContext}
              data={buckets}
              x="key"
              xScale={scaleBand()}
              y="count"
              yScale={scaleLinear()}
              yNice
              padding={chartPadding}
              bandPadding={0.2}
              tooltipContext
              highlight={{ area: true }}
              props={{
                bars: {
                  fill: barColor,
                  // layerchart 2.0 types fillOpacity as number, so set the
                  // CSS var via the style attribute instead.
                  style: 'fill-opacity: var(--metric-bar-fill-opacity, 0.85)',
                  stroke: 'var(--color-base-300)',
                  strokeWidth: 1,
                },
                xAxis: {
                  ...axisBuckets(unit),
                  format: bucketLabelForKey,
                },
                yAxis: axisCount(),
              }}
            >
              {#snippet tooltip()}
                <Tooltip.Root>
                  {#snippet children({ data }: { data: Bucket })}
                    <Tooltip.Header class="text-center"
                      >{formatBucketRange(data)}</Tooltip.Header
                    >
                    <Tooltip.List>
                      <Tooltip.Item
                        label="count"
                        value={data.count}
                        format="integer"
                      />
                      <Tooltip.Item
                        label="of total"
                        value={formatPct(data.count)}
                      />
                    </Tooltip.List>
                  {/snippet}
                </Tooltip.Root>
              {/snippet}

              {#snippet aboveMarks({ context }: { context: any })}
                {@const xs = context.xScale}
                {@const bw =
                  typeof xs.bandwidth === 'function' ? xs.bandwidth() : 0}
                {@const yTop = context.yRange[1]}
                {@const yBot = context.yRange[0]}
                {#if pinnedBucket}
                  {@const step = typeof xs.step === 'function' ? xs.step() : bw}
                  {@const outer =
                    typeof xs.padding === 'function'
                      ? (xs.padding() * step) / 2
                      : 0}
                  {@const x0 = xs(pinnedBucket.key)}
                  <Rect
                    x={x0 != null ? x0 - outer : 0}
                    y={Math.min(yTop, yBot)}
                    width={step}
                    height={Math.abs(yBot - yTop)}
                    class="value-bucket-pin-highlight"
                  />
                {/if}
                {#if keyboardFocused && keyboardBucket}
                  {@const keyboardStep =
                    typeof xs.step === 'function' ? xs.step() : bw}
                  {@const keyboardOuter =
                    typeof xs.padding === 'function'
                      ? (xs.padding() * keyboardStep) / 2
                      : 0}
                  {@const keyboardX = xs(keyboardBucket.key)}
                  <Rect
                    x={keyboardX != null ? keyboardX - keyboardOuter : 0}
                    y={Math.min(yTop, yBot)}
                    width={keyboardStep}
                    height={Math.abs(yBot - yTop)}
                    class="chart-keyboard-cursor-band"
                  />
                {/if}
                {#each quantileMarks as m (m.key)}
                  {@const x0 = xs(buckets[m.bucketIndex].key)}
                  {@const px = x0 + bw * m.fraction}
                  <g class="quantile-marker" style:--marker-color={m.color}>
                    <title>{m.label} {formatQuantileValue(m.value)}</title>
                    <Line
                      x1={px}
                      x2={px}
                      y1={yTop}
                      y2={yBot}
                      class="quantile-line"
                    />
                  </g>
                {/each}
              {/snippet}
            </BarChart>
            {#each quantileLabelPlacements as mark (mark.key)}
              <div
                class="series-stat-tooltip series-stat-tooltip--above"
                style:left="{mark.left}px"
                style:top="{mark.top}px"
                title={mark.title}
                aria-hidden="true"
              >
                <div
                  class="chart-selection-legend chart-selection-legend--stat"
                >
                  <ul class="chart-selection-legend__rows">
                    <li class="chart-selection-legend__row">
                      <span
                        class="chart-selection-legend__dot"
                        style:--color={mark.color}
                        aria-hidden="true"
                      ></span>
                      <span class="chart-selection-legend__label"
                        >{mark.statLabel}</span
                      >
                      <span class="chart-selection-legend__value"
                        >{mark.valueText}</span
                      >
                    </li>
                  </ul>
                </div>
              </div>
            {/each}
          </MetricChartPlot>
        </div>
      </div>
      <ChartKeyboardSurface
        id="metric-histogram-keyboard-surface"
        label={enableValueBucketPin
          ? 'Histogram distribution chart'
          : 'Histogram snapshot distribution chart'}
        roleDescription="interactive histogram"
        instructions={enableValueBucketPin
          ? 'Use Left and Right to inspect buckets. Home and End move to the first and last buckets. Enter or Space pins the current bucket. Escape clears chart selection.'
          : 'Use Left and Right to inspect snapshot buckets. Home and End move to the first and last buckets. Escape clears chart selection.'}
        readout={keyboardReadout}
        shortcuts={keyboardShortcuts}
        onKeydown={handleChartKeyboard}
        onFocusChange={handleKeyboardFocusChange}
      />
    </div>
  </div>
{:else}
  <MetricChartEmpty {height} message="No buckets to chart" />
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .metric-histogram-bar-chart {
    @apply flex min-h-0 w-full min-w-0 flex-col;
  }

  .metric-histogram-bar-chart--value-pin :global(.lc-bars-bar) {
    opacity: 1 !important;
  }

  .metric-histogram-bar-chart__header {
    @apply flex shrink-0 items-start justify-between gap-2 px-1 pb-1 pt-0.5;
  }

  .metric-histogram-bar-chart__header-end {
    @apply ml-auto flex shrink-0 flex-col items-end gap-1;
    pointer-events: none;
  }

  .metric-histogram-bar-chart__value-pin-legend
    :global(.chart-selection-legend__rows) {
    grid-template-columns: auto auto;
    column-gap: 0.35rem;
    row-gap: 0.12rem;
  }

  .metric-histogram-bar-chart__value-pin-legend
    :global(.chart-selection-legend__dot) {
    display: none;
  }

  .metric-histogram-bar-chart__value-pin-legend
    :global(.chart-selection-legend__label) {
    color: var(--color-subtle);
  }

  .metric-histogram-bar-chart__value-pin-legend
    :global(.chart-selection-legend__label::after) {
    content: ':';
  }

  .metric-histogram-bar-chart__value-pin-legend
    :global(.chart-selection-legend__value) {
    @apply text-base-content;
  }

  .metric-histogram-bar-chart__plot {
    @apply relative min-h-0 min-w-0 flex-1 overflow-hidden;
  }

  /* Horizontal scroll appears only when the natural chart width
     (bucketCount * BAR_SLOT_WIDTH) exceeds the parent. Below that
     threshold the inner .histogram-chart sits at parent width and
     no scroll bar appears. */
  .histogram-chart-scroll {
    @apply h-full w-full overflow-x-auto overflow-y-hidden rounded-lg;
  }

  .histogram-chart-wrapper--clickable {
    cursor: pointer;
  }

  /* Match heatmap: hover band must not eat column clicks. */
  .histogram-chart-wrapper--clickable :global(.lc-highlight-area) {
    pointer-events: none;
  }

  .series-stat-tooltip {
    position: absolute;
    pointer-events: none;
    z-index: 2;
    width: max-content;
    max-width: none;
  }

  .series-stat-tooltip--above {
    transform: translate(-50%, calc(-100% - 8px));
  }

  .quantile-marker {
    pointer-events: none;
  }
</style>
