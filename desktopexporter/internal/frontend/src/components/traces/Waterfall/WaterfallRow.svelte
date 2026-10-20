<script lang="ts">
  import Identifier from '@/components/shared/Identifier.svelte'
  import TableValueCell from '@/components/shared/TableValueCell.svelte'
  import { columnLabel, type WaterfallColumn } from './waterfall-columns'
  import type { SpanData } from '@/types/api-types'
  import type { WaterfallRowData } from './WaterfallView.svelte'
  import { formatDuration } from '@/utils/time'
  import TreeGutter from '@/components/shared/TreeGutter.svelte'
  import TreeChildCount from '@/components/shared/TreeChildCount.svelte'
  import TreeToggle from '@/components/shared/TreeToggle.svelte'
  import type { WaterfallGridLayout } from './WaterfallTimeAxisHeader.svelte'
  import WaterfallGrid from './WaterfallGrid.svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { BiohazardIcon } from '@hugeicons/core-free-icons'
  import TimelineMarkers from './TimelineMarkers.svelte'
  import {
    BAR_LABEL_GAP_REM,
    DURATION_GUTTER_REM,
    MARKER_CLUSTER_DISTANCE_REM,
    OUTSIDE_RECORD_SLOT_REM,
    TIMELINE_LEFT_INSET_REM,
    WATERFALL_BAR_HEIGHT_REM,
    clusterTimelineRecords,
  } from './timeline-markers'
  import { remToPx } from '@/state/panel-width'
  import { columnCell, type SizedWaterfallColumn } from './waterfall-columns'

  type Props = {
    row: WaterfallRowData
    barGrid: WaterfallGridLayout
    selected: boolean
    tabbable: boolean
    visible: boolean
    subtreeCollapsed: boolean
    rowIndex: number
    spanColWidth: number
    optionalColumns?: SizedWaterfallColumn[]
    timelineWidth: number
    traceStart: bigint
    traceEnd: bigint
    matched?: boolean
    onRowClick: () => void
    onInspectColumn?: (column: WaterfallColumn) => void
    onToggleExpand: () => void
    onSelectTimelineRecord: (
      record: import('./timeline-markers').TimelineRecord
    ) => void
  }

  let {
    row,
    barGrid,
    selected,
    tabbable,
    visible,
    subtreeCollapsed,
    rowIndex,
    spanColWidth,
    optionalColumns = [],
    timelineWidth,
    traceStart,
    traceEnd,
    matched = false,
    onRowClick,
    onInspectColumn,
    onToggleExpand,
    onSelectTimelineRecord,
  }: Props = $props()

  let span = $derived(row.spanNode.spanData)

  /**
   * Two different messages, because the spans are in two different positions.
   * Every span in a stranded subtree is *affected*: it was recovered by the
   * cycle-aware walk and would otherwise be missing entirely. Exactly one is
   * the retained cycle point: its reported parent is at or below it in the
   * recovered branch, but that does not identify which parent link is wrong.
   */
  let cycleLabel = $derived(
    row.spanNode.salvaged
      ? row.spanNode.cyclePoint
        ? "Cycle detected: this span's reported parent is at or below it in " +
          'the recovered branch, so following parent IDs would loop instead ' +
          'of reaching the trace root. Check the parent assignments in the ' +
          'emitting service.'
        : 'Recovered from a broken part of this trace: a parent link forms a ' +
          'loop, so these spans have no place under the root.'
      : ''
  )
  let durationLabel = $derived(formatDuration(span.endTime - span.startTime))

  let hasChildren = $derived(row.tree.childrenCount > 0)
  let ariaLevel = $derived(row.spanNode.depth + 1)

  let markers = $derived(
    clusterTimelineRecords(
      row.records,
      traceStart,
      traceEnd,
      timelineWidth,
      remToPx(MARKER_CLUSTER_DISTANCE_REM)
    )
  )
</script>

{#snippet treeContent()}
  <TreeGutter
    depth={row.spanNode.depth}
    tree={row.tree}
    color={row.color}
    {subtreeCollapsed}
  />
  {#if hasChildren}
    <TreeChildCount
      class="waterfall-row__child-count"
      count={row.tree.childrenCount}
      title="{row.tree.childrenCount} child {row.tree.childrenCount === 1
        ? 'span'
        : 'spans'}"
    />
  {/if}
{/snippet}

<!-- The virtual list wraps this row in divs, so its production role must be explicit. -->
<!-- svelte-ignore a11y_no_redundant_roles -->
<tr
  role="row"
  class="waterfall-row item-state"
  class:table-row--selected={selected}
  class:waterfall-row--matched={matched}
  data-span-id={span.spanID}
  style:visibility={visible ? 'visible' : 'collapse'}
  tabindex={tabbable && visible ? 0 : -1}
  onclick={() => {
    if (window.getSelection()?.isCollapsed !== false) onRowClick()
  }}
  aria-hidden={!visible ? true : undefined}
  aria-level={ariaLevel}
  aria-rowindex={rowIndex}
  aria-selected={selected}
  aria-expanded={hasChildren ? !subtreeCollapsed : undefined}
>
  <td
    role="rowheader"
    class="waterfall-row__td-name px-2 py-0 align-middle"
    style:width="{spanColWidth}px"
  >
    <div class="flex min-w-0 items-center gap-1">
      {#if hasChildren}
        <TreeToggle
          class="waterfall-row__tree-toggle"
          label="{subtreeCollapsed ? 'Expand' : 'Collapse'} {span.name}, {row
            .tree.childrenCount} child {row.tree.childrenCount === 1
            ? 'span'
            : 'spans'}"
          expanded={!subtreeCollapsed}
          onclick={onToggleExpand}
        >
          {@render treeContent()}
        </TreeToggle>
      {:else}
        {@render treeContent()}
      {/if}
      <div
        class="waterfall-row__title flex-1 text-left text-sm text-base-content"
        title={span.name}
      >
        <TableValueCell
          label="span field: name"
          text={span.name}
          inset={false}
          onInspect={() =>
            onInspectColumn
              ? onInspectColumn({ source: 'span', key: 'name', field: true })
              : onRowClick()}
        />
      </div>
      {#if matched}
        <span
          class="badge badge-xs badge-primary waterfall-row__match flex-none"
          >Match</span
        >
      {/if}
      {#if cycleLabel}
        <span
          class="waterfall-row__cycle"
          class:waterfall-row__cycle--cycle-point={row.spanNode.cyclePoint}
          title={cycleLabel}
          aria-label={cycleLabel}
          role="img"
        >
          {#if row.spanNode.cyclePoint}
            <HugeiconsIcon
              icon={BiohazardIcon}
              size="1em"
              strokeWidth={1.5}
              aria-hidden="true"
            />
          {:else}
            <span aria-hidden="true">{'\u26A0'}</span>
          {/if}
        </span>
      {/if}
      <span class="col-resize-marker" aria-hidden="true"></span>
    </div>
  </td>
  {#each optionalColumns as column (column.id)}
    {@const cell = columnCell(span, column)}
    <td
      role="gridcell"
      class="p-0 align-middle text-sm"
      style:width="{column.width}px"
      data-column-id={column.id}
      title={cell.title}
    >
      {#if cell.missing}
        <span class="waterfall-row__missing block truncate text-base-content/50"
          >{cell.text}</span
        >
      {:else}
        <TableValueCell
          label={columnLabel(column)}
          text={cell.text}
          numeric={cell.numeric}
          onInspect={() =>
            onInspectColumn ? onInspectColumn(column) : onRowClick()}
        >
          {#if column.field && column.source === 'span' && ['traceId', 'spanId', 'parentSpanId'].includes(column.key)}<Identifier
              value={cell.text}
            />{:else}{cell.text}{/if}
        </TableValueCell>
      {/if}
      <span class="col-resize-marker" aria-hidden="true"></span>
    </td>
  {/each}
  <td role="gridcell" class="waterfall-row__td-bar p-0 align-middle">
    <div
      class="waterfall-row__bar-area"
      style:--bar-color={row.color}
      style:--timeline-left-inset="{TIMELINE_LEFT_INSET_REM}rem"
      style:--duration-gutter="{DURATION_GUTTER_REM}rem"
      style:--outside-record-slot="{OUTSIDE_RECORD_SLOT_REM}rem"
      style:--bar-label-gap="{BAR_LABEL_GAP_REM}rem"
      style:--waterfall-bar-height="{WATERFALL_BAR_HEIGHT_REM}rem"
    >
      <div class="waterfall-row__timeline-frame">
        <div
          class="waterfall-row__bar"
          style:left="{row.offsetPercent}%"
          style:width="{row.widthPercent}%"
        ></div>
        <WaterfallGrid layout={barGrid} />
        <span
          class="waterfall-row__bar-label"
          style:left="calc({row.offsetPercent + row.widthPercent}% +
          var(--bar-label-gap))"
        >
          {durationLabel}
        </span>
        {#if markers.length > 0}
          <div class="waterfall-row__markers">
            <TimelineMarkers
              {markers}
              spanColor={row.color}
              spanStartTime={span.startTime}
              onSelectRecord={onSelectTimelineRecord}
            />
          </div>
        {/if}
      </div>
    </div>
  </td>
</tr>

<style lang="postcss">
  @reference "../../../app.css";
  .waterfall-row {
    @apply cursor-pointer border-none;
    height: var(--table-row-h);
  }

  .waterfall-row__title {
    @apply min-w-0 flex-1;
  }
  .waterfall-row__missing {
    padding-inline: var(--table-cell-inline-padding, 0.5rem);
  }

  /* Marks a span the cycle-aware walk recovered. flex-none so it survives the
     title's truncation rather than being squeezed out of a narrow column --
     the badge is the reason the row is worth reading. One glyph, two colors:
     every stranded span warns in the theme's warning gold, and only the
     retained cycle point escalates to the error red. */
  .waterfall-row__cycle {
    @apply flex-none text-warning text-xs leading-none;
  }

  .waterfall-row__cycle--cycle-point {
    @apply text-error text-sm font-bold;
  }

  .waterfall-row__bar-area {
    @apply relative flex items-center;
    height: var(--table-row-h);
  }

  .waterfall-row__timeline-frame {
    @apply absolute top-0 bottom-0;
    left: var(--timeline-left-inset);
    right: calc(var(--duration-gutter) + var(--outside-record-slot));
  }

  /* `--bar-color` on the bar area tints the span pill. */
  .waterfall-row__bar {
    @apply absolute z-[1] rounded-full top-1/2 -translate-y-1/2 border-0 overflow-hidden;
    height: var(--waterfall-bar-height);
    min-width: 2px;
    background-color: var(--bar-color);
    transition: background-color 0.12s ease;
  }

  .waterfall-row__markers {
    @apply pointer-events-none absolute inset-0 z-20 overflow-visible;
  }

  .waterfall-row__bar-label {
    @apply absolute z-[4] text-base tabular-nums whitespace-nowrap leading-none;
    top: 50%;
    transform: translateY(-50%);
    color: var(--color-base-content);
  }

  .waterfall-row--matched {
    --item-idle-bg: var(--color-highlight-high);
  }
</style>
