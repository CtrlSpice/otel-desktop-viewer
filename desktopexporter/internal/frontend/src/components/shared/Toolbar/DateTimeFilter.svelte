<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatDateTimeRangeLabel } from '@/utils/time'
  import {
    createPopoverID,
    setupAnchorPopover,
    type PopoverAnchor,
  } from '@/components/shared/utils/anchor-popover'
  import PaneHeader from '@/components/shared/PaneHeader.svelte'
  import PresetTimeRanges from './PresetTimeRanges.svelte'
  import TimeRangeFilterBody from './TimeRangeFilterBody.svelte'

  let ctx = getTimeContext()
  if (!ctx) {
    throw new Error(
      'Time context not found. Make sure createTimeContext() is called at the root level.'
    )
  }

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
    if (!ctx?.selection) return 'Change time range'
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
    class: className = '',
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
  class={className}
  popovertarget={popoverID}
  aria-expanded={popoverOpen}
  aria-label={ariaLabel}
  data-tip="Time range"
>
  <svg
    class="h-[17px] w-[17px] shrink-0"
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

<div
  bind:this={popoverEl}
  popover="auto"
  id={popoverID}
  class="anchor-popover anchor-popover--anchored anchor-popover--wide"
>
  <PaneHeader mode="toolbar" ariaLabel="Time range presets">
    {#snippet right()}
      <PresetTimeRanges />
    {/snippet}
  </PaneHeader>
  <div class="anchor-popover__body">
    <TimeRangeFilterBody />
  </div>
</div>
