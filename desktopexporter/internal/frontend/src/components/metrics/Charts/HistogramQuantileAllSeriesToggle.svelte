<script lang="ts">
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'

  const ctx = getMetricViewContext()
</script>

{#if ctx.showAllSeriesQuantileAggregateToggleVisible}
  <label
    class="all-series-aggregate-toggle"
    title="Show p50/p95/p99 merged across every series on this metric"
  >
    <input
      type="checkbox"
      class="checkbox checkbox-xs checkbox-soft all-series-aggregate-toggle__checkbox"
      checked={ctx.showAllSeriesQuantileAggregate}
      aria-label="Show all series percentiles"
      onchange={(e) =>
        ctx.setShowAllSeriesQuantileAggregate(
          (e.currentTarget as HTMLInputElement).checked
        )}
    />
    <span class="all-series-aggregate-toggle__label">All series percentiles</span>
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
