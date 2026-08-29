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

<div class="metric-chart-control-bar heatmap-control-bar">
  {#if legendEntries.length > 0}
    <ul class="heatmap-control-bar__legend" aria-label="Heatmap count scale">
      {#each legendEntries as entry (entry.label)}
        <li class="heatmap-control-bar__legend-entry">
          <span
            class="heatmap-control-bar__swatch"
            style:background-color={entry.color}
            aria-hidden="true"
          ></span>
          <span class="text-rp-subtle">{entry.label}</span>
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .metric-chart-control-bar {
    @apply flex shrink-0 flex-wrap items-center gap-x-4 gap-y-1 bg-base-200 px-3 py-2;
  }

  .heatmap-control-bar__legend {
    @apply m-0 flex min-w-0 w-full list-none flex-wrap p-0 text-xs;
    gap: 0.25rem 0.75rem;
  }

  .heatmap-control-bar__legend-entry {
    @apply inline-flex items-center gap-1.5 whitespace-nowrap;
  }

  .heatmap-control-bar__swatch {
    @apply inline-block h-3 w-3 shrink-0 rounded-sm;
  }
</style>
