<script lang="ts">
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import { QUANTILE_LABELS } from '@/components/metrics/utils/histogram-aggregation'

  const ctx = getMetricViewContext()

  const quantileRadioName = 'histogram-quantile-percentile'

  function selectQuantileOverlay(quantileKey: string) {
    ctx.setActiveQuantileOverlay(quantileKey)
  }
</script>

<div class="metric-chart-control-bar" aria-label="Quantile chart controls">
  <fieldset class="quantile-percentile-radios">
    <legend class="sr-only">Quantile percentile</legend>
    {#each QUANTILE_LABELS as { key, label } (key)}
      <label class="quantile-percentile-radios__option">
        <input
          type="radio"
          name={quantileRadioName}
          class="radio radio-sm radio-soft quantile-percentile-radios__input"
          value={key}
          checked={ctx.activeQuantileOverlays.has(key)}
          aria-label={label}
          onchange={() => selectQuantileOverlay(key)}
        />
        <span class="quantile-percentile-radios__label">{label}</span>
      </label>
    {/each}
  </fieldset>
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .metric-chart-control-bar {
    @apply flex shrink-0 flex-wrap items-center gap-x-4 gap-y-1 bg-base-200 px-3 py-2;
  }

  .quantile-percentile-radios {
    @apply m-0 flex min-w-0 flex-wrap items-center gap-x-4 gap-y-1 border-0 p-0;
  }

  .quantile-percentile-radios__option {
    @apply inline-flex shrink-0 cursor-pointer items-center gap-1.5
           rounded-full px-2 py-1 text-xs font-medium;
    color: var(--color-base-content);
  }

  .quantile-percentile-radios__input {
    @apply shrink-0;
  }

  .quantile-percentile-radios__label {
    @apply whitespace-nowrap;
    color: var(--color-muted);
  }
</style>
