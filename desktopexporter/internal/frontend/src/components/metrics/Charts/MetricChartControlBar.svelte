<script lang="ts">
  /*
   * Bottom control strip for the gauge/sum chart pane: optional
   * all-series aggregate toggle + stat overlay toggle.
   */
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import AllSeriesAggregateToggle from '@/components/metrics/Charts/AllSeriesAggregateToggle.svelte'
  import ChartStatOverlaysToggle from '@/components/metrics/Charts/ChartStatOverlaysToggle.svelte'

  const ctx = getMetricViewContext()

  let visible = $derived(
    ctx.showAllSeriesAggregateToggleVisible ||
      ctx.showChartStatOverlaysToggleVisible
  )
</script>

{#if visible}
  <div class="metric-chart-control-bar" aria-label="Chart controls">
    <AllSeriesAggregateToggle />
    <ChartStatOverlaysToggle />
  </div>
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .metric-chart-control-bar {
    @apply flex shrink-0 flex-wrap items-center gap-x-4 gap-y-1
           border-t border-base-300/60 bg-base-200 px-3 py-2;
  }
</style>
