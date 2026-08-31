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
  <div
    class="metric-chart-control-bar"
    role="group"
    aria-label="Chart controls"
  >
    <AllSeriesAggregateToggle />
    <ChartStatOverlaysToggle />
  </div>
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .metric-chart-control-bar {
    @apply flex shrink-0 flex-wrap items-center gap-x-4 gap-y-1 bg-base-200 px-3 py-2;
  }

  .metric-chart-control-bar :global(.chart-control-toggle) {
    transition:
      background-color 0.12s ease,
      outline-color 0.12s ease;
  }

  .metric-chart-control-bar
    :global(.chart-control-toggle:has(input:focus-visible)) {
    background-color: color-mix(
      in oklab,
      var(--color-primary) 10%,
      var(--color-base-200)
    );
    outline: var(--focus-ring-width) solid var(--focus-ring-color);
    outline-offset: var(--focus-ring-offset);
  }

  @media (forced-colors: active) {
    .metric-chart-control-bar
      :global(.chart-control-toggle:has(input:focus-visible)) {
      outline: 2px solid Highlight;
    }
  }
</style>
