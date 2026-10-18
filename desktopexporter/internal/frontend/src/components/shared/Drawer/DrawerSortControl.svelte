<script module lang="ts">
  import type { SortDirection } from '@/contexts/signal-list-page.svelte'

  export type SortOption<T extends string> = {
    value: T
    label: string
  }
</script>

<script lang="ts" generics="T extends string">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { ArrowDown02Icon, Sorting05Icon } from '@hugeicons/core-free-icons'
  import {
    createPopoverID,
    setupAnchorPopover,
  } from '@/components/shared/utils/anchor-popover'

  let {
    options,
    value,
    direction,
    onchange,
  }: {
    options: SortOption<T>[]
    value: T
    direction: SortDirection
    onchange: (value: T, direction: SortDirection) => void
  } = $props()

  const popoverID = createPopoverID('sort-popover')
  let triggerEl = $state<HTMLButtonElement | null>(null)
  let popoverEl = $state<HTMLDivElement | null>(null)
  let open = $state(false)
  // Keep the visible target label in sync; the direction control is separate.
  const currentLabel = $derived(
    options.find(option => option.value === value)?.label ?? 'Sort'
  )

  $effect(() => {
    const trigger = triggerEl
    const popover = popoverEl
    if (!trigger || !popover) return
    return setupAnchorPopover({
      trigger,
      popover,
      anchor: 'below-end',
      onOpenChange: next => {
        open = next
        if (next) {
          popover
            .querySelector<HTMLButtonElement>('[aria-pressed="true"]')
            ?.focus()
        }
      },
    })
  })

  function selectSort(option: SortOption<T>) {
    onchange(option.value, direction)
    popoverEl?.hidePopover()
    triggerEl?.focus()
  }
</script>

<button
  bind:this={triggerEl}
  type="button"
  popovertarget={popoverID}
  aria-expanded={open}
  class="sort-trigger header-icon-button m-1 text-sm tooltip tooltip-bottom"
  aria-label={`Sort by: ${currentLabel}`}
  data-tip={`Sort by: ${currentLabel}`}
>
  <HugeiconsIcon
    icon={Sorting05Icon}
    size={16}
    strokeWidth={1.5}
    class="shrink-0"
    aria-hidden="true"
  />
  <span class="shrink-0">Sort by:</span>
  <span class="min-w-0 truncate text-base-content">{currentLabel}</span>
</button>
<button
  type="button"
  class="header-icon-button my-1 mr-1 ml-auto"
  aria-label={direction === 'asc' ? 'Sort descending' : 'Sort ascending'}
  title={direction === 'asc'
    ? 'Ascending — switch to descending'
    : 'Descending — switch to ascending'}
  onclick={() => onchange(value, direction === 'asc' ? 'desc' : 'asc')}
>
  <HugeiconsIcon
    icon={ArrowDown02Icon}
    class="shrink-0 {direction === 'asc' ? 'rotate-180' : ''}"
    size={16}
    strokeWidth={1.5}
    aria-hidden="true"
  />
</button>
<div
  bind:this={popoverEl}
  id={popoverID}
  popover="auto"
  class="anchor-popover anchor-popover--anchored sort-popover"
  role="group"
  aria-label="Sort by"
>
  {#each options as option (option.value)}
    <button
      type="button"
      aria-pressed={option.value === value}
      onclick={() => selectSort(option)}
      class="item-state item-state--menu flex w-full cursor-pointer items-center gap-3 px-3 py-2 text-left font-sans text-sm font-normal"
    >
      {option.label}
    </button>
  {/each}
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .sort-trigger {
    @apply min-w-0 w-auto shrink gap-2 px-2 text-left;
  }

  .sort-popover {
    @apply gap-1 p-2;
    width: 16rem;
    max-width: calc(100vw - 1.5rem);
    max-height: min(85vh, calc(100vh - 2rem));
    overflow-y: auto;
  }
</style>
