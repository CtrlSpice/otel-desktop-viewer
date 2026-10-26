<script lang="ts">
  import type { MetricViewContext } from '@/contexts/metric-view-context.svelte'
  import {
    combinedLineEnabled,
    withCombinedLine,
  } from '@/components/metrics/utils/aggregation'

  let { ctx }: { ctx: MetricViewContext } = $props()
  const combinedModes = ['sum', 'avg'] as const
</script>

{#if ctx.showChartStatOverlaysToggleVisible}
  {#if ctx.isHistogramKind}
    <label
      class="btn btn-xs chart-header-control chart-stat-overlays-toggle tooltip tooltip-top"
      data-tip="p50, p95 and p99 of the combined checked series"
    >
      <input
        type="checkbox"
        class="checkbox checkbox-xs"
        checked={ctx.showSelectionStatOverlays}
        aria-label="Show Percentiles"
        onchange={event =>
          ctx.setShowSelectionStatOverlays(event.currentTarget.checked)}
      />
      <span>Percentiles</span>
    </label>
  {:else}
    {#each combinedModes as mode (mode)}
      {#if ctx.availableAggregationViews.includes(mode)}
        <label
          class="btn btn-xs chart-header-control chart-stat-overlays-toggle tooltip tooltip-top"
          data-tip={`${mode === 'sum' ? 'Sum' : 'Sample-weighted average'} of the checked series in each time bucket`}
        >
          <input
            type="checkbox"
            class="checkbox checkbox-xs"
            checked={combinedLineEnabled(ctx.aggregationView, mode)}
            aria-label={mode === 'sum'
              ? 'Show combined sum'
              : 'Show combined average'}
            onchange={event =>
              ctx.setAggregationView(
                withCombinedLine(
                  ctx.aggregationView,
                  mode,
                  event.currentTarget.checked
                )
              )}
          />
          <span>{mode === 'sum' ? 'Sum' : 'Average'}</span>
        </label>
      {/if}
    {/each}
  {/if}
{/if}

<style lang="postcss">
  @reference "../../../app.css";
  .chart-stat-overlays-toggle {
    @apply relative shrink-0 gap-2 whitespace-nowrap;
  }
</style>
