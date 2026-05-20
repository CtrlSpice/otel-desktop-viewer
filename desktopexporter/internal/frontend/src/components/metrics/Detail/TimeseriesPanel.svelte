<script lang="ts">
  /*
   * TimeseriesPanel: the bottom half of the metrics page main split.
   * One FieldGroup per timeseries. The header row carries the visibility
   * checkbox, inline attribute pairs (only labels that differ across
   * all series in the list when 2+ exist), datapoint-count badge;
   * expand for full attribute fields. Series without attributes use a
   * plain header row (no FieldGroup / caret).
   */
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import type { LegendTimeseries as PanelTimeseries } from '@/types/metric-chart-types'
  import type { MetricTimeseries } from '@/types/api-types'
  import { MAX_VISIBLE_TIMESERIES } from '@/components/metrics/utils/metric-timeseries-visible'
  import { chartNeutral, readableTextColor } from '@/utils/chart-palette'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import MetricField from '@/components/metrics/Detail/MetricField.svelte'
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

  let timeseriesByKey = $derived.by((): Map<string, MetricTimeseries> => {
    const m = ctx.metric
    if (!m) return new Map()
    return new Map(m.timeseries.map((ts) => [ts.attributesKey, ts]))
  })

  let capReached = $derived(
    !ctx.isHistogramKind && visibleKeys.size >= MAX_VISIBLE_TIMESERIES
  )
  let visibleCount = $derived(
    rows.filter((r) => visibleKeys.has(r.key)).length
  )

  /** Attribute keys that differ across rows (stable regardless of checkbox). */
  let differingAttrKeys = $derived.by((): Set<string> | null => {
    if (rows.length < 2) return null

    const allKeys = new Set<string>()
    for (const row of rows) {
      for (const a of row.attributes) allKeys.add(a.key)
    }

    const differing = new Set<string>()
    for (const key of allKeys) {
      const signatures = new Set<string>()
      for (const row of rows) {
        const a = row.attributes.find((x) => x.key === key)
        signatures.add(a?.value ?? '')
      }
      if (signatures.size > 1) differing.add(key)
    }
    if (differing.size === 0) return null
    return differing
  })

  function headerAttrs(
    attrs: PanelTimeseries['attributes']
  ): PanelTimeseries['attributes'] {
    if (!differingAttrKeys) return attrs
    return attrs.filter((a) => differingAttrKeys.has(a.key))
  }

  function toggle(key: string, checked: boolean) {
    ctx.toggleTimeseriesVisible(key, checked)
  }

  function attrsTooltip(attrs: PanelTimeseries['attributes']): string {
    if (attrs.length === 0) return 'default series'
    return attrs.map((a) => `${a.key}: ${a.value}`).join(' ')
  }

  function setTimeseriesOpen(key: string, open: boolean) {
    if (open) ctx.expandedTimeseries.add(key)
    else ctx.expandedTimeseries.delete(key)
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
    <div class="ts-panel__header-end">
      <span
        class="ts-panel__count"
        class:ts-panel__count--cap={capReached}
      >
        {visibleCount} / {rows.length} visible
      </span>
      <button
        type="button"
        class="btn btn-ghost btn-xs ts-panel__uncheck-all"
        disabled={visibleCount === 0}
        onclick={() => ctx.clearAllTimeseriesVisible()}
      >
        Uncheck all
      </button>
    </div>
  </div>

  <div class="ts-panel__list">
    {#each rows as ts, i (ts.key)}
      {@const checked = visibleKeys.has(ts.key)}
      {@const checkboxDisabled = !checked && capReached}
      {@const seriesColor = ctx.timeseriesColorByKey.get(ts.key)}
      {@const color = checked && seriesColor ? seriesColor : chartNeutral()}
      {@const fg =
        checked && seriesColor ? readableTextColor(seriesColor) : chartNeutral()}
      {@const hasAttrs = ts.attributes.length > 0}
      {@const rowHeaderAttrs = headerAttrs(ts.attributes)}
      {@const tooltip = attrsTooltip(ts.attributes)}
      {@const headerLabel =
        rowHeaderAttrs.length > 0 ? attrsTooltip(rowHeaderAttrs) : 'timeseries'}
      {@const metricTs = timeseriesByKey.get(ts.key)}
      {@const expanded = ctx.expandedTimeseries.has(ts.key)}
      {@const isLast = i === rows.length - 1 && !capReached}

      {#if hasAttrs}
        <div class="ts-row-wrap">
        <FieldGroup
          label={headerLabel}
          open={expanded}
          onOpenChange={(open) => setTimeseriesOpen(ts.key, open)}
          last={isLast}
        >
          {#snippet headerAction()}
            <label
              class="ts-row__check-label"
              class:ts-row__check-label--disabled={checkboxDisabled}
              title={tooltip}
            >
              <input
                type="checkbox"
                class="checkbox checkbox-xs checkbox-soft ts-row__checkbox"
                style:--input-color={color}
                style:color={fg}
                {checked}
                disabled={checkboxDisabled}
                onchange={(e) =>
                  toggle(
                    ts.key,
                    (e.currentTarget as HTMLInputElement).checked
                  )}
              />
            </label>
            <div class="ts-row__attrs">
              {#each rowHeaderAttrs as attr (attr.key)}
                <span class="ts-row__attr">
                  <span class="detail-cell__key">{attr.key}:</span>
                  <span class="detail-cell__value">{attr.value}</span>
                </span>
              {/each}
            </div>
            {#if ts.badge}
              <span class="ts-row__badge badge-count">{ts.badge}</span>
            {/if}
          {/snippet}

          {#if metricTs}
            <table class="detail-fields w-full" aria-label="Timeseries fields">
              <tbody>
                {#each metricTs.attributes as attr (attr.key)}
                  <MetricField
                    fieldName={attr.key}
                    fieldValue={attr.value}
                    fieldType={attr.type}
                  />
                {/each}
              </tbody>
            </table>
          {:else}
            <p class="ts-fields-empty">Timeseries not found</p>
          {/if}
        </FieldGroup>
        </div>
      {:else}
        <div
          class="ts-row ts-row--plain"
          class:ts-row--last={isLast}
        >
          <label
            class="ts-row__check-label"
            class:ts-row__check-label--disabled={checkboxDisabled}
            title={tooltip}
          >
            <input
              type="checkbox"
              class="checkbox checkbox-xs checkbox-soft ts-row__checkbox"
              style:--input-color={color}
              style:color={fg}
              {checked}
              disabled={checkboxDisabled}
              onchange={(e) =>
                toggle(ts.key, (e.currentTarget as HTMLInputElement).checked)}
            />
          </label>
          <span class="ts-row__default-label">default series</span>
          {#if ts.badge}
            <span class="ts-row__badge badge-count">{ts.badge}</span>
          {/if}
        </div>
        {#if !isLast}
          <div class="separator" aria-hidden="true"></div>
        {/if}
      {/if}
    {/each}
  </div>

  {#if capReached && !ctx.isHistogramKind}
    <p class="ts-panel__cap-note">
      Cap of {MAX_VISIBLE_TIMESERIES} timeseries reached. Uncheck one to enable
      another.
    </p>
  {/if}
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .ts-panel {
    @apply flex h-full min-h-0 min-w-0 flex-col overflow-hidden;
  }

  .ts-panel__header {
    @apply relative flex shrink-0 items-center justify-between gap-3 rounded-t-none px-3 py-2 bg-base-300;
  }

  .ts-panel__resize-handle {
    @apply absolute left-0 right-0 z-10 cursor-row-resize touch-none;
    top: calc(var(--resize-bar-hit-width) / -2);
    height: var(--resize-bar-hit-width);
  }

  .ts-panel__title {
    @apply truncate text-sm font-semibold tracking-tight;
    color: var(--color-base-content);
  }

  .ts-panel__header-end {
    @apply flex shrink-0 items-center gap-2;
  }

  .ts-panel__count {
    @apply shrink-0 text-sm tabular-nums;
    color: var(--color-subtle);
  }

  .ts-panel__uncheck-all {
    @apply shrink-0;
  }

  .ts-panel__count--cap {
    @apply font-semibold text-warning;
  }

  .ts-panel__list {
    @apply m-0 flex min-h-0 flex-1 flex-col gap-0 overflow-y-auto p-0;
    list-style: none;
    scrollbar-width: thin;
  }

  .ts-row--plain {
    --fg-inline: 0.75rem;
    padding-inline: var(--fg-inline);
    @apply flex min-h-[var(--table-row-h)] items-center gap-2 border-b border-base-300/30 text-sm;
  }

  .ts-row--plain .ts-row__default-label {
    @apply min-w-0 flex-1;
  }

  .ts-row--plain .ts-row__badge {
    @apply ml-auto shrink-0;
  }

  .ts-row--plain.ts-row--last {
    @apply border-b-0;
  }

  .ts-panel :global(.field-group__header-row) {
    @apply min-h-[var(--table-row-h)] min-w-0 flex-1 items-center gap-2 py-0;
  }

  .ts-panel :global(.field-group__content) {
    @apply pb-2 pt-0;
  }

  .ts-row__check-label {
    @apply flex shrink-0 cursor-pointer items-center py-1 pr-2;
  }

  .ts-row__check-label--disabled {
    @apply cursor-not-allowed opacity-60;
  }

  .ts-row__checkbox {
    @apply shrink-0;
  }

  .ts-row__attrs {
    @apply flex min-w-0 flex-1 flex-wrap items-baseline gap-x-2 gap-y-0.5 overflow-hidden py-1;
  }

  .ts-row__attr {
    @apply inline-flex min-w-0 max-w-full items-baseline;
  }

  .ts-row__default-label {
    @apply text-sm italic;
    color: var(--color-muted);
  }

  .ts-row__badge {
    @apply shrink-0;
  }

  .ts-fields-empty {
    @apply m-0 py-2 text-sm italic;
    color: var(--color-muted);
  }

  .ts-panel__cap-note {
    @apply m-0 px-3 pb-2 text-sm text-warning/80;
  }
</style>
