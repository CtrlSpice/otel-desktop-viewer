<script lang="ts">
  import { getChartContext, type AxisProps } from 'layerchart'
  import {
    metricAxisTickPositions,
    metricDotGuides,
  } from '@/components/metrics/utils/metric-dot-grid'

  let {
    xAxis,
    yAxis,
  }: {
    xAxis: Omit<AxisProps, 'placement'>
    yAxis: Omit<AxisProps, 'placement'>
  } = $props()
  const chart = getChartContext()
  const clipID = $props.id()
  let columns = $derived(metricDotGuides(
    metricAxisTickPositions(chart.xScale, chart.width, xAxis, 80), chart.width, 'end'
  ))
  let rows = $derived(metricDotGuides(
    metricAxisTickPositions(chart.yScale, chart.height, yAxis, 50), chart.height, 'start'
  ))
</script>

<g class="metric-dot-grid" aria-hidden="true" clip-path="url(#{clipID})">
  <defs>
    <clipPath id={clipID}>
      <rect
        width={chart.width}
        height={chart.height}
      />
    </clipPath>
  </defs>
  {#each rows as row (row.position)}
    {#each columns as column (column.position)}
      <circle
        cx={column.position}
        cy={row.position}
        style:opacity="calc(var(--guide-dot-opacity) * {Math.min(row.opacity, column.opacity)})"
      />
    {/each}
  {/each}
</g>

<style lang="postcss">
  @reference "../../../app.css";

  .metric-dot-grid {
    pointer-events: none;
  }

  .metric-dot-grid circle {
    fill: var(--guide-dot-color);
    stroke: none;
    r: var(--guide-dot-radius);
  }
</style>
