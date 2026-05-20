<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte';
  import FieldGroup from '@/components/shared/FieldGroup.svelte';
  import { DateTimeIcon } from '@/icons';
  import {
    formatDateTimeMs,
    loadRecentTimeRanges,
    MAX_RECENT_TIME_RANGES,
    type RecentTimeRange,
  } from '@/utils/time';

  let ctx = getTimeContext();
  if (!ctx) {
    throw new Error(
      'Time context not found. Make sure createTimeContext() is called at the root level.'
    );
  }

  let { last = false }: { last?: boolean } = $props();

  let recentTimeRanges = $state<RecentTimeRange[]>([]);

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

<FieldGroup label="Recently Used" {last}>
  {#snippet heading()}
    <DateTimeIcon class="h-3.5 w-3.5 shrink-0 text-base-content/55" />
    <span>Recently Used</span>
    {#if recentTimeRanges.length > 0}
      <span class="badge-count">{recentTimeRanges.length}</span>
    {/if}
  {/snippet}
  {#if recentTimeRanges.length === 0}
    <div class="recent-range-empty">
      No recent time ranges
    </div>
  {:else}
    {#each recentTimeRanges as entry, index}
      {@const startFmt = formatDateTimeMs(entry.start, ctx.timezone)}
      {@const endFmt = formatDateTimeMs(entry.end, ctx.timezone)}
      <button
        class="recent-range-button"
        class:recent-range-button--active={ctx.selection.type === 'recent' &&
          entry.start === ctx.selection.start &&
          entry.end === ctx.selection.end}
        onclick={() => applyRecentTimeRange(index)}
      >
        <span class="recent-range-value">{startFmt.dateTime}</span>
        <span class="recent-range-sep" aria-hidden="true">-</span>
        <span class="recent-range-value">{endFmt.dateTime}</span>
      </button>
    {/each}
  {/if}
</FieldGroup>

<style lang="postcss">
  @reference "../../../app.css";

  .recent-range-empty {
    @apply py-2 text-sm text-base-content/60;
  }

  .recent-range-button {
    box-sizing: border-box;
    min-height: var(--table-row-h);
    @apply flex w-full items-center gap-1.5 whitespace-nowrap rounded-none border-none bg-transparent px-0 py-0 text-left text-sm leading-snug;
    @apply text-base-content transition-colors duration-150;
    @apply focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30 focus-visible:ring-offset-0;
    @apply cursor-pointer;
  }

  .recent-range-button:hover,
  .recent-range-button:focus-visible {
    @apply bg-base-300/40;
  }

  .recent-range-button--active {
    @apply text-primary;
  }

  .recent-range-value {
    @apply text-xs font-mono tracking-tight tabular-nums;
  }

  .recent-range-sep {
    @apply text-xs;
    color: var(--color-subtle);
  }
</style>
