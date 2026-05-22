<script lang="ts">
  /*
   * Master toggle for selection stat overlays (min / max / avg lines
   * and labels on the selected series). Lives in the chart control bar.
   */
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'

  const ctx = getMetricViewContext()
</script>

{#if ctx.showChartStatOverlaysToggleVisible}
  <label class="chart-stat-overlays-toggle" title="Show min, max, and avg overlays">
    <input
      type="checkbox"
      class="checkbox checkbox-xs checkbox-soft chart-stat-overlays-toggle__checkbox"
      checked={ctx.showSelectionStatOverlays}
      aria-label="Show chart stat overlays"
      onchange={(e) =>
        ctx.setShowSelectionStatOverlays(
          (e.currentTarget as HTMLInputElement).checked
        )}
    />
    <span class="chart-stat-overlays-toggle__label">Overlays</span>
  </label>
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .chart-stat-overlays-toggle {
    @apply inline-flex shrink-0 cursor-pointer items-center gap-1.5
           rounded-full px-2 py-1 text-xs font-medium;
    color: var(--color-base-content);
  }

  .chart-stat-overlays-toggle__checkbox {
    @apply shrink-0;
  }

  .chart-stat-overlays-toggle__label {
    @apply whitespace-nowrap;
    color: var(--color-muted);
  }
</style>
