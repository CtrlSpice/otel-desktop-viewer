<script lang="ts">
  import { SvelteSet } from 'svelte/reactivity'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { ArrowRight01Icon } from '@hugeicons/core-free-icons'
  import AttributeRows from '@/components/shared/AttributeRows.svelte'
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
  import Sparkline from '@/components/metrics/Charts/Sparkline.svelte'
  import { attributeValueLabel } from '@/components/shared/attribute-label'

  const ctx = getMetricViewContext()
  const id = $props.id()
  const expandedAttributes = new SvelteSet<string>()
  let hoveredSeriesKey = $state<string | null>(null)
  let focusedSeriesKey = $state<string | null>(null)
  let sort = $state<{ key: string; direction: 'asc' | 'desc' } | null>(null)

  $effect.pre(() => {
    void ctx.metric?.metricRef
    hoveredSeriesKey = null
    focusedSeriesKey = null
    sort = null
    expandedAttributes.clear()
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
    new Map(
      ctx.metric?.timeseries.map(series => [series.seriesRef, series]) ?? []
    )
  )

  function toggleAttributes(key: string) {
    if (expandedAttributes.has(key)) expandedAttributes.delete(key)
    else expandedAttributes.add(key)
  }
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
    { id: 'controls', label: 'Attributes', min: 32, width: 32, fixed: true },
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
    { id: 'show', label: 'Show', min: 56, width: 56, fixed: true },
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
</script>

{#snippet colour(key: string, label: string)}
  {@const color = ctx.savedTimeseriesColorByKey.get(key)}
  {#if !ctx.isHistogramKind && color && ctx.metric}
    <GraphColorPicker
      scope={`metric:${ctx.metric.metricRef}`}
      identity={key}
      label={`Colour for ${label}`}
      {color}
    />
  {/if}
{/snippet}

<div class="ts-panel" role="region" aria-label="Timeseries">
  <ResizableTable
    {columns}
    caption="Metric series"
    storageKey="metric-series-table:widths"
    pinnedCount={2}
    selectable
  >
    {#snippet header(layout)}
      <tr>
        <th
          scope="col"
          class="table-cell"
          class:table-cell--pinned={layout.pinned.controls !== undefined}
          style:left="{layout.pinned.controls}px"
          ><span class="sr-only">Attributes</span></th
        >
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
        <th scope="col" class="text-right">Show</th>
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
        {@const attributes = timeseriesByKey.get(ts.key)?.attributes ?? []}
        {@const expanded = expandedAttributes.has(ts.key)}
        {@const detailID = `${id}-attributes-${ts.key}`}
        <tr
          class="ts-row item-state"
          data-series-key={ts.key}
          data-selectable-row
          use:selectableTableRow={{
            selected: ctx.selectedSeriesKey === ts.key,
            onSelect: () => ctx.selectSeries(ts.key),
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
                disabled={!attributes.length}
                aria-label={`${expanded ? 'Collapse' : 'Expand'} attributes for ${headerLabel}`}
                aria-expanded={expanded}
                aria-controls={detailID}
                onclick={() => toggleAttributes(ts.key)}
              >
                <HugeiconsIcon
                  icon={ArrowRight01Icon}
                  size={16}
                  strokeWidth={1.5}
                  class={expanded ? 'rotate-90' : ''}
                />
              </button>
            </div>
          </td>
          {#each table.columns as column (column.key)}
            <td
              class="table-cell ts-row__attrs"
              class:table-cell--pinned={layout.pinned[`attr:${column.key}`] !==
                undefined}
              style:left="{layout.pinned[`attr:${column.key}`]}px"
              title={attrsTooltip(ts.attributes)}
              >{#if column === table.columns[0]}<div
                  class="flex min-w-0 items-center"
                >
                  {@render colour(ts.key, headerLabel)}<button
                    type="button"
                    class="min-w-0 text-left"
                    aria-label={`Select ${headerLabel}`}
                    onclick={() => ctx.selectSeries(ts.key)}
                    >{column.cells.get(ts.key)!.label}</button
                  >
                </div>{:else}{column.cells.get(ts.key)!.label}{/if}</td
            >
          {/each}
          {#if identityColumn}<td
              class="table-cell"
              title={attrsTooltip(ts.attributes)}
            >
              <div class="flex min-w-0 items-center">
                {#if table.columns.length === 0}{@render colour(
                    ts.key,
                    headerLabel
                  )}{/if}
                <button
                  type="button"
                  class="max-w-full text-left"
                  aria-label={`Select ${headerLabel}`}
                  onclick={() => ctx.selectSeries(ts.key)}
                  >{#if label.needsIdentity}<Identifier
                      value={ts.key}
                    />{:else}{label.text}{/if}</button
                >
              </div>
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
          <td class="table-cell text-right">
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
          </td>
        </tr>
        {#if expanded}
          <tr id={detailID} data-series-attributes={ts.key}>
            <td colspan={columns.length} class="p-0">
              <table
                class="detail-fields w-full"
                aria-label={`Attributes for ${headerLabel}`}
              >
                <tbody
                  ><AttributeRows
                    {attributes}
                    owner={`timeseries ${ts.key}`}
                  /></tbody
                >
              </table>
            </td>
          </tr>
        {/if}
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
    @apply inline-flex size-4 shrink-0 cursor-pointer items-center justify-center rounded-sm text-subtle focus-visible:outline-2 focus-visible:outline-primary disabled:text-muted disabled:cursor-default;
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
</style>
