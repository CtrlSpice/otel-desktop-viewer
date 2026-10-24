<script lang="ts">
  import type { Snippet } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { Cancel01Icon } from '@hugeicons/core-free-icons'
  import {
    createPopoverID,
    setupAnchorPopover,
    type PopoverAnchor,
  } from '@/components/shared/utils/anchor-popover'

  let {
    label,
    tooltip,
    anchor = 'below-end',
    role = 'group',
    count = 0,
    custom = false,
    compact = false,
    triggerClass,
    popoverClass = '',
    focusSelected = false,
    showHeader = true,
    showCount = false,
    icon,
    onClear,
    clearable = !!onClear && count > 0,
    onOpenChange,
    footer,
    children,
  }: {
    label: string
    tooltip?: string
    anchor?: PopoverAnchor
    role?: 'group' | 'dialog'
    count?: number
    custom?: boolean
    compact?: boolean
    triggerClass?: string
    popoverClass?: string
    focusSelected?: boolean
    showHeader?: boolean
    showCount?: boolean
    icon?: Snippet
    onClear?: () => void
    clearable?: boolean
    onOpenChange?: (open: boolean) => void
    footer?: Snippet<[close: () => void]>
    children: Snippet<[close: () => void]>
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
      anchor,
      onOpenChange: value => {
        open = value
        if (value && focusSelected) {
          popover
            ?.querySelector<HTMLButtonElement>('[aria-pressed="true"]')
            ?.focus({ preventScroll: true })
        }
        onOpenChange?.(value)
      },
    })
  })

  function close() {
    popover?.hidePopover()
    trigger?.focus({ preventScroll: true })
  }
</script>

<button
  bind:this={trigger}
  type="button"
  class={triggerClass ??
    (compact
      ? 'btn btn-xs chart-header-control tooltip tooltip-left'
      : 'btn btn-ghost btn-sm rounded-full text-sm font-normal')}
  class:text-primary={count > 0 || custom}
  aria-label={compact ? label : `${label} filter`}
  data-tip={compact ? (tooltip ?? label) : undefined}
  aria-expanded={open}
  aria-haspopup={role === 'dialog' ? 'dialog' : undefined}
  aria-controls={id}
  popovertarget={id}
>
  {#if compact}{@render icon?.()}{:else}{label}{/if}{#if count > 0}<span
      class={compact && !showCount
        ? 'sr-only'
        : showCount
          ? 'tabular-nums text-subtle'
          : 'tabular-nums'}>{showCount ? `[${count}]` : `(${count})`}</span
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
  {role}
  aria-label={`${label} options`}
  class="anchor-popover anchor-popover--anchored anchor-popover--form filter-popover {popoverClass}"
>
  {#if showHeader || clearable}<div class="filter-popover__header">
      <span class="font-normal">{label}</span>
      {#if clearable}<button
          type="button"
          class="btn btn-ghost btn-circle btn-sm tooltip tooltip-bottom"
          aria-label={`Clear ${label.toLowerCase()} filter`}
          data-tip={`Clear ${label.toLowerCase()}`}
          onclick={onClear}
          ><HugeiconsIcon
            icon={Cancel01Icon}
            size={16}
            strokeWidth={1.5}
            aria-hidden="true"
          /></button
        >{/if}
    </div>{/if}
  {@render children(close)}
  {#if footer}<div class="filter-popover__footer">
      {@render footer(close)}
    </div>{/if}
</div>

<style lang="postcss">
  @reference "../../../app.css";
  .filter-popover {
    @apply overflow-y-auto;
  }
  .filter-popover__header {
    @apply mb-2 flex min-h-8 items-center gap-2;
  }
  .filter-popover__footer {
    @apply sticky bottom-0 mt-2 flex shrink-0 gap-2 bg-inherit;
  }
  .filter-popover__footer :global(.btn) {
    @apply flex-1 rounded-full;
  }
  .filter-popover__footer :global(.btn:disabled) {
    @apply bg-transparent;
  }
</style>
