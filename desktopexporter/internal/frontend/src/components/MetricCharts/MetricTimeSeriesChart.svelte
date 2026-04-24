<script lang="ts">
  import { AreaChart } from 'layerchart'
  import { scaleTime } from 'd3-scale'
  import type { DataPoint, GaugeDataPoint, SumDataPoint } from '@/types/api-types'

  type ChartPoint = { date: Date; value: number }

  type Props = {
    datapoints: DataPoint[]
    height?: number
  }

  let { datapoints, height = 250 }: Props = $props()

  let chartData = $derived.by(() => {
    const points: ChartPoint[] = []
    for (const dp of datapoints) {
      if (dp.metricType !== 'Gauge' && dp.metricType !== 'Sum') continue
      const typed = dp as GaugeDataPoint | SumDataPoint
      const value = typed.doubleValue ?? typed.intValue ?? 0
      const ts = Number(dp.timestamp / 1_000_000n)
      points.push({ date: new Date(ts), value })
    }
    points.sort((a, b) => a.date.getTime() - b.date.getTime())
    return points
  })
</script>

{#if chartData.length > 0}
  <div class="metric-time-series-chart" style:height="{height}px">
    <AreaChart
      data={chartData}
      x="date"
      y="value"
      xScale={scaleTime()}
      yNice
      padding={{ top: 16, right: 8, bottom: 32, left: 48 }}
      tooltipContext
    />
  </div>
{:else}
  <div class="flex items-center justify-center text-base-content/40 text-sm" style:height="{height}px">
    No datapoints to chart
  </div>
{/if}

<style lang="postcss">
  @reference "../../app.css";

  .metric-time-series-chart {
    @apply w-full overflow-hidden rounded-lg;
  }
</style>
