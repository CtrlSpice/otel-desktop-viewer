<script lang="ts">
  /*
   * TimeseriesPanel: the bottom half of the metrics page main split.
   * One collapsible row per timeseries belonging to the currently-selected
   * metric. Each row carries a colored checkbox (color matches the chart
   * line — the checkbox IS the color indicator), the attribute set, and
   * a datapoint count badge. Expanding a row with attributes shows them
   * as a key/value list via <details>/<summary>.
   *
   * Visibility-set rules (delegated to the context's SvelteSet writes):
   *   - Min 1: when only one row is checked, that row's checkbox is
   *     locked so the chart can't go fully blank.
   *   - Max MAX_VISIBLE_TIMESERIES (10): when the cap is reached every
   *     unchecked row's checkbox is disabled so the palette never wraps.
   */
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import type { Timeseries as PanelTimeseries } from '@/components/MetricCharts/TimeseriesLegend.svelte'
  import {
    MAX_VISIBLE_TIMESERIES,
    timeseriesColor,
    timeseriesForegroundColor,
  } from '@/utils/timeseries-palette'
  import { ArrowDownIcon } from '@/icons'
  import type { SvelteSet } from 'svelte/reactivity'

  const ctx = getMetricViewContext()

  let rows = $derived<PanelTimeseries[]>(
    ctx.isHistogramKind
      ? ctx.histogramLegendTimeseries
      : ctx.gaugeSumLegendTimeseries
  )
  let visibleKeys = $derived<SvelteSet<string>>(
    ctx.isHistogramKind ? ctx.histogramVisible : ctx.gaugeSumVisible
  )

  let capReached = $derived(visibleKeys.size >= MAX_VISIBLE_TIMESERIES)
  let isLastChecked = $derived(visibleKeys.size === 1)

  function toggle(key: string, checked: boolean) {
    if (checked) {
      visibleKeys.add(key)
    } else {
      visibleKeys.delete(key)
    }
  }

  function attrsTooltip(attrs: PanelTimeseries['attributes']): string {
    if (attrs.length === 0) return 'default timeseries'
    return attrs.map((a) => `${a.key}=${a.value}`).join(', ')
  }
</script>

<div class="ts-panel" role="region" aria-label="Timeseries">
  <div class="ts-panel__header">
    <span class="ts-panel__title">Timeseries</span>
    <span
      class="ts-panel__count"
      class:ts-panel__count--cap={capReached}
    >
      {visibleKeys.size} / {Math.min(rows.length, MAX_VISIBLE_TIMESERIES)} visible
    </span>
  </div>

  <div class="ts-panel__list">
    {#each rows as ts, i (ts.key)}
      {@const checked = visibleKeys.has(ts.key)}
      {@const disabledByCap = !checked && capReached}
      {@const disabledByFloor = checked && isLastChecked}
      {@const disabled = disabledByCap || disabledByFloor}
      {@const colorIdx = ctx.timeseriesColorIndex.get(ts.key) ?? i}
      {@const color = timeseriesColor(colorIdx)}
      {@const fg = timeseriesForegroundColor(colorIdx)}
      {@const tooltip = disabledByFloor
        ? 'At least one timeseries must remain selected'
        : attrsTooltip(ts.attributes)}
      {@const hasAttrs = ts.attributes.length > 0}

      <details class="ts-row" class:ts-row--disabled={disabled}>
        <summary class="ts-row__summary">
          <label class="ts-row__check-label" title={tooltip}>
            <input
              type="checkbox"
              class="checkbox checkbox-xs ts-row__checkbox"
              style:--input-color={color}
              style:color={fg}
              {checked}
              {disabled}
              onclick={(e) => e.stopPropagation()}
              onchange={(e) =>
                toggle(ts.key, (e.currentTarget as HTMLInputElement).checked)}
            />
          </label>

          <span class="ts-row__attrs">
            {#if !hasAttrs}
              <span class="ts-row__attrs-empty">default timeseries</span>
            {:else}
              {#each ts.attributes as attr (attr.key)}
                <span class="ts-row__attr">
                  <span class="ts-row__attr-key">{attr.key}</span>
                  <span class="ts-row__attr-eq">=</span>
                  <span class="ts-row__attr-value">{attr.value}</span>
                </span>
              {/each}
            {/if}
          </span>

          {#if hasAttrs}
            <ArrowDownIcon class="ts-row__caret" aria-hidden="true" />
          {/if}
        </summary>

        {#if hasAttrs}
          <div class="ts-row__expansion">
            {#if ts.badge}
              <div class="ts-row__field">
                <span class="ts-row__field-key">datapoints:</span>
                <span class="ts-row__field-value">{ts.badge}</span>
              </div>
            {/if}
            {#each ts.attributes as attr (attr.key)}
              <div class="ts-row__field">
                <span class="ts-row__field-key">{attr.key}:</span>
                <span class="ts-row__field-value">{attr.value}</span>
              </div>
            {/each}
          </div>
        {/if}
      </details>
    {/each}
  </div>

  {#if capReached}
    <p class="ts-panel__cap-note">
      Cap of {MAX_VISIBLE_TIMESERIES} timeseries reached. Uncheck one to enable
      another.
    </p>
  {/if}
</div>

<style lang="postcss">
  @reference "../../app.css";

  .ts-panel {
    @apply flex h-full min-h-0 min-w-0 flex-col gap-2 overflow-hidden;
  }

  .ts-panel__header {
    @apply flex shrink-0 items-baseline justify-between px-3 pt-2;
  }

  .ts-panel__title {
    @apply text-sm font-medium;
    color: var(--color-subtle);
  }

  .ts-panel__count {
    @apply text-sm tabular-nums;
    color: var(--color-subtle);
  }

  .ts-panel__count--cap {
    @apply font-semibold text-warning;
  }

  .ts-panel__list {
    @apply m-0 flex min-h-0 flex-1 flex-col gap-0 overflow-y-auto p-0;
    list-style: none;
    scrollbar-width: thin;
  }

  .ts-row {
    @apply border-b border-base-300/30;
  }

  .ts-row:last-child {
    @apply border-b-0;
  }

  .ts-row--disabled {
    @apply opacity-60;
  }

  .ts-row__summary {
    @apply flex cursor-pointer select-none list-none items-center gap-2 px-3 py-1;
    @apply hover:bg-base-300/30;
  }

  .ts-row__summary::marker,
  .ts-row__summary::-webkit-details-marker {
    display: none;
  }

  .ts-row--disabled .ts-row__summary {
    @apply cursor-not-allowed;
  }

  .ts-row__check-label {
    @apply flex shrink-0 cursor-pointer items-center mr-1;
  }

  .ts-row--disabled .ts-row__check-label {
    @apply cursor-not-allowed;
  }

  .ts-row__checkbox {
    @apply shrink-0;
  }

  .ts-row__attrs {
    @apply flex min-w-0 flex-1 flex-wrap items-baseline gap-x-2 gap-y-0.5 overflow-hidden font-mono text-sm;
  }

  .ts-row__attrs-empty {
    @apply italic;
    color: var(--color-muted);
  }

  .ts-row__attr {
    @apply inline-flex min-w-0 max-w-full items-baseline;
  }

  .ts-row__attr-key {
    @apply shrink-0;
    color: var(--color-subtle);
  }

  .ts-row__attr-eq {
    @apply mx-0.5 shrink-0;
    color: var(--color-muted);
  }

  .ts-row__attr-value {
    @apply truncate text-base-content;
  }

  .ts-row__badge {
    @apply shrink-0 text-sm tabular-nums;
    color: var(--color-muted);
  }

  .ts-row__summary :global(.ts-row__caret) {
    @apply ml-auto h-3.5 w-3.5 transition-transform duration-150;
    color: var(--color-muted);
    transform: rotate(-90deg);
  }

  details[open] > .ts-row__summary :global(.ts-row__caret) {
    transform: rotate(0deg);
  }

  .ts-row__expansion {
    @apply flex flex-col pl-10 pr-2;
  }

  .ts-row__field {
    @apply flex items-baseline gap-2 py-1 text-sm;
  }

  .ts-row__field-key {
    @apply shrink-0 font-mono;
    color: var(--color-subtle);
  }

  .ts-row__field-value {
    @apply font-mono text-base-content;
  }

  .ts-panel__cap-note {
    @apply m-0 px-3 pb-2 text-sm text-warning/80;
  }
</style>
