<script lang="ts">
  import type { SpanData } from '@/types/api-types'
  import type { WaterfallRowData } from './WaterfallView.svelte'
  import { formatDuration } from '@/utils/time'
  import { getServiceName } from '@/utils/resource'
  import WaterfallTreeGutter from './WaterfallTreeGutter.svelte'
  import WaterfallEventDots from './WaterfallEventDots.svelte'

  type Props = {
    row: WaterfallRowData
    barGridPercents: readonly number[]
    selected: boolean
    visible: boolean
    subtreeCollapsed: boolean
    spanColWidth: number
    serviceColWidth: number
    matched?: boolean
    onRowClick: () => void
    onToggleExpand: () => void
  }

  let {
    row,
    barGridPercents,
    selected,
    visible,
    subtreeCollapsed,
    spanColWidth,
    serviceColWidth,
    matched = false,
    onRowClick,
    onToggleExpand,
  }: Props = $props()

  let span = $derived(row.spanNode.spanData)
  let durationLabel = $derived(formatDuration(span.endTime - span.startTime))
  let serviceName = $derived(getServiceName(span.resource) ?? 'unknown')

  const MIN_LABEL_INSIDE_PCT = 12
  const LABEL_FLIP_SIDE_PCT = 50
  let barFitsLabel = $derived(row.widthPercent > MIN_LABEL_INSIDE_PCT)
  let labelOnLeft = $derived(
    !barFitsLabel && row.offsetPercent > LABEL_FLIP_SIDE_PCT
  )
  let hasChildren = $derived(row.tree.childrenCount > 0)
  let ariaLevel = $derived(row.spanNode.depth + 1)
</script>

<tr
  class="waterfall-row"
  class:waterfall-row--selected={selected}
  class:waterfall-row--error={row.colorToken === 'error'}
  class:waterfall-row--matched={matched}
  data-span-id={span.spanID}
  style:visibility={visible ? 'visible' : 'collapse'}
  tabindex={selected && visible ? 0 : -1}
  onclick={onRowClick}
  aria-hidden={!visible ? true : undefined}
  aria-level={ariaLevel}
  aria-selected={selected}
  aria-expanded={hasChildren ? !subtreeCollapsed : undefined}
>
  <td
    class="waterfall-row__td-name p-0 pl-2 align-middle"
    style:width="{spanColWidth}px"
  >
    <div class="flex min-w-0 items-center gap-1">
      <WaterfallTreeGutter
        depth={row.spanNode.depth}
        tree={row.tree}
        colorToken={row.colorToken}
        {subtreeCollapsed}
        {onToggleExpand}
      />
      <span
        class="waterfall-row__title truncate text-sm text-base-content"
        title={span.name}
      >
        {span.name}
      </span>
      <span class="col-resize-marker" aria-hidden="true"></span>
    </div>
  </td>
  <td
    class="waterfall-row__td-service p-0 align-middle text-sm text-base-content/60"
    title={serviceName}
    style:width="{serviceColWidth}px"
  >
    <span class="block truncate pl-2 pr-1">{serviceName}</span>
    <span class="col-resize-marker" aria-hidden="true"></span>
  </td>
  <td class="waterfall-row__td-bar p-0 align-middle">
    <div class="waterfall-row__bar-area">
      <div
        class="waterfall-row__bar waterfall-bar--{row.colorToken}"
        style:left="{row.offsetPercent}%"
        style:width="{row.widthPercent}%"
      ></div>
      <div class="waterfall-row__bar-grid" aria-hidden="true">
        {#each barGridPercents as p}
          <div class="waterfall-row__grid-line" style:left="{p}%"></div>
        {/each}
      </div>
      {#if row.eventMarkers.length > 0}
        <div class="waterfall-row__event-markers">
          <WaterfallEventDots
            markers={row.eventMarkers}
            colorToken={row.colorToken}
          />
        </div>
      {/if}
      {#if barFitsLabel}
        <span
          class="waterfall-row__bar-label waterfall-row__bar-label--inside"
          style:left="{row.offsetPercent}%"
          style:width="{row.widthPercent}%"
        >
          {durationLabel}
        </span>
      {/if}
      {#if !barFitsLabel}
        <span
          class="waterfall-row__bar-label waterfall-row__bar-label--outside"
          class:waterfall-row__bar-label--left={labelOnLeft}
          style:left={labelOnLeft
            ? undefined
            : `${row.offsetPercent + row.widthPercent + 0.5}%`}
          style:right={labelOnLeft
            ? `${100 - row.offsetPercent + 0.5}%`
            : undefined}
        >
          {durationLabel}
        </span>
      {/if}
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

  .waterfall-row--selected {
    outline: 1px solid color-mix(in oklab, var(--color-primary) 50%, transparent);
    outline-offset: -1px;
    position: relative;
    z-index: 1;
  }

  .waterfall-row__title {
    @apply min-w-0 flex-1;
  }

  .waterfall-row__bar-area {
    @apply relative flex items-center;
    height: var(--table-row-h);
    margin-left: 1.25rem;
    margin-right: 1.75rem;
  }

  .waterfall-row__bar {
    @apply absolute z-[1] h-3.5 rounded-sm top-1/2 -translate-y-1/2;
    min-width: 2px;
  }

  .waterfall-row__bar-grid {
    @apply pointer-events-none absolute inset-0 z-[2];
  }

  .waterfall-row__grid-line {
    @apply absolute top-0 bottom-0 w-px -translate-x-1/2 bg-base-content/10;
  }

  /* z-[3] layer: event dots (above grid, below labels). */
  .waterfall-row__event-markers {
    @apply pointer-events-none absolute inset-0 z-[3];
  }

  .waterfall-row__bar-label {
    @apply text-[10px] font-mono whitespace-nowrap;
  }

  .waterfall-row__bar-label--inside {
    @apply absolute z-[4] flex h-3.5 min-w-[2px] items-center justify-start overflow-hidden rounded-sm px-1 text-base-100 truncate;
    top: 50%;
    transform: translateY(-50%);
    line-height: 14px;
    pointer-events: none;
  }

  .waterfall-row__bar-label--outside {
    @apply absolute z-[4] text-base-content/60;
    line-height: 14px;
    top: 50%;
    transform: translateY(-50%);
  }

  .waterfall-bar--gold {
    background-color: var(--color-warning);
  }
  .waterfall-bar--pine {
    background-color: var(--color-secondary);
  }
  .waterfall-bar--foam {
    background-color: var(--color-accent);
  }
  .waterfall-bar--iris {
    background-color: var(--color-primary);
  }
  .waterfall-bar--rose {
    background-color: var(--color-rose);
  }
  .waterfall-bar--error {
    background-color: var(--color-error);
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
  .waterfall-row--matched.waterfall-row--selected {
    background-color: color-mix(
      in oklab,
      var(--color-primary) 22%,
      transparent
    );
  }
</style>
