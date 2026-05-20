<script module lang="ts">
  import { formatChartAxisTime } from '@/utils/chart-time-axis'
  import type { Timezone } from '@/utils/time'

  const rotatedX = { rotate: 315, textAnchor: 'end' } as const

  const rotatedXBand = {
    ...rotatedX,
    verticalAnchor: 'middle',
    dy: 8,
  } as const

  /** Fallback plot height (px) before the chart pane is measured. */
  export const DEFAULT_METRIC_CHART_HEIGHT = 300

  /** Floor for fluid chart height when the pane is measured. */
  export const MIN_METRIC_CHART_HEIGHT = 280

  /** Shared plot inset for line, bar, and heatmap charts. */
  export const chartPadding = {
    top: 24,
    bottom: 72,
    left: 72,
    right: 72,
  } as const

  function valueYAxis(label: string) {
    return {
      label: label.trim() || 'value',
      rule: true as const,
      format: 'metric' as const,
    }
  }

  /** Time on the x-axis (line charts, heatmap). */
  export function axisTime(timezone: Timezone) {
    return {
      rule: true as const,
      format: (tick: Date | number) => formatChartAxisTime(tick, timezone),
      tickLabelProps: rotatedX,
    }
  }

  /** Bucket bounds on the bottom x-axis (histogram bar chart). */
  export function axisBuckets(unit: string) {
    const trimmed = unit.trim()
    return {
      label: trimmed ? `value (${trimmed})` : 'value',
      rule: true as const,
      tickLabelProps: rotatedXBand,
    }
  }

  /** Unit or fixed label on the y-axis. */
  export function axisValue(label: string) {
    return valueYAxis(label)
  }

  export function axisCount() {
    return valueYAxis('count')
  }
</script>

<script lang="ts">
  import type { Snippet } from 'svelte'

  type Props = {
    /** Plot height in px. Omit when the parent already sizes the box. */
    height?: number
    /** Plot width in px (e.g. scrollable histogram). */
    width?: number
    class?: string
    children: Snippet
  }

  let { height, width, class: className = '', children }: Props = $props()
</script>

<div
  class="metric-chart-view metric-chart-plot {className}"
  style:height={height != null ? `${height}px` : undefined}
  style:width={width != null ? `${width}px` : undefined}
>
  {@render children()}
</div>

<style lang="postcss">
  @reference "../app.css";

  .metric-chart-plot {
    @apply relative w-full;
  }
</style>
