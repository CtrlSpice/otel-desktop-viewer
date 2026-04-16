<script lang="ts">
  import { BarChart } from 'layerchart'
  import { scaleBand, scaleLinear } from 'd3-scale'
  import type {
    HistogramDataPoint,
    ExponentialHistogramDataPoint,
  } from '@/types/api-types'

  type Bucket = { label: string; count: number }

  type Props = {
    datapoint: HistogramDataPoint | ExponentialHistogramDataPoint
    height?: number
  }

  let { datapoint, height = 250 }: Props = $props()

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
      if (i === 0) {
        label = `(-∞, ${bounds[0]}]`
      } else if (i < bounds.length) {
        label = `(${bounds[i - 1]}, ${bounds[i]}]`
      } else {
        label = `(${bounds[bounds.length - 1]}, +∞)`
      }
      result.push({ label, count: counts[i] })
    }
    return result
  }

  function buildExpHistogramBuckets(dp: ExponentialHistogramDataPoint): Bucket[] {
    const result: Bucket[] = []
    const base = Math.pow(2, Math.pow(2, -dp.scale))

    if (dp.zeroCount > 0) {
      result.push({ label: '0', count: dp.zeroCount })
    }

    for (let i = 0; i < dp.positiveBucketCounts.length; i++) {
      const idx = dp.positiveBucketOffset + i
      const lo = Math.pow(base, idx).toPrecision(3)
      const hi = Math.pow(base, idx + 1).toPrecision(3)
      result.push({ label: `(${lo}, ${hi}]`, count: dp.positiveBucketCounts[i] })
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
</script>

<div>
  <div class="histogram-stats">
    <span>count: <strong>{stats.count}</strong></span>
    <span>sum: <strong>{stats.sum.toFixed(2)}</strong></span>
    <span>min: <strong>{stats.min.toFixed(2)}</strong></span>
    <span>max: <strong>{stats.max.toFixed(2)}</strong></span>
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
        bandPadding={0.15}
        padding={{ top: 16, right: 8, bottom: 48, left: 48 }}
        tooltipContext
      />
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
</style>
