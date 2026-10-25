<script lang="ts">
  import ChartOverlayLabel from './ChartOverlayLabel.svelte'
  import { createOverlayEmphasis } from '@/components/metrics/utils/overlay-emphasis.svelte'

  import ChartSticky from '@/components/metrics/Charts/ChartSticky.svelte'
  import HistogramContributionPattern from './HistogramContributionPattern.svelte'
  import type { ChartStickySize } from '@/components/metrics/utils/chart-sticky-position'
  import { histogramCardPosition } from '@/components/metrics/utils/histogram-card-position'
  import {
    BarChart,
    Line,
    Rect,
    Tooltip,
    scaleInvert,
    type ChartState,
  } from 'layerchart'
  import {
    scaleBand,
    scaleLinear,
    scaleThreshold,
    type ScaleBand,
  } from 'd3-scale'
  import ChartKeyboardSurface from '@/components/metrics/Charts/ChartKeyboardSurface.svelte'
  import { chartScrollDiscovery } from '@/components/metrics/utils/chart-scroll-discovery'
  import MetricChartEmpty from '@/components/metrics/Charts/MetricChartEmpty.svelte'
  import ChartSelectionLegend, {
    type SelectionLegendRow,
  } from '@/components/metrics/Charts/ChartSelectionLegend.svelte'
  import MetricChartPlot, {
    axisBuckets,
    axisCount,
    chartPadding as fallbackPadding,
    DEFAULT_METRIC_CHART_HEIGHT,
  } from '@/components/metrics/Charts/MetricChartPlot.svelte'
  import { measureChartAxisGutter } from '@/components/metrics/utils/chart-axis-gutter'

  import { computeHistogramBarColorScale } from '@/components/metrics/utils/heatmap-color-scale'
  import { themeSignal } from '@/state/theme.svelte'
  import { expBuckets } from '@/components/metrics/utils/histogram-quantile'
  import {
    formatMetricValue,
    formatMetricHoverValue,
    formatMetricCount,
    metricNumberPrecision,
  } from '@/components/metrics/utils/format-metric-value'
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import {
    HISTOGRAM_QUANTILE_DASH,
    histogramQuantileColor,
  } from '@/components/metrics/utils/heatmap-quantiles'
  import {
    exponentialBucketIdentity,
    exponentialZeroBucketIdentity,
    exponentialZeroBucketLabel,
    formatHistogramBound,
    formatHistogramRange,
    indexedHistogramRangeIdentity,
  } from '@/components/metrics/utils/histogram-bucket'
  import {
    isChartActivationKey,
    orderedCursorCommandForKey,
  } from '@/components/metrics/utils/chart-keyboard-cursor'
  import { createOrderedChartKeyboardCursor } from '@/components/metrics/utils/chart-keyboard-state.svelte'
  import {
    histogramSliceToChartDatapoint,
    type HistogramChartDataPoint,
  } from '@/components/metrics/utils/histogram-aggregation'

  const emphasis = createOverlayEmphasis()
  let yAxisInset = $state<number>(fallbackPadding.left)
  const chartPadding = $derived({ ...fallbackPadding, left: yAxisInset })

  // lo/hi position quantile markers and may be infinite or both zero.
  type Bucket = {
    key: string
    label: string
    count: number
    lo: number
    hi: number
    zeroThreshold?: number
  }

  function histogramBandScale(context: ChartState): ScaleBand<string> {
    // SAFETY: This BarChart receives scaleBand() below. LayerChart copies that
    // scale while preserving its kind, but its public context erases the type.
    return context.xScale as ScaleBand<string>
  }

  type Props = {
    datapoint: HistogramChartDataPoint
    /** Metric `unit` for axis labelling (e.g. "ms", "bytes"). Optional;
     * the x-axis title shows just "value" when unit is empty. */
    unit?: string
    height?: number
    onClearSelection?: () => void
  }

  let {
    datapoint,
    unit = '',
    height = DEFAULT_METRIC_CHART_HEIGHT,
    onClearSelection,
  }: Props = $props()

  let plotAreaHeight = $state(0)
  const ctx = getMetricViewContext()
  const contributionPatternID = $props.id()
  let pinnedBucketKey = $state<string | null>(null)

  // Keys must match Go's strconv.FormatFloat(q, 'f', -1, 64) output.
  const QUANTILE_LABELS = [
    { key: '0.5', label: 'p50' },
    { key: '0.95', label: 'p95' },
    { key: '0.99', label: 'p99' },
  ] as const
  type QuantileKey = (typeof QUANTILE_LABELS)[number]['key']

  // Quantiles are computed by the store.
  let quantiles = $derived({
    '0.5': datapoint.quantiles?.['0.5'] ?? null,
    '0.95': datapoint.quantiles?.['0.95'] ?? null,
    '0.99': datapoint.quantiles?.['0.99'] ?? null,
  } satisfies Record<QuantileKey, number | null>)

  let buckets = $derived.by((): Bucket[] => {
    if (datapoint.metricType === 'Histogram') {
      return buildHistogramBuckets(datapoint)
    }
    return buildExpHistogramBuckets(datapoint)
  })

  let bucketByKey = $derived(
    new Map(buckets.map(bucket => [bucket.key, bucket]))
  )

  let contributionByKey = $derived.by(() => {
    const selection = ctx.histogramContribution
    if (!selection?.point) return new Map<string, number>()
    const point = histogramSliceToChartDatapoint(
      selection.point,
      selection.datapointID,
      'Delta',
      1
    )
    const selectedBuckets =
      point.metricType === 'Histogram'
        ? buildHistogramBuckets(point)
        : buildExpHistogramBuckets(point)
    return new Map(selectedBuckets.map(bucket => [bucket.key, bucket.count]))
  })

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
    return (
      `Bucket ${cursor.index + 1} of ${buckets.length}, ${formatBucketRange(bucket)}. ` +
      `Count ${bucket.count}. ${formatPct(bucket.count)} of total.`
    )
  })
  const keyboardShortcuts = [
    'ArrowLeft',
    'ArrowRight',
    'Home',
    'End',
    'Enter',
    'Space',
    'Escape',
  ]

  let pinDatapointID = $state<string | undefined>(undefined)

  $effect(() => {
    const id = `${datapoint.id}:${ctx.histogramContribution?.datapointID ?? ''}`
    if (pinDatapointID !== undefined && pinDatapointID !== id) {
      pinnedBucketKey = null
    }
    pinDatapointID = id
  })

  function buildHistogramBuckets(
    dp: Extract<HistogramChartDataPoint, { metricType: 'Histogram' }>
  ): Bucket[] {
    const bounds = dp.explicitBounds
    const counts = dp.bucketCounts
    const digits = metricNumberPrecision(bounds)
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
        label = formatHistogramRange(-Infinity, bounds[0], digits)
        lo = -Infinity
        hi = bounds[0]
      } else if (i < bounds.length) {
        label = formatHistogramRange(bounds[i - 1], bounds[i], digits)
        lo = bounds[i - 1]
        hi = bounds[i]
      } else {
        label = `(${formatHistogramBound(bounds[bounds.length - 1], digits)}, +∞)`
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
    dp: Extract<HistogramChartDataPoint, { metricType: 'ExponentialHistogram' }>
  ): Bucket[] {
    const negativeCount = dp.negativeBucketCounts.length
    const sourceBuckets = expBuckets(
      dp.scale,
      dp.negativeBucketOffset,
      dp.negativeBucketCounts,
      dp.zeroCount,
      dp.positiveBucketOffset,
      dp.positiveBucketCounts
    )
    const digits = metricNumberPrecision(
      sourceBuckets.flatMap(bucket => [bucket.lo, bucket.hi])
    )
    return sourceBuckets.map((bucket, index) => {
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
            : formatHistogramRange(bucket.lo, bucket.hi, digits),
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
    key: QuantileKey
    label: string
    value: number
    bucketIndex: number
    // Position within the bar [0, 1]. 0 = left edge, 1 = right edge.
    fraction: number
    color: string
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
      if (!ctx.activeQuantileOverlays.has(key)) continue
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
        color: histogramQuantileColor(key),
      })
    }
    return marks
  })

  const QUANTILE_ORDER = {
    '0.5': 0,
    '0.95': 1,
    '0.99': 2,
  } satisfies Record<QuantileKey, number>

  let quantileLabelPlacements = $derived.by(() => {
    const ctx = chartContext
    if (!ctx || quantileMarks.length === 0) return []

    const xs = histogramBandScale(ctx)
    const ys = ctx.yScale
    const bw = xs.bandwidth()
    const plotLeft = ctx.padding.left
    const plotTop = ctx.padding.top
    const plotCeiling = ctx.yRange[1] + plotTop + 4
    const STAGGER_PX = 32
    const COLLIDE_PX = 88

    type Draft = {
      key: QuantileKey
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
        sortOrder: QUANTILE_ORDER[m.key],
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

  function bucketLabelForKey(key: string): string {
    return bucketByKey.get(key)?.label ?? key
  }

  // Use the heatmap legend's palette, scaled to the counts in this distribution.
  let barColorScale = $derived.by(() => {
    const palette = computeHistogramBarColorScale(datapoint, themeSignal.value)
    return scaleThreshold<number, string>()
      .domain(palette.thresholds)
      .range(palette.range)
  })

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
  let chartContext = $state<ChartState | undefined>(undefined)

  // Round inside the measured bars; neither the count height nor bucket scale
  // changes. Short bars cap their corners and omit outlines below one pixel.
  let barGeometry = $derived.by(() => {
    const context = chartContext
    if (!context) return []
    const xs = histogramBandScale(context)
    const bandwidth = xs.bandwidth()
    const gap = Math.min(6, bandwidth * 0.16)
    const width = Math.max(0, bandwidth - gap)
    const baseline = context.yScale(0)
    return buckets.map(bucket => {
      const y = context.yScale(bucket.count)
      const height = Math.max(0, baseline - y)
      return {
        bucket,
        x: (xs(bucket.key) ?? 0) + gap / 2,
        y,
        width,
        height,
        radius: Math.min(10, width / 4, height / 2),
      }
    })
  })
  let hoveredBucketKey = $derived(chartContext?.tooltip.data?.key ?? null)

  let chartRenderHeight = $derived(plotAreaHeight > 0 ? plotAreaHeight : height)

  let chartWidth = $derived.by(() => {
    const natural = buckets.length * BAR_SLOT_WIDTH
    const lower = Math.max(
      MIN_CHART_WIDTH,
      natural,
      chartPadding.left + chartPadding.right + BAR_SLOT_WIDTH
    )
    if (parentWidth <= 0) return lower
    return Math.max(parentWidth, lower)
  })

  let histogramScroll = $state<HTMLElement | undefined>(undefined)

  function togglePinnedBucket(bucket: Bucket) {
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
      if (!keyboardBucket) return
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
    const xs = histogramBandScale(context)
    const x = xs(bucket.key)
    if (x == null) return
    const width = xs.bandwidth()
    const left = context.padding.left + x
    const right = left + width
    if (left < viewport.scrollLeft) {
      viewport.scrollLeft = Math.max(0, left - context.padding.left)
    } else if (right > viewport.scrollLeft + viewport.clientWidth) {
      viewport.scrollLeft = right - viewport.clientWidth + context.padding.right
    }
  })

  function handlePlotClick(
    event: MouseEvent & { currentTarget: HTMLDivElement }
  ) {
    if (!chartContext || buckets.length === 0) return
    const root = event.currentTarget.querySelector('.lc-root-container')
    if (!root) return
    const rect = root.getBoundingClientRect()
    const pointX = event.clientX - rect.left
    const pointY = event.clientY - rect.top
    const { padding, xRange, yRange } = chartContext
    const xScale = histogramBandScale(chartContext)

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

  function formatBucketRange(b: Bucket): string {
    const { lo, hi } = b
    const u = unit ? ` ${unit}` : ''
    const digits = metricNumberPrecision([lo, hi], 6)
    const formatNumber = (value: number) => formatHistogramBound(value, digits)
    if (b.zeroThreshold !== undefined) {
      return b.zeroThreshold > 0
        ? `${exponentialZeroBucketLabel(b.zeroThreshold, digits)}${u}`
        : `= 0${u}`
    }
    if (!Number.isFinite(lo) && !Number.isFinite(hi)) return 'all values'
    if (!Number.isFinite(lo)) return `≤ ${formatNumber(hi)}${u}`
    if (!Number.isFinite(hi)) return `> ${formatNumber(lo)}${u}`
    if (lo === hi) return `= ${formatNumber(lo)}${u}`
    return `${formatNumber(lo)} – ${formatNumber(hi)}${u}`
  }

  // Percentages use rendered buckets because negative exponential buckets are hidden.
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
        valueText: formatMetricCount(pinnedBucket.count),
      },
      {
        key: 'share',
        color: 'var(--color-base-content)',
        label: 'of total',
        valueText: formatPct(pinnedBucket.count),
      },
    ]
  })

  let scrollLeft = $state(0)
  let selectionAnchor = $derived.by(() => {
    const context = chartContext
    if (!context || !pinnedBucket) return null
    const scale = histogramBandScale(context)
    const x = scale(pinnedBucket.key)
    if (x === undefined) return null
    return {
      x: context.padding.left + x + scale.bandwidth() / 2 - scrollLeft,
      y: context.padding.top + context.yScale(pinnedBucket.count),
    }
  })

  function positionSelectionCard(size: ChartStickySize) {
    const context = chartContext
    const selected = barGeometry.find(bar => bar.bucket.key === pinnedBucketKey)
    if (!context || !selected) return { left: 8, top: 8 }
    const offsetX = context.padding.left - scrollLeft
    const offsetY = context.padding.top
    const rect = (bar: (typeof barGeometry)[number]) => ({
      left: offsetX + bar.x,
      top: offsetY + bar.y,
      width: bar.width,
      height: bar.height,
    })
    return histogramCardPosition({
      ...size,
      selected: rect(selected),
      bars: barGeometry.map(rect),
      plot: {
        left: offsetX + Math.min(...context.xRange),
        top: offsetY + Math.min(...context.yRange),
        width: Math.abs(context.xRange[1] - context.xRange[0]),
        height: Math.abs(context.yRange[1] - context.yRange[0]),
      },
    })
  }
  let chartAxes = $derived({
    xAxis: {
      ...axisBuckets(unit),
      format: bucketLabelForKey,
      labelProps: {
        x: chartContext?.xRange[1],
        y: chartContext?.yRange[0],
        dy: -8,
        textAnchor: 'end' as const,
        verticalAnchor: 'end' as const,
      },
    },
    yAxis: axisCount(chartPadding.left),
  })
</script>

{#if buckets.length > 0}
  <div class="metric-histogram-bar-chart" style:height="{height}px">
    <div
      class="metric-histogram-bar-chart__plot chart-keyboard-surface-host"
      bind:clientWidth={parentWidth}
      bind:clientHeight={plotAreaHeight}
    >
      <div
        class="histogram-chart-scroll"
        bind:this={histogramScroll}
        use:chartScrollDiscovery={{
          visit: ctx.metric?.metricRef,
          overflows: parentWidth > 0 && chartWidth - parentWidth > 1,
        }}
        onscroll={event => (scrollLeft = event.currentTarget.scrollLeft)}
      >
        <!-- Pointer coordinates stay on the chart-sized hit target; the
             named outer surface owns all equivalent keyboard handling. -->
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div
          class="histogram-chart-wrapper"
          use:measureChartAxisGutter={left => (yAxisInset = left)}
          style:width="{chartWidth}px"
          style:height="{chartRenderHeight}px"
          onclick={handlePlotClick}
        >
          <MetricChartPlot
            class="histogram-chart"
            height={chartRenderHeight}
            width={chartWidth}
            padding={chartPadding}
          >
            <BarChart
              bind:context={chartContext}
              data={buckets}
              x="key"
              xScale={scaleBand<string>()}
              y="count"
              yScale={scaleLinear()}
              yNice
              padding={chartPadding}
              bandPadding={0.2}
              tooltipContext
              highlight={{ area: { class: 'histogram-hover-column' } }}
              props={chartAxes}
            >
              {#snippet grid()}{''}{/snippet}
              {#snippet marks()}
                {#each barGeometry as bar (bar.bucket.key)}
                  <g
                    class="histogram-bar"
                    style:--histogram-bar-color={barColorScale(
                      bar.bucket.count
                    )}
                    class:histogram-bar--active={bar.bucket.key ===
                      hoveredBucketKey ||
                      bar.bucket.key === pinnedBucketKey ||
                      (keyboardFocused &&
                        bar.bucket.key === keyboardBucket?.key)}
                  >
                    <rect
                      class="lc-bars-bar histogram-bar__fill"
                      x={bar.x}
                      y={bar.y}
                      width={bar.width}
                      height={bar.height}
                      rx={bar.radius}
                    />
                    {#if bar.height > 1 && bar.width > 1}
                      <rect
                        class="histogram-bar__liner"
                        x={bar.x + 0.5}
                        y={bar.y + 0.5}
                        width={bar.width - 1}
                        height={bar.height - 1}
                        rx={Math.max(0, bar.radius - 0.5)}
                      />
                    {/if}
                  </g>
                {/each}
              {/snippet}

              {#snippet tooltip()}
                <Tooltip.Root class="chart-hover-card">
                  {#snippet children({ data }: { data: Bucket })}
                    <Tooltip.Header class="text-center"
                      >{formatBucketRange(data)}</Tooltip.Header
                    >
                    <Tooltip.List>
                      <Tooltip.Item
                        label="count"
                        value={data.count}
                        format={formatMetricCount}
                      />
                      {#if ctx.histogramContribution}
                        <Tooltip.Item
                          label="selected contribution"
                          value={`${formatMetricCount(contributionByKey.get(data.key) ?? 0)} of ${formatMetricCount(data.count)}`}
                        />
                      {/if}
                      <Tooltip.Item
                        label="of total"
                        value={formatPct(data.count)}
                      />
                    </Tooltip.List>
                  {/snippet}
                </Tooltip.Root>
              {/snippet}

              {#snippet aboveMarks({ context }: { context: ChartState })}
                {@const xs = histogramBandScale(context)}
                {@const bw = xs.bandwidth()}
                {@const yTop = context.yRange[1]}
                {@const yBot = context.yRange[0]}
                {#if pinnedBucket}
                  {@const step = xs.step()}
                  {@const outer = (xs.padding() * step) / 2}
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
                  {@const keyboardStep = xs.step()}
                  {@const keyboardOuter = (xs.padding() * keyboardStep) / 2}
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
                  {@const x0 = xs(buckets[m.bucketIndex].key) ?? 0}
                  {@const px = x0 + bw * m.fraction}
                  <g
                    class="quantile-marker"
                    class:chart-overlay--emphasized={emphasis.has(m.key)}
                    style:--marker-color={m.color}
                    style:--quantile-color={m.color}
                  >
                    <title
                      >{m.label} {formatMetricHoverValue(m.value)} {unit}</title
                    >
                    <Line
                      x1={px}
                      x2={px}
                      y1={yTop}
                      y2={yBot}
                      class="quantile-line-backing"
                    />
                    <Line
                      x1={px}
                      x2={px}
                      y1={yTop}
                      y2={yBot}
                      class="quantile-line"
                      style="stroke-dasharray: {HISTOGRAM_QUANTILE_DASH}"
                    />
                  </g>
                {/each}
                <g class="histogram-contributions" aria-hidden="true">
                  {#each barGeometry as bar, index (bar.bucket.key)}
                    {@const contribution =
                      contributionByKey.get(bar.bucket.key) ?? 0}
                    {#if contribution > 0}
                      {@const y = context.yScale(contribution)}
                      {@const contributionHeight = Math.max(
                        0,
                        context.yScale(0) - y
                      )}
                      {@const radius = Math.min(
                        bar.radius,
                        contributionHeight / 2
                      )}
                      {@const color = barColorScale(bar.bucket.count)}
                      {@const patternID = `${contributionPatternID}-${index}`}
                      <HistogramContributionPattern id={patternID} {color} />
                      <rect
                        x={bar.x}
                        {y}
                        width={bar.width}
                        height={contributionHeight}
                        rx={radius}
                        fill={color}
                      />
                      <rect
                        class="histogram-bar__contribution"
                        data-contribution-count={contribution}
                        x={bar.x}
                        {y}
                        width={bar.width}
                        height={contributionHeight}
                        rx={radius}
                        fill={`url(#${patternID})`}
                        style:--contribution-color={color}
                      />
                    {/if}
                  {/each}
                </g>
              {/snippet}
            </BarChart>
            {#each quantileLabelPlacements as mark (mark.key)}
              <div
                class="series-stat-tooltip series-stat-tooltip--above"
                style:left="{mark.left}px"
                style:top="{mark.top}px"
                title={mark.title}
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
                        ><ChartOverlayLabel
                          label={mark.statLabel}
                          active={emphasis.has(mark.key)}
                          onclick={() => emphasis.toggle(mark.key)}
                        /></span
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
      {#if pinnedBucket}
        <ChartSticky
          anchor={selectionAnchor}
          positioner={positionSelectionCard}
        >
          <div class="metric-histogram-bar-chart__value-pin-legend">
            <ChartSelectionLegend
              timestamp={formatBucketRange(pinnedBucket)}
              rows={valuePinLegendRows}
            />
          </div>
        </ChartSticky>
      {/if}
      <ChartKeyboardSurface
        id="metric-histogram-keyboard-surface"
        label="Histogram distribution chart"
        roleDescription="interactive histogram"
        instructions="Use Left and Right to inspect buckets. Home and End move to the first and last buckets. Enter or Space pins the current bucket. Escape clears chart selection."
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

  .histogram-bar {
    pointer-events: none;
  }

  .histogram-bar__fill {
    fill: var(--histogram-bar-color);
    stroke: none;
  }

  .histogram-bar__liner {
    fill: none;
    stroke: color-mix(
      in oklab,
      var(--histogram-bar-color) 80%,
      var(--color-base-content)
    );
    stroke-opacity: 0.4;
    stroke-width: 1;
  }

  .histogram-contributions {
    pointer-events: none;
  }

  .histogram-bar__contribution {
    stroke: color-mix(
      in oklab,
      var(--contribution-color) 80%,
      var(--color-base-content)
    );
    stroke-opacity: 0.4;
    stroke-width: 1;
  }

  .histogram-bar--active .histogram-bar__fill {
    fill: color-mix(
      in oklab,
      var(--histogram-bar-color) 85%,
      var(--color-base-content)
    );
  }

  .histogram-bar--active .histogram-bar__liner {
    stroke-opacity: 0.75;
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

  .histogram-chart-wrapper {
    cursor: pointer;
  }

  .series-stat-tooltip--above {
    transform: translate(-50%, calc(-100% - 8px));
  }

  .quantile-marker {
    pointer-events: none;
  }
</style>
