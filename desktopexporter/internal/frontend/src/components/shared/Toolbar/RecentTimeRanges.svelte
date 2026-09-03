<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import { DateTimeIcon } from '@/icons'
  import {
    formatDateTimeMs,
    loadRecentTimeRanges,
    MAX_RECENT_TIME_RANGES,
    type RecentTimeRange,
  } from '@/utils/time'

  let ctx = getTimeContext()
  if (!ctx) {
    throw new Error(
      'Time context not found. Make sure createTimeContext() is called at the root level.'
    )
  }

  let recentTimeRanges = $state<RecentTimeRange[]>([])

  $effect(() => {
    void ctx.selection
    recentTimeRanges = loadRecentTimeRanges().slice(0, MAX_RECENT_TIME_RANGES)
  })

  function applyRecentTimeRange(index: number) {
    let entry = recentTimeRanges[index]
    if (!entry) return
    ctx.setSelection({ type: 'recent', start: entry.start, end: entry.end })
  }
</script>

<FieldGroup label="Recently Used">
  {#snippet heading()}
    <DateTimeIcon class="h-3.5 w-3.5 shrink-0 text-base-content/55" />
    <span>Recently Used</span>
    {#if recentTimeRanges.length > 0}
      <span class="badge-count">{recentTimeRanges.length}</span>
    {/if}
  {/snippet}
  {#if recentTimeRanges.length === 0}
    <div class="recent-range-empty">No recent time ranges</div>
  {:else}
    <ul class="recent-range-list" aria-label="Recently used time ranges">
      {#each recentTimeRanges as entry, index}
        {@const startFmt = formatDateTimeMs(entry.start, ctx.tz)}
        {@const endFmt = formatDateTimeMs(entry.end, ctx.tz)}
        {@const active =
          ctx.selection.type === 'recent' &&
          entry.start === ctx.selection.start &&
          entry.end === ctx.selection.end}
        <li class="recent-range-item">
          <button
            type="button"
            class="recent-range-button"
            class:recent-range-button--active={active}
            aria-pressed={active}
            onclick={() => applyRecentTimeRange(index)}
          >
            <span class="recent-range-endpoint">
              <span class="recent-range-label">Start</span>
              <span class="recent-range-value">{startFmt.dateTime}</span>
            </span>
            <span class="recent-range-endpoint">
              <span class="recent-range-label">End</span>
              <span class="recent-range-value">{endFmt.dateTime}</span>
            </span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</FieldGroup>

<style lang="postcss">
  @reference "../../../app.css";

  .recent-range-empty {
    @apply py-2 text-sm text-base-content/60;
  }

  .recent-range-list {
    margin-block: 0;
    margin-inline: calc(var(--fg-inline) * -1);
    @apply list-none border-y border-base-300/50 bg-base-100/30 p-0;
  }

  .recent-range-item + .recent-range-item {
    @apply border-t border-base-300/50;
  }

  .recent-range-button {
    box-sizing: border-box;
    padding-inline: var(--fg-inline);
    @apply grid w-full grid-cols-[max-content_minmax(0,1fr)] gap-x-3 gap-y-0.5 rounded-none border-none bg-transparent py-1.5 text-left leading-snug;
    @apply text-base-content transition-colors duration-150;
    @apply focus-visible:outline-none;
    @apply cursor-pointer;
  }

  .recent-range-button:hover,
  .recent-range-button:focus-visible {
    @apply bg-base-300/40;
  }

  .recent-range-button:focus-visible {
    box-shadow: inset 0 0 0 var(--focus-ring-width) var(--focus-ring-color);
  }

  .recent-range-button--active {
    @apply bg-primary/10 text-primary;
  }

  .recent-range-button--active:hover,
  .recent-range-button--active:focus-visible {
    @apply bg-primary/15;
  }

  .recent-range-endpoint {
    display: contents;
  }

  .recent-range-label {
    @apply text-[0.625rem] font-medium text-base-content/45;
  }

  .recent-range-value {
    @apply min-w-0 truncate text-left font-mono text-xs tracking-tight tabular-nums;
  }
</style>
