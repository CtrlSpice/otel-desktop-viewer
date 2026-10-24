<script module lang="ts">
  import { formatChartAxisTime } from '@/components/metrics/utils/chart-time-axis'
  import {
    formatMetricValue,
    formatMetricCount,
  } from '@/components/metrics/utils/format-metric-value'
  import type { Timezone } from '@/utils/time'

  const rotatedX = { rotate: 315, textAnchor: 'end' } as const
  const aboveYAxis = {
    labelPlacement: 'start' as const,
    labelProps: { dy: 8, textAnchor: 'start' as const },
  }

  const rotatedXBand = {
    ...rotatedX,
    verticalAnchor: 'middle',
    dy: 8,
  } as const

  /** Fallback plot height (px) before the chart pane is measured. */
  export const DEFAULT_METRIC_CHART_HEIGHT = 300

  /** Floor for fluid chart height when the pane is measured. */
  export const MIN_METRIC_CHART_HEIGHT = 280

  /** Fallback insets before rendered axis labels can be measured. */
  export const chartPadding = {
    top: 24,
    bottom: 96,
    left: 136,
    right: 72,
  } as const

  /** A horizontal unit above the value axis needs no side-label gutter. */
  export const timeSeriesChartPadding = {
    ...chartPadding,
    left: 96,
    right: 80,
  } as const

  function valueYAxis(label: string, left: number) {
    return {
      label: label.trim() || 'value',
      ...aboveYAxis,
      rule: false as const,
      tickLabelProps: { dx: -left, textAnchor: 'start' as const },
      format: (value: number) => formatMetricValue(value),
    }
  }

  /** Time on the x-axis (line charts, heatmap). */
  export function axisTime(timezone: Timezone) {
    return {
      rule: false as const,
      format: (tick: Date | number) => formatChartAxisTime(tick, timezone),
      tickLabelProps: rotatedX,
    }
  }

  /** Bucket bounds on the bottom x-axis (histogram bar chart). */
  export function axisBuckets(unit: string) {
    const { label, rule } = axisBucketBounds(unit)
    return {
      label,
      rule,
      tickLabelProps: rotatedXBand,
    }
  }

  /** Bucket bound rows on the heatmap y-axis (color encodes count). */
  export function axisBucketBounds(
    unit: string,
    left: number = chartPadding.left
  ) {
    const trimmed = unit.trim()
    return {
      label: trimmed ? `value (${trimmed})` : 'value',
      ...aboveYAxis,
      rule: false as const,
      tickLabelProps: { dx: -left, textAnchor: 'start' as const },
    }
  }

  /** Unit or fixed label on the y-axis. */
  export function axisValue(label: string, left: number = chartPadding.left) {
    return valueYAxis(label, left)
  }

  export function axisCount(left: number = chartPadding.left) {
    return { ...valueYAxis('count', left), format: formatMetricCount }
  }
</script>

<script lang="ts">
  import type { Snippet } from 'svelte'
  import MetricAxes from './MetricAxes.svelte'

  type Props = {
    /** Plot height in px. Omit when the parent already sizes the box. */
    height?: number
    /** Plot width in px (e.g. scrollable histogram). */
    width?: number
    padding?: { top: number; right: number; bottom: number; left: number }
    class?: string
    children: Snippet
  }

  let {
    height,
    width,
    padding = chartPadding,
    class: className = '',
    children,
  }: Props = $props()
</script>

<div
  class="metric-chart-view metric-chart-plot {className}"
  style:height={height != null ? `${height}px` : undefined}
  style:width={width != null ? `${width}px` : undefined}
>
  {@render children()}
  <MetricAxes {padding} />
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .metric-chart-plot {
    @apply relative w-full;
  }
</style>
