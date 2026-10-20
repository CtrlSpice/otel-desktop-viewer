<script lang="ts">
  import { getChartContext, type AxisProps } from 'layerchart'
  import {
    metricAxisTickPositions,
    metricDotGuides,
    metricCellEdgeGuides,
  } from '@/components/metrics/utils/metric-dot-grid'

  let {
    xAxis,
    yAxis,
    cellEdges = false,
    cellStride = 1,
  }: {
    xAxis: Omit<AxisProps, 'placement'>
    yAxis: Omit<AxisProps, 'placement'>
    cellEdges?: boolean
    cellStride?: 1 | 2
  } = $props()
  const chart = getChartContext()
  const clipID = $props.id()
  let xTicks = $derived(metricAxisTickPositions(chart.xScale, chart.width, xAxis, 80))
  let yTicks = $derived(metricAxisTickPositions(chart.yScale, chart.height, yAxis, 50))
  let columns = $derived(cellEdges
    ? metricCellEdgeGuides(chart.width, chart.xScale.domain().length, cellStride)
    : metricDotGuides(xTicks, chart.width))
  let rows = $derived(cellEdges
    ? metricCellEdgeGuides(chart.height, chart.yScale.domain().length, cellStride)
    : metricDotGuides(yTicks, chart.height))
</script>

<g class="metric-dot-grid" class:metric-dot-grid--cell-edges={cellEdges} aria-hidden="true" clip-path="url(#{clipID})">
  <defs>
    <clipPath id={clipID}>
      <rect
        width={chart.width}
        height={chart.height}
      />
    </clipPath>
  </defs>
  {#each rows as row (row)}
    {#each columns as column (column)}
      <circle
        cx={column}
        cy={row}
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
    opacity: var(--guide-dot-opacity);
    stroke: none;
    r: var(--guide-dot-radius);
  }
</style>
