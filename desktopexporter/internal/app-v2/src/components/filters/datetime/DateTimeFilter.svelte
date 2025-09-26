<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte';
  import { getLocalTimezoneName } from '@/utils/time';
  import PresetTimeRanges from './PresetTimeRanges.svelte';
  import CustomTimeRange from './CustomTimeRange.svelte';
  import RecentTimeRanges from './RecentTimeRanges.svelte';
  import '@/components/filters/filters.css';

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
</script>

<div class="relative">
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
    <!-- Narrow Layout: Tabs -->
    <div class="tabs tabs-border">
      <input
        type="radio"
        name="datetime_tabs"
        class="tab"
        aria-label="Quick Select"
        checked={activeTab === 'preset'}
        onchange={() => (activeTab = 'preset')}
      />
      <div class="tab-content">
        <PresetTimeRanges />
      </div>

      <input
        type="radio"
        name="datetime_tabs"
        class="tab"
        aria-label="Custom Range"
        checked={activeTab === 'custom'}
        onchange={() => (activeTab = 'custom')}
      />
      <div class="tab-content">
        <CustomTimeRange {isNarrow} />
      </div>

      <input
        type="radio"
        name="datetime_tabs"
        class="tab"
        aria-label="Recent"
        checked={activeTab === 'recent'}
        onchange={() => (activeTab = 'recent')}
      />
      <div class="tab-content">
        <RecentTimeRanges />
      </div>
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
      <div class="text-sm text-base-content/80">
        {ctx.timezone === 'UTC'
          ? 'Coordinated Universal Time UTC'
          : getLocalTimezoneName()}
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
  .timezone-toggle {
    @apply w-full flex items-center justify-between hover:bg-base-200 transition-colors rounded p-2;
  }
</style>
