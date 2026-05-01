<script lang="ts">
  import { AreaChart } from 'layerchart'
  import type { SparklinePoint } from '@/types/api-types'

  type SparkPoint = { date: number; value: number }

  type Props = {
    points: SparklinePoint[]
    height?: number
    width?: number
    /** Spark stroke/fill — use `metricTypeSeriesColor()` to match type badge */
    seriesColor?: string
  }

  let { points, height = 32, width, seriesColor = 'var(--color-primary)' }: Props =
    $props()

  let sparkData = $derived.by((): SparkPoint[] => {
    return points
      .map((p) => ({ date: Number(p.timestamp / 1_000_000n), value: p.value }))
      .sort((a, b) => a.date - b.date)
  })
</script>

{#if sparkData.length >= 2}
  <AreaChart
    data={sparkData}
    x="date"
    y="value"
    axis={false}
    grid={false}
    series={[
      {
        key: 'default',
        label: 'value',
        value: 'value',
        color: seriesColor,
      },
    ]}
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
