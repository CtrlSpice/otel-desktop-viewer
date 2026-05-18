<script lang="ts">
  /*
   * TimeseriesPanel: the bottom half of the metrics page main split.
   * One collapsible row per timeseries belonging to the currently-selected
   * metric. Each row carries a colored checkbox (color matches the chart
   * line — the checkbox IS the color indicator), the attribute set, and
   * a datapoint count badge. Expanding a row with attributes shows them
   * as a key/value list via <FieldGroup> (headerAction keeps the
   * checkbox outside the disclosure control).
   *
   * Visibility-set rules (delegated to the context's SvelteSet writes):
   *   - Max MAX_VISIBLE_TIMESERIES (10): when the cap is reached every
   *     unchecked row's checkbox is disabled so the palette never wraps.
   *   - All unchecked is allowed; the chart shows an empty-state message
   *     when nothing is selected.
   */
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import type { Timeseries as PanelTimeseries } from '@/components/MetricCharts/legend-types'
  import {
    MAX_VISIBLE_TIMESERIES,
    timeseriesColor,
    timeseriesForegroundColor,
  } from '@/utils/timeseries-palette'
  import FieldGroup from '@/components/FieldGroup.svelte'
  import { getContext } from 'svelte'
  import type { SvelteSet } from 'svelte/reactivity'
  import {
    PANEL_SPLIT_RESIZE_KEY,
    type PanelSplitResizeContext,
  } from '@/contexts/panel-split-resize-context.svelte'

  const ctx = getMetricViewContext()
  const panelSplitResize = getContext<PanelSplitResizeContext | undefined>(
    PANEL_SPLIT_RESIZE_KEY
  )

  let resizeHandleEl = $state<HTMLElement | null>(null)

  $effect(() => {
    panelSplitResize?.registerHandle(resizeHandleEl)
    return () => panelSplitResize?.registerHandle(null)
  })

  let rows = $derived<PanelTimeseries[]>(
    ctx.isHistogramKind
      ? ctx.histogramLegendTimeseries
      : ctx.gaugeSumLegendTimeseries
  )
  let visibleKeys = $derived<SvelteSet<string>>(
    ctx.isHistogramKind ? ctx.histogramVisible : ctx.gaugeSumVisible
  )

  let capReached = $derived(visibleKeys.size >= MAX_VISIBLE_TIMESERIES)
  let visibleCap = $derived(Math.min(rows.length, MAX_VISIBLE_TIMESERIES))

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
    {#if panelSplitResize}
      <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
      <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
      <div
        class="ts-panel__resize-handle"
        bind:this={resizeHandleEl}
        role="separator"
        aria-orientation="horizontal"
        aria-label="Resize chart and timeseries panels"
        aria-valuenow={panelSplitResize.ariaNow}
        aria-valuemin={panelSplitResize.ariaMin}
        aria-valuemax={panelSplitResize.ariaMax}
        tabindex="0"
        onpointerdown={panelSplitResize.onPointerDown}
        onpointermove={panelSplitResize.onPointerMove}
        onpointerup={panelSplitResize.onPointerUp}
        ondblclick={panelSplitResize.onDoubleClick}
        onkeydown={panelSplitResize.onKeydown}
      ></div>
    {/if}
    <span class="ts-panel__title">Timeseries</span>
    <span
      class="ts-panel__count"
      class:ts-panel__count--cap={capReached}
    >
      {visibleKeys.size} / {visibleCap} visible
    </span>
  </div>

  <div class="ts-panel__list">
    {#each rows as ts, i (ts.key)}
      {@const checked = visibleKeys.has(ts.key)}
      {@const disabled = !checked && capReached}
      {@const colorIdx = ctx.timeseriesColorIndex.get(ts.key) ?? i}
      {@const color = timeseriesColor(colorIdx)}
      {@const fg = timeseriesForegroundColor(colorIdx)}
      {@const tooltip = attrsTooltip(ts.attributes)}
      {@const hasAttrs = ts.attributes.length > 0}

      {#if hasAttrs}
        <div class="ts-row-wrap" class:ts-row-wrap--disabled={disabled}>
          <FieldGroup
            label={attrsTooltip(ts.attributes)}
            open={false}
            last={i === rows.length - 1 && !capReached}
          >
            {#snippet headerAction()}
              <label class="ts-row__check-label" title={tooltip}>
                <input
                  type="checkbox"
                  class="checkbox checkbox-xs ts-row__checkbox"
                  style:--input-color={color}
                  style:color={fg}
                  {checked}
                  {disabled}
                  onchange={(e) =>
                    toggle(
                      ts.key,
                      (e.currentTarget as HTMLInputElement).checked
                    )}
                />
              </label>
              <span class="ts-row__attrs">
                {#each ts.attributes as attr (attr.key)}
                  <span class="ts-row__attr">
                    <span class="ts-row__attr-key">{attr.key}</span>
                    <span class="ts-row__attr-eq">=</span>
                    <span class="ts-row__attr-value">{attr.value}</span>
                  </span>
                {/each}
              </span>
            {/snippet}
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
          </FieldGroup>
        </div>
      {:else}
        <div
          class="ts-row"
          class:ts-row--disabled={disabled}
          class:ts-row--last={i === rows.length - 1}
        >
          <label class="ts-row__check-label" title={tooltip}>
            <input
              type="checkbox"
              class="checkbox checkbox-xs ts-row__checkbox"
              style:--input-color={color}
              style:color={fg}
              {checked}
              {disabled}
              onchange={(e) =>
                toggle(ts.key, (e.currentTarget as HTMLInputElement).checked)}
            />
          </label>
          <div class="ts-row__attrs ts-row__attrs--static">
            <span class="ts-row__attrs-empty">default timeseries</span>
          </div>
        </div>
      {/if}
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
    @apply flex h-full min-h-0 min-w-0 flex-col overflow-hidden;
  }

  /* Match metrics-page__header / PaneHeader title bar (flush, no top radius). */
  .ts-panel__header {
    @apply relative flex shrink-0 items-center justify-between gap-3 rounded-t-none px-3 py-2 bg-base-300;
  }

  /* Thin hit strip centered on the header top edge (same height as
     --resize-bar-hit-width). Sits half above the bar so the seam
     reads as the chart/timeseries boundary. */
  .ts-panel__resize-handle {
    @apply absolute left-0 right-0 z-10 cursor-row-resize touch-none;
    top: calc(var(--resize-bar-hit-width) / -2);
    height: var(--resize-bar-hit-width);
  }

  .ts-panel__title {
    @apply truncate text-sm font-semibold tracking-tight;
    color: var(--color-base-content);
  }

  .ts-panel__count {
    @apply shrink-0 text-sm tabular-nums;
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

  .ts-row-wrap--disabled {
    @apply opacity-60;
  }

  .ts-row-wrap--disabled :global(.field-group__caret-btn) {
    @apply pointer-events-none;
  }

  .ts-panel :global(.field-group__header-row) {
    @apply min-w-0 flex-1 items-center gap-2 py-1 pl-3 pr-1;
  }

  .ts-panel :global(.field-group__content) {
    @apply pb-2 pl-10 pr-3 pt-0;
  }

  /* Default-timeseries rows (no attributes to expand). */
  .ts-row {
    @apply flex items-stretch border-b border-base-300/30 pl-3;
  }

  .ts-row--last {
    @apply border-b-0;
  }

  .ts-row--disabled {
    @apply opacity-60;
  }

  .ts-row__check-label {
    @apply flex shrink-0 cursor-pointer items-center py-1 pr-2;
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

  .ts-row__attrs--static {
    @apply select-none py-1 pr-3;
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
