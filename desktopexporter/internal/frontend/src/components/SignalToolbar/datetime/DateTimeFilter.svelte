<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatDateTimeRange } from '@/utils/time'
  import TimeRangeFilterBody from './TimeRangeFilterBody.svelte'

  // Get time context
  let ctx = getTimeContext()
  if (!ctx) {
    throw new Error(
      'Time context not found. Make sure createTimeContext() is called at the root level.'
    )
  }

  let dialogOpen = $state(false)
  let dialogElement = $state<HTMLDialogElement | null>(null)
  let timeTriggerEl = $state<HTMLButtonElement | null>(null)

  const POPOVER_MARGIN = 12
  const POPOVER_GAP = 8

  function positionTimePopover() {
    let trigger = timeTriggerEl
    let dialog = dialogElement
    if (!trigger || !dialog || !dialog.open) return

    let vw = window.innerWidth
    let vh = window.innerHeight
    let rect = trigger.getBoundingClientRect()
    let panelWidth = dialog.getBoundingClientRect().width
    if (panelWidth < 8) return

    /* Right edges aligned with trigger; clamp so the panel stays on-screen */
    let left = rect.right - panelWidth
    left = Math.max(
      POPOVER_MARGIN,
      Math.min(left, vw - panelWidth - POPOVER_MARGIN)
    )

    let top = rect.bottom + POPOVER_GAP
    /* Leave a reasonable minimum height so presets/custom don’t feel crushed */
    let maxHeight = Math.max(280, vh - top - POPOVER_MARGIN)

    dialog.style.left = `${left}px`
    dialog.style.top = `${top}px`
    dialog.style.right = 'auto'
    dialog.style.bottom = 'auto'
    dialog.style.maxHeight = `${maxHeight}px`
  }

  function openTimePopover() {
    dialogElement?.showModal()
    dialogOpen = true
    requestAnimationFrame(() => {
      positionTimePopover()
      requestAnimationFrame(() => positionTimePopover())
    })
  }

  // Check if closedby attribute is supported (static check, done once)
  const supportsClosedBy = 'closedBy' in HTMLDialogElement.prototype

  // Dialog listeners when the element is bound
  $effect(() => {
    if (dialogElement) {
      const handleClose = () => {
        dialogOpen = dialogElement?.open ?? false
      }

      const handleCancel = () => {
        dialogOpen = false
      }

      // Fallback for browsers without closedby support (e.g., Safari)
      const handleClickOutside = (event: MouseEvent) => {
        if (!supportsClosedBy && dialogElement) {
          const rect = dialogElement.getBoundingClientRect()
          const isInDialog =
            rect.top <= event.clientY &&
            event.clientY <= rect.top + rect.height &&
            rect.left <= event.clientX &&
            event.clientX <= rect.left + rect.width

          if (!isInDialog) {
            dialogElement.close()
          }
        }
      }

      dialogElement.addEventListener('close', handleClose)
      dialogElement.addEventListener('cancel', handleCancel)

      // Only add click listener if closedby is not supported
      if (!supportsClosedBy) {
        dialogElement.addEventListener('click', handleClickOutside)
      }

      // Update initial state
      dialogOpen = dialogElement.open

      return () => {
        dialogElement?.removeEventListener('close', handleClose)
        dialogElement?.removeEventListener('cancel', handleCancel)
        if (!supportsClosedBy) {
          dialogElement?.removeEventListener('click', handleClickOutside)
        }
      }
    }
  })

  $effect(() => {
    if (!dialogOpen) return
    function onResize() {
      positionTimePopover()
    }
    window.addEventListener('resize', onResize)
    return () => window.removeEventListener('resize', onResize)
  })

  // Track previous time values to detect changes
  let previousStartTime = $state(ctx.selection?.start)
  let previousEndTime = $state(ctx.selection?.end)

  // Close dialog when time selection changes
  $effect(() => {
    const currentStartTime = ctx.selection?.start
    const currentEndTime = ctx.selection?.end

    // Check if time values actually changed
    const startTimeChanged = currentStartTime !== previousStartTime
    const endTimeChanged = currentEndTime !== previousEndTime

    if (startTimeChanged || endTimeChanged) {
      dialogElement?.close()
    }

    previousStartTime = currentStartTime
    previousEndTime = currentEndTime
  })

  // Get display text for current time selection
  function getDisplayText(): string {
    if (!ctx?.selection) {
      return 'Select time range'
    }

    return formatDateTimeRange(
      ctx.selection.start,
      ctx.selection.end,
      ctx.timezone
    )
  }

  let displayLabel = $derived(getDisplayText())
</script>

<!-- Range text + round soft-primary clock trigger on the right -->
<div class="datetime-filter">
  <span class="datetime-filter__range-text" title={displayLabel}>
    {displayLabel}
  </span>
  <button
    bind:this={timeTriggerEl}
    type="button"
    class="datetime-filter__trigger btn btn-soft btn-primary btn-sm btn-circle"
    class:btn-active={dialogOpen}
    onclick={openTimePopover}
    aria-expanded={dialogOpen}
    aria-label={`Change time range, ${displayLabel}`}
    title={displayLabel}
  >
    <span class="datetime-filter__trigger-icon" aria-hidden="true">
      <svg
        class="h-4 w-4"
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
    </span>
  </button>
</div>

<!-- Dialog Content -->
<dialog
  bind:this={dialogElement}
  id="datetime-dialog"
  class="datetime-dialog dropdown-content"
  closedby="any"
>
  <TimeRangeFilterBody />
</dialog>

<style lang="postcss">
  @reference "../../../app.css";

  .datetime-filter {
    @apply flex min-w-0 max-w-full items-center gap-2;
  }

  .datetime-filter__range-text {
    @apply min-w-0 flex-1 truncate text-left text-sm leading-snug text-base-content/85;
  }

  .datetime-filter__trigger {
    @apply shrink-0 border-0 shadow-none;
  }

  .datetime-filter__trigger-icon {
    @apply inline-flex items-center justify-center text-current;
  }

  .datetime-dialog {
    @apply px-0 pb-2 pt-0;
    position: fixed;
    margin: 0;
    box-sizing: border-box;
    width: 24rem;
    max-width: calc(100vw - 1.5rem);
    overflow-y: auto;

    @apply bg-base-100 rounded-lg text-sm shadow-lg;
    @apply border border-base-300 text-base-content;
  }

  .datetime-dialog::backdrop {
    background-color: rgba(0, 0, 0, 0.2);
    backdrop-filter: blur(1px);
    transition:
      opacity 0.4s ease-in-out,
      backdrop-filter 0.4s ease-in-out;
  }
</style>
