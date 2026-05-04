<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatDateTimeRange } from '@/utils/time'
  import TimeRangeFilterBody from './TimeRangeFilterBody.svelte'

  let ctx = getTimeContext()
  if (!ctx) {
    throw new Error(
      'Time context not found. Make sure createTimeContext() is called at the root level.'
    )
  }

  let popoverEl = $state<HTMLDivElement | null>(null)
  let popoverOpen = $state(false)

  const popoverId = `datetime-popover-${Math.random().toString(36).slice(2, 8)}`
  const anchorName = `--datetime-anchor-${Math.random().toString(36).slice(2, 8)}`

  let previousStartTime = $state(ctx.selection?.start)
  let previousEndTime = $state(ctx.selection?.end)

  $effect(() => {
    const currentStartTime = ctx.selection?.start
    const currentEndTime = ctx.selection?.end

    const startTimeChanged = currentStartTime !== previousStartTime
    const endTimeChanged = currentEndTime !== previousEndTime

    if (startTimeChanged || endTimeChanged) {
      popoverEl?.hidePopover()
    }

    previousStartTime = currentStartTime
    previousEndTime = currentEndTime
  })

  $effect(() => {
    if (!popoverEl) return
    const handleToggle = (e: ToggleEvent) => {
      popoverOpen = e.newState === 'open'
    }
    popoverEl.addEventListener('toggle', handleToggle)
    return () => popoverEl?.removeEventListener('toggle', handleToggle)
  })

  function getDisplayText(): string {
    if (!ctx?.selection) return 'Select time range'
    return formatDateTimeRange(
      ctx.selection.start,
      ctx.selection.end,
      ctx.timezone
    )
  }

  let displayLabel = $derived(getDisplayText())

  let {
    class: className = '',
    triggerVariant = 'icon',
    inJoin = false,
  }: {
    class?: string
    triggerVariant?: 'icon' | 'select' | 'dropdown'
    inJoin?: boolean
  } = $props()
</script>

{#if triggerVariant === 'select'}
  <button
    type="button"
    class="datetime-select-trigger {className}"
    class:datetime-select-trigger--join={inJoin}
    class:datetime-select-trigger--open={popoverOpen}
    popovertarget={popoverId}
    style:anchor-name={anchorName}
    aria-expanded={popoverOpen}
    aria-label={`Change time range, ${displayLabel}`}
  >
    <svg
      class="h-[17px] w-[17px] shrink-0 text-base-content/50"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="1.5"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <circle cx="12" cy="12" r="10" />
      <path d="M12 8v4l2 2" />
    </svg>
    <span class="datetime-filter__range-text">{displayLabel}</span>
    <svg
      class="h-3 w-3 shrink-0 text-base-content/40"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="2"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      <path d="M6 9l6 6 6-6" />
    </svg>
  </button>
{:else if triggerVariant === 'dropdown'}
  <button
    type="button"
    class="datetime-dropdown-trigger {className}"
    class:datetime-dropdown-trigger--open={popoverOpen}
    popovertarget={popoverId}
    style:anchor-name={anchorName}
    aria-expanded={popoverOpen}
    aria-label={`Change time range, ${displayLabel}`}
  >
    <svg
      class="h-3.5 w-3.5 shrink-0"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="1.5"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <circle cx="12" cy="12" r="10" />
      <path d="M12 8v4l2 2" />
    </svg>
    <span class="datetime-dropdown-trigger__text">{displayLabel}</span>
  </button>
{:else}
  <button
    type="button"
    class={className}
    popovertarget={popoverId}
    style:anchor-name={anchorName}
    aria-expanded={popoverOpen}
    aria-label={`Change time range, ${displayLabel}`}
    title={displayLabel}
  >
    <svg
      class="h-3.5 w-3.5 shrink-0"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="1.5"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <circle cx="12" cy="12" r="10" />
      <path d="M12 8v4l2 2" />
    </svg>
  </button>
{/if}

<div
  bind:this={popoverEl}
  popover="auto"
  id={popoverId}
  class="datetime-popover"
  style:position-anchor={anchorName}
>
  <TimeRangeFilterBody />
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .datetime-select-trigger {
    @apply flex min-w-0 cursor-pointer items-center gap-1.5 rounded-lg border border-base-300 bg-base-100 px-2.5 py-1.5 text-xs text-base-content/70 transition-[color,border-color,box-shadow] duration-150;
  }

  .datetime-filter__range-text {
    @apply min-w-0 flex-1 truncate text-left;
  }

  .datetime-select-trigger:not(.datetime-select-trigger--join):hover {
    @apply border-base-content/30 text-base-content;
  }

  .datetime-select-trigger--open:not(.datetime-select-trigger--join) {
    @apply border-primary/40 text-primary ring-1 ring-primary/20;
  }

  .datetime-select-trigger:focus-visible {
    outline: var(--focus-ring-width) solid var(--focus-ring-color);
    outline-offset: var(--focus-ring-offset);
  }

  /* DaisyUI join row — mirror drawer-editor-btn inactive hover (cannot @apply `.drawer-editor-btn` here). */
  .datetime-select-trigger--join {
    @apply join-item flex min-h-0 h-[2.25rem] min-w-0 flex-1 items-center justify-start gap-2 rounded-none border border-transparent bg-transparent px-2.5 py-0 text-xs font-normal tracking-normal text-base-content/55 shadow-none transition-[color,background-color,box-shadow] duration-200 hover:bg-base-200/80 hover:text-base-content;
  }

  .datetime-select-trigger--join.datetime-select-trigger--open {
    @apply z-[1] border-transparent bg-primary/15 text-primary shadow-sm shadow-primary/10 ring-1 ring-primary/20;
  }

  /* Dropdown trigger: compact clock + label in a btn-like shape */
  .datetime-dropdown-trigger {
    @apply btn btn-xs btn-ghost flex items-center gap-1 text-xs font-normal text-base-content/70 transition-colors duration-150;
  }

  .datetime-dropdown-trigger:hover {
    @apply text-base-content;
  }

  .datetime-dropdown-trigger--open {
    @apply bg-primary/10 text-primary;
  }

  .datetime-dropdown-trigger__text {
    @apply truncate max-w-[10rem];
  }

  .datetime-popover {
    @apply px-0 pb-2 pt-0;
    margin: 0;
    box-sizing: border-box;
    width: 24rem;
    max-width: calc(100vw - 1.5rem);
    overflow-y: auto;

    @apply bg-base-100 rounded-lg text-sm shadow-lg;
    @apply border border-base-300 text-base-content;

    inset: unset;
    top: anchor(bottom, 0);
    left: anchor(left, 0);
  }
</style>
