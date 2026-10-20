<script lang="ts">
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import { computeHeatmapLegendEntries } from '@/components/metrics/utils/heatmap-legend'
  import { computeHistogramBarColorScale } from '@/components/metrics/utils/heatmap-color-scale'
  import { themeSignal } from '@/state/theme.svelte'

  const ctx = getMetricViewContext()

  let legendEntries = $derived.by(() => {
    if (ctx.activeHistogramTab === 'histogram') {
      const datapoint = ctx.histogramChartDatapoint
      return datapoint
        ? computeHistogramBarColorScale(datapoint, themeSignal.value)
            .legendEntries
        : []
    }
    const points = ctx.heatmapBucketSeries
    if (!points || points.length === 0) return []
    return computeHeatmapLegendEntries(points, themeSignal.value)
  })
</script>

{#if legendEntries.length > 0}
  <div class="heatmap-legend">
    <div
      class="heatmap-legend__entries"
      role="list"
      aria-label={ctx.activeHistogramTab === 'histogram'
        ? 'Histogram count scale'
        : 'Heatmap count scale'}
    >
      {#each legendEntries as entry (entry.label)}
        <div class="heatmap-legend__entry" role="listitem">
          <span
            class="heatmap-legend__swatch"
            style:background-color={entry.color}
            aria-hidden="true"
          ></span>
          <span>{entry.label}</span>
        </div>
      {/each}
    </div>
  </div>
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .heatmap-legend {
    @apply min-w-0 text-sm font-normal text-base-content;
  }

  .heatmap-legend__entries {
    @apply flex min-w-0 flex-wrap items-center gap-x-4 gap-y-2;
  }

  .heatmap-legend__entry {
    @apply inline-flex items-center gap-2 whitespace-nowrap tabular-nums;
  }

  .heatmap-legend__swatch {
    @apply inline-block size-2 shrink-0 rounded-full;
  }
</style>
