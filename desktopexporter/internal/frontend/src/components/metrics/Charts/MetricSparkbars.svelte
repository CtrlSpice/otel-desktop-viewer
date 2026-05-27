<script lang="ts">
  import { BarChart } from 'layerchart'

  type SparkBar = { index: number; value: number }

  type Props = {
    buckets: number[]
    height?: number
    width?: number
    /** Bar fill — use `metricTypeSeriesColor()` to match type badge */
    seriesColor?: string
  }

  let {
    buckets,
    height = 32,
    width,
    seriesColor = 'var(--color-primary)',
  }: Props = $props()

  let sparkData = $derived(
    buckets.map((value, index): SparkBar => ({ index, value }))
  )
</script>

{#if sparkData.length >= 1}
  <!-- Disable tooltip/highlight contexts: see MetricSparkline for the
       full rationale. The drawer is a list view, not an analysis surface.
       bandPadding=0.3 (was 0.1) widens the gap between bars; with sparks
       at ~96px wide and only 5-9 buckets, the previous setting gave a
       chunky "brick wall" silhouette. More gap = bars feel like bars. -->
  <BarChart
    data={sparkData}
    x="index"
    y="value"
    axis={false}
    grid={false}
    bandPadding={0.3}
    tooltipContext={false}
    highlight={false}
    series={[
      {
        key: 'default',
        label: 'value',
        value: 'value',
        color: seriesColor,
      },
    ]}
    props={{ bars: { radius: 1, strokeWidth: 0 } }}
    {height}
    {width}
  />
{:else}
  <div class="flex items-center justify-center text-base-content/30 text-[0.6rem]" style:height="{height}px">
    —
  </div>
{/if}
