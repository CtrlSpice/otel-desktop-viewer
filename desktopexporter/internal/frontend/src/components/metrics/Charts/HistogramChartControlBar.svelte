<script lang="ts">
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import type { HistogramScope } from '@/contexts/metric-view-context.svelte'

  const ctx = getMetricViewContext()

  const scopeRadioName = 'histogram-distribution-scope'

  const scopeOptions: { id: HistogramScope; label: string }[] = [
    { id: 'window', label: 'Whole window' },
    { id: 'bucket', label: 'Snapshot' },
  ]

  function selectScope(scope: HistogramScope) {
    ctx.setHistogramScope(scope)
  }
</script>

<div class="metric-chart-control-bar" aria-label="Histogram chart controls">
  <fieldset class="histogram-scope-radios">
    <legend class="sr-only">Histogram distribution scope</legend>
    {#each scopeOptions as { id, label } (id)}
      <label class="histogram-scope-radios__option">
        <input
          type="radio"
          name={scopeRadioName}
          class="radio radio-sm radio-soft histogram-scope-radios__input"
          value={id}
          checked={ctx.histogramScope === id}
          aria-label={label}
          onchange={() => selectScope(id)}
        />
        <span class="histogram-scope-radios__label">{label}</span>
      </label>
    {/each}
  </fieldset>
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .metric-chart-control-bar {
    @apply flex shrink-0 flex-wrap items-center gap-x-4 gap-y-1 bg-base-200 px-3 py-2;
  }

  .histogram-scope-radios {
    @apply m-0 flex min-w-0 flex-wrap items-center gap-x-4 gap-y-1 border-0 p-0;
  }

  .histogram-scope-radios__option {
    @apply inline-flex shrink-0 cursor-pointer items-center gap-1.5
           rounded-full px-2 py-1 text-xs font-medium;
    color: var(--color-base-content);
  }

  .histogram-scope-radios__input {
    @apply shrink-0;
  }

  .histogram-scope-radios__label {
    @apply whitespace-nowrap;
    color: var(--color-muted);
  }
</style>
