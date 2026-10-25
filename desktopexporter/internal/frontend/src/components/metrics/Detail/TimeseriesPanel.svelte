<script lang="ts">
  import { tick } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { ArrowRight01Icon } from '@hugeicons/core-free-icons'
  import {
    getMetricViewContext,
    SPARKLINE_WIDTH_PX,
  } from '@/contexts/metric-view-context.svelte'
  import type { LegendTimeseries as PanelTimeseries } from '@/types/metric-chart-types'
  import { MAX_VISIBLE_TIMESERIES } from '@/components/metrics/utils/metric-timeseries-visible'
  import { chartNeutral } from '@/utils/chart-palette'
  import GraphColorPicker from '@/components/shared/GraphColorPicker.svelte'
  import Identifier from '@/components/shared/Identifier.svelte'
  import ResizableTable from '@/components/shared/ResizableTable.svelte'
  import TableSortButton from '@/components/shared/TableSortButton.svelte'
  import TableColumnHeading from '@/components/shared/TableColumnHeading.svelte'
  import { selectableTableRow } from '@/components/shared/utils/selectable-table-row'
  import { compareSeriesCells } from '@/components/metrics/utils/minimal-series-labels'
  import SeriesDatapointList from './SeriesDatapointList.svelte'
  import Sparkline from '@/components/metrics/Charts/Sparkline.svelte'
  import { detailPanel } from '@/state/detail-panel.svelte'
  import { attributeValueLabel } from '@/components/shared/attribute-label'

  const ctx = getMetricViewContext()
  const id = $props.id()
  let panel: HTMLDivElement
  let hoveredSeriesKey = $state<string | null>(null)
  let focusedSeriesKey = $state<string | null>(null)
  let sort = $state<{ key: string; direction: 'asc' | 'desc' } | null>(null)

  $effect.pre(() => {
    void ctx.metric?.metricRef
    hoveredSeriesKey = null
    focusedSeriesKey = null
    sort = null
  })
  $effect(() => {
    ctx.previewSeriesKey = ctx.isHistogramKind
      ? null
      : (hoveredSeriesKey ?? focusedSeriesKey)
    return () => {
      ctx.previewSeriesKey = null
    }
  })

  const rows = $derived(
    ctx.isHistogramKind
      ? ctx.histogramLegendTimeseries
      : ctx.gaugeSumLegendTimeseries
  )
  const table = $derived(ctx.seriesTable)
  const checkedKeys = $derived(ctx.visibleSeries)
  const capReached = $derived(
    !ctx.isHistogramKind && checkedKeys.size >= MAX_VISIBLE_TIMESERIES
  )
  const timeseriesByKey = $derived(
    new Map(ctx.metric?.timeseries.map(ts => [ts.seriesRef, ts]) ?? [])
  )
  const identityColumn = $derived(
    table.columns.length === 0 ||
      [...table.labels.values()].some(label => label.needsIdentity)
  )
  const previewColumn = $derived(
    !ctx.isHistogramKind &&
      !ctx.isUnsafeTemporality &&
      [...ctx.sparklineByKey.values()].some(points => points.length > 0)
  )
  const columns = $derived([
    { id: 'controls', label: 'Show', min: 64, width: 64, fixed: true },
    ...(!ctx.isHistogramKind
      ? [{ id: 'colour', label: 'Colour', min: 64, width: 64, fixed: true }]
      : []),
    ...table.columns.map(column => ({
      id: `attr:${column.key}`,
      label: column.key,
      min: 112,
      width: 176,
    })),
    ...(identityColumn
      ? [{ id: 'identity', label: 'Series identity', min: 128, width: 176 }]
      : []),
    ...(previewColumn
      ? [{ id: 'preview', label: 'Preview', min: 104, width: 104, fixed: true }]
      : []),
  ])
  const sortedRows = $derived.by(() => {
    const column = table.columns.find(column => column.key === sort?.key)
    if (!column || !sort) return rows
    const direction = sort.direction === 'asc' ? 1 : -1
    return [...rows].sort(
      (a, b) =>
        direction *
          compareSeriesCells(
            column.cells.get(a.key)!,
            column.cells.get(b.key)!
          ) || a.key.localeCompare(b.key)
    )
  })

  function sortBy(key: string) {
    sort =
      sort?.key === key
        ? sort.direction === 'asc'
          ? { key, direction: 'desc' }
          : null
        : { key, direction: 'asc' }
  }

  function attrsTooltip(attrs: PanelTimeseries['attributes']): string {
    return (
      attrs.map(a => `${a.key}: ${attributeValueLabel(a.value)}`).join(' ') ||
      'default series'
    )
  }

  function toggleOpen(key: string) {
    const wasOpen = ctx.expandedTimeseries.has(key)
    ctx.expandedTimeseries.clear()
    if (!wasOpen) ctx.expandedTimeseries.add(key)
  }

  $effect(() => {
    const dpID = ctx.selectedDatapointID
    void detailPanel.revealRequest
    const seriesKey = ctx.selectedSeriesKey
    if (!dpID || !seriesKey || !detailPanel.open) return
    ctx.expandedTimeseries.clear()
    ctx.expandedTimeseries.add(seriesKey)
    let cancelled = false
    void tick().then(async () => {
      if (cancelled) return
      const disclosure = panel.closest('details')
      if (disclosure) {
        disclosure.open = true
        await tick()
      }
      if (!cancelled)
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
  <ResizableTable
    {columns}
    caption="Metric series"
    storageKey="metric-series-table:widths"
    pinnedCount={ctx.isHistogramKind ? 2 : 3}
    selectable
  >
    {#snippet header(layout)}
      <tr>
        <th
          scope="col"
          class="table-cell"
          class:table-cell--pinned={layout.pinned.controls !== undefined}
          style:left="{layout.pinned.controls}px">Show</th
        >
        {#if !ctx.isHistogramKind}<th
            scope="col"
            class="table-cell"
            class:table-cell--pinned={layout.pinned.colour !== undefined}
            style:left="{layout.pinned.colour}px"
            ><span class="sr-only">Colour</span></th
          >{/if}
        {#each table.columns as column (column.key)}
          <th
            scope="col"
            class="table-cell"
            class:table-cell--pinned={layout.pinned[`attr:${column.key}`] !==
              undefined}
            style:left="{layout.pinned[`attr:${column.key}`]}px"
            aria-sort={sort?.key === column.key
              ? sort.direction === 'asc'
                ? 'ascending'
                : 'descending'
              : 'none'}
          >
            <div class="flex items-center gap-2">
              <TableSortButton
                label={column.key}
                direction={sort?.key === column.key
                  ? sort.direction
                  : undefined}
                onclick={() => sortBy(column.key)}
                inset={false}
              >
                <TableColumnHeading label={column.key} />
              </TableSortButton>
            </div>
          </th>
        {/each}
        {#if identityColumn}<th scope="col">Series identity</th>{/if}
        {#if previewColumn}<th scope="col">Preview</th>{/if}
      </tr>
    {/snippet}
    {#snippet children(layout)}
      {#each sortedRows as ts (ts.key)}
        {@const checked = checkedKeys.has(ts.key)}
        {@const seriesColor = ctx.isHistogramKind
          ? undefined
          : ctx.savedTimeseriesColorByKey.get(ts.key)}
        {@const color = checked && seriesColor ? seriesColor : chartNeutral()}
        {@const label = table.labels.get(ts.key)!}
        {@const headerLabel = label.needsIdentity
          ? `${label.text} · ${ts.key}`
          : label.text}
        {@const expanded = ctx.expandedTimeseries.has(ts.key)}
        {@const detailID = `${id}-series-${ts.key}`}
        {@const metricTs = timeseriesByKey.get(ts.key)}
        <tr
          class="ts-row item-state"
          data-series-key={ts.key}
          data-selectable-row
          aria-expanded={expanded}
          aria-controls={detailID}
          use:selectableTableRow={{
            selected: expanded,
            onSelect: () => toggleOpen(ts.key),
          }}
          onpointerenter={() => (hoveredSeriesKey = ts.key)}
          onpointerleave={() => (hoveredSeriesKey = null)}
          onfocusin={() => (focusedSeriesKey = ts.key)}
          onfocusout={() => (focusedSeriesKey = null)}
        >
          <td
            class="table-cell"
            class:table-cell--pinned={layout.pinned.controls !== undefined}
            style:left="{layout.pinned.controls}px"
          >
            <div class="flex items-center gap-2">
              <button
                type="button"
                class="ts-row__caret"
                aria-label={`${expanded ? 'Collapse' : 'Expand'} ${headerLabel}`}
                aria-expanded={expanded}
                aria-controls={detailID}
                onclick={() => toggleOpen(ts.key)}
              >
                <HugeiconsIcon
                  icon={ArrowRight01Icon}
                  size={16}
                  strokeWidth={1.5}
                  class={expanded ? 'rotate-90' : ''}
                />
              </button>
              <input
                type="checkbox"
                class="checkbox checkbox-xs ts-row__checkbox"
                {checked}
                disabled={!checked && capReached}
                aria-label={`Show ${headerLabel} in chart`}
                onchange={event =>
                  ctx.toggleTimeseriesVisible(
                    ts.key,
                    event.currentTarget.checked
                  )}
              />
            </div>
          </td>
          {#if !ctx.isHistogramKind}<td
              class="table-cell"
              class:table-cell--pinned={layout.pinned.colour !== undefined}
              style:left="{layout.pinned.colour}px"
            >
              {#if seriesColor && ctx.metric}
                <GraphColorPicker
                  scope={`metric:${ctx.metric.metricRef}`}
                  identity={ts.key}
                  label={`Colour for ${headerLabel}`}
                  color={seriesColor}
                />
              {/if}
            </td>{/if}
          {#each table.columns as column (column.key)}
            <td
              class="table-cell ts-row__attrs"
              class:table-cell--pinned={layout.pinned[`attr:${column.key}`] !==
                undefined}
              style:left="{layout.pinned[`attr:${column.key}`]}px"
              title={attrsTooltip(ts.attributes)}
              >{column.cells.get(ts.key)!.label}</td
            >
          {/each}
          {#if identityColumn}<td
              class="table-cell"
              title={attrsTooltip(ts.attributes)}
            >
              {#if label.needsIdentity}<Identifier
                  value={ts.key}
                />{:else}{label.text}{/if}
            </td>{/if}
          {#if previewColumn}<td
              ><span class="ts-row__sparkline">
                <Sparkline
                  points={ctx.sparklineByKey.get(ts.key) ?? []}
                  {color}
                  width={SPARKLINE_WIDTH_PX}
                />
              </span></td
            >{/if}
        </tr>
        <tr
          id={detailID}
          class="ts-row__detail"
          data-series-detail={ts.key}
          hidden={!expanded}
        >
          <td colspan={columns.length}>
            {#if metricTs}
              <SeriesDatapointList
                datapoints={ctx.seriesDatapoints(ts.key) ?? metricTs.datapoints}
                attributes={metricTs.attributes}
                attributeOwner={`timeseries ${ts.key}`}
                flush
              />
            {:else}<p class="text-sm text-muted">Timeseries not found</p>{/if}
          </td>
        </tr>
      {/each}
      {#if rows.length === 0}<tr
          ><td colspan={columns.length} class="text-subtle">No series.</td></tr
        >{/if}
    {/snippet}
  </ResizableTable>
  {#if capReached}
    <p class="px-3 py-2 text-sm">
      Cap of {MAX_VISIBLE_TIMESERIES} checked series reached. Uncheck one to enable
      another.
    </p>
  {/if}
</div>

<style lang="postcss">
  @reference '../../../app.css';
  .ts-panel {
    @apply flex min-h-0 min-w-0 flex-1 flex-col;
  }
  .ts-row__caret {
    @apply inline-flex size-4 shrink-0 cursor-pointer items-center justify-center rounded-sm text-subtle focus-visible:outline-2 focus-visible:outline-primary;
  }
  .ts-row__attrs {
    @apply text-sm;
    overflow-wrap: anywhere;
  }
  .ts-row__sparkline {
    @apply block w-24;
  }
  .ts-row__sparkline :global(.sparkline) {
    @apply block h-[18px] w-full;
  }
  .ts-row__detail > td {
    padding: 0;
  }
</style>
