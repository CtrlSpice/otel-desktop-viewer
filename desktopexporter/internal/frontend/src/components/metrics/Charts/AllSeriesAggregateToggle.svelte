<script lang="ts">
  /*
   * Optional toggle for the all-series aggregate line. Sits in the
   * chart meta row between the PaneHeader and the plot. The checked-series
   * aggregate is always on when applicable; this control only gates
   * the second line that spans every series on the metric.
   */
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import { aggregateAllToggleLabel } from '@/components/metrics/utils/aggregation'

  const ctx = getMetricViewContext()

  let label = $derived(aggregateAllToggleLabel(ctx.aggregationView))
</script>

{#if ctx.showAllSeriesAggregateToggleVisible}
  <label class="all-series-aggregate-toggle" title={label}>
    <input
      type="checkbox"
      class="checkbox checkbox-xs checkbox-soft all-series-aggregate-toggle__checkbox"
      checked={ctx.showAllSeriesAggregate}
      aria-label={label}
      onchange={(e) =>
        ctx.setShowAllSeriesAggregate(
          (e.currentTarget as HTMLInputElement).checked
        )}
    />
    <span class="all-series-aggregate-toggle__label">{label}</span>
  </label>
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .all-series-aggregate-toggle {
    @apply inline-flex shrink-0 cursor-pointer items-center gap-1.5
           rounded-full px-2 py-1 text-xs font-medium;
    color: var(--color-base-content);
  }

  .all-series-aggregate-toggle__checkbox {
    @apply shrink-0;
  }

  .all-series-aggregate-toggle__label {
    @apply whitespace-nowrap tabular-nums;
    color: var(--color-muted);
  }
</style>
