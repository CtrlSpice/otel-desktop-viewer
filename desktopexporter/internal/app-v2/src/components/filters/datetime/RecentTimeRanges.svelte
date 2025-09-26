<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte';
  import { formatDateTimeRange } from '@/utils/time';
  import '@/components/filters/filters.css';

  // Get time context
  let ctx = getTimeContext();
  if (!ctx) {
    throw new Error(
      'Time context not found. Make sure createTimeContext() is called at the root level.'
    );
  }

  // ===== RECENT TIME RANGES =====
  interface RecentTimeRange {
    start: number;
    end: number;
    usedAt: number;
  }

  // Recently used time ranges
  let recentTimeRanges = $state<RecentTimeRange[]>([]);

  // Load recent ranges from localStorage
  $effect(() => {
    let saved = localStorage.getItem('datetime-filter-recent');
    if (saved) {
      recentTimeRanges = JSON.parse(saved);
    } else {
      recentTimeRanges = [];
    }
  });

  function updateRecentRanges(start: number, end: number, usedAt: number) {
    // Check if this time range already exists
    let existingIndex = recentTimeRanges.findIndex(
      entry => entry.start === start && entry.end === end
    );

    if (existingIndex !== -1) {
      // Update existing entry
      let updated = [...recentTimeRanges];
      updated[existingIndex] = { ...updated[existingIndex], usedAt };
      let sorted = updated.sort((a, b) => b.usedAt - a.usedAt);
      recentTimeRanges = sorted;
    } else {
      // Add new entry
      let updated = [{ start, end, usedAt }, ...recentTimeRanges]
        .sort((a, b) => b.usedAt - a.usedAt)
        .slice(0, 10);
      recentTimeRanges = updated;
    }

    // Save to localStorage
    localStorage.setItem(
      'datetime-filter-recent',
      JSON.stringify(recentTimeRanges)
    );
  }

  function applyRecentTimeRange(index: number) {
    let entry = recentTimeRanges[index];
    if (!entry) return;

    let now = Date.now();

    // First: Update the recent ranges (move to top)
    updateRecentRanges(entry.start, entry.end, now);

    // Then: Update the time context
    ctx.setSelection(entry.start, entry.end, 'recent');
  }

  // Expose updateRecentRanges for use by parent components
  export { updateRecentRanges };
</script>

<div class="space-y-3">
  <div class="section-header--hide-narrow">Recently Used</div>
  <div class="space-y-0">
    {#if recentTimeRanges.length === 0}
      <div class="w-full text-left px-2 py-1 text-sm text-base-content/60">
        No recent time ranges
      </div>
    {:else}
      {#each recentTimeRanges as entry, index}
        <button
          class="list-button {ctx.selection.type === 'recent' && index === 0
            ? 'selection-indicator--active'
            : ''}"
          onclick={() => applyRecentTimeRange(index)}
        >
          <span
            >{formatDateTimeRange(entry.start, entry.end, ctx.timezone)}</span
          >
        </button>
      {/each}
    {/if}
  </div>
</div>
