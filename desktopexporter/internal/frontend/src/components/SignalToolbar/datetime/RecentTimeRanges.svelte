<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte';
  import { formatDateTimeRange } from '@/utils/time';
  import {
    loadRecentTimeRanges,
    type RecentTimeRange,
  } from '@/utils/recent-time-ranges';

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
    recentTimeRanges = loadRecentTimeRanges();
  });

  function applyRecentTimeRange(index: number) {
    let entry = recentTimeRanges[index];
    if (!entry) return;
    ctx.setSelection(entry.start, entry.end, 'recent');
  }
</script>

<div class="min-w-0">
  <div class="section-header mb-1 text-xs font-semibold uppercase tracking-wide text-base-content/70">
    Recently Used
  </div>
  <div class="max-h-[80px] space-y-0 overflow-y-auto">
    {#if recentTimeRanges.length === 0}
      <div class="w-full px-1.5 py-0.5 text-left text-xs text-base-content/60">
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
          {formatDateTimeRange(entry.start, entry.end, ctx.timezone)}
        </button>
      {/each}
    {/if}
  </div>
</div>

<style lang="postcss">
  @reference "../../../app.css";
  .recent-range-button {
    @apply block w-full rounded-md border-none bg-transparent px-1.5 py-1 text-left text-xs leading-snug;
    @apply text-base-content/90 transition-colors;
    @apply hover:bg-base-200/90 focus-visible:bg-base-200/90;
    @apply focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30 focus-visible:ring-offset-0;
    @apply cursor-pointer;
  }

  .recent-range-button--active {
    @apply bg-primary/20 font-semibold text-primary;
    @apply ring-1 ring-inset ring-primary/20;
  }

  .recent-range-button--active:hover {
    @apply bg-primary/30;
  }
</style>
