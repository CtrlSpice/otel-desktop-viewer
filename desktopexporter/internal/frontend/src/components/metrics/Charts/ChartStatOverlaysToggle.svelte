<script lang="ts">
  /*
   * Master toggle for selection stat annotations.
   * Lives in the metric header.
   */
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'

  const ctx = getMetricViewContext()
  let { joined = false }: { joined?: boolean } = $props()
</script>

{#if ctx.showChartStatOverlaysToggleVisible}
  <label
    class="btn btn-xs chart-header-control chart-stat-overlays-toggle chart-control-toggle tooltip tooltip-top"
    class:join-item={joined}
    data-tip={ctx.isHistogramKind
      ? 'p50, p95 and p99 of the combined checked series'
      : ctx.rateSlopeOverlayAvailable
        ? 'Min/max and selected rate slope'
        : 'Min/max and average'}
  >
    <input
      type="checkbox"
      class="checkbox checkbox-xs chart-stat-overlays-toggle__checkbox"
      checked={ctx.showSelectionStatOverlays}
      aria-label="Show chart stat overlays"
      onchange={e => ctx.setShowSelectionStatOverlays(e.currentTarget.checked)}
    />
    <span class="chart-stat-overlays-toggle__label">Overlays</span>
  </label>
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .btn.chart-stat-overlays-toggle {
    @apply relative shrink-0 gap-2;
    box-shadow: none;
  }

  .chart-stat-overlays-toggle__checkbox {
    @apply rounded-full;
    border-color: var(--color-muted);
    background: var(--color-base-200);
    color: var(--color-primary);
    box-shadow: none;
  }

  .chart-stat-overlays-toggle__checkbox::before {
    transition: none;
  }

  .chart-stat-overlays-toggle__label {
    @apply whitespace-nowrap;
  }
</style>
