<script lang="ts">
  import { AreaChart, Line, Text } from 'layerchart'
  import { scaleTime } from 'd3-scale'
  import type { DataPoint, GaugeDataPoint, SumDataPoint } from '@/types/api-types'

  type ChartPoint = { date: Date; value: number }

  type Props = {
    datapoints: DataPoint[]
    height?: number
    /** Timestamp (ns, as bigint) of a datapoint to highlight on the chart.
     * When set, draws a vertical rule + small label at that x-coordinate
     * so a click on the datapoints list visually anchors to its point. */
    highlightedTimestamp?: bigint | null
  }

  let { datapoints, height = 250, highlightedTimestamp = null }: Props = $props()

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

  // Resolve the highlighted timestamp to a chart-domain Date so we can
  // place the vertical rule. Fall back to null when no highlight or the
  // value falls outside the loaded range.
  let highlightDate = $derived.by((): Date | null => {
    if (highlightedTimestamp === null || highlightedTimestamp === undefined) {
      return null
    }
    return new Date(Number(highlightedTimestamp / 1_000_000n))
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
    >
      {#snippet aboveMarks({ context }: { context: any })}
        {#if highlightDate}
          {@const px = context.xScale(highlightDate)}
          {@const yTop = context.yRange[1]}
          {@const yBot = context.yRange[0]}
          <g class="highlight-marker">
            <Line
              x1={px}
              x2={px}
              y1={yTop}
              y2={yBot}
              class="highlight-rule"
            />
            <Text
              value="selected"
              x={px}
              y={yTop}
              dy={-2}
              textAnchor="middle"
              verticalAnchor="end"
              class="highlight-label"
            />
          </g>
        {/if}
      {/snippet}
    </AreaChart>
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

  .highlight-marker :global(.highlight-rule) {
    stroke: var(--color-primary);
    stroke-width: 1.5;
    stroke-dasharray: 4 3;
    opacity: 0.85;
  }

  .highlight-marker :global(.highlight-label) {
    fill: var(--color-primary);
    font-size: 10px;
    font-weight: 600;
    pointer-events: none;
  }
</style>
