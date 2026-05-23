<script lang="ts">
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import ChartOverlayToggles from '@/components/metrics/Charts/ChartOverlayToggles.svelte'
  import HistogramQuantileAllSeriesToggle from '@/components/metrics/Charts/HistogramQuantileAllSeriesToggle.svelte'
  import { QUANTILE_LABELS } from '@/components/metrics/utils/histogram-aggregation'

  const ctx = getMetricViewContext()

  const quantileOverlays = QUANTILE_LABELS.map(({ key, label }) => ({
    id: key,
    fallbackLabel: label,
  }))

  function toggleQuantileOverlay(quantileKey: string) {
    const active = ctx.activeQuantileOverlays.has(quantileKey)
    if (
      active &&
      ctx.quantileDrillDownActive &&
      ctx.quantileDrillDownKey === quantileKey
    ) {
      ctx.clearQuantileDrillDown()
      return
    }
    ctx.setActiveQuantileOverlay(quantileKey, !active)
  }
</script>

<div class="metric-chart-control-bar" aria-label="Quantile chart controls">
  <ChartOverlayToggles
    overlays={quantileOverlays}
    activeOverlays={ctx.activeQuantileOverlays}
    onToggle={toggleQuantileOverlay}
    ariaLabel="Quantile overlays"
  />
  <HistogramQuantileAllSeriesToggle />
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .metric-chart-control-bar {
    @apply flex shrink-0 flex-wrap items-center gap-x-4 gap-y-1 bg-base-200 px-3 py-2;
  }
</style>
