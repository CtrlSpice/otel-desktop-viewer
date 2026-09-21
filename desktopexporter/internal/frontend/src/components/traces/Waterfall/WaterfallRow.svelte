<script lang="ts">
  import type { SpanData } from '@/types/api-types'
  import type { WaterfallRowData } from './WaterfallView.svelte'
  import { formatDuration } from '@/utils/time'
  import { getServiceName } from '@/utils/resource'
  import WaterfallTreeGutter from './WaterfallTreeGutter.svelte'
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

  type Props = {
    row: WaterfallRowData
    barGridPercents: readonly number[]
    selected: boolean
    tabbable: boolean
    visible: boolean
    subtreeCollapsed: boolean
    rowIndex: number
    spanColWidth: number
    serviceColWidth: number
    timelineWidth: number
    traceStart: bigint
    traceEnd: bigint
    matched?: boolean
    onRowClick: () => void
    onToggleExpand: () => void
    onSelectTimelineRecord: (
      record: import('./timeline-markers').TimelineRecord
    ) => void
  }

  let {
    row,
    barGridPercents,
    selected,
    tabbable,
    visible,
    subtreeCollapsed,
    rowIndex,
    spanColWidth,
    serviceColWidth,
    timelineWidth,
    traceStart,
    traceEnd,
    matched = false,
    onRowClick,
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
  let serviceName = $derived(getServiceName(span.resource) ?? 'unknown')

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

<!-- The virtual list wraps this row in divs, so its production role must be explicit. -->
<!-- svelte-ignore a11y_no_redundant_roles -->
<tr
  role="row"
  class="waterfall-row"
  class:table-row--selected={selected}
  class:waterfall-row--error={row.isError}
  class:waterfall-row--matched={matched}
  data-span-id={span.spanID}
  style:visibility={visible ? 'visible' : 'collapse'}
  tabindex={tabbable && visible ? 0 : -1}
  onclick={onRowClick}
  aria-hidden={!visible ? true : undefined}
  aria-level={ariaLevel}
  aria-rowindex={rowIndex}
  aria-selected={selected}
  aria-expanded={hasChildren ? !subtreeCollapsed : undefined}
>
  <td
    role="rowheader"
    class="waterfall-row__td-name p-0 pl-2 align-middle"
    style:width="{spanColWidth}px"
  >
    <div class="flex min-w-0 items-center gap-1">
      <WaterfallTreeGutter
        depth={row.spanNode.depth}
        tree={row.tree}
        color={row.color}
        {subtreeCollapsed}
        {onToggleExpand}
      />
      <span
        class="waterfall-row__title truncate text-sm text-base-content"
        title={span.name}
      >
        {span.name}
      </span>
      {#if matched}
        <span
          class="badge badge-xs badge-soft badge-primary waterfall-row__match flex-none"
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
  <td
    role="gridcell"
    class="waterfall-row__td-service p-0 align-middle text-sm"
    title={serviceName}
    style:width="{serviceColWidth}px"
  >
    <span class="block truncate pl-2 pr-1">{serviceName}</span>
    <span class="col-resize-marker" aria-hidden="true"></span>
  </td>
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
        <div class="waterfall-row__bar-grid" aria-hidden="true">
          {#each barGridPercents as p}
            <div class="waterfall-row__grid-line" style:left="{p}%"></div>
          {/each}
        </div>
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
    @apply cursor-pointer border-none bg-transparent;
    height: var(--table-row-h);
  }

  .waterfall-row:hover {
    background-color: var(--table-hover-bg);
  }

  .waterfall-row__title {
    @apply min-w-0 flex-1;
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

  .waterfall-row__td-service {
    color: var(--color-base-content);
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
    opacity: var(--waterfall-bar-opacity, 0.7);
    transition: opacity 0.12s ease;
  }

  .waterfall-row:hover .waterfall-row__bar {
    opacity: min(1, calc(var(--waterfall-bar-opacity, 0.7) + 0.1));
  }

  .waterfall-row:global(.table-row--selected) .waterfall-row__bar {
    opacity: min(1, calc(var(--waterfall-bar-opacity, 0.7) + 0.15));
  }

  .waterfall-row__bar-grid {
    @apply pointer-events-none absolute inset-0 z-[2];
  }

  .waterfall-row__grid-line {
    @apply absolute top-0 bottom-0 w-px -translate-x-1/2 bg-base-content/10;
  }

  .waterfall-row__markers {
    @apply pointer-events-none absolute inset-0 z-20 overflow-visible;
  }

  .waterfall-row__bar-label {
    @apply absolute z-[4] text-[9px] tabular-nums whitespace-nowrap leading-none;
    top: 50%;
    transform: translateY(-50%);
    color: var(--color-base-content);
  }

  .waterfall-row--error .waterfall-row__title {
    @apply text-error;
  }

  .waterfall-row--matched {
    background-color: color-mix(
      in oklab,
      var(--color-primary) 12%,
      transparent
    );
  }

  .waterfall-row--matched:hover,
  .waterfall-row--matched:global(.table-row--selected) {
    background-color: color-mix(
      in oklab,
      var(--color-primary) 22%,
      transparent
    );
  }
</style>
