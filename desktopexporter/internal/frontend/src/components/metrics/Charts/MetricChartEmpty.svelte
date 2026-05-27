<script lang="ts">
  import { DEFAULT_METRIC_CHART_HEIGHT } from '@/components/metrics/Charts/MetricChartPlot.svelte'

  type Props = {
    height?: number
    message: string
  }

  let { height = DEFAULT_METRIC_CHART_HEIGHT, message }: Props = $props()
</script>

<div
  class="metric-chart-empty"
  style:height="{height}px"
  role="status"
>
  {message}
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .metric-chart-empty {
    @apply flex items-center justify-center text-sm text-base-content/40;
  }
</style>
