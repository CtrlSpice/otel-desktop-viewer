<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte';
  import { getLocalTimezoneName, formatDateTimeRange } from '@/utils/time';
  import PresetTimeRanges from './PresetTimeRanges.svelte';
  import CustomTimeRange from './CustomTimeRange.svelte';
  import RecentTimeRanges from './RecentTimeRanges.svelte';
  import '@/components/PageHeader/PageHeader.css';

  // Get time context
  let ctx = getTimeContext();
  if (!ctx) {
    throw new Error(
      'Time context not found. Make sure createTimeContext() is called at the root level.'
    );
  }

  // Tab management for narrow screens
  let activeTab = $state<'preset' | 'custom' | 'recent'>(
    ctx.selection?.type || 'preset'
  );

  // Screen size detection
  let isNarrow = $state(false);
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

    activeTab = ctx.selection?.type || 'preset';
    previousStartTime = currentStartTime;
    previousEndTime = currentEndTime;
  });

  // Check screen size on mount and resize
  function checkScreenSize() {
    isNarrow = window.innerWidth < 768; // md breakpoint
  }

  // Set up resize listener
  $effect(() => {
    checkScreenSize();
    window.addEventListener('resize', checkScreenSize);
    return () => window.removeEventListener('resize', checkScreenSize);
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

{#if isNarrow}
  <!-- Narrow Screen Button -->
  <div
    class="tooltip tooltip-bottom tooltip-bottom-right"
    data-tip={getDisplayText()}
  >
    <button
      popovertarget="datetime-popover"
      style="anchor-name: --datetime-anchor"
      class="btn btn-circle btn-sm"
      aria-label="Time Filter"
    >
      <svg
        class="w-4 h-4"
        fill="none"
        stroke="currentColor"
        viewBox="0 0 24 24"
      >
        <g stroke-width="1.5">
          <circle cx="12" cy="12" r="10" />
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            d="M12 8v4l2 2"
          />
        </g>
      </svg>
    </button>
  </div>
{:else}
  <!-- Wide Screen Button -->
  <button
    class="input input-bordered input-sm inline-flex items-center gap-2 text-xs"
    popovertarget="datetime-popover"
    style="anchor-name: --datetime-anchor"
  >
    <svg class="w-4 h-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
      <g stroke-width="1.5">
        <circle cx="12" cy="12" r="10" />
        <path stroke-linecap="round" stroke-linejoin="round" d="M12 8v4l2 2" />
      </g>
    </svg>
    <span>{getDisplayText()}</span>
    <svg
      class="w-3 h-3 transition-transform duration-200 {popoverOpen
        ? 'rotate-180'
        : ''}"
      fill="none"
      stroke="currentColor"
      viewBox="0 0 24 24"
    >
      <path
        stroke-linecap="round"
        stroke-linejoin="round"
        stroke-width="2"
        d="M19 9l-7 7-7-7"
      ></path>
    </svg>
  </button>
{/if}

<!-- Popover Content -->
<div id="datetime-popover" class="datetime-popover" popover="auto">
  {#if !isNarrow}
    <!-- Wide Layout: Three columns -->
    <div class="flex">
      <!-- Left Side - Preset Time Ranges -->
      <div class="min-w-40">
        <PresetTimeRanges />
      </div>

      <!-- Vertical separator -->
      <div class="filter-separator"></div>

      <!-- Middle Section - Custom Time Range -->
      <div class="flex-1 space-y-3 min-w-64">
        <CustomTimeRange />
      </div>

      <!-- Vertical separator -->
      <div class="filter-separator"></div>

      <!-- Right Section - Recently Used -->
      <div class="space-y-3 min-w-40">
        <RecentTimeRanges />
      </div>
    </div>
  {:else}
    <!-- Narrow Layout: Icon Buttons -->
    <div class="space-y-4">
      <!-- Button Navigation -->
      <div class="flex gap-2 px-2">
        <button
          class="btn btn-sm {activeTab === 'preset'
            ? 'btn-primary'
            : 'btn-outline'} flex items-center gap-2 flex-1"
          onclick={() => (activeTab = 'preset')}
          aria-label="Quick Select"
        >
          <svg
            class="w-4 h-4"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <path
              stroke-linejoin="round"
              stroke-width="1.5"
              d="m15 2l.539 2.392a5.39 5.39 0 0 0 4.07 4.07L22 9l-2.392.539a5.39 5.39 0 0 0-4.07 4.07L15 16l-.539-2.392a5.39 5.39 0 0 0-4.07-4.07L8 9l2.392-.539a5.39 5.39 0 0 0 4.07-4.07zM7 12l.385 1.708a3.85 3.85 0 0 0 2.907 2.907L12 17l-1.708.385a3.85 3.85 0 0 0-2.907 2.907L7 22l-.385-1.708a3.85 3.85 0 0 0-2.907-2.907L2 17l1.708-.385a3.85 3.85 0 0 0 2.907-2.907z"
            />
          </svg>
          <span class="font-normal">Presets</span>
        </button>

        <button
          class="btn btn-sm {activeTab === 'custom'
            ? 'btn-primary'
            : 'btn-outline'} flex items-center gap-2 flex-1"
          onclick={() => (activeTab = 'custom')}
          aria-label="Custom Range"
        >
          <svg
            class="w-4 h-4"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <g
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="1.5"
            >
              <path
                d="M16 2v4M8 2v4m13 6c0-3.771 0-5.657-1.172-6.828S16.771 4 13 4h-2C7.229 4 5.343 4 4.172 5.172S3 8.229 3 12v2c0 3.771 0 5.657 1.172 6.828S7.229 22 11 22M3 10h18"
              />
              <path
                d="M18.267 18.701L17 18v-1.733M21 18a4 4 0 1 1-8 0a4 4 0 0 1 8 0"
              />
            </g>
          </svg>
          <span class="font-normal">Custom</span>
        </button>

        <button
          class="btn btn-sm {activeTab === 'recent'
            ? 'btn-primary'
            : 'btn-outline'} flex items-center gap-2 flex-1"
          onclick={() => (activeTab = 'recent')}
          aria-label="Recent"
        >
          <svg
            class="w-4 h-4"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
          >
            <g
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="1.5"
            >
              <path
                d="M19 10.5V10c0-3.771 0-5.657-1.172-6.828S14.771 2 11 2S5.343 2 4.172 3.172S3 6.229 3 10v4.5c0 3.287 0 4.931.908 6.038q.25.304.554.554C5.57 22 7.212 22 10.5 22M7 7h8m-8 4h4"
              />
              <path
                d="m18 18.5l-1.5-.55V15.5m-4.5 2a4.5 4.5 0 1 0 9 0a4.5 4.5 0 0 0-9 0"
              />
            </g>
          </svg>
          <span class="font-normal">Recent</span>
        </button>
      </div>

      <!-- Content based on active tab -->
      {#if activeTab === 'preset'}
        <PresetTimeRanges />
      {:else if activeTab === 'custom'}
        <CustomTimeRange {isNarrow} />
      {:else if activeTab === 'recent'}
        <RecentTimeRanges />
      {/if}
    </div>
  {/if}

  <!-- Bottom - Timezone Selector -->
  <div class="mt-2 pt-3 border-t border-base-300">
    <button
      class="timezone-toggle"
      onclick={() => {
        ctx.setTimezone(ctx.timezone === 'UTC' ? 'local' : 'UTC');
      }}
    >
      <div class="flex items-center gap-2">
        <svg
          class="w-4 h-4"
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linejoin="round"
            stroke-width="1.5"
            d="M12 22C6.477 22 2 17.523 2 12a9.97 9.97 0 0 1 2.99-7.132M12 22c-.963-.714-.81-1.544-.326-2.375c.743-1.278.743-1.278.743-2.98c0-1.704 1.012-2.502 4.583-1.788c1.605.321 2.774-1.896 4.857-1.164M12 22c4.946 0 9.053-3.59 9.857-8.307m0 0Q22 12.867 22 12c0-4.881-3.498-8.946-8.123-9.824m0 0c.51.94.305 2.06-.774 2.487c-1.76.697-.5 1.98-2 2.773c-1 .528-2.499.396-3.998-1.189c-.79-.834-1.265-1.29-2.115-1.379m8.887-2.692A10 10 0 0 0 12 2a9.97 9.97 0 0 0-7.01 2.868"
          />
        </svg>
        <span class="text-sm text-base-content/80">
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
          fill="none"
          stroke="currentColor"
          viewBox="0 0 24 24"
        >
          <path
            stroke-linecap="round"
            stroke-linejoin="round"
            stroke-width="2"
            d="M8 9l4-4 4 4m0 6l-4 4-4-4"
          ></path>
        </svg>
      </div>
    </button>
  </div>
</div>

<style>
  .datetime-popover {
    /* Layout & Positioning */
    @apply bg-base-100 border border-base-300 rounded-md shadow-lg;
    @apply px-0 py-2 mx-0 my-2;
    position-anchor: --datetime-anchor;
    top: anchor(--datetime-anchor bottom);
    left: anchor(--datetime-anchor left);
    width: anchor(--datetime-anchor width);

    /* Visual Styling */
    @apply min-w-96 text-base-content;
  }

  .timezone-toggle {
    @apply w-full flex items-center justify-between hover:bg-base-200 transition-colors rounded p-2;
  }

  /* Custom tooltip positioning for bottom-right */
  .tooltip-bottom-right {
    position: relative;
  }

  .tooltip-bottom-right::before {
    transform: translateX(-10px) !important;
  }
</style>
