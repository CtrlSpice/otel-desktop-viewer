<script lang="ts">
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import { computeHeatmapLegendEntries } from '@/components/metrics/utils/heatmap-legend'
  import { themeSignal } from '@/state/theme.svelte'

  const ctx = getMetricViewContext()

  let legendEntries = $derived.by(() => {
    const points = ctx.heatmapBucketSeries
    if (!points || points.length === 0) return []
    return computeHeatmapLegendEntries(points, themeSignal.value)
  })
</script>

{#if legendEntries.length > 0}
  <div class="heatmap-legend">
    <span class="heatmap-legend__label">Observations per cell</span>
    <ul class="heatmap-legend__entries" aria-label="Heatmap count scale">
      {#each legendEntries as entry (entry.label)}
        <li class="heatmap-legend__entry">
          <span
            class="heatmap-legend__swatch"
            style:background-color={entry.color}
            aria-hidden="true"
          ></span>
          <span>{entry.label}</span>
        </li>
      {/each}
    </ul>
  </div>
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .heatmap-legend {
    @apply flex min-w-0 shrink-0 flex-wrap items-center gap-x-4 gap-y-2 px-3 py-2 text-xs text-base-content;
  }

  .heatmap-legend__label {
    @apply shrink-0 font-medium;
  }

  .heatmap-legend__entries {
    @apply m-0 flex min-w-0 list-none flex-wrap items-center gap-x-4 gap-y-2 p-0;
  }

  .heatmap-legend__entry {
    @apply inline-flex items-center gap-2 whitespace-nowrap tabular-nums;
  }

  .heatmap-legend__swatch {
    @apply inline-block h-4 w-4 shrink-0 rounded-md;
    outline: 1px solid
      color-mix(in oklab, var(--color-base-content) 15%, transparent);
    outline-offset: -1px;
  }
</style>
