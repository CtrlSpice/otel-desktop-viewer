<script lang="ts">
  import TreeRowIdentity from '@/components/shared/TreeRowIdentity.svelte'
  import SpanBar from '@/components/shared/SpanBar.svelte'
  import WaterfallTimeGuides from './WaterfallTimeGuides.svelte'
  import type { WaterfallAxis } from './WaterfallTimeAxisHeader.svelte'
  import type { WaterfallRowData } from './waterfall-layout'
  import { columnCell, type SizedWaterfallColumn } from './waterfall-columns'
  import { formatDuration } from '@/utils/time'
  import {
    BAR_LABEL_GAP_REM,
    DURATION_GUTTER_REM,
    OUTSIDE_RECORD_SLOT_REM,
    TIMELINE_LEFT_INSET_REM,
    WATERFALL_BAR_HEIGHT_REM,
  } from './timeline-markers'

  let {
    row,
    selected,
    spanColWidth,
    optionalColumns,
    timeAxis,
    offset,
    onSelect,
  }: {
    row: WaterfallRowData
    selected: boolean
    spanColWidth: number
    optionalColumns: SizedWaterfallColumn[]
    timeAxis: WaterfallAxis
    offset: number
    onSelect: () => void
  } = $props()
  const span = $derived(row.spanNode.spanData)
  const duration = $derived(
    span.endTime < span.startTime
      ? 'Invalid timing'
      : formatDuration(span.endTime - span.startTime)
  )
  const columns = $derived(
    [spanColWidth, ...optionalColumns.map(column => column.width)]
      .map(width => `${width}px`)
      .join(' ') + ' minmax(0, 1fr)'
  )
</script>

<button
  type="button"
  class="sticky-ancestor-row item-state"
  class:table-row--selected={selected}
  aria-label="Go to ancestor {span.name}, level {row.spanNode.depth + 1}"
  aria-current={selected ? 'true' : undefined}
  title="Go to {span.name} · {span.spanID}"
  data-sticky-span={span.spanID}
  style:grid-template-columns={columns}
  style:transform="translateY({offset}px)"
  onclick={onSelect}
>
  <span class="sticky-ancestor-row__name" aria-hidden="true">
    <TreeRowIdentity
      name={span.name}
      depth={row.spanNode.depth}
      tree={row.tree}
      color={row.color}
    >
      <span class="block truncate">{span.name}</span>
    </TreeRowIdentity>
  </span>
  {#each optionalColumns as column (column.id)}
    {@const cell = columnCell(span, column)}
    <span class="sticky-ancestor-row__attribute" aria-hidden="true">
      {cell.text}
    </span>
  {/each}
  <span
    class="sticky-ancestor-row__bar-area"
    aria-hidden="true"
    style:--bar-color={row.color}
    style:--waterfall-bar-height="{WATERFALL_BAR_HEIGHT_REM}rem"
  >
    <span
      class="sticky-ancestor-row__timeline"
      style:left="{TIMELINE_LEFT_INSET_REM}rem"
      style:right="{DURATION_GUTTER_REM + OUTSIDE_RECORD_SLOT_REM}rem"
    >
      <WaterfallTimeGuides axis={timeAxis} />
      <SpanBar
        offsetPercent={row.offsetPercent}
        widthPercent={row.widthPercent}
      />
      <span
        class="sticky-ancestor-row__duration"
        style:left="calc({row.offsetPercent + row.widthPercent}% + {BAR_LABEL_GAP_REM}rem)"
      >
        {duration}
      </span>
    </span>
  </span>
</button>

<style lang="postcss">
  @reference "../../../app.css";
  .sticky-ancestor-row {
    @apply relative grid w-full cursor-pointer items-center border-0 p-0 text-left text-sm font-normal text-base-content;
    height: var(--sticky-row-height, var(--table-row-h));
    --item-idle-bg: var(--color-base-100);
    border-radius: 0;
    box-shadow: inset 0 -1px 0 var(--color-muted);
    pointer-events: auto;
    outline-offset: -2px;
  }
  .sticky-ancestor-row__name {
    @apply min-w-0 overflow-hidden px-2;
  }
  .sticky-ancestor-row__attribute {
    @apply block min-w-0 truncate;
    padding-inline: var(--table-cell-inline-padding);
  }
  .sticky-ancestor-row__bar-area {
    @apply relative h-full min-w-0;
  }
  .sticky-ancestor-row__timeline {
    @apply absolute top-0 bottom-0;
  }
  .sticky-ancestor-row__duration {
    @apply absolute whitespace-nowrap text-base leading-none tabular-nums;
    top: 50%;
    transform: translateY(-50%);
  }
</style>
