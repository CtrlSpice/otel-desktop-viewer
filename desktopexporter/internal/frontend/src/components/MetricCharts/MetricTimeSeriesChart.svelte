<script lang="ts" module>
  import type {
    GaugeDataPoint,
    MetricTimeseries,
    SumDataPoint,
  } from '@/types/api-types'

  export type ChartPoint = { date: Date; value: number }

  /**
   * One per-attribute timeseries projected for layerchart. The
   * component renders one line per timeseries that's still in
   * `visibleKeys`; hidden timeseries are skipped entirely (no
   * greyed-out ghost line) so the chart stays readable when the user
   * is iteratively narrowing the visible set.
   *
   * Lives in <script module> so callers can import the type directly.
   */
  export type ChartTimeseries = {
    /** Stable per-timeseries id. The canonical "key=value|..." string
     * from MetricTimeseries.attributesKey. The legend uses the same
     * key, so checking/unchecking maps 1:1. */
    key: string
    /** Human label for the layerchart series. The chart's tooltip
     * surfaces this. We feed in the same canonical attribute string
     * so the tooltip and the legend agree. */
    label: string
    points: ChartPoint[]
  }

  /**
   * Project backend-grouped MetricTimeseries into the {date, value}
   * shape layerchart wants. The grouping itself is already done -- the
   * backend emits one MetricTimeseries per (metric, attribute-set), and
   * timeseries arrive ordered "newest activity first" (latest dp
   * timestamp desc). We preserve that order so positional colour
   * assignment in the legend matches the chart line colour 1:1.
   *
   * Datapoints arrive timestamp-desc inside each timeseries; we re-sort
   * ascending here because layerchart's LineChart expects
   * monotonically-increasing x values to draw a connected line.
   */
  export function timeseriesToChartTimeseries(
    timeseries: MetricTimeseries[]
  ): {
    chartTimeseries: ChartTimeseries[]
    /** Convenience: same `key` strings the timeseries have, in the
     * same order. Caller can seed `visibleKeys` from this without
     * having to map over `chartTimeseries`. */
    keys: string[]
  } {
    const chartTimeseries: ChartTimeseries[] = []
    const keys: string[] = []

    for (const ts of timeseries) {
      const points: ChartPoint[] = []
      for (const dp of ts.datapoints) {
        if (dp.metricType !== 'Gauge' && dp.metricType !== 'Sum') continue
        const typed = dp as GaugeDataPoint | SumDataPoint
        const value = typed.doubleValue ?? typed.intValue ?? 0
        const t = Number(dp.timestamp / 1_000_000n)
        points.push({ date: new Date(t), value })
      }
      points.sort((a, b) => a.date.getTime() - b.date.getTime())
      chartTimeseries.push({
        key: ts.attributesKey,
        label: ts.attributesKey === '' ? 'default' : ts.attributesKey,
        points,
      })
      keys.push(ts.attributesKey)
    }

    return { chartTimeseries, keys }
  }
</script>

<script lang="ts">
  import { LineChart, Line, Text } from 'layerchart'
  import { scaleTime } from 'd3-scale'
  import { timeseriesColor } from '@/utils/timeseries-palette'

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
  }

  let {
    timeseries,
    visibleKeys,
    height = 250,
    highlightedTimestamp = null,
  }: Props = $props()

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
      .filter((ts) => visibleKeys.has(ts.key))
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
</script>

{#if visiblePointCount > 0}
  <div class="metric-time-series-chart" style:height="{height}px">
    <LineChart
      x="date"
      y="value"
      xScale={scaleTime()}
      yNice
      padding={{ top: 16, right: 8, bottom: 64, left: 48 }}
      tooltipContext
      series={chartSeries}
      props={{
        xAxis: {
          tickLabelProps: {
            rotate: 315,
            textAnchor: 'end',
            verticalAnchor: 'middle',
            dy: 8,
          },
        },
      }}
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
    </LineChart>
  </div>
{:else}
  <div
    class="flex items-center justify-center text-base-content/40 text-sm"
    style:height="{height}px"
  >
    {#if timeseries.length === 0}
      No datapoints to chart
    {:else if visibleKeys.size === 0}
      No timeseries selected — pick one or more in the legend
    {:else}
      No datapoints in selected timeseries
    {/if}
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

