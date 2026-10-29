<script lang="ts">
  import { untrack } from 'svelte'
  import { SvelteMap, SvelteSet } from 'svelte/reactivity'
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
  import {
    compareSeriesCells,
    type SeriesCell,
  } from '@/components/metrics/utils/minimal-series-labels'
  import Sparkline from '@/components/metrics/Charts/Sparkline.svelte'
  import { attributeValueLabel } from '@/components/shared/attribute-label'
  import { telemetryAPI } from '@/services/telemetry-service'
  import type { Attribute } from '@/types/api-types'

  const ctx = getMetricViewContext()
  const metricRef = $derived(ctx.metric?.metricRef)
  const id = $props.id()
  const expandedAttributes = new SvelteSet<string>()
  const receivedAttributes = new SvelteMap<string, Attribute[]>()
  const attributeErrors = new SvelteMap<string, string>()
  const loadingAttributes = new SvelteSet<string>()
  const attributeRequests = new Map<string, AbortController>()
  let hoveredSeriesKey = $state<string | null>(null)
  let focusedSeriesKey = $state<string | null>(null)
  let sort = $state<{ key: string; direction: 'asc' | 'desc' } | null>(null)

  $effect.pre(() => {
    void metricRef
    hoveredSeriesKey = null
    focusedSeriesKey = null
    sort = null
    expandedAttributes.clear()
    receivedAttributes.clear()
    attributeErrors.clear()
    loadingAttributes.clear()
  })
  // A refreshed response can have the same metric/series IDs and new context.
  // Keep disclosures, but replace exact values and reject every older request.
  $effect.pre(() => {
    void ctx.metric
    receivedAttributes.clear()
    attributeErrors.clear()
    loadingAttributes.clear()
    return () => {
      for (const controller of attributeRequests.values()) controller.abort()
      attributeRequests.clear()
    }
  })
  $effect(() => {
    void ctx.metric
    const expanded = [...expandedAttributes]
    untrack(() => {
      for (const key of expanded) void loadAttributes(key)
    })
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
    if (expandedAttributes.has(key)) {
      expandedAttributes.delete(key)
      attributeRequests.get(key)?.abort()
      attributeRequests.delete(key)
      loadingAttributes.delete(key)
    } else {
      expandedAttributes.add(key)
    }
  }
  async function loadAttributes(key: string) {
    const series = timeseriesByKey.get(key)
    const owner = metricRef
    if (
      !owner ||
      !series ||
      series.attributesComplete !== false ||
      receivedAttributes.has(key) ||
      loadingAttributes.has(key)
    )
      return
    loadingAttributes.add(key)
    const controller = new AbortController()
    attributeRequests.set(key, controller)
    attributeErrors.delete(key)
    try {
      const received = await telemetryAPI.getMetricSeries(
        owner,
        key,
        null,
        null,
        false,
        controller.signal
      )
      if (controller.signal.aborted) return
      if (!received) throw new Error('Series is no longer available')
      receivedAttributes.set(key, received.attributes)
    } catch (error) {
      if (!controller.signal.aborted)
        attributeErrors.set(
          key,
          error instanceof Error
            ? error.message
            : 'Failed to load series attributes'
        )
    } finally {
      if (attributeRequests.get(key) === controller) {
        attributeRequests.delete(key)
        loadingAttributes.delete(key)
      }
    }
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
    ...(!ctx.isHistogramKind
      ? [{ id: 'colour', label: 'Colour', min: 32, width: 32, fixed: true }]
      : []),
    ...table.columns.map(column => ({
      id: `attr:${column.key}`,
      label: column.key,
      min: [...column.cells.values()].some(
        cell => identifierValue(column.key, cell) !== undefined
      )
        ? 160
        : 112,
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
      attrs
        .map(
          attribute =>
            `${attribute.key}: ${attributeValueLabel(attribute.value)}`
        )
        .join(' ') || 'default series'
    )
  }
  function identifierValue(key: string, cell: SeriesCell): string | undefined {
    if (!/(?:^|[._])(?:id|uuid)$/i.test(key)) return undefined
    const values = cell.values
    const value = values[0]
    return values.length === 1 && value?.kind === 'string'
      ? value.value
      : undefined
  }
</script>

{#snippet cellContent(key: string, cell: SeriesCell)}
  {@const identifier = identifierValue(key, cell)}
  <span class="block max-w-full truncate" title={cell.preview ?? cell.label}>
    {#if identifier !== undefined}<Identifier
        value={identifier}
      />{:else}{cell.preview ?? cell.label}{/if}
  </span>
{/snippet}

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
    pinnedCount={ctx.isHistogramKind ? 2 : 3}
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
        {#if !ctx.isHistogramKind}<th
            scope="col"
            class="table-cell ts-row__colour"
            class:table-cell--pinned={layout.pinned.colour !== undefined}
            style:left="{layout.pinned.colour}px"
            ><span class="sr-only">Colour</span></th
          >{/if}
        {#each table.columns as column (column.key)}
          <th
            scope="col"
            class="table-cell"
            class:ts-row__first-value={!ctx.isHistogramKind &&
              column === table.columns[0]}
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
        {#if identityColumn}<th
            scope="col"
            class:ts-row__first-value={!ctx.isHistogramKind &&
              table.columns.length === 0}>Series identity</th
          >{/if}
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
        {@const series = timeseriesByKey.get(ts.key)}
        {@const attributes =
          receivedAttributes.get(ts.key) ?? series?.attributes ?? []}
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
              {#if attributes.length || series?.attributesComplete === false}
                <button
                  type="button"
                  class="ts-row__caret"
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
              {/if}
            </div>
          </td>
          {#if !ctx.isHistogramKind}<td
              class="table-cell ts-row__colour"
              class:table-cell--pinned={layout.pinned.colour !== undefined}
              style:left="{layout.pinned.colour}px"
              >{@render colour(ts.key, headerLabel)}</td
            >{/if}
          {#each table.columns as column (column.key)}
            <td
              class="table-cell ts-row__attrs"
              class:ts-row__first-value={!ctx.isHistogramKind &&
                column === table.columns[0]}
              class:table-cell--pinned={layout.pinned[`attr:${column.key}`] !==
                undefined}
              style:left="{layout.pinned[`attr:${column.key}`]}px"
              title={series?.labelPreviews !== undefined
                ? headerLabel
                : attrsTooltip(ts.attributes)}
              >{#if column === table.columns[0]}<button
                  type="button"
                  class="block max-w-full text-left"
                  aria-label={`Select ${headerLabel}`}
                  onclick={() => ctx.selectSeries(ts.key)}
                  >{@render cellContent(
                    column.key,
                    column.cells.get(ts.key)!
                  )}</button
                >
              {:else}{@render cellContent(
                  column.key,
                  column.cells.get(ts.key)!
                )}{/if}</td
            >
          {/each}
          {#if identityColumn}<td
              class="table-cell"
              class:ts-row__first-value={!ctx.isHistogramKind &&
                table.columns.length === 0}
              title={series?.labelPreviews !== undefined
                ? headerLabel
                : attrsTooltip(ts.attributes)}
            >
              <button
                type="button"
                class="max-w-full text-left"
                aria-label={`Select ${headerLabel}`}
                onclick={() => ctx.selectSeries(ts.key)}
                >{#if label.needsIdentity}<Identifier
                    value={ts.key}
                  />{:else}{label.text}{/if}</button
              >
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
              {#if loadingAttributes.has(ts.key)}
                <p role="status" class="px-3 py-2 text-subtle">
                  Loading series attributes…
                </p>
              {:else if attributeErrors.has(ts.key)}
                <p role="alert" class="px-3 py-2 text-papaya">
                  {attributeErrors.get(ts.key)}
                </p>
                <button
                  type="button"
                  class="btn btn-ghost mx-3"
                  onclick={() => loadAttributes(ts.key)}
                  >Retry attributes</button
                >
              {:else}
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
              {/if}
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
    @apply inline-flex size-4 shrink-0 cursor-pointer items-center justify-center rounded-sm text-subtle focus-visible:outline-2 focus-visible:outline-primary;
  }
  .ts-row__attrs {
    @apply text-sm;
  }
  .ts-row__colour {
    padding-inline: 0;
  }
  .ts-row__first-value {
    padding-inline-start: 0;
  }
  .ts-row__sparkline {
    @apply block w-24;
  }
  .ts-row__sparkline :global(.sparkline) {
    @apply block h-[18px] w-full;
  }
</style>
