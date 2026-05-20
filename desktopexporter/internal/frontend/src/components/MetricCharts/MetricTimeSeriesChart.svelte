<script lang="ts">
  import { LineChart, Line, Text } from 'layerchart'
  import { scaleTime } from 'd3-scale'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import MetricChartEmpty from '@/components/MetricChartEmpty.svelte'
  import MetricChartPlot, {
    axisTime,
    axisValue,
    chartPadding,
    DEFAULT_METRIC_CHART_HEIGHT,
  } from '@/components/MetricChartPlot.svelte'
  import { chartNeutral } from '@/utils/chart-palette'
  import type { ChartTimeseries } from './chart-types'

  type Props = {
    /** Pre-grouped timeseries. The Nth entry's colour is read from
     * `ctx.timeseriesChartColors[N]` (same array the legend swatches
     * use), so legend row N's swatch always matches chart line N. */
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
    height = DEFAULT_METRIC_CHART_HEIGHT,
    highlightedTimestamp = null,
    unit = '',
  }: Props = $props()

  // Palette + colour-index map come from the metric view context so the
  // legend and the chart can never disagree. The chart used to build its
  // own palette here; that produced two callsites that could drift on
  // theme switch or metric-type change.
  const ctx = getMetricViewContext()

  const timeContext = getTimeContext()

  // Build the layerchart series array on the fly. Each entry carries
  // its own pre-grouped data so we don't re-traverse on every chart
  // re-render. Colour is looked up via the context's colour-index map
  // (keyed by attributesKey), not by position in this prop's array --
  // so toggling visibility never shifts a line's colour and the legend
  // swatch at `colorIdx` always matches chart line at `colorIdx`.
  let chartSeries = $derived.by(() => {
    return timeseries
      .map(ts => {
        return {
          key: ts.key,
          label: ts.label,
          data: ts.points,
          color: ctx.timeseriesColorByKey.get(ts.key) ?? chartNeutral(),
        }
      })
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
