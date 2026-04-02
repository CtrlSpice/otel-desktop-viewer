<script lang="ts">
  import type { SpanData } from '@/types/api-types'
  import type { WaterfallRowData } from './WaterfallView.svelte'
  import { formatDuration } from '@/utils/duration'
  import WaterfallTreeGutter from './WaterfallTreeGutter.svelte'

  function getServiceName(span: SpanData): string {
    const svc = span.resource.attributes.find(a => a.key === 'service.name')
    return svc?.value ?? 'unknown'
  }

  type Props = {
    row: WaterfallRowData
    selected: boolean
    visible: boolean
    subtreeCollapsed: boolean
    onRowClick: () => void
    onToggleExpand: () => void
  }

  let {
    row,
    selected,
    visible,
    subtreeCollapsed,
    onRowClick,
    onToggleExpand,
  }: Props = $props()

  let span = $derived(row.spanNode.spanData)
  let durationLabel = $derived(formatDuration(span.endTime - span.startTime))
  let serviceName = $derived(getServiceName(span))
  let barFitsLabel = $derived(row.widthPercent > 8)
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
  <td
    class="waterfall-row__td-name p-0 align-middle"
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
        class="waterfall-row__title truncate text-xs text-base-content"
        title={span.name}
      >
        {span.name}
      </span>
    </div>
  </td>
  <td
    class="waterfall-row__td-service p-0 align-middle text-xs text-base-content/60"
    title={serviceName}
  >
    <span class="block truncate pl-2 pr-1">{serviceName}</span>
  </td>
  <td class="waterfall-row__td-bar p-0 px-4 align-middle">
    <div class="waterfall-row__bar-area relative h-7 w-full">
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
      {#if !barFitsLabel}
        <span
          class="waterfall-row__bar-label waterfall-row__bar-label--outside"
          style:left="{row.offsetPercent + row.widthPercent + 0.5}%"
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
    @apply flex items-center;
  }

  .waterfall-row__bar {
    @apply absolute h-3.5 rounded-sm;
    min-width: 2px;
  }

  .waterfall-row__bar-label {
    @apply text-[10px] font-mono whitespace-nowrap;
  }

  .waterfall-row__bar-label--inside {
    @apply px-1 truncate text-base-100 block;
    line-height: 14px;
  }

  .waterfall-row__bar-label--outside {
    @apply absolute text-base-content/60;
    line-height: 14px;
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
