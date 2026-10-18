<script lang="ts">
  /*
   * One collapsible row per series, with its attributes and datapoints in
   * one table. Resource metadata belongs to the metric Fields section.
   */
  import { tick } from 'svelte'
  import {
    getMetricViewContext,
    SPARKLINE_WIDTH_PX,
  } from '@/contexts/metric-view-context.svelte'
  import type { LegendTimeseries as PanelTimeseries } from '@/types/metric-chart-types'
  import type { MetricSeriesViewData } from '@/types/api-types'
  import { MAX_VISIBLE_TIMESERIES } from '@/components/metrics/utils/metric-timeseries-visible'
  import { formatMetricValue } from '@/components/metrics/utils/format-metric-value'
  import type { SeriesStat } from '@/components/metrics/utils/aggregation'
  import { chartNeutral, readableTextColor } from '@/utils/chart-palette'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import SeriesDatapointList from '@/components/metrics/Detail/SeriesDatapointList.svelte'
  import Sparkline from '@/components/metrics/Charts/Sparkline.svelte'
  import { dedupeAttributes } from '@/components/metrics/utils/dedupe-attributes'
  import { detailPanel } from '@/state/detail-panel.svelte'
  import {
    attributeValueCanonical,
    attributeValueLabel,
  } from '@/components/shared/attribute-label'

  const ctx = getMetricViewContext()

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

  /** Attribute keys that differ across rows (stable regardless of checkbox). */
  let differingAttrKeys = $derived.by((): Set<string> | null => {
    if (rows.length < 2) return null

    const allKeys = new Set<string>()
    for (const row of rows) {
      for (const a of dedupeAttributes(row.attributes)) allKeys.add(a.key)
    }

    const differing = new Set<string>()
    for (const key of allKeys) {
      const signatures = new Set<string>()
      for (const row of rows) {
        const a = dedupeAttributes(row.attributes).find(x => x.key === key)
        signatures.add(a ? attributeValueCanonical(a.value) : '')
      }
      if (signatures.size > 1) differing.add(key)
    }
    if (differing.size === 0) return null
    return differing
  })

  function headerAttrs(
    attrs: PanelTimeseries['attributes']
  ): PanelTimeseries['attributes'] {
    const unique = dedupeAttributes(attrs)
    if (!differingAttrKeys) return unique
    return unique.filter(a => differingAttrKeys.has(a.key))
  }

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

  const STAT_LABEL: Record<SeriesStat, string> = {
    min: 'min',
    max: 'max',
    avg: 'avg',
    total: 'Σ',
  }

  function statBadgeTitle(stat: SeriesStat, value: number): string {
    const unit = ctx.metric?.unit ?? ''
    const formatted = formatMetricValue(value)
    if (ctx.aggregationView === 'rate') {
      const rateUnit =
        unit.trim() === '' || unit === '1' ? '/s' : `${unit.trim()}/s`
      return `${stat} ${formatted}${rateUnit ? ` ${rateUnit}` : ''}`
    }
    if (unit.trim() && unit !== '1') {
      return `${stat} ${formatted} ${unit.trim()}`
    }
    return `${stat} ${formatted}`
  }

  $effect(() => {
    const dpID = ctx.selectedDatapointID
    void detailPanel.revealRequest
    const seriesKey = ctx.selectedSeriesKey
    if (!dpID || !seriesKey || !detailPanel.open) return

    ctx.expandedTimeseries.add(seriesKey)

    tick().then(() => {
      document
        .querySelector(`[data-dp-id="${dpID}"]`)
        ?.scrollIntoView({ block: 'nearest' })
    })
  })
</script>

<div class="ts-panel" role="region" aria-label="Timeseries">
  {#each rows as ts (ts.key)}
    {@const checked = visibleKeys.has(ts.key)}
    {@const checkboxDisabled = !checked && capReached}
    {@const seriesColor = ctx.timeseriesColorByKey.get(ts.key)}
    {@const color = checked && seriesColor ? seriesColor : chartNeutral()}
    {@const fg =
      checked && seriesColor ? readableTextColor(seriesColor) : chartNeutral()}
    {@const hasAttrs = dedupeAttributes(ts.attributes).length > 0}
    {@const rowHeaderAttrs = headerAttrs(ts.attributes)}
    {@const tooltip = attrsTooltip(ts.attributes)}
    {@const headerLabel =
      hasAttrs && rowHeaderAttrs.length > 0
        ? attrsTooltip(rowHeaderAttrs)
        : 'default series'}
    {@const metricTs = timeseriesByKey.get(ts.key)}
    {@const expanded = ctx.expandedTimeseries.has(ts.key)}
    {@const sparklinePoints = ctx.sparklineByKey.get(ts.key) ?? []}
    {@const sparklineColor =
      checked && seriesColor ? seriesColor : chartNeutral()}
    {@const sparklineSuppressed =
      ctx.isHistogramKind || ctx.isUnsafeTemporality}
    {@const rowStats = ctx.seriesStatsByKey.get(ts.key)}
    {@const statBadges = ctx.availableSeriesStatBadges}
    {@const showStatBadges =
      !sparklineSuppressed && statBadges.length > 0 && rowStats !== undefined}

    <FieldGroup
      label={headerLabel}
      card
      name="metric-series"
      open={expanded}
      onOpenChange={open => setTimeseriesOpen(ts.key, open)}
    >
      {#snippet headerAction()}
        <span class="ts-row__header">
          <span
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
                aria-label={`Show ${headerLabel} in chart`}
                onchange={e => toggle(ts.key, e.currentTarget.checked)}
              />
            </label>
            <span
              class="ts-row__attrs ts-row__attrs-text"
              class:ts-row__default-label={!hasAttrs}
              title={tooltip}
              >{hasAttrs
                ? attrsTooltip(rowHeaderAttrs)
                : 'default series'}</span
            >
            {#if !sparklineSuppressed}
              <!-- The store reduced these points to this width, so the
                       constant is shared rather than repeated here. -->
              <span class="ts-row__sparkline">
                <Sparkline
                  points={sparklinePoints}
                  color={sparklineColor}
                  width={SPARKLINE_WIDTH_PX}
                />
              </span>
            {/if}
          </span>
          {#if showStatBadges}
            <span class="ts-row__stats-row" aria-label="Series stats">
              {#each statBadges as stat (stat)}
                {@const value = rowStats[stat]}
                {#if value !== undefined}
                  <span class="badge-count" title={statBadgeTitle(stat, value)}>
                    {STAT_LABEL[stat]}
                    {formatMetricValue(value)}
                  </span>
                {/if}
              {/each}
            </span>
          {/if}
        </span>
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
    /* Fixed chrome widths for sparkline inset (checkbox-xs + label pad). */
    --ts-check-col: 1.5rem;
    --ts-header-gap: 0.5rem;
    --ts-content-inset: calc(var(--ts-check-col) + var(--ts-header-gap));
  }

  .ts-panel :global(.field-group) {
    --fg-inline: 0;
  }

  .ts-row__header {
    @apply flex min-w-0 w-full flex-col gap-0.5;
  }

  .ts-row__title-row {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) minmax(0, 128px);
    align-items: center;
    gap: 0.5rem;
    min-height: var(--table-row-h);
    min-width: 0;
    width: 100%;
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

  .ts-row__stats-row {
    @apply flex min-w-0 flex-wrap items-center justify-end gap-0.5;
    padding-left: var(--ts-content-inset);
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

  .ts-row__attrs-text,
  .ts-row__default-label {
    @apply block truncate text-sm font-medium;
  }

  .ts-row__default-label {
    @apply italic;
    color: var(--color-muted);
  }

  .ts-fields-empty {
    @apply m-0 px-3 py-2 text-sm italic;
    color: var(--color-muted);
  }

  .ts-panel__cap-note {
    @apply m-0 px-3 pb-0 text-sm text-warning/80;
  }
</style>
