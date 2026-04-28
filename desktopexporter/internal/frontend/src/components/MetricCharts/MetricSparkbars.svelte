<script lang="ts">
  import { BarChart } from 'layerchart'

  type SparkBar = { index: number; value: number }

  type Props = {
    buckets: number[]
    height?: number
    width?: number
  }

  let { buckets, height = 32, width }: Props = $props()

  let sparkData = $derived(
    buckets.map((value, index): SparkBar => ({ index, value }))
  )
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
