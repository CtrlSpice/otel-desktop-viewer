<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte';
  import { formatDateTimeRange } from '@/utils/time';
  import TimeRangeFilterBody from './TimeRangeFilterBody.svelte';

  type Props = {
    /** When false, clock only (no range text / chevron). Default true. */
    showLabel?: boolean;
    /** Override label text; default is formatted range from context. */
    label?: string;
  };

  let { showLabel = true, label: labelOverride }: Props = $props();

  // Get time context
  let ctx = getTimeContext();
  if (!ctx) {
    throw new Error(
      'Time context not found. Make sure createTimeContext() is called at the root level.'
    );
  }

  let dialogOpen = $state(false);
  let dialogElement = $state<HTMLDialogElement | null>(null);
  let timeTriggerEl = $state<HTMLButtonElement | null>(null);

  const POPOVER_MARGIN = 12;
  const POPOVER_GAP = 8;

  function positionTimePopover() {
    let trigger = timeTriggerEl;
    let dialog = dialogElement;
    if (!trigger || !dialog || !dialog.open) return;

    let vw = window.innerWidth;
    let vh = window.innerHeight;
    let rect = trigger.getBoundingClientRect();
    let panelWidth = dialog.getBoundingClientRect().width;
    if (panelWidth < 8) return;

    /* Right edges aligned with trigger; clamp so the panel stays on-screen */
    let left = rect.right - panelWidth;
    left = Math.max(
      POPOVER_MARGIN,
      Math.min(left, vw - panelWidth - POPOVER_MARGIN)
    );

    let top = rect.bottom + POPOVER_GAP;
    /* Leave a reasonable minimum height so presets/custom don’t feel crushed */
    let maxHeight = Math.max(280, vh - top - POPOVER_MARGIN);

    dialog.style.left = `${left}px`;
    dialog.style.top = `${top}px`;
    dialog.style.right = 'auto';
    dialog.style.bottom = 'auto';
    dialog.style.maxHeight = `${maxHeight}px`;
  }

  function openTimePopover() {
    dialogElement?.showModal();
    dialogOpen = true;
    requestAnimationFrame(() => {
      positionTimePopover();
      requestAnimationFrame(() => positionTimePopover());
    });
  }

  // Check if closedby attribute is supported (static check, done once)
  const supportsClosedBy = 'closedBy' in HTMLDialogElement.prototype;

  // Dialog listeners when the element is bound
  $effect(() => {
    if (dialogElement) {
      const handleClose = () => {
        dialogOpen = dialogElement?.open ?? false;
      };

      const handleCancel = () => {
        dialogOpen = false;
      };

      // Fallback for browsers without closedby support (e.g., Safari)
      const handleClickOutside = (event: MouseEvent) => {
        if (!supportsClosedBy && dialogElement) {
          const rect = dialogElement.getBoundingClientRect();
          const isInDialog = (
            rect.top <= event.clientY &&
            event.clientY <= rect.top + rect.height &&
            rect.left <= event.clientX &&
            event.clientX <= rect.left + rect.width
          );

          if (!isInDialog) {
            dialogElement.close();
          }
        }
      };

      dialogElement.addEventListener('close', handleClose);
      dialogElement.addEventListener('cancel', handleCancel);
      
      // Only add click listener if closedby is not supported
      if (!supportsClosedBy) {
        dialogElement.addEventListener('click', handleClickOutside);
      }
      
      // Update initial state
      dialogOpen = dialogElement.open;

      return () => {
        dialogElement?.removeEventListener('close', handleClose);
        dialogElement?.removeEventListener('cancel', handleCancel);
        if (!supportsClosedBy) {
          dialogElement?.removeEventListener('click', handleClickOutside);
        }
      };
    }
  });

  $effect(() => {
    if (!dialogOpen) return;
    function onResize() {
      positionTimePopover();
    }
    window.addEventListener('resize', onResize);
    return () => window.removeEventListener('resize', onResize);
  });

  // Track previous time values to detect changes
  let previousStartTime = $state(ctx.selection?.start);
  let previousEndTime = $state(ctx.selection?.end);

  // Close dialog when time selection changes
  $effect(() => {
    const currentStartTime = ctx.selection?.start;
    const currentEndTime = ctx.selection?.end;

    // Check if time values actually changed
    const startTimeChanged = currentStartTime !== previousStartTime;
    const endTimeChanged = currentEndTime !== previousEndTime;

    if (startTimeChanged || endTimeChanged) {
      dialogElement?.close();
    }

    previousStartTime = currentStartTime;
    previousEndTime = currentEndTime;
  });

  // Get display text for current time selection
  function getDisplayText(): string {
    if (!ctx?.selection) {
      return 'Select time range';
    }

    return formatDateTimeRange(
      ctx.selection.start,
      ctx.selection.end,
      ctx.timezone
    );
  }

  let displayLabel = $derived(
    labelOverride !== undefined ? labelOverride : getDisplayText()
  );
</script>

<!-- Time Filter Button -->
<button
  bind:this={timeTriggerEl}
  type="button"
  class="toolbar-filter-trigger toolbar-filter-trigger--time"
  class:toolbar-filter-trigger--active={dialogOpen}
  class:toolbar-filter-trigger--compact={!showLabel}
  onclick={openTimePopover}
  aria-label={showLabel ? `Time range: ${displayLabel}` : 'Time range'}
  aria-expanded={dialogOpen}
  title={displayLabel}
>
  <span class="toolbar-filter-trigger__icon" aria-hidden="true">
    <svg class="h-4 w-4" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
      <circle cx="12" cy="12" r="10" />
      <path d="M12 8v4l2 2" />
    </svg>
  </span>
  {#if showLabel}
    <span class="toolbar-filter-trigger__label">{displayLabel}</span>
  {/if}
  {#if showLabel}
    <svg
      class="popover-indicator h-3 w-3 shrink-0 opacity-60 {dialogOpen
        ? 'popover-indicator--open'
        : ''}"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      stroke-width="1.5"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      <path d="M18 9s-4.419 6-6 6s-6-6-6-6" />
    </svg>
  {/if}
</button>

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
  .datetime-dialog {
    /* Width from CSS (not squeezed to vw-16); script only positions using measured width */
    @apply px-0 pb-2 pt-0;
    position: fixed;
    margin: 0;
    box-sizing: border-box;
    width: 24rem;
    max-width: calc(100vw - 1.5rem);
    min-width: min(24rem, calc(100vw - 1.5rem));
    overflow-y: auto;

    @apply bg-base-100 rounded-md shadow-lg;
    @apply border border-base-300 text-base-content;
  }

  .datetime-dialog::backdrop {
    background-color: rgba(0, 0, 0, 0.2);
    backdrop-filter: blur(1px);
    transition: opacity 0.4s ease-in-out, backdrop-filter 0.4s ease-in-out;
  }

</style>
