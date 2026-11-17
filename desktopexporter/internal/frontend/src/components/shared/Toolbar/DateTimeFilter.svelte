<script lang="ts">
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import { Time02Icon } from '@hugeicons/core-free-icons'
  import {
    getTimeContext,
    TIME_RANGE_PRESETS,
  } from '@/contexts/time-context.svelte'
  import { formatDateTimeRangeLabel } from '@/utils/time'
  import { wallClock } from '@/state/wall-clock.svelte'
  import {
    createPopoverID,
    setupAnchorPopover,
    type PopoverAnchor,
  } from '@/components/shared/utils/anchor-popover'
  import PresetTimeRanges from './PresetTimeRanges.svelte'
  import TimeRangeFilterBody from './TimeRangeFilterBody.svelte'
  import IconButton from '../IconButton.svelte'

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

  const rangeLabel = $derived.by(() => {
    const selection = ctx.selection
    if (selection.type === 'all') return 'All time'
    if (selection.type === 'preset') {
      const preset = TIME_RANGE_PRESETS.find(
        preset => preset.durationMs === selection.durationMs
      )!
      return `Last ${preset.label}`
    }
    return formatDateTimeRangeLabel(selection.start, selection.end, ctx.tz, {
      style: 'compact',
      now: Number(wallClock.now / 1_000_000n),
    })
  })
  const exactRangeLabel = $derived.by(() => {
    const selection = ctx.selection
    if (selection.type === 'all' || selection.type === 'preset') {
      return rangeLabel
    }
    return formatDateTimeRangeLabel(selection.start, selection.end, ctx.tz, {
      includeTimezone: true,
    })
  })
  const ariaLabel = $derived(`Change time range, ${exactRangeLabel}`)

  let {
    class: className = '',
    popoverAnchor = 'below-end',
  }: {
    class?: string
    /** below-end = below trigger, right-aligned (open drawer); outward = right of trigger (collapsed rail). */
    popoverAnchor?: PopoverAnchor
  } = $props()
</script>

<IconButton
  bind:element={triggerEl}
  class={className}
  popovertarget={popoverID}
  aria-expanded={popoverOpen}
  label={ariaLabel}
  tooltip={popoverOpen ? '' : `Change time range, ${rangeLabel}`}
>
  <HugeiconsIcon
    icon={Time02Icon}
    size={17}
    strokeWidth={1.5}
    aria-hidden="true"
  />
</IconButton>

<div
  bind:this={popoverEl}
  popover="auto"
  id={popoverID}
  class="anchor-popover anchor-popover--anchored anchor-popover--form datetime-popover gap-2"
>
  <div class="min-w-0 shrink-0" role="toolbar" aria-label="Time range presets">
    <PresetTimeRanges />
  </div>
  <div class="anchor-popover__body">
    <TimeRangeFilterBody />
  </div>
</div>

<style>
  .datetime-popover {
    width: 32rem;
    font-family: var(--font-sans);
    container-type: inline-size;
  }
</style>
