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

<div class="pl-2">
  <div
    class="section-header mb-2 flex items-center text-sm font-semibold text-base-content"
  >
    <svg
      class="w-4 h-4 mr-2"
      viewBox="0 0 24 24"
    >
      <g>
        <path
          d="M19 10.5V10c0-3.771 0-5.657-1.172-6.828S14.771 2 11 2S5.343 2 4.172 3.172S3 6.229 3 10v4.5c0 3.287 0 4.931.908 6.038q.25.304.554.554C5.57 22 7.212 22 10.5 22M7 7h8m-8 4h4"
        />
        <path
          d="m18 18.5l-1.5-.55V15.5m-4.5 2a4.5 4.5 0 1 0 9 0a4.5 4.5 0 0 0-9 0"
        />
      </g>
    </svg>
    Recently Used
  </div>
  <div class="space-y-0 max-h-[84px] overflow-y-auto pr-2">
    {#if recentTimeRanges.length === 0}
      <div class="w-full text-left py-1 text-sm text-base-content/60">
        No recent time ranges
      </div>
    {:else}
      {#each recentTimeRanges as entry, index}
        <button
          class="recent-range-button"
          onclick={() => applyRecentTimeRange(index)}
        >
          {#if ctx.selection.type === 'recent' && index === 0}
            <svg class="w-4 h-4 mr-2 text-primary" viewBox="0 0 24 24">
              <path d="m5 14l3.5 3.5L19 6.5" />
            </svg>
          {:else}
            <span class="w-4 h-4 mr-2"></span>
          {/if}
          <span
            >{formatDateTimeRange(entry.start, entry.end, ctx.timezone)}</span
          >
        </button>
      {/each}
    {/if}
  </div>
</div>

<style lang="postcss">
  /* Match .timezone-toggle inner rhythm: section p-2 + control px-2 */
  .recent-range-button {
    @apply w-full text-left px-2 py-2 text-sm rounded;
    @apply flex items-center gap-0 hover:bg-base-200 focus:bg-base-200 transition-colors;
    @apply border-none bg-transparent cursor-pointer;
  }
</style>
