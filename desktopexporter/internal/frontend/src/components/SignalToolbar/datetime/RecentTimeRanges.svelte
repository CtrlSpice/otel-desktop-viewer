<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte';
  import {
    formatDateTimeRange,
    loadRecentTimeRanges,
    MAX_RECENT_TIME_RANGES,
    type RecentTimeRange,
  } from '@/utils/time';

  // Get time context
  let ctx = getTimeContext();
  if (!ctx) {
    throw new Error(
      'Time context not found. Make sure createTimeContext() is called at the root level.'
    );
  }

  let recentTimeRanges = $state<RecentTimeRange[]>([]);

  // Keep list in sync when selection changes (setSelection writes localStorage recents)
  $effect(() => {
    void ctx.selection.start;
    void ctx.selection.end;
    void ctx.selection.type;
    recentTimeRanges = loadRecentTimeRanges().slice(0, MAX_RECENT_TIME_RANGES);
  });

  function applyRecentTimeRange(index: number) {
    let entry = recentTimeRanges[index];
    if (!entry) return;
    ctx.setSelection(entry.start, entry.end, 'recent');
  }
</script>

<div class="min-w-0 w-full">
  <div class="recent-section-heading px-3">
    <span class="table-header-typography">Recently Used</span>
  </div>
  <div class="recent-ranges-list space-y-0">
    {#if recentTimeRanges.length === 0}
      <div
        class="recent-range-empty flex w-full items-center px-3 text-left text-sm text-base-content/60"
      >
        No recent time ranges
      </div>
    {:else}
      {#each recentTimeRanges as entry, index}
        <button
          class="recent-range-button"
          class:recent-range-button--active={ctx.selection.type === 'recent' &&
            entry.start === ctx.selection.start &&
            entry.end === ctx.selection.end}
          onclick={() => applyRecentTimeRange(index)}
        >
          <span class="min-w-0 truncate">
            {formatDateTimeRange(entry.start, entry.end, ctx.timezone)}
          </span>
        </button>
      {/each}
    {/if}
  </div>
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .recent-section-heading {
    box-sizing: border-box;
    display: flex;
    align-items: center;
    height: var(--table-header-h);
    min-height: var(--table-header-h);
    @apply py-1;
  }

  .recent-ranges-list > :nth-child(odd) {
    background-color: var(--table-zebra-bg);
  }

  .recent-range-empty {
    box-sizing: border-box;
    height: var(--table-row-h);
    min-height: var(--table-row-h);
  }

  .recent-range-button {
    box-sizing: border-box;
    height: var(--table-row-h);
    min-height: var(--table-row-h);
    @apply flex w-full items-center rounded-none border-none bg-transparent px-3 py-0 text-left text-sm leading-snug;
    @apply text-base-content/90 transition-colors duration-150;
    @apply focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30 focus-visible:ring-offset-0;
    @apply cursor-pointer;
  }

  .recent-range-button:hover,
  .recent-range-button:focus-visible {
    background-color: var(--table-hover-bg);
  }

  .recent-range-button--active {
    @apply bg-primary/20 font-semibold text-primary;
    @apply ring-1 ring-inset ring-primary/20;
  }

  .recent-range-button--active:hover {
    @apply bg-primary/30;
  }
</style>
