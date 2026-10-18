<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatDateTimeRangeLabel } from '@/utils/time'
  import {
    createPopoverID,
    setupAnchorPopover,
    type PopoverAnchor,
  } from '@/components/shared/utils/anchor-popover'
  import PresetTimeRanges from './PresetTimeRanges.svelte'
  import TimeRangeFilterBody from './TimeRangeFilterBody.svelte'

  const ctx = getTimeContext()

  let popoverEl = $state<HTMLDivElement | null>(null)
  let triggerEl = $state<HTMLButtonElement | null>(null)
  let popoverOpen = $state(false)

  const popoverID = createPopoverID('datetime-popover')

  let previousSelection = $state(ctx.selection)

  $effect(() => {
    const currentSelection = ctx.selection
    if (currentSelection !== previousSelection) {
      popoverEl?.hidePopover()
    }
    previousSelection = currentSelection
  })

  function presetLabel(durationMs: number): string {
    const day = 24 * 60 * 60_000
    const hour = 60 * 60_000
    if (durationMs % day === 0) return `${durationMs / day}d`
    if (durationMs % hour === 0) return `${durationMs / hour}h`
    return `${durationMs / 60_000}m`
  }

  $effect(() => {
    const popover = popoverEl
    const trigger = triggerEl
    if (!popover || !trigger) return
    return setupAnchorPopover({
      popover,
      trigger,
      anchor: popoverAnchor,
      onOpenChange: open => {
        popoverOpen = open
      },
    })
  })

  let ariaLabel = $derived.by(() => {
    const label =
      ctx.selection.type === 'all'
        ? 'All time'
        : ctx.selection.type === 'preset'
          ? `Last ${presetLabel(ctx.selection.durationMs)}`
          : formatDateTimeRangeLabel(
              ctx.selection.start,
              ctx.selection.end,
              ctx.tz,
              { includeTimezone: true }
            )
    return `Change time range, ${label}`
  })

  let {
    class:
      className = 'btn btn-circle btn-ghost btn-sm shrink-0 text-sm text-base-content',
    popoverAnchor = 'below-end',
  }: {
    class?: string
    /** below-end = below trigger, right-aligned (open drawer); outward = right of trigger (collapsed rail). */
    popoverAnchor?: PopoverAnchor
  } = $props()
</script>

<button
  bind:this={triggerEl}
  type="button"
  class="time-filter-trigger {className}"
  popovertarget={popoverID}
  aria-expanded={popoverOpen}
  aria-label={ariaLabel}
>
  <svg
    class="h-[17px] w-[17px] shrink-0"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.5"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <circle cx="12" cy="12" r="10" />
    <path d="M12 8v4l2 2" />
  </svg>
</button>

<div
  bind:this={popoverEl}
  popover="auto"
  id={popoverID}
  class="anchor-popover anchor-popover--anchored anchor-popover--wide"
>
  <div class="min-w-0 shrink-0" role="toolbar" aria-label="Time range presets">
    <PresetTimeRanges />
  </div>
  <div class="anchor-popover__body">
    <TimeRangeFilterBody />
  </div>
</div>

<style>
  .time-filter-trigger {
    border: 1px solid var(--color-base-300);
    background-color: var(--color-base-200);
    box-shadow: none;
  }

  .time-filter-trigger:hover {
    background-color: var(--color-base-300);
  }
</style>
