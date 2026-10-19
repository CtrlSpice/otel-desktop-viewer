<script lang="ts">
  import type { Snippet } from 'svelte'
  import {
    createPopoverID,
    setupAnchorPopover,
  } from '@/components/shared/utils/anchor-popover'

  let {
    label,
    count = 0,
    custom = false,
    compact = false,
    icon,
    onClear,
    children,
  }: {
    label: string
    count?: number
    custom?: boolean
    compact?: boolean
    icon?: Snippet
    onClear: () => void
    children: Snippet
  } = $props()

  const id = createPopoverID('toolbar-filter')
  let trigger = $state<HTMLButtonElement>()
  let popover = $state<HTMLDivElement>()
  let open = $state(false)
  $effect(() => {
    if (!trigger || !popover) return
    return setupAnchorPopover({
      trigger,
      popover,
      anchor: 'below-end',
      onOpenChange: value => (open = value),
    })
  })
</script>

<button
  bind:this={trigger}
  type="button"
  class={compact
    ? 'btn btn-xs chart-header-control tooltip tooltip-left'
    : 'btn btn-ghost btn-sm rounded-full text-sm font-normal'}
  class:text-primary={count > 0 || custom}
  aria-label={compact ? label : `${label} filter`}
  data-tip={compact ? label : undefined}
  aria-expanded={open}
  aria-controls={id}
  popovertarget={id}
>
  {#if compact}{@render icon?.()}{:else}{label}{/if}{#if count > 0}<span
      class={compact ? 'sr-only' : 'tabular-nums'}>({count})</span
    >{:else if custom}<span class="sr-only">: custom query</span>{/if}
  {#if !compact}<svg
      class="popover-indicator size-3"
      class:popover-indicator--open={open}
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="1.5"
      aria-hidden="true"><path d="m6 9 6 6 6-6" /></svg
    >{/if}
</button>
<div
  bind:this={popover}
  {id}
  popover="auto"
  role="group"
  aria-label={`${label} options`}
  class="anchor-popover anchor-popover--anchored filter-popover"
>
  <div class="filter-popover__header">
    <span class="font-semibold">{label}</span>
    {#if count > 0}<button
        type="button"
        class="btn btn-ghost btn-sm text-sm"
        aria-label={`Clear ${label.toLowerCase()} filter`}
        onclick={onClear}>Clear</button
      >{/if}
  </div>
  {@render children()}
</div>

<style lang="postcss">
  @reference "../../../app.css";
  .filter-popover {
    @apply w-80 max-w-[calc(100vw-1rem)] overflow-y-auto p-3 text-sm;
  }
  .filter-popover__header {
    @apply mb-2 flex min-h-8 items-center justify-between gap-2;
  }
</style>
