<script lang="ts">
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import { computeHeatmapLegendEntries } from '@/components/metrics/utils/heatmap-legend'
  import { computeHistogramBarColorScale } from '@/components/metrics/utils/heatmap-color-scale'
  import { themeSignal } from '@/state/theme.svelte'
  import { legendRowEnds } from '@/components/metrics/utils/legend-rows'

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
  let availableWidth = $state(0)
  let entryBounds = $state<DOMRectReadOnly[]>([])
  let gapWidth = $state(0)
  let legendElement = $state<HTMLDivElement>()
  $effect(() => {
    void availableWidth
    if (legendElement) gapWidth = Number.parseFloat(getComputedStyle(legendElement).columnGap)
  })
  let rowEnds = $derived(legendRowEnds(
    legendEntries.map((_, index) => entryBounds[index]?.width ?? 0),
    availableWidth,
    gapWidth
  ))
</script>

{#if legendEntries.length > 0}
  <div class="heatmap-legend">
    <div
      class="heatmap-legend__entries"
      bind:this={legendElement}
      bind:clientWidth={availableWidth}
      role="list"
      aria-label={ctx.activeHistogramTab === 'histogram'
        ? 'Histogram count scale'
        : 'Heatmap count scale'}
    >
      {#each rowEnds as end, row}
        {@const start = row === 0 ? 0 : rowEnds[row - 1]!}
        <div class="heatmap-legend__row" role="presentation">
          {#each legendEntries.slice(start, end) as entry, index (entry.label)}
            <div
              class="heatmap-legend__entry"
              role="listitem"
              bind:contentRect={entryBounds[start + index]}
            >
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
    @apply min-w-0 text-sm font-normal text-base-content;
  }

  .heatmap-legend__entries {
    @apply flex min-w-0 flex-col items-start gap-x-4 gap-y-2;
  }

  .heatmap-legend__row {
    @apply flex items-center;
    column-gap: inherit;
  }

  .heatmap-legend__entry {
    @apply inline-flex shrink-0 items-center gap-2 whitespace-nowrap tabular-nums;
  }

  .heatmap-legend__swatch {
    @apply inline-block size-2 shrink-0 rounded-full;
  }
</style>
