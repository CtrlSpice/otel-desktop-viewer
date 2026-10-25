<script lang="ts">
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import { computeHeatmapLegendEntries } from '@/components/metrics/utils/heatmap-legend'
  import { computeHistogramBarColorScale } from '@/components/metrics/utils/heatmap-color-scale'
  import { themeSignal } from '@/state/theme.svelte'
  import LegendRows from '@/components/shared/LegendRows.svelte'

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
    <LegendRows
      items={legendEntries}
      alignedColumns={6}
      itemKey={entry => entry.label}
      label={ctx.activeHistogramTab === 'histogram'
        ? 'Histogram count scale'
        : 'Heatmap count scale'}
    >
      {#snippet children(entry)}
        <span
          class="heatmap-legend__swatch"
          style:background-color={entry.color}
          aria-hidden="true"
        ></span>
        <span>{entry.label}</span>
      {/snippet}
    </LegendRows>
  </div>
{/if}

<style lang="postcss">
  @reference "../../../app.css";

  .heatmap-legend {
    @apply min-w-0 shrink-0 px-1 py-2 text-sm font-normal text-base-content;
    --legend-row-gap: 0.5rem;
    --legend-entry-height: 1.5em;
  }

  .heatmap-legend__swatch {
    @apply inline-block size-2 shrink-0 rounded-full;
  }
</style>
