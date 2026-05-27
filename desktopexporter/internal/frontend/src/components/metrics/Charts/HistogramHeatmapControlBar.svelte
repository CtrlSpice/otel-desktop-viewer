<script lang="ts">
  import { Legend } from 'layerchart'
  import { scaleOrdinal } from 'd3-scale'
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import { computeHeatmapLegendEntries } from '@/components/metrics/utils/heatmap-legend'
  import { themeSignal } from '@/state/theme.svelte'

  const ctx = getMetricViewContext()

  let legendEntries = $derived.by(() => {
    const points = ctx.heatmapBucketSeries
    if (!points || points.length === 0) return []
    return computeHeatmapLegendEntries(points, themeSignal.value)
  })

  let legendScale = $derived.by(() => {
    if (legendEntries.length === 0) return null
    return scaleOrdinal<string, string>()
      .domain(legendEntries.map(entry => entry.label))
      .range(legendEntries.map(entry => entry.color))
  })
</script>

<div class="metric-chart-control-bar heatmap-control-bar" aria-label="Heatmap chart controls">
  {#if legendScale}
    <Legend
      scale={legendScale}
      orientation="horizontal"
      variant="swatches"
      classes={{
        root: 'heatmap-control-bar__legend',
        label: 'text-xs text-rp-subtle',
      }}
    />
  {/if}
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .metric-chart-control-bar {
    @apply flex shrink-0 flex-wrap items-center gap-x-4 gap-y-1 bg-base-200 px-3 py-2;
  }

  .heatmap-control-bar :global(.heatmap-control-bar__legend) {
    @apply block min-w-0 w-full;
  }

  .heatmap-control-bar :global(.lc-legend-swatch-group) {
    @apply flex min-w-0 flex-wrap;
    gap: 0.25rem 0.75rem;
  }

  .heatmap-control-bar :global(.lc-legend-swatch-button) {
    cursor: default;
  }
</style>
