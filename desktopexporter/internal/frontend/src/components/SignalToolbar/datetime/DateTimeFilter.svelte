<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatDateTimeRangeLabel } from '@/utils/time'
  import {
    createPopoverId,
    setupAnchorPopover,
    type PopoverAnchor,
  } from '@/utils/anchor-popover'
  import PaneHeader from '@/components/PaneHeader.svelte'
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

  const popoverId = createPopoverId('datetime-popover')

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
    const label = formatDateTimeRangeLabel(
      ctx.selection.start,
      ctx.selection.end,
      ctx.timezone,
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
  popovertarget={popoverId}
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
  >
    <circle cx="12" cy="12" r="10" />
    <path d="M12 8v4l2 2" />
  </svg>
</button>

<div
  bind:this={popoverEl}
  popover="auto"
  id={popoverId}
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
