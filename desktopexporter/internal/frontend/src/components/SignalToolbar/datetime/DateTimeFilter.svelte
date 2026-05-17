<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatDateTimeRangeLabel } from '@/utils/time'
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

  const popoverId = `datetime-popover-${Math.random().toString(36).slice(2, 8)}`
  const rootFontSizePx =
    parseFloat(getComputedStyle(document.documentElement).fontSize) || 16
  const inwardGapPx = rootFontSizePx * 0.5
  const outwardGapPx = rootFontSizePx * 1.5

  function positionPopover() {
    if (!popoverEl || !triggerEl) return
    const rect = triggerEl.getBoundingClientRect()
    popoverEl.style.position = 'fixed'
    popoverEl.style.inset = 'auto'
    popoverEl.style.right = 'auto'
    popoverEl.style.bottom = 'auto'

    if (popoverAnchor === 'outward') {
      popoverEl.style.top = `${rect.top}px`
      popoverEl.style.left = `${rect.right + outwardGapPx}px`
    } else {
      popoverEl.style.top = `${rect.bottom + inwardGapPx}px`
      popoverEl.style.left = `${rect.left}px`
    }
  }

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
    const handleBeforeToggle = (e: ToggleEvent) => {
      if (e.newState === 'open') positionPopover()
    }
    const handleToggle = (e: ToggleEvent) => {
      popoverOpen = e.newState === 'open'
      if (popoverOpen) positionPopover()
    }
    popoverEl.addEventListener('beforetoggle', handleBeforeToggle)
    popoverEl.addEventListener('toggle', handleToggle)
    return () => {
      popoverEl?.removeEventListener('beforetoggle', handleBeforeToggle)
      popoverEl?.removeEventListener('toggle', handleToggle)
    }
  })

  $effect(() => {
    if (!popoverOpen) return
    const reposition = () => positionPopover()
    window.addEventListener('resize', reposition)
    window.addEventListener('scroll', reposition, true)
    return () => {
      window.removeEventListener('resize', reposition)
      window.removeEventListener('scroll', reposition, true)
    }
  })

  function getDisplayText(): string {
    if (!ctx?.selection) return 'Select time range'
    return formatDateTimeRangeLabel(
      ctx.selection.start,
      ctx.selection.end,
      ctx.timezone,
      { includeTimezone: true }
    )
  }

  let displayLabel = $derived(getDisplayText())

  let {
    class: className = '',
    popoverAnchor = 'inward',
  }: {
    class?: string
    /** inward = below trigger (open drawer); outward = right of trigger (collapsed rail, 1.5rem gap). */
    popoverAnchor?: 'inward' | 'outward'
  } = $props()
</script>

<div
    class="datetime-icon-tooltip-wrap"
    class:tooltip={!popoverOpen}
    class:tooltip-right={!popoverOpen}
    data-tip={displayLabel}
  >
    <button
      bind:this={triggerEl}
      type="button"
      class={className}
      popovertarget={popoverId}
      aria-expanded={popoverOpen}
      aria-label={`Change time range, ${displayLabel}`}
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
  </div>

<div
  bind:this={popoverEl}
  popover="auto"
  id={popoverId}
  class="datetime-popover"
  class:datetime-popover--inward={popoverAnchor === 'inward'}
  class:datetime-popover--outward={popoverAnchor === 'outward'}
>
  <PaneHeader mode="toolbar" ariaLabel="Time range presets">
    {#snippet right()}
      <PresetTimeRanges />
    {/snippet}
  </PaneHeader>
  <div class="datetime-popover__body">
    <TimeRangeFilterBody />
  </div>
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .datetime-icon-tooltip-wrap {
    @apply inline-flex shrink-0;
  }

  .datetime-popover {
    @apply overflow-hidden px-0 pb-0 pt-0;
    margin: 0;
    box-sizing: border-box;
    width: 30rem;
    max-width: calc(100vw - 1.5rem);
    max-height: min(85vh, calc(100vh - 2rem));

    @apply rounded-xl bg-base-200 text-sm text-base-content;
    @apply border border-base-300 shadow-surface;

    inset: unset;
  }

  .datetime-popover::backdrop {
    background-color: rgb(0 0 0 / 0.12);
    backdrop-filter: blur(1px);
  }

  .datetime-popover--inward,
  .datetime-popover--outward {
    position: fixed;
  }

  /* flex only while open — unconditional `display:flex` overrides the UA
     `display:none` that hides a closed popover, leaving a visible remnant. */
  .datetime-popover:popover-open {
    @apply flex flex-col;
  }

  .datetime-popover__body {
    @apply min-h-0 flex-1 overflow-y-auto py-2;
    overscroll-behavior: contain;
  }
</style>
