<script lang="ts">
  import { LineChart, Line, Tooltip } from 'layerchart'
  import { bisector } from 'd3-array'
  import { scaleTime } from 'd3-scale'
  import { curveStepAfter } from 'd3-shape'
  import { formatMetricValue } from '@/components/metrics/utils/format-metric-value'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatDateTime } from '@/utils/time'
  import MetricChartEmpty from '@/components/metrics/Charts/MetricChartEmpty.svelte'
  import MetricChartPlot, {
    axisTime,
    axisValue,
    chartPadding,
    DEFAULT_METRIC_CHART_HEIGHT,
  } from '@/components/metrics/Charts/MetricChartPlot.svelte'
  import { chartNeutral } from '@/utils/chart-palette'
  import ChartSelectionLegend, {
    type SelectionLegendRow,
  } from '@/components/metrics/Charts/ChartSelectionLegend.svelte'
  import {
    AGG_KEY_ALL,
    AGG_KEY_SELECTED,
    AGG_KEY_TOTAL,
    aggregateLineLabel,
    type AggregateLineKey,
    type AggregationView,
  } from '@/components/metrics/utils/aggregation'
  import type { ChartTimeseries } from '@/types/metric-chart-types'

  /** Render order inside the Totals section: checked → all. */
  const AGG_TOTAL_ORDER: Record<string, number> = {
    [AGG_KEY_SELECTED]: 0,
    [AGG_KEY_ALL]: 1,
    [AGG_KEY_TOTAL]: 1,
  }

  function isAggregateKey(key: string): key is AggregateLineKey {
    return (
      key === AGG_KEY_SELECTED ||
      key === AGG_KEY_ALL ||
      key === AGG_KEY_TOTAL
    )
  }

  function aggregateLabelForKey(key: string): string {
    return isAggregateKey(key)
      ? aggregateLineLabel(key, aggregationView)
      : key
  }

  /** Nearest point at `x` — layerchart's default tooltip matches exact
   *  timestamps only, which breaks when raw and aggregate grids differ. */
  function nearestValueAt(
    points: readonly { date: Date; value: number }[],
    x: Date
  ): number | undefined {
    if (points.length === 0) return undefined
    const t = x.getTime()
    const i = bisector((p: { date: Date }) => p.date.getTime()).left(points, t)
    const lo = Math.max(0, i - 1)
    const hi = Math.min(points.length - 1, i)
    const dl = Math.abs(points[lo]!.date.getTime() - t)
    const dh = Math.abs(points[hi]!.date.getTime() - t)
    return (dl <= dh ? points[lo] : points[hi])!.value
  }

  function tooltipXDate(context: {
    tooltip: { data: unknown }
    x: (d: unknown) => unknown
    valueAxis: string
  }): Date | null {
    const d = context.tooltip.data
    if (d == null) return null
    const v = context.x(d)
    if (v instanceof Date) return v
    if (v != null) return new Date(v as string | number)
    return null
  }

  type Props = {
    /** Pre-grouped, already visibility-filtered timeseries. The caller
     *  (MetricViewContext via MetricChartView) drops timeseries the
     *  user has unchecked in the legend BEFORE handing the array down,
     *  so the chart trusts every entry here is meant to render. */
    timeseries: ChartTimeseries[]
    height?: number
    /** Timestamp (ns, as bigint) of a datapoint to highlight on the
     * chart. When set, draws a vertical rule at that x-coordinate so
     * a click on the datapoints list visually anchors to its point. */
    highlightedTimestamp?: bigint | null
    /** Attributes key of the timeseries owning the selected datapoint.
     *  When set alongside `highlightedTimestamp`, the chart draws a
     *  colored dot on the rule at that series's value. Aggregates
     *  always get dots too -- the selected series is the one we'd
     *  otherwise not be able to identify just from x-position. */
    selectedSeriesKey?: string | null
    /** Metric unit for the y-axis label (e.g. "ms", "bytes"). */
    unit?: string
    /** Per-timeseries color lookup keyed by `ChartTimeseries.key`. The
     *  caller owns this map (in practice it comes from the metric view
     *  context's color-index) so the chart line and the legend swatch
     *  for the same key always agree. Missing keys fall back to the
     *  neutral palette color. */
    colorByKey: ReadonlyMap<string, string>
    /** Message rendered when the chart has nothing to draw. The caller
     *  knows whether the cause is "no timeseries on this metric", "all
     *  unchecked", or "filtered to empty"; we just render whatever
     *  string it picks. Defaults to a generic fallback. */
    emptyMessage?: string
    /** Active chart view (Sum / Avg / Rate). Drives the aggregate-row
     *  glyph in the tooltip; omit or 'raw' when no aggregates render. */
    aggregationView?: AggregationView
  }

  let {
    timeseries,
    height = DEFAULT_METRIC_CHART_HEIGHT,
    highlightedTimestamp = null,
    selectedSeriesKey = null,
    unit = '',
    colorByKey,
    emptyMessage = 'No datapoints to chart',
    aggregationView = 'raw',
  }: Props = $props()

  const timeContext = getTimeContext()

  // Build the layerchart series array on the fly. Each entry carries
  // its own pre-grouped data so we don't re-traverse on every chart
  // re-render. Colour is looked up via the caller-provided `colorByKey`
  // map (keyed by attributesKey), not by position in this prop's array --
  // so toggling visibility never shifts a line's colour and the legend
  // swatch always matches the chart line for the same key.
  /** Per-series Spline props for cross-series aggregate lines.
   *
   *  - `stroke-dasharray`: visually distinguish aggregates from raw.
   *    Note: SVG attribute name (kebab-case) — Svelte does not
   *    translate JSX-style `strokeDasharray` to the SVG attribute.
   *  - `curve: curveStepAfter`: aggregates are per-bucket scalars
   *    ("average over this window", "rate during this window"). A
   *    smooth interpolation between bucket centers would imply
   *    continuity that doesn't exist; step-after draws a literal
   *    staircase that holds each bucket's value until the next
   *    bucket starts. Raw lines stay smooth because each raw point
   *    is an actual sample, not a window aggregate. */
  const AGG_LINE_PROPS = {
    'stroke-dasharray': '6 4',
    curve: curveStepAfter,
  } as const

  let chartSeries = $derived.by(() => {
    return timeseries.map(ts => ({
      key: ts.key,
      label: ts.label,
      data: ts.points,
      color: colorByKey.get(ts.key) ?? chartNeutral(),
      ...(isAggregateKey(ts.key) ? { props: AGG_LINE_PROPS } : {}),
    }))
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

  // Rate view: the y-axis is in "<unit> per second," so append "/s"
  // to the displayed unit. OTLP's dimensionless marker is "1", which
  // reads as "1/s" — that's noisy, so collapse it to just "/s" so the
  // axis says something honest ("events per second") without the
  // literal "1" leaking through. Empty unit + rate also collapses to
  // "/s" for the same reason.
  let yAxisLabel = $derived.by((): string => {
    const u = unit.trim()
    if (aggregationView === 'rate') {
      if (u === '' || u === '1') return '/s'
      return `${u}/s`
    }
    return u || 'value'
  })

  /** Series the selection rule should drop colored dots on. Always
   *  empty when nothing is selected. Otherwise: the user-selected
   *  series (if it's still in `chartSeries`) plus every aggregate
   *  currently rendered. Aggregates included unconditionally so users
   *  can read each aggregate's value at the selected x-coordinate
   *  without hover-tracking the cursor. Values resolved via the same
   *  `nearestValueAt` lookup the tooltip uses, so raw and aggregate
   *  grids that disagree on exact timestamps still produce dots. */
  type SelectionDot = {
    key: string
    color: string
    value: number
    isSelected: boolean
  }
  let selectionDots = $derived.by((): SelectionDot[] => {
    if (highlightDate === null) return []
    const dots: SelectionDot[] = []
    for (const s of chartSeries) {
      const isSelected = s.key === selectedSeriesKey
      const isAggregate = isAggregateKey(s.key)
      if (!isSelected && !isAggregate) continue
      const v = nearestValueAt(s.data, highlightDate)
      if (v === undefined || !Number.isFinite(v)) continue
      dots.push({ key: s.key, color: s.color, value: v, isSelected })
    }
    return dots
  })

  /** Pre-formatted timestamp string for the legend card header. Reuses
   *  the project-wide `formatDateTime` helper (millisecond resolution,
   *  timezone-aware) so the legend, hover tooltip header, and the
   *  datapoints list all read identical timestamps. */
  let selectionTimestampText = $derived.by((): string => {
    if (highlightDate === null) return ''
    return formatDateTime(
      highlightDate.getTime(),
      timeContext.timezone,
      'milliseconds'
    )
  })

  /** Mini-legend rows derived from the dots already computed for the
   *  chart markers. Aggregates render first in the same order the
   *  tooltip uses (Checked → All → Unchecked), then the selected
   *  series row last so the eye lands on it as the "anchor" after
   *  scanning the totals it should be compared against. We don't
   *  filter here -- the chart already filtered to "what's on screen"
   *  via selectionDots, so the legend is a 1:1 textual companion of
   *  the visible dots. */
  let selectionLegendRows = $derived.by((): SelectionLegendRow[] => {
    if (selectionDots.length === 0) return []
    const dots = selectionDots
      .slice()
      .sort((a, b) => {
        if (a.isSelected !== b.isSelected) return a.isSelected ? 1 : -1
        const ao = AGG_TOTAL_ORDER[a.key] ?? 99
        const bo = AGG_TOTAL_ORDER[b.key] ?? 99
        return ao - bo
      })
    const labelByKey = new Map(chartSeries.map(s => [s.key, s.label] as const))
    return dots.map((d): SelectionLegendRow => {
      const isAggregate = isAggregateKey(d.key)
      const label = isAggregate
        ? aggregateLabelForKey(d.key)
        : (labelByKey.get(d.key) ?? d.key)
      return {
        key: d.key,
        color: d.color,
        label,
        glyph: null,
        glyphTitle: null,
        valueText: formatMetricValue(d.value),
        isPrimary: d.isSelected,
      }
    })
  })
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
        tooltipContext={{ mode: 'bisect-x' }}
        series={chartSeries}
        props={{
          xAxis: axisTime(timeContext.timezone),
          yAxis: axisValue(yAxisLabel),
        }}
      >
        {#snippet tooltip({ context }: { context: any })}
          {@const xDate = tooltipXDate(context)}
          {@const rawItems = chartSeries
            .filter(s => !isAggregateKey(s.key))
            .slice()
            .sort((a, b) =>
              String(a.label ?? a.key).localeCompare(String(b.label ?? b.key))
            )}
          {@const aggItems = chartSeries
            .filter(s => isAggregateKey(s.key))
            .slice()
            .sort(
              (a, b) =>
                (AGG_TOTAL_ORDER[a.key] ?? 99) - (AGG_TOTAL_ORDER[b.key] ?? 99)
            )}
          {@const headerLabel =
            xDate != null
              ? formatDateTime(xDate.getTime(), timeContext.timezone, 'milliseconds')
              : undefined}
          <Tooltip.Root {context}>
            {#snippet children()}
              <Tooltip.Header value={headerLabel} />
              <Tooltip.List>
                {#each rawItems as s (s.key)}
                  {@const value =
                    xDate != null ? nearestValueAt(s.data, xDate) : undefined}
                  {#if value !== undefined}
                    <Tooltip.Item
                      label={s.label}
                      {value}
                      color={s.color}
                      format={formatMetricValue}
                      valueAlign="right"
                    />
                  {/if}
                {/each}
                {#if aggItems.length > 0 && xDate != null}
                  <Tooltip.Separator />
                  <div class="lc-tooltip-agg-row">
                    {#each aggItems as s, i (s.key)}
                      {@const value = nearestValueAt(s.data, xDate)}
                      {#if value !== undefined}
                        {#if i > 0}
                          <span class="lc-tooltip-agg-sep" aria-hidden="true"
                            >·</span
                          >
                        {/if}
                        <span class="lc-tooltip-agg-seg">
                          <span
                            class="lc-tooltip-agg-dot"
                            style:--color={s.color}
                          ></span>
                          <span class="lc-tooltip-agg-label"
                            >{aggregateLabelForKey(s.key)}</span
                          >
                          <span class="lc-tooltip-agg-value"
                            >{formatMetricValue(value)}</span
                          >
                        </span>
                      {/if}
                    {/each}
                  </div>
                {/if}
              </Tooltip.List>
            {/snippet}
          </Tooltip.Root>
        {/snippet}

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
              {#each selectionDots as dot (dot.key)}
                {@const py = context.yScale(dot.value)}
                <!-- Halo ring drawn first so the colored dot sits on
                     top. Stroke-only so the line's own color shows
                     through the center, keeping the dot readable
                     against overlapping series. -->
                <circle
                  cx={px}
                  cy={py}
                  r="8"
                  class="selection-dot-halo"
                  class:selection-dot-halo--selected={dot.isSelected}
                />
                <circle
                  cx={px}
                  cy={py}
                  r="6"
                  fill={dot.color}
                  class="selection-dot"
                  class:selection-dot--selected={dot.isSelected}
                />
              {/each}
            </g>
          {/if}
        {/snippet}
      </LineChart>
      {#if selectionLegendRows.length > 0}
        <!-- Pinned mini-legend: floats in the top-right of the plot
             box so the chart pane drives positioning. pointer-events
             are off on the card itself so the user can still hover
             through it onto the chart to read the live tooltip. -->
        <div class="metric-time-series-chart__selection-legend">
          <ChartSelectionLegend
            timestamp={selectionTimestampText}
            rows={selectionLegendRows}
          />
        </div>
      {/if}
    </MetricChartPlot>
  </div>
{:else}
  <MetricChartEmpty {height} message={emptyMessage} />
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .metric-time-series-chart {
    @apply w-full rounded-lg;
  }

  /* Host for the pinned mini-legend. Lives inside MetricChartPlot's
     relative box so it tracks the chart's actual rendered frame, not
     the outer panel. `pointer-events: none` on the host so neither
     the chart's hover nor any click escapes are blocked; the legend
     card itself also has pointer-events: none. */
  .metric-time-series-chart__selection-legend {
    position: absolute;
    top: 0.5rem;
    right: 0.5rem;
    pointer-events: none;
    z-index: 1;
  }

  /* Aggregate summary: one row spanning the tooltip grid (Checked · All ·
     Unchecked with color dots + values). */
  :global(.lc-tooltip-agg-row) {
    grid-column: 1 / -1;
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    justify-content: center;
    gap: 0.35rem 0.5rem;
    margin-top: 2px;
    font-size: 0.75rem;
    line-height: 1.35;
    text-align: center;
  }

  :global(.lc-tooltip-agg-op) {
    font-size: 0.8em;
    font-weight: 600;
    line-height: 1;
    opacity: 0.85;
  }

  :global(.lc-tooltip-agg-seg) {
    display: inline-flex;
    align-items: baseline;
    gap: 0.25rem;
    white-space: nowrap;
  }

  :global(.lc-tooltip-agg-dot) {
    display: inline-block;
    width: 7px;
    height: 7px;
    flex-shrink: 0;
    align-self: center;
    border-radius: 9999px;
    background-color: var(--color);
  }

  :global(.lc-tooltip-agg-label) {
    font-weight: 500;
    color: color-mix(
      in oklab,
      var(--color-surface-content, currentColor) 75%,
      transparent
    );
  }

  :global(.lc-tooltip-agg-value) {
    font-variant-numeric: tabular-nums;
    font-weight: 600;
  }

  :global(.lc-tooltip-agg-sep) {
    opacity: 0.45;
    user-select: none;
  }
</style>
