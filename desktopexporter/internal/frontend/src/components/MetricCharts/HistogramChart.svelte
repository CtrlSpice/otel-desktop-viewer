<script lang="ts">
  import { BarChart, Line, Text, Tooltip } from 'layerchart'
  import { scaleBand, scaleLinear } from 'd3-scale'
  import { telemetryAPI } from '@/services/telemetry-service'
  import { metricTypeSeriesColor } from '@/utils/metric-type'
  import type {
    HistogramDataPoint,
    ExponentialHistogramDataPoint,
  } from '@/types/api-types'

  // lo/hi are the numeric bucket bounds; they may be -Infinity, +Infinity, or
  // (for the exp-histogram zero bucket) both 0. Used to position quantile
  // markers within their bar.
  type Bucket = { label: string; count: number; lo: number; hi: number }

  // Where the p50/p95/p99 markers' values come from:
  //   * 'datapoint' (default): fetch via getDatapointQuantiles(datapoint.id).
  //     Right answer for the Snapshot tab where the chart shows ONE specific
  //     datapoint's distribution.
  //   * 'merged': fetch via getMetricMergedQuantiles using the
  //     `metricID` + `windowStartMs/EndMs` props. Right answer for the
  //     Aggregated tab where the chart shows a synthetic dp built by summing
  //     the metric's bucket vectors across the time window. We can't use
  //     datapoint.id here because that synthetic dp doesn't exist in the DB.
  //   * 'none': skip the fetch entirely. Used when the caller wants the bar
  //     chart without quantile markers (e.g. an error path for the
  //     merged bucket data where we'd want to show "no quantiles because
  //     we don't have a valid merged result either").
  type QuantileSource = 'datapoint' | 'merged' | 'none'

  type Props = {
    datapoint: HistogramDataPoint | ExponentialHistogramDataPoint
    /** Metric `unit` for axis labelling (e.g. "ms", "bytes"). Optional;
     * the x-axis title shows just "value" when unit is empty. */
    unit?: string
    height?: number
    /** How to source quantile values; see QuantileSource above. */
    quantileSource?: QuantileSource
    /** Required when quantileSource='merged'. Real metrics.id (db key),
     * NOT the synthetic id of the assembled `datapoint`. */
    metricID?: string
    /** Required when quantileSource='merged'. Same window as the bucket
     * series fetch that produced the merged `datapoint`. */
    windowStartMs?: number
    windowEndMs?: number
  }

  let {
    datapoint,
    unit = '',
    height = 250,
    quantileSource = 'datapoint',
    metricID,
    windowStartMs,
    windowEndMs,
  }: Props = $props()

  // Hardcoded for now; the plan defers configurable quantiles to a future pass.
  // Keys must match Go's strconv.FormatFloat(q, 'f', -1, 64) output.
  const QUANTILES = [0.5, 0.95, 0.99]
  const QUANTILE_LABELS: { key: string; label: string }[] = [
    { key: '0.5', label: 'p50' },
    { key: '0.95', label: 'p95' },
    { key: '0.99', label: 'p99' },
  ]

  let quantiles = $state<Record<string, number | null> | null>(null)

  // Lazy fetch keyed on (source, datapoint id, metric id, window). Cleanup
  // function flips a closure-scoped flag so responses arriving after a prop
  // change get dropped rather than overwriting the now-stale state.
  //
  // The three branches are kept side-by-side rather than abstracted because
  // they each have distinct fetch shapes and "loading" semantics:
  //   * datapoint: 1 fetch keyed on dp.id
  //   * merged: 1 fetch keyed on metric.id + window
  //   * none: no fetch; quantiles stays null and rendering skips markers
  $effect(() => {
    const source = quantileSource
    const dpId = datapoint.id
    const mid = metricID
    const ws = windowStartMs
    const we = windowEndMs
    let cancelled = false
    quantiles = null

    if (source === 'none') {
      // Fast-path: no fetch, no markers. Setting quantiles to {} (not null)
      // tells the marker logic "we tried, there's just nothing" so the
      // chart renders without the loading-shaped em-dash placeholders.
      quantiles = {}
      return
    }

    if (source === 'merged') {
      // Defensive: merged mode needs the metric id + window. If a caller
      // forgot, render no markers rather than invoking the RPC with garbage.
      if (!mid || ws === undefined || we === undefined) {
        console.warn(
          "HistogramChart: quantileSource='merged' requires metricID + windowStartMs + windowEndMs"
        )
        quantiles = {}
        return
      }
      telemetryAPI
        .getMetricMergedQuantiles(mid, QUANTILES, ws, we)
        .then(result => {
          if (!cancelled) quantiles = result
        })
        .catch(err => {
          if (cancelled) return
          // Merged quantile fetch failure is non-fatal: the chart
          // itself is the load-bearing thing, markers are a nice-to-have.
          // Surface to console so the dev sees what went wrong (e.g.
          // bounds mismatch on a malformed metric) but render the bars.
          console.warn('getMetricMergedQuantiles failed:', err)
          quantiles = {}
        })
      return () => {
        cancelled = true
      }
    }

    // 'datapoint' (default).
    telemetryAPI
      .getDatapointQuantiles(dpId, QUANTILES)
      .then(result => {
        if (!cancelled) quantiles = result
      })
      .catch(err => {
        if (cancelled) return
        console.warn('getDatapointQuantiles failed:', err)
        quantiles = {}
      })
    return () => {
      cancelled = true
    }
  })

  // Loading: '…'. NULL or missing key (empty buckets / total=0): em-dash.
  function formatQuantile(key: string): string {
    if (quantiles === null) return '…'
    const v = quantiles[key]
    if (v === null || v === undefined) return '—'
    return v.toFixed(2)
  }

  let buckets = $derived.by((): Bucket[] => {
    if (datapoint.metricType === 'Histogram') {
      return buildHistogramBuckets(datapoint)
    }
    return buildExpHistogramBuckets(datapoint)
  })

  function buildHistogramBuckets(dp: HistogramDataPoint): Bucket[] {
    const bounds = dp.explicitBounds
    const counts = dp.bucketCounts
    const result: Bucket[] = []
    for (let i = 0; i < counts.length; i++) {
      let label: string
      let lo: number
      let hi: number
      if (i === 0) {
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
      result.push({ label, count: counts[i], lo, hi })
    }
    return result
  }

  function buildExpHistogramBuckets(dp: ExponentialHistogramDataPoint): Bucket[] {
    const result: Bucket[] = []
    const base = Math.pow(2, Math.pow(2, -dp.scale))

    if (dp.zeroCount > 0) {
      result.push({ label: '0', count: dp.zeroCount, lo: 0, hi: 0 })
    }

    for (let i = 0; i < dp.positiveBucketCounts.length; i++) {
      const idx = dp.positiveBucketOffset + i
      const lo = Math.pow(base, idx)
      const hi = Math.pow(base, idx + 1)
      result.push({
        label: `(${lo.toPrecision(3)}, ${hi.toPrecision(3)}]`,
        count: dp.positiveBucketCounts[i],
        lo,
        hi,
      })
    }

    return result
  }

  let stats = $derived.by(() => {
    return {
      count: datapoint.count,
      sum: datapoint.sum,
      min: datapoint.min,
      max: datapoint.max,
    }
  })

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
  // For unbounded edge buckets (-∞ or +∞ side) and the exp-histogram zero
  // bucket, we can't compute a meaningful linear fraction, so we center the
  // marker in the bar (fraction = 0.5).
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
    if (!quantiles) return []
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

  let chartWidth = $derived.by(() => {
    const natural = buckets.length * BAR_SLOT_WIDTH
    const lower = Math.max(MIN_CHART_WIDTH, natural)
    if (parentWidth <= 0) return lower
    return Math.max(parentWidth, natural)
  })

  // --- Tooltip helpers ---

  function formatBucketRange(b: Bucket): string {
    const { lo, hi } = b
    const u = unit ? ` ${unit}` : ''
    if (!Number.isFinite(lo) && !Number.isFinite(hi)) return b.label
    if (!Number.isFinite(lo)) return `< ${formatNumber(hi)}${u}`
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
  // chart in front of the user; the raw datapoint.count is still shown
  // in the stats row above for the absolute number.
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

  // Axis titles. Pulled into derived so the unit drives both ends without
  // template-side string splicing.
  let xAxisTitle = $derived(unit ? `value (${unit})` : 'value')
  const yAxisTitle = 'count'
</script>

<div>
  <div class="histogram-stats">
    <span>count: <strong>{stats.count}</strong></span>
    <span>sum: <strong>{stats.sum.toFixed(2)}</strong></span>
    <span>min: <strong>{stats.min.toFixed(2)}</strong></span>
    <span>max: <strong>{stats.max.toFixed(2)}</strong></span>
    {#each QUANTILE_LABELS as { key, label } (key)}
      <span>{label}: <strong>{formatQuantile(key)}</strong></span>
    {/each}
    {#if datapoint.metricType === 'ExponentialHistogram'}
      <span>scale: <strong>{datapoint.scale}</strong></span>
      <span>zeros: <strong>{datapoint.zeroCount}</strong></span>
    {/if}
  </div>

  {#if buckets.length > 0}
    <!-- Outer measurement wrapper sets the scroll viewport; inner sized
         wrapper holds the chart at its natural width so the scroll bar
         only appears when the natural width exceeds the parent. -->
    <div
      class="histogram-measure"
      style:height="{height}px"
      bind:clientWidth={parentWidth}
    >
      <div class="histogram-chart-scroll" style:height="{height}px">
        <div
          class="histogram-chart"
          style:height="{height}px"
          style:width="{chartWidth}px"
        >
          <BarChart
            data={buckets}
            x="label"
            xScale={scaleBand()}
            y="count"
            yScale={scaleLinear()}
            yNice
            bandPadding={0.2}
            padding={{ top: 16, right: 8, bottom: 64, left: 64 }}
            tooltipContext
            props={{
              bars: { fill: barColor, fillOpacity: 0.85 },
              xAxis: {
                title: xAxisTitle,
                tickLabelProps: {
                  rotate: 315,
                  textAnchor: 'end',
                  verticalAnchor: 'middle',
                  dy: 8,
                },
              },
              yAxis: { title: yAxisTitle, format: 'metric' },
            }}
          >
            {#snippet tooltip()}
              <Tooltip.Root>
                {#snippet children({ data }: { data: Bucket })}
                  <Tooltip.Header class="text-center">{formatBucketRange(data)}</Tooltip.Header>
                  <Tooltip.List>
                    <Tooltip.Item label="count" value={data.count} format="integer" />
                    <Tooltip.Item label="of total" value={formatPct(data.count)} />
                  </Tooltip.List>
                {/snippet}
              </Tooltip.Root>
            {/snippet}

            {#snippet aboveMarks({ context }: { context: any })}
              {@const xs = context.xScale}
              {@const bw = typeof xs.bandwidth === 'function' ? xs.bandwidth() : 0}
              {@const yTop = context.yRange[1]}
              {@const yBot = context.yRange[0]}
              {#each quantileMarks as m (m.key)}
                {@const x0 = xs(buckets[m.bucketIndex].label)}
                {@const px = x0 + bw * m.fraction}
                <g class="quantile-marker" style:--marker-color={m.color}>
                  <title>{m.label}: {m.value.toFixed(2)}</title>
                  <Line
                    x1={px}
                    x2={px}
                    y1={yTop}
                    y2={yBot}
                    class="quantile-line"
                  />
                  <Text
                    value={m.label}
                    x={px}
                    y={yTop}
                    dy={-2}
                    textAnchor="middle"
                    verticalAnchor="end"
                    class="quantile-label"
                  />
                </g>
              {/each}
            {/snippet}
          </BarChart>
        </div>
      </div>
    </div>
  {:else}
    <div class="flex items-center justify-center text-base-content/40 text-sm" style:height="{height}px">
      No buckets to chart
    </div>
  {/if}
</div>

<style lang="postcss">
  @reference "../../app.css";

  .histogram-stats {
    @apply flex flex-wrap gap-x-4 gap-y-1 px-2 py-2;
  }

  .histogram-measure {
    @apply w-full;
  }

  /* Horizontal scroll appears only when the natural chart width
     (bucketCount * BAR_SLOT_WIDTH) exceeds the parent. Below that
     threshold the inner .histogram-chart sits at parent width and
     no scroll bar appears. */
  .histogram-chart-scroll {
    @apply w-full overflow-x-auto overflow-y-hidden rounded-lg;
  }

  .histogram-chart {
    /* width set inline; height inherited from parent. */
  }

  .quantile-marker {
    pointer-events: auto;
  }

  .quantile-marker :global(.quantile-line) {
    stroke: var(--marker-color, currentColor);
    stroke-width: 1.5;
    stroke-dasharray: 4 3;
    opacity: 0.75;
  }

  .quantile-marker :global(.quantile-label) {
    fill: var(--marker-color, currentColor);
    font-size: 10px;
    font-weight: 600;
    pointer-events: none;
  }
</style>
