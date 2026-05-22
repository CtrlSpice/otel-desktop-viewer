<script lang="ts">
  /*
   * TimeseriesPanel: per-series rows in the detail pane Series tab.
   * One FieldGroup per timeseries. The header row carries the visibility
   * checkbox, inline attribute label, and sparkline on one row;
   * expand for attribute fields and a nested Datapoints section.
   */
  import { tick } from 'svelte'
  import { SvelteSet } from 'svelte/reactivity'
  import { getMetricViewContext } from '@/contexts/metric-view-context.svelte'
  import type { LegendTimeseries as PanelTimeseries } from '@/types/metric-chart-types'
  import type { MetricTimeseries } from '@/types/api-types'
  import { MAX_VISIBLE_TIMESERIES } from '@/components/metrics/utils/metric-timeseries-visible'
  import { chartNeutral, readableTextColor } from '@/utils/chart-palette'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import MetricField from '@/components/metrics/Detail/MetricField.svelte'
  import SeriesDatapointList from '@/components/metrics/Detail/SeriesDatapointList.svelte'
  import Sparkline from '@/components/metrics/Charts/Sparkline.svelte'

  const ctx = getMetricViewContext()
  const expandedDatapointSections = new SvelteSet<string>()

  let rows = $derived<PanelTimeseries[]>(
    ctx.isHistogramKind
      ? ctx.histogramLegendTimeseries
      : ctx.gaugeSumLegendTimeseries
  )
  let visibleKeys = $derived(
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

  function setDatapointsOpen(key: string, open: boolean) {
    if (open) expandedDatapointSections.add(key)
    else expandedDatapointSections.delete(key)
  }

  function seriesKeyForDatapointId(dpId: string): string | null {
    const m = ctx.metric
    if (!m) return null
    for (const ts of m.timeseries) {
      if (ts.datapoints.some((dp) => dp.id === dpId)) {
        return ts.attributesKey
      }
    }
    return null
  }

  $effect(() => {
    ctx.metric?.id
    expandedDatapointSections.clear()
  })

  $effect(() => {
    const dpId = ctx.selectedDatapointId
    if (!dpId) return

    const seriesKey = seriesKeyForDatapointId(dpId)
    if (seriesKey) {
      ctx.expandedTimeseries.add(seriesKey)
      expandedDatapointSections.add(seriesKey)
    }

    tick().then(() => {
      document
        .querySelector(`[data-dp-id="${dpId}"]`)
        ?.scrollIntoView({ block: 'nearest' })
    })
  })
</script>

<div class="ts-panel" role="region" aria-label="Timeseries">
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
        hasAttrs && rowHeaderAttrs.length > 0
          ? attrsTooltip(rowHeaderAttrs)
          : 'default series'}
      {@const metricTs = timeseriesByKey.get(ts.key)}
      {@const expanded = ctx.expandedTimeseries.has(ts.key)}
      {@const datapointsOpen = expandedDatapointSections.has(ts.key)}
      {@const isLast = i === rows.length - 1 && !capReached}
      {@const sparklinePoints = ctx.sparklineByKey.get(ts.key) ?? []}
      {@const sparklineColor =
        checked && seriesColor ? seriesColor : chartNeutral()}
      {@const sparklineSuppressed =
        ctx.isHistogramKind || ctx.isUnspecifiedTemporality}

      <div class="ts-row-wrap">
        <FieldGroup
          label={headerLabel}
          open={expanded}
          onOpenChange={(open) => setTimeseriesOpen(ts.key, open)}
          last={isLast}
        >
          {#snippet headerAction()}
            <div
              class="ts-row__title-row"
              class:ts-row__title-row--no-sparkline={sparklineSuppressed}
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
                    toggle(
                      ts.key,
                      (e.currentTarget as HTMLInputElement).checked
                    )}
                />
              </label>
              <div class="ts-row__attrs" title={tooltip}>
                {#if hasAttrs}
                  <span class="ts-row__attrs-text">{attrsTooltip(rowHeaderAttrs)}</span>
                {:else}
                  <span class="ts-row__default-label">default series</span>
                {/if}
              </div>
              {#if !sparklineSuppressed}
                <div class="ts-row__sparkline">
                  <Sparkline
                    points={sparklinePoints}
                    color={sparklineColor}
                    width={128}
                  />
                </div>
              {/if}
            </div>
          {/snippet}

          {#if metricTs}
            {#if hasAttrs}
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
            {/if}
            <FieldGroup
              label="Datapoints"
              count={metricTs.datapoints.length}
              open={datapointsOpen}
              onOpenChange={(open) => setDatapointsOpen(ts.key, open)}
              last
            >
              <SeriesDatapointList datapoints={metricTs.datapoints} flush />
            </FieldGroup>
          {:else}
            <p class="ts-fields-empty">Timeseries not found</p>
          {/if}
        </FieldGroup>
      </div>
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
    @apply flex min-w-0 flex-col p-2;
    --ts-caret-col: 0.875rem;
    /* Fixed chrome widths for sparkline inset (checkbox-xs + label pad). */
    --ts-check-col: 1.5rem;
    --ts-header-gap: 0.5rem;
    --ts-content-inset: calc(var(--ts-check-col) + var(--ts-header-gap));
  }

  .ts-panel__list {
    @apply m-0 flex flex-col gap-0 p-0;
    list-style: none;
  }

  .ts-panel :global(.field-group) {
    --fg-inline: 0;
  }

  .ts-panel :global(.field-group__content),
  .ts-panel :global(.field-group__header-row),
  .ts-panel :global(.field-group__heading) {
    padding-inline: 0;
  }

  .ts-panel :global(.field-group__content) {
    @apply pb-2 pt-0;
  }

  /* Expanded series body: align with label text + sparkline column. */
  .ts-panel :global(.field-group__header-row + .field-group__content) {
    padding-left: var(--ts-content-inset);
  }

  .ts-panel :global(.detail-fields .detail-cell) {
    @apply pl-0 pr-2 text-sm;
  }

  .ts-panel :global(.field-group__heading) {
    @apply text-sm;
    display: grid;
    grid-template-columns: minmax(0, 1fr) auto var(--ts-caret-col);
    align-items: center;
    gap: 0.5rem;
  }

  .ts-panel :global(.field-group__heading :global(.field-group__caret)) {
    @apply ml-0 justify-self-end;
  }

  .ts-panel :global(.field-group__header-row) {
    @apply min-w-0 items-center gap-x-2 py-0;
    display: grid;
    grid-template-columns: minmax(0, 1fr) var(--ts-caret-col);
  }

  .ts-panel :global(.field-group__caret-btn) {
    @apply h-3.5 w-3.5 min-h-0 min-w-0 justify-self-end self-center p-0;
    grid-column: 2;
    grid-row: 1;
  }

  .ts-row__title-row {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) minmax(0, 128px);
    align-items: center;
    gap: 0.5rem;
    min-height: var(--table-row-h);
    min-width: 0;
  }

  .ts-row__title-row--no-sparkline {
    grid-template-columns: auto minmax(0, 1fr);
  }

  .ts-row__sparkline {
    @apply min-w-0 shrink-0 justify-self-end;
    max-width: 128px;
  }

  .ts-row__sparkline :global(.sparkline) {
    @apply block h-[18px] w-full max-w-[128px];
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
    @apply min-w-0 overflow-hidden py-1;
  }

  .ts-row__attrs-text {
    @apply block truncate text-sm;
  }

  .ts-row__default-label {
    @apply text-sm italic;
    color: var(--color-muted);
  }

  .ts-fields-empty {
    @apply m-0 py-2 text-sm italic;
    color: var(--color-muted);
  }

  .ts-panel__cap-note {
    @apply m-0 pb-0 text-sm text-warning/80;
  }
</style>
