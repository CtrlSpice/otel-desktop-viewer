<script lang="ts">
  /*
   * Master toggle for selection stat annotations.
   * Lives in the metric header.
   */
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'

  const ctx = getMetricViewContext()
  const overlayLabel = $derived(
    ctx.isHistogramKind ? 'Percentiles' : 'Statistics'
  )
</script>

{#if ctx.showChartStatOverlaysToggleVisible}
  <label
    class="btn btn-xs chart-header-control chart-stat-overlays-toggle tooltip tooltip-top"
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
      aria-label="Show {overlayLabel}"
      onchange={e => ctx.setShowSelectionStatOverlays(e.currentTarget.checked)}
    />
    <span class="chart-stat-overlays-toggle__label">{overlayLabel}</span>
  </label>
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .chart-stat-overlays-toggle {
    @apply relative shrink-0 gap-2;
  }

  .chart-stat-overlays-toggle__label {
    @apply whitespace-nowrap;
  }
</style>
