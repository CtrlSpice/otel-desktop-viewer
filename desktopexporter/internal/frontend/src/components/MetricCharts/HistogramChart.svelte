<script lang="ts">
  import { BarChart, Line, Text, Tooltip } from 'layerchart'
  import { scaleBand, scaleLinear } from 'd3-scale'
  import { telemetryAPI } from '@/services/telemetry-service'
  import type {
    HistogramDataPoint,
    ExponentialHistogramDataPoint,
  } from '@/types/api-types'

  // lo/hi are the numeric bucket bounds; they may be -Infinity, +Infinity, or
  // (for the exp-histogram zero bucket) both 0. Used to position quantile
  // markers within their bar.
  type Bucket = { label: string; count: number; lo: number; hi: number }

  type Props = {
    datapoint: HistogramDataPoint | ExponentialHistogramDataPoint
    height?: number
  }

  let { datapoint, height = 250 }: Props = $props()

  // Hardcoded for now; the plan defers configurable quantiles to a future pass.
  // Keys must match Go's strconv.FormatFloat(q, 'f', -1, 64) output.
  const QUANTILES = [0.5, 0.95, 0.99]
  const QUANTILE_LABELS: { key: string; label: string }[] = [
    { key: '0.5', label: 'p50' },
    { key: '0.95', label: 'p95' },
    { key: '0.99', label: 'p99' },
  ]

  let quantiles = $state<Record<string, number | null> | null>(null)

  // Lazy fetch keyed on datapoint id. Cleanup function flips a closure-scoped
  // flag so that responses arriving after the user switches datapoints are
  // dropped rather than overwriting the now-stale state.
  $effect(() => {
    const dpId = datapoint.id
    let cancelled = false
    quantiles = null
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
    <div class="histogram-chart" style:height="{height}px">
      <BarChart
        data={buckets}
        x="label"
        xScale={scaleBand()}
        y="count"
        yScale={scaleLinear()}
        yNice
        bandPadding={0.2}
        padding={{ top: 16, right: 8, bottom: 48, left: 48 }}
        tooltipContext
        props={{
          xAxis: {
            tickLabelProps: {
              rotate: 315,
              textAnchor: 'end',
              verticalAnchor: 'middle',
              dy: 8,
            },
          },
          yAxis: { format: 'metric' },
        }}
      >
        {#snippet tooltip()}
          <Tooltip.Root>
            {#snippet children({ data })}
              <Tooltip.Header class="text-center">{data.label}</Tooltip.Header>
              <Tooltip.List>
                <Tooltip.Item label="count" value={data.count} format="integer" />
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
  {:else}
    <div class="flex items-center justify-center text-base-content/40 text-sm" style:height="{height}px">
      No buckets to chart
    </div>
  {/if}
</div>

<style lang="postcss">
  @reference "../../app.css";

  .histogram-stats {
    @apply flex flex-wrap gap-x-4 gap-y-1 px-2 py-2 text-xs text-base-content/70;
  }

  .histogram-stats strong {
    @apply text-base-content tabular-nums;
  }

  .histogram-chart {
    @apply w-full overflow-hidden rounded-lg;
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
