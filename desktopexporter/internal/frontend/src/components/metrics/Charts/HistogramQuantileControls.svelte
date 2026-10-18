<script lang="ts">
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import { QUANTILE_LABELS } from '@/components/metrics/utils/histogram-aggregation'
  import {
    HISTOGRAM_QUANTILE_DASH,
    histogramQuantileColor,
  } from '@/components/metrics/utils/heatmap-quantiles'

  const ctx = getMetricViewContext()
</script>

<fieldset class="histogram-quantile-controls">
  <legend class="sr-only">Histogram quantile overlays</legend>
  {#each QUANTILE_LABELS as { key, label } (key)}
    <label
      class="histogram-quantile-controls__option tooltip tooltip-bottom"
      data-tip="{label}: bucket-derived estimate of the combined checked series"
    >
      <input
        type="checkbox"
        class="checkbox checkbox-xs checkbox-soft"
        checked={ctx.activeQuantileOverlays.has(key)}
        aria-label="Show {label}"
        onchange={event =>
          ctx.setQuantileOverlayVisible(key, event.currentTarget.checked)}
      />
      <svg width="20" height="10" viewBox="0 0 20 10" aria-hidden="true"
        ><line
          x1="0"
          x2="20"
          y1="5"
          y2="5"
          stroke={histogramQuantileColor(key)}
          stroke-width="2"
          stroke-dasharray={HISTOGRAM_QUANTILE_DASH}
        /></svg
      >
      <span>{label}</span>
    </label>
  {/each}
</fieldset>

<style lang="postcss">
  @reference "../../../app.css";
  .histogram-quantile-controls {
    @apply m-0 flex min-w-0 max-w-full flex-wrap items-center gap-1 border-0 p-0;
  }
  .histogram-quantile-controls__option {
    @apply inline-flex cursor-pointer items-center gap-1.5 rounded-full px-2 py-1 text-xs text-base-content;
  }
</style>
