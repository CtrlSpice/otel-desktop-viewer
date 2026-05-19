<script lang="ts">
  import { LineChart, Line, Text } from 'layerchart'
  import { scaleTime } from 'd3-scale'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import MetricChartEmpty from '@/components/MetricChartEmpty.svelte'
  import MetricChartPlot, {
    axisTime,
    axisValue,
    chartPadding,
  } from '@/components/MetricChartPlot.svelte'
  import { timeseriesColor } from '@/utils/timeseries-palette'
  import type { ChartTimeseries } from './chart-types'

  type Props = {
    /** Pre-grouped timeseries. The Nth entry's colour is
     * `timeseriesColor(N)`, matching the TimeseriesLegend's positional
     * colouring -- so legend row N's swatch is the same colour as
     * chart line N. */
    timeseries: ChartTimeseries[]
    /** Set of timeseries keys currently checked in the legend.
     * Timeseries not in this set are filtered out before being passed
     * to layerchart. */
    visibleKeys: Set<string>
    height?: number
    /** Timestamp (ns, as bigint) of a datapoint to highlight on the
     * chart. When set, draws a vertical rule + small label at that
     * x-coordinate so a click on the datapoints list visually
     * anchors to its point. */
    highlightedTimestamp?: bigint | null
    /** Metric unit for the y-axis label (e.g. "ms", "bytes"). */
    unit?: string
  }

  let {
    timeseries,
    visibleKeys,
    height = 250,
    highlightedTimestamp = null,
    unit = '',
  }: Props = $props()

  const timeContext = getTimeContext()

  // Build the layerchart series array on the fly. Each entry carries
  // its own pre-grouped data so we don't re-traverse on every chart
  // re-render. Colour comes from the timeseries' index in the *full*
  // list (not the visible list), so toggling one off and on doesn't
  // shift colours of its neighbours.
  let chartSeries = $derived.by(() => {
    return timeseries
      .map((ts, i) => ({
        key: ts.key,
        label: ts.label,
        data: ts.points,
        color: timeseriesColor(i),
      }))
      .filter(ts => visibleKeys.has(ts.key))
  })

  // Total visible point count -- if every series is empty (or all
  // hidden) we render a placeholder instead of an empty chart frame
  // so the user knows the absence is real, not a load state.
  let visiblePointCount = $derived.by(() => {
    let n = 0
    for (const ts of chartSeries) n += ts.data.length
    return n
  })

  // Resolve the highlighted timestamp to a chart-domain Date so we can
  // place the vertical rule. Fall back to null when no highlight or
  // the value falls outside the loaded range.
  let highlightDate = $derived.by((): Date | null => {
    if (highlightedTimestamp === null || highlightedTimestamp === undefined) {
      return null
    }
    return new Date(Number(highlightedTimestamp / 1_000_000n))
  })

  let yAxisLabel = $derived(unit.trim() || 'value')
</script>

{#if visiblePointCount > 0}
  <div class="metric-time-series-chart">
    <MetricChartPlot {height}>
      <LineChart
        x="date"
        y="value"
        xScale={scaleTime()}
        yNice
        padding={chartPadding}
        tooltipContext
        series={chartSeries}
        props={{
          xAxis: axisTime(timeContext.timezone),
          yAxis: axisValue(yAxisLabel),
        }}
      >
        {#snippet aboveMarks({ context }: { context: any })}
          {#if highlightDate}
            {@const px = context.xScale(highlightDate)}
            {@const yTop = context.yRange[1]}
            {@const yBot = context.yRange[0]}
            <g>
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
      </LineChart>
    </MetricChartPlot>
  </div>
{:else}
  <MetricChartEmpty
    {height}
    message={timeseries.length === 0
      ? 'No datapoints to chart'
      : visibleKeys.size === 0
        ? 'Nothing to see here — select a timeseries below'
        : 'No datapoints in selected timeseries'}
  />
{/if}

<style lang="postcss">
  @reference "../../app.css";

  .metric-time-series-chart {
    @apply w-full rounded-lg;
  }
</style>
