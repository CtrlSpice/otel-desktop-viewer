<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte';
  import { getLocalTimezoneName, formatDateTimeRange } from '@/utils/time';
  import PresetTimeRanges from './PresetTimeRanges.svelte';
  import CustomTimeRange from './CustomTimeRange.svelte';
  import RecentTimeRanges from './RecentTimeRanges.svelte';

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
</script>

<!-- Time Filter Button -->
<button
  bind:this={timeTriggerEl}
  type="button"
  class="input input-bordered inline-flex h-10 min-h-10 items-center gap-2 px-3 py-0 text-sm"
  onclick={openTimePopover}
>
  <svg class="w-4 h-4" viewBox="0 0 24 24">
    <g>
      <circle cx="12" cy="12" r="10" />
      <path d="M12 8v4l2 2" />
    </g>
  </svg>
  <span>{getDisplayText()}</span>
  <svg
    class="w-3 h-3 popover-indicator {dialogOpen
      ? 'popover-indicator--open'
      : ''}"
    viewBox="0 0 24 24"
  >
    <path d="M18 9s-4.419 6-6 6s-6-6-6-6" />
  </svg>
</button>

<!-- Dialog Content -->
<dialog
  bind:this={dialogElement}
  id="datetime-dialog"
  class="datetime-dialog dropdown-content"
  closedby="any"
>
  <!-- Vertical stacked layout -->
  <div>
    <!-- Preset Time Ranges -->
    <PresetTimeRanges />

    <!-- Custom Time Range -->
    <div class="p-2">
      <CustomTimeRange />
    </div>

    <!-- Horizontal separator -->
    <div class="border-t border-base-300"></div>

    <!-- Timezone Selector -->
    <div class="p-2">
      <div class="min-w-0">
        <div
          class="section-header mb-1 text-xs font-semibold uppercase tracking-wide text-base-content/70"
        >
          Timezone
        </div>
        <button
          type="button"
          class="timezone-toggle"
          onclick={() => {
            ctx.setTimezone(ctx.timezone === 'UTC' ? 'local' : 'UTC');
          }}
        >
          <div class="flex min-w-0 items-center gap-1.5 font-semibold leading-snug">
            <svg class="h-3.5 w-3.5 shrink-0" viewBox="0 0 24 24">
              <path
                stroke-linejoin="round"
                stroke-width="1.5"
                d="M12 22C6.477 22 2 17.523 2 12a9.97 9.97 0 0 1 2.99-7.132M12 22c-.963-.714-.81-1.544-.326-2.375c.743-1.278.743-1.278.743-2.98c0-1.704 1.012-2.502 4.583-1.788c1.605.321 2.774-1.896 4.857-1.164M12 22c4.946 0 9.053-3.59 9.857-8.307m0 0Q22 12.867 22 12c0-4.881-3.498-8.946-8.123-9.824m0 0c.51.94.305 2.06-.774 2.487c-1.76.697-.5 1.98-2 2.773c-1 .528-2.499.396-3.998-1.189c-.79-.834-1.265-1.29-2.115-1.379m8.887-2.692A10 10 0 0 0 12 2a9.97 9.97 0 0 0-7.01 2.868"
              />
            </svg>
            <span class="min-w-0 truncate">
              {ctx.timezone === 'UTC'
                ? 'Coordinated Universal Time UTC'
                : getLocalTimezoneName()}
            </span>
          </div>
          <div class="flex shrink-0 items-center gap-1 text-base-content/60">
            <span>{ctx.timezone === 'UTC' ? 'UTC+0' : 'Local'}</span>
            <svg
              class="h-3.5 w-3.5"
              viewBox="0 0 24 24"
            >
              <path
                d="M8 9l4-4 4 4m0 6l-4 4-4-4"
              ></path>
            </svg>
          </div>
        </button>
      </div>
    </div>

    <!-- Horizontal separator -->
    <div class="border-t border-base-300"></div>

    <!-- Recently Used -->
    <div class="p-2">
      <RecentTimeRanges />
    </div>
  </div>
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

  .timezone-toggle {
    @apply flex w-full cursor-pointer items-center justify-between gap-2 rounded-md border-none bg-transparent px-1.5 py-1 text-left text-xs transition-colors;
    @apply text-base-content/90 hover:bg-base-200/90 focus-visible:bg-base-200/90;
    @apply focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30 focus-visible:ring-offset-0;
  }
</style>
