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

  /** Min bar width (% of timeline) before the duration sits inside the
   *  pill. Rounded-full caps eat ~one bar height of usable width, so this
   *  is higher than a square bar would need. */
  const MIN_LABEL_INSIDE_PCT = 24
  const LABEL_FLIP_SIDE_PCT = 50
  let barFitsLabel = $derived(row.widthPercent > MIN_LABEL_INSIDE_PCT)
  let labelOnLeft = $derived(
    !barFitsLabel && row.offsetPercent > LABEL_FLIP_SIDE_PCT
  )
  let hasChildren = $derived(row.tree.childrenCount > 0)
  let ariaLevel = $derived(row.spanNode.depth + 1)

  // Event markers arrive as %-of-trace (`marker.percent`). The visible
  // dots layer lives *inside* the bar pill so its `overflow: hidden` +
  // rounded edges clip ticks to the pill's vertical and horizontal
  // bounds. That means we need to rescale every marker from
  // %-of-timeline → %-of-bar before rendering it inside the pill. The
  // tooltip-target layer stays in %-of-timeline space because it sits
  // in a sibling layer that has to escape the bar to paint above
  // labels.
  let barEventMarkers = $derived.by(() => {
    if (row.widthPercent <= 0) return []
    return row.eventMarkers.map(m => ({
      ...m,
      percent: ((m.percent - row.offsetPercent) / row.widthPercent) * 100,
    }))
  })
</script>

<tr
  class="waterfall-row"
  class:table-row--selected={selected}
  class:waterfall-row--error={row.isError}
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
      <span class="col-resize-marker" aria-hidden="true"></span>
    </div>
  </td>
  <td
    class="waterfall-row__td-service p-0 align-middle text-sm"
    title={serviceName}
    style:width="{serviceColWidth}px"
  >
    <span class="block truncate pl-2 pr-1">{serviceName}</span>
    <span class="col-resize-marker" aria-hidden="true"></span>
  </td>
  <td class="waterfall-row__td-bar p-0 align-middle">
    <div class="waterfall-row__bar-area" style:--bar-color={row.color}>
      <div
        class="waterfall-row__bar"
        style:left="{row.offsetPercent}%"
        style:width="{row.widthPercent}%"
      >
        {#if barEventMarkers.length > 0}
          <div class="waterfall-row__event-dots">
            <WaterfallEventDots
              markers={barEventMarkers}
              color={row.color}
              layer="dots"
            />
          </div>
        {/if}
      </div>
      <div class="waterfall-row__bar-grid" aria-hidden="true">
        {#each barGridPercents as p}
          <div class="waterfall-row__grid-line" style:left="{p}%"></div>
        {/each}
      </div>
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
      {#if row.eventMarkers.length > 0}
        <div class="waterfall-row__event-tooltips">
          <WaterfallEventDots
            markers={row.eventMarkers}
            color={row.color}
            layer="tooltips"
          />
        </div>
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

  .waterfall-row__title {
    @apply min-w-0 flex-1;
  }

  .waterfall-row__td-service {
    color: var(--color-subtle);
  }

  .waterfall-row__bar-area {
    @apply relative flex items-center;
    --waterfall-bar-height: 0.875rem;
    height: var(--table-row-h);
    margin-left: 1.25rem;
    margin-right: 1.75rem;
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

  /* Visible dots are rendered inside the bar pill (which is
     `overflow: hidden` via `rounded-full` + a fixed h/w), so they're
     clipped to the bar's bounds on both axes automatically. */
  .waterfall-row__event-dots {
    @apply pointer-events-none absolute inset-0;
  }

  /* Tooltip hit targets stay in a sibling layer above the bar labels so
     the hover popup escapes the bar's clip and paints over duration text.
     Hit-area sizing (taller box, inset to clear the rounded caps) is on
     the individual targets in WaterfallEventDots; this container stays
     spanning the whole timeline so `marker.percent` math is unchanged. */
  .waterfall-row__event-tooltips {
    @apply pointer-events-none absolute inset-0 z-20 overflow-visible;
  }

  .waterfall-row__bar-label {
    @apply pointer-events-none;
  }

  .waterfall-row__bar-label--inside {
    @apply absolute z-[4] flex min-w-0 items-center justify-start overflow-hidden
           truncate px-0.5 text-[9px] tabular-nums leading-none rounded-full;
    top: 50%;
    height: var(--waterfall-bar-height);
    max-height: var(--waterfall-bar-height);
    transform: translateY(-50%);
    color: var(--waterfall-bar-label-color, var(--color-base-200));
  }

  .waterfall-row__bar-label--outside {
    @apply absolute z-[4] text-[9px] tabular-nums whitespace-nowrap leading-none;
    top: 50%;
    transform: translateY(-50%);
    color: var(--color-subtle);
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
