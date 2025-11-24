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

  let popoverOpen = $state(false);

  // Listen for popover open/close events
  $effect(() => {
    const popover = document.getElementById('datetime-popover');
    if (popover) {
      const handleToggle = () => {
        popoverOpen = popover.matches(':popover-open');
      };

      popover.addEventListener('toggle', handleToggle);
      return () => popover.removeEventListener('toggle', handleToggle);
    }
  });

  // Track previous time values to detect changes
  let previousStartTime = $state(ctx.selection?.start);
  let previousEndTime = $state(ctx.selection?.end);

  // Close popover when time selection changes
  $effect(() => {
    const currentStartTime = ctx.selection?.start;
    const currentEndTime = ctx.selection?.end;

    // Check if time values actually changed
    const startTimeChanged = currentStartTime !== previousStartTime;
    const endTimeChanged = currentEndTime !== previousEndTime;

    if (startTimeChanged || endTimeChanged) {
      document.getElementById('datetime-popover')?.hidePopover();
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
  class="input input-bordered input-sm inline-flex items-center gap-2 text-xs"
  popovertarget="datetime-popover"
  style="anchor-name: --datetime-anchor"
>
  <svg class="w-4 h-4" viewBox="0 0 24 24">
    <g>
      <circle cx="12" cy="12" r="10" />
      <path d="M12 8v4l2 2" />
    </g>
  </svg>
  <span>{getDisplayText()}</span>
  <svg
    class="w-3 h-3 popover-indicator {popoverOpen
      ? 'popover-indicator--open'
      : ''}"
    viewBox="0 0 24 24"
  >
    <path d="M18 9s-4.419 6-6 6s-6-6-6-6" />
  </svg>
</button>

<!-- Popover Content -->
<div id="datetime-popover" class="popover datetime-popover" popover="auto">
  <!-- Vertical stacked layout -->
  <div>
    <!-- Preset Time Ranges -->
    <PresetTimeRanges />

    <!-- Custom Time Range -->
    <div class="py-2">
      <CustomTimeRange />
    </div>

    <!-- Horizontal separator -->
    <div class="border-t border-base-300"></div>

    <!-- Timezone Selector -->
    <div class="p-2">
      <button
        class="timezone-toggle"
        onclick={() => {
          ctx.setTimezone(ctx.timezone === 'UTC' ? 'local' : 'UTC');
        }}
      >
        <div class="flex items-center font-semibold text-base-content gap-2">
          <svg
            class="w-4 h-4"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linejoin="round"
              stroke-width="1.5"
              d="M12 22C6.477 22 2 17.523 2 12a9.97 9.97 0 0 1 2.99-7.132M12 22c-.963-.714-.81-1.544-.326-2.375c.743-1.278.743-1.278.743-2.98c0-1.704 1.012-2.502 4.583-1.788c1.605.321 2.774-1.896 4.857-1.164M12 22c4.946 0 9.053-3.59 9.857-8.307m0 0Q22 12.867 22 12c0-4.881-3.498-8.946-8.123-9.824m0 0c.51.94.305 2.06-.774 2.487c-1.76.697-.5 1.98-2 2.773c-1 .528-2.499.396-3.998-1.189c-.79-.834-1.265-1.29-2.115-1.379m8.887-2.692A10 10 0 0 0 12 2a9.97 9.97 0 0 0-7.01 2.868"
            />
          </svg>
          <span class="text-sm ">
            {ctx.timezone === 'UTC'
              ? 'Coordinated Universal Time UTC'
              : getLocalTimezoneName()}
          </span>
        </div>
        <div class="flex items-center gap-2">
          <span class="text-sm text-base-content/60">
            {ctx.timezone === 'UTC' ? 'UTC+0' : 'Local'}
          </span>
          <svg
            class="w-4 h-4 text-base-content/60"
            viewBox="0 0 24 24"
          >
            <path
              d="M8 9l4-4 4 4m0 6l-4 4-4-4"
            ></path>
          </svg>
        </div>
      </button>
    </div>

    <!-- Horizontal separator -->
    <div class="border-t border-base-300"></div>

    <!-- Recently Used -->
    <div class="py-2">
      <RecentTimeRanges />
    </div>
  </div>
</div>

<style>
  .datetime-popover {
    /* Layout & Positioning */
    @apply dropdown-content;
    @apply px-0 pb-2 pt-0 mx-0 my-2;
    position-anchor: --datetime-anchor;
    top: anchor(--datetime-anchor bottom);
    left: anchor(--datetime-anchor left);

    /* Visual Styling */
    @apply bg-base-100 rounded-md shadow-lg;
    @apply border border-base-300 text-base-content;
    @apply min-w-96;
  }

  .timezone-toggle {
    @apply w-full flex items-center justify-between hover:bg-base-200 transition-colors rounded p-2;
  }
</style>
