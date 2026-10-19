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
    return computeHeatmapLegendEntries(points, themeSignal.value, ctx.metricType)
  })
  let legendRows = $derived(
    [
      legendEntries.slice(0, Math.ceil(legendEntries.length / 2)),
      legendEntries.slice(Math.ceil(legendEntries.length / 2)),
    ].filter(row => row.length > 0)
  )
</script>

{#if legendEntries.length > 0}
  <div class="heatmap-legend">
    <div
      class="heatmap-legend__entries"
      style:--legend-columns={Math.ceil(legendEntries.length / 2)}
      role="list"
      aria-label={ctx.activeHistogramTab === 'histogram'
        ? 'Histogram count scale'
        : 'Heatmap count scale'}
    >
      {#each legendRows as row, index (index)}
        <div class="heatmap-legend__row" role="presentation">
          {#each row as entry (entry.label)}
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
      {/each}
    </div>
  </div>
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .heatmap-legend {
    @apply flex min-w-0 shrink-0 flex-wrap items-center justify-end gap-x-4 gap-y-2 px-3 py-2 text-xs text-base-content;
    container-type: inline-size;
  }

  .heatmap-legend__entries {
    @apply grid min-w-0 gap-x-4 gap-y-2;
    grid-template-columns: repeat(var(--legend-columns), max-content);
  }

  .heatmap-legend__row {
    @apply grid min-w-0 items-center gap-x-4 gap-y-2;
    grid-column: 1 / -1;
    grid-template-columns: subgrid;
  }

  @container (max-width: 36rem) {
    .heatmap-legend__entries {
      grid-template-columns: repeat(min(var(--legend-columns), 3), max-content);
    }
  }

  @container (max-width: 20rem) {
    .heatmap-legend__entries {
      grid-template-columns: repeat(min(var(--legend-columns), 2), max-content);
    }
  }

  @container (max-width: 12rem) {
    .heatmap-legend__entries {
      grid-template-columns: max-content;
    }
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
