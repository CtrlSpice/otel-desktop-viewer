<script lang="ts">
  import { AreaChart } from 'layerchart'
  import type { DataPoint, GaugeDataPoint, SumDataPoint } from '@/types/api-types'

  type SparkPoint = { date: number; value: number }

  type Props = {
    datapoints: DataPoint[]
    height?: number
    width?: number
  }

  let { datapoints, height = 32, width }: Props = $props()

  let sparkData = $derived.by((): SparkPoint[] => {
    const points: SparkPoint[] = []
    for (const dp of datapoints) {
      if (dp.metricType !== 'Gauge' && dp.metricType !== 'Sum') continue
      const typed = dp as GaugeDataPoint | SumDataPoint
      const value = typed.doubleValue ?? typed.intValue ?? 0
      points.push({ date: Number(dp.timestamp / 1_000_000n), value })
    }
    points.sort((a, b) => a.date - b.date)
    return points
  })
</script>

{#if sparkData.length >= 2}
  <AreaChart
    data={sparkData}
    x="date"
    y="value"
    axis={false}
    grid={false}
    props={{
      highlight: { points: { r: 3, class: 'stroke-2 stroke-surface-100' } },
    }}
    {height}
    {width}
  />
{:else}
  <div class="flex items-center justify-center text-base-content/30 text-[0.6rem]" style:height="{height}px">
    —
  </div>
{/if}
