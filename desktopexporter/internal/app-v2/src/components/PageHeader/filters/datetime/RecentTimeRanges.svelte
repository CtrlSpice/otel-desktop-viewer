<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte';
  import { formatDateTimeRange } from '@/utils/time';
  import '@/components/PageHeader/PageHeader.css';

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
  <div class="section-header--hide-narrow">
    <svg
      class="w-4 h-4 mr-2"
      fill="none"
      stroke="currentColor"
      viewBox="0 0 24 24"
    >
      <g stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5">
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
