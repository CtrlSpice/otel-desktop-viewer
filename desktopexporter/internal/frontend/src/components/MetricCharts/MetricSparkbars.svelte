<script lang="ts">
  import { BarChart } from 'layerchart'
  import type { DataPoint } from '@/types/api-types'

  type SparkBar = { index: number; value: number }

  type Props = {
    datapoints: DataPoint[]
    height?: number
    width?: number
  }

  let { datapoints, height = 32, width }: Props = $props()

  let sparkData = $derived.by((): SparkBar[] => {
    const bars: SparkBar[] = []
    for (const dp of datapoints) {
      if (dp.metricType !== 'Histogram' && dp.metricType !== 'ExponentialHistogram') continue
      bars.push({ index: bars.length, value: dp.count })
    }
    return bars
  })
</script>

{#if sparkData.length >= 1}
  <BarChart
    data={sparkData}
    x="index"
    y="value"
    axis={false}
    grid={false}
    bandPadding={0.1}
    props={{ bars: { radius: 1, strokeWidth: 0 } }}
    {height}
    {width}
  />
{:else}
  <div class="flex items-center justify-center text-base-content/30 text-[0.6rem]" style:height="{height}px">
    —
  </div>
{/if}
