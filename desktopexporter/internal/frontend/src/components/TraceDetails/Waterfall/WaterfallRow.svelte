<script lang="ts">
  import type { SpanData } from '@/types/api-types'
  import type { WaterfallRowData } from './WaterfallView.svelte'
  import { formatDuration } from '@/utils/duration'
  import WaterfallTreeGutter from './WaterfallTreeGutter.svelte'
  import WaterfallEventDots from './WaterfallEventDots.svelte'

  function getServiceName(span: SpanData): string {
    const svc = span.resource.attributes.find(a => a.key === 'service.name')
    return svc?.value ?? 'unknown'
  }

  type Props = {
    row: WaterfallRowData
    barGridPercents: readonly number[]
    selected: boolean
    visible: boolean
    subtreeCollapsed: boolean
    onRowClick: () => void
    onToggleExpand: () => void
  }

  let {
    row,
    barGridPercents,
    selected,
    visible,
    subtreeCollapsed,
    onRowClick,
    onToggleExpand,
  }: Props = $props()

  let span = $derived(row.spanNode.spanData)
  let durationLabel = $derived(formatDuration(span.endTime - span.startTime))
  let serviceName = $derived(getServiceName(span))

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
  data-span-id={span.spanID}
  style:visibility={visible ? 'visible' : 'collapse'}
  tabindex={selected && visible ? 0 : -1}
  onclick={onRowClick}
  aria-hidden={!visible ? true : undefined}
  aria-level={ariaLevel}
  aria-selected={selected}
  aria-expanded={hasChildren ? !subtreeCollapsed : undefined}
>
  <td class="waterfall-row__td-name p-0 pl-2 align-middle">
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
    </div>
  </td>
  <td
    class="waterfall-row__td-service p-0 align-middle text-sm text-base-content/60"
    title={serviceName}
  >
    <span class="block truncate pl-2 pr-1">{serviceName}</span>
  </td>
  <td class="waterfall-row__td-bar p-0 align-middle">
    <div class="waterfall-row__bar-area">
      <div
        class="waterfall-row__bar waterfall-bar--{row.colorToken}"
        style:left="{row.offsetPercent}%"
        style:width="{row.widthPercent}%"
      >
        {#if barFitsLabel}
          <span
            class="waterfall-row__bar-label waterfall-row__bar-label--inside"
          >
            {durationLabel}
          </span>
        {/if}
      </div>
      <div class="waterfall-row__bar-grid" aria-hidden="true">
        {#each barGridPercents as p}
          <div class="waterfall-row__grid-line" style:left="{p}%"></div>
        {/each}
      </div>
      {#if row.eventMarkers.length > 0}
        <WaterfallEventDots markers={row.eventMarkers} colorToken={row.colorToken} />
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
  .waterfall-row {
    @apply cursor-pointer border-none bg-transparent transition-colors duration-100;
  }

  .waterfall-row:nth-child(even) {
    @apply bg-base-200/20;
  }

  .waterfall-row:hover {
    @apply bg-base-200/50;
    position: relative;
    z-index: 20;
  }

  .waterfall-row:focus-visible {
    @apply outline-none ring-2 ring-primary/40 ring-inset z-[1];
  }

  .waterfall-row--selected {
    @apply bg-primary/10 hover:bg-primary/15;
    box-shadow: inset 0 0 0 1px hsl(var(--p) / 0.3);
  }

  .waterfall-row__title {
    @apply min-w-0 flex-1;
  }

  .waterfall-row__bar-area {
    @apply relative flex items-center h-7;
    margin-left: 16px;
    margin-right: 24px;
  }

  .waterfall-row__bar {
    @apply absolute z-[1] h-3.5 rounded-sm top-1/2 -translate-y-1/2;
    min-width: 2px;
  }

  .waterfall-row__bar-grid {
    @apply pointer-events-none absolute inset-0 z-[2];
  }

  .waterfall-row__grid-line {
    @apply absolute top-0 bottom-0 w-px -translate-x-1/2 bg-base-content/5;
  }

  .waterfall-row__bar-label {
    @apply text-[10px] font-mono whitespace-nowrap;
  }

  .waterfall-row__bar-label--inside {
    @apply relative z-[3] flex h-full items-center px-1 text-base-100 truncate;
    line-height: 14px;
  }

  .waterfall-row__bar-label--outside {
    @apply absolute z-[3] text-base-content/60;
    line-height: 14px;
    top: 50%;
    transform: translateY(-50%);
  }

  .waterfall-bar--gold {
    background-color: var(--rp-gold);
  }
  .waterfall-bar--pine {
    background-color: var(--rp-pine);
  }
  .waterfall-bar--foam {
    background-color: var(--rp-foam);
  }
  .waterfall-bar--iris {
    background-color: var(--rp-iris);
  }
  .waterfall-bar--rose {
    background-color: var(--rp-rose);
  }
  .waterfall-bar--error {
    background-color: var(--rp-love);
  }

  .waterfall-row--error .waterfall-row__title {
    @apply text-error;
  }
</style>
