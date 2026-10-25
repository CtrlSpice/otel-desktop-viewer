<script lang="ts">
  /*
   * One collapsible row per series, with its attributes and datapoints in
   * one table. Resource metadata belongs to the metric Fields section.
   */
  import { tick } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { ArrowRight01Icon } from '@hugeicons/core-free-icons'
  import {
    getMetricViewContext,
    SPARKLINE_WIDTH_PX,
  } from '@/contexts/metric-view-context.svelte'
  import type { LegendTimeseries as PanelTimeseries } from '@/types/metric-chart-types'
  import type { MetricSeriesViewData } from '@/types/api-types'
  import { MAX_VISIBLE_TIMESERIES } from '@/components/metrics/utils/metric-timeseries-visible'
  import { chartNeutral, readableTextColor } from '@/utils/chart-palette'
  import { themeSignal } from '@/state/theme.svelte'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import GraphColorPicker from '@/components/shared/GraphColorPicker.svelte'
  import Identifier from '@/components/shared/Identifier.svelte'
  import { minimalSeriesLabels } from '@/components/metrics/utils/minimal-series-labels'
  import SeriesDatapointList from '@/components/metrics/Detail/SeriesDatapointList.svelte'
  import Sparkline from '@/components/metrics/Charts/Sparkline.svelte'
  import { dedupeAttributes } from '@/components/metrics/utils/dedupe-attributes'
  import { detailPanel } from '@/state/detail-panel.svelte'
  import { attributeValueLabel } from '@/components/shared/attribute-label'

  const ctx = getMetricViewContext()
  let panel: HTMLDivElement
  let hoveredSeriesKey = $state<string | null>(null)
  let focusedSeriesKey = $state<string | null>(null)

  $effect.pre(() => {
    void ctx.metric?.metricRef
    hoveredSeriesKey = null
    focusedSeriesKey = null
  })
  $effect(() => {
    ctx.previewSeriesKey = ctx.isHistogramKind
      ? null
      : (hoveredSeriesKey ?? focusedSeriesKey)
    return () => {
      ctx.previewSeriesKey = null
    }
  })

  let rows = $derived<PanelTimeseries[]>(
    ctx.isHistogramKind
      ? ctx.histogramLegendTimeseries
      : ctx.gaugeSumLegendTimeseries
  )
  let visibleKeys = $derived(ctx.visibleSeries)

  let timeseriesByKey = $derived.by((): Map<string, MetricSeriesViewData> => {
    const m = ctx.metric
    if (!m) return new Map()
    return new Map(m.timeseries.map(ts => [ts.seriesRef, ts]))
  })

  let capReached = $derived(
    !ctx.isHistogramKind && visibleKeys.size >= MAX_VISIBLE_TIMESERIES
  )

  const labels = $derived(minimalSeriesLabels(rows))

  function toggle(key: string, checked: boolean) {
    ctx.toggleTimeseriesVisible(key, checked)
  }

  function attrsTooltip(attrs: PanelTimeseries['attributes']): string {
    const unique = dedupeAttributes(attrs)
    if (unique.length === 0) return 'default series'
    return unique
      .map(a => `${a.key}: ${attributeValueLabel(a.value)}`)
      .join(' ')
  }

  function setTimeseriesOpen(key: string, open: boolean) {
    if (open) ctx.expandedTimeseries.add(key)
    else ctx.expandedTimeseries.delete(key)
  }

  $effect(() => {
    const dpID = ctx.selectedDatapointID
    void detailPanel.revealRequest
    const seriesKey = ctx.selectedSeriesKey
    if (!dpID || !seriesKey || !detailPanel.open) return

    ctx.expandedTimeseries.add(seriesKey)

    let cancelled = false
    void tick().then(async () => {
      if (cancelled) return
      const disclosure = panel.closest('details')
      if (disclosure) {
        disclosure.open = true
        await tick()
      }
      if (cancelled) return
      panel
        .querySelector(`[data-dp-id="${CSS.escape(dpID)}"]`)
        ?.scrollIntoView({ block: 'nearest' })
    })
    return () => {
      cancelled = true
    }
  })
</script>

<div class="ts-panel" role="region" aria-label="Timeseries" bind:this={panel}>
  {#each rows as ts (ts.key)}
    {@const checked = visibleKeys.has(ts.key)}
    {@const checkboxDisabled = !checked && capReached}
    {@const seriesColor = ctx.isHistogramKind
      ? undefined
      : ctx.timeseriesColorByKey.get(ts.key)}
    {@const color = checked && seriesColor ? seriesColor : chartNeutral()}
    {@const fg = checked
      ? seriesColor
        ? readableTextColor(seriesColor, themeSignal.value)
        : 'var(--color-neutral-content)'
      : chartNeutral()}
    {@const hasAttrs = dedupeAttributes(ts.attributes).length > 0}
    {@const compactLabel = labels.get(ts.key)!}
    {@const tooltip = attrsTooltip(ts.attributes)}
    {@const headerLabel = compactLabel.needsIdentity
      ? `${compactLabel.text} · ${ts.key}`
      : compactLabel.text}
    {@const metricTs = timeseriesByKey.get(ts.key)}
    {@const expanded = ctx.expandedTimeseries.has(ts.key)}
    {@const sparklinePoints = ctx.sparklineByKey.get(ts.key) ?? []}
    {@const sparklineColor =
      checked && seriesColor ? seriesColor : chartNeutral()}
    {@const showSparkline =
      !ctx.isHistogramKind &&
      !ctx.isUnsafeTemporality &&
      sparklinePoints.length > 0}

    <FieldGroup
      label={headerLabel}
      card
      name="metric-series"
      open={expanded}
      onOpenChange={open => setTimeseriesOpen(ts.key, open)}
      onHeaderHover={active => (hoveredSeriesKey = active ? ts.key : null)}
      onHeaderFocus={active => (focusedSeriesKey = active ? ts.key : null)}
    >
      {#snippet headerAction()}
        <div
          class="ts-row__title-row"
          class:ts-row__title-row--preview={showSparkline}
          style:--colour-column={seriesColor && ctx.metric ? 'auto' : ''}
        >
          <span
            class="ts-row__caret"
            class:rotate-90={expanded}
            title={`${expanded ? 'Collapse' : 'Expand'} ${headerLabel}`}
            aria-hidden="true"
          >
            <HugeiconsIcon
              icon={ArrowRight01Icon}
              size={16}
              strokeWidth={1.5}
            />
          </span>
          <label
            class="ts-row__check-label"
            class:ts-row__check-label--disabled={checkboxDisabled}
            title={tooltip}
          >
            <input
              type="checkbox"
              class="checkbox checkbox-xs rounded-full ts-row__checkbox"
              class:ts-row__checkbox--histogram={ctx.isHistogramKind}
              style:--input-color={color}
              style:color={ctx.isHistogramKind
                ? 'var(--color-base-content)'
                : fg}
              {checked}
              disabled={checkboxDisabled}
              aria-label={`Show ${headerLabel} in chart`}
              onchange={e => toggle(ts.key, e.currentTarget.checked)}
            />
          </label>
          <span
            class="ts-row__attrs ts-row__attrs-text"
            class:ts-row__default-label={!hasAttrs}
            title={tooltip}
          >
            {compactLabel.text}
            {#if compactLabel.needsIdentity}
              · <Identifier value={ts.key} />
            {/if}
          </span>
          {#if showSparkline}
            <span class="ts-row__sparkline">
              <Sparkline
                points={sparklinePoints}
                color={sparklineColor}
                width={SPARKLINE_WIDTH_PX}
              />
            </span>
          {/if}
          {#if seriesColor && ctx.metric}
            <div class="ts-row__colour-edit">
              <GraphColorPicker
                scope={`metric:${ctx.metric.metricRef}`}
                identity={ts.key}
                label={`Colour for ${headerLabel}`}
                color={seriesColor}
              />
            </div>
          {/if}
        </div>
      {/snippet}

      {#if metricTs}
        <!-- What arrived, once it has: metricTs.datapoints are the
                 store's merged buckets for a reduced histogram, and this list
                 is the view that has to show the telemetry itself. The merged
                 rows stand in for the moment the fetch is in flight, so the
                 group is never briefly empty. -->
        {@const rawDatapoints = ctx.seriesDatapoints(ts.key)}
        {@const listDatapoints = rawDatapoints ?? metricTs.datapoints}
        <SeriesDatapointList
          datapoints={listDatapoints}
          attributes={metricTs.attributes}
          attributeOwner={`timeseries ${ts.key}`}
          flush
        />
      {:else}
        <p class="ts-fields-empty">Timeseries not found</p>
      {/if}
    </FieldGroup>
  {/each}

  {#if capReached && !ctx.isHistogramKind}
    <p class="ts-panel__cap-note">
      Cap of {MAX_VISIBLE_TIMESERIES} timeseries reached. Uncheck one to enable another.
    </p>
  {/if}
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .ts-panel {
    @apply flex min-w-0 flex-col gap-1;
  }

  .ts-panel :global(.field-group) {
    --fg-inline: 0;
  }

  .ts-row__title-row {
    display: grid;
    grid-template-columns: auto auto minmax(0, 1fr) var(--colour-column,);
    align-items: center;
    gap: 0.5rem;
    min-height: var(--table-row-h);
    min-width: 0;
    width: 100%;
  }

  .ts-row__caret {
    @apply inline-flex size-4 shrink-0 items-center justify-center text-(--color-subtle);
  }

  .ts-row__title-row--preview {
    grid-template-columns: auto auto minmax(0, 1fr) minmax(0, 96px) var(
        --colour-column,
      );
  }

  .ts-row__sparkline {
    @apply min-w-0 shrink-0 justify-self-end;
    max-width: 96px;
  }

  .ts-row__colour-edit {
    @apply pointer-events-none opacity-0;
  }

  :global(.field-group__heading:hover) .ts-row__colour-edit,
  .ts-row__title-row:focus-within .ts-row__colour-edit {
    @apply pointer-events-auto opacity-100;
  }

  @media (hover: none) {
    .ts-row__colour-edit {
      @apply pointer-events-auto opacity-100;
    }
  }

  .ts-row__sparkline :global(.sparkline) {
    @apply block h-[18px] w-full max-w-[96px];
  }

  .ts-row__check-label {
    @apply flex shrink-0 cursor-pointer items-center py-1;
  }

  .ts-row__check-label--disabled {
    @apply cursor-not-allowed opacity-60;
  }

  .ts-row__checkbox {
    @apply shrink-0;
  }

  .ts-row__checkbox--histogram {
    background-color: transparent;
    border-color: color-mix(
      in oklab,
      var(--color-base-content) 45%,
      transparent
    );
    box-shadow: none;
  }

  .ts-row__attrs {
    @apply min-w-0 overflow-hidden py-1;
  }

  .ts-row__attrs-text,
  .ts-row__default-label {
    @apply block text-lg font-normal;
    overflow-wrap: anywhere;
  }

  .ts-row__default-label {
    @apply italic;
    color: var(--color-subtle);
  }

  .ts-fields-empty {
    @apply m-0 px-3 py-2 text-sm italic;
    color: var(--color-muted);
  }

  .ts-panel__cap-note {
    @apply m-0 px-3 pb-0 text-sm text-base-content;
  }
</style>
