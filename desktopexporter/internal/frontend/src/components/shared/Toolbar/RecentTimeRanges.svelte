<script lang="ts">
  import { tick } from 'svelte'
  import { HugeiconsIcon } from '@hugeicons/svelte'
  import Cancel01Icon from '@hugeicons/core-free-icons/Cancel01Icon'
  import DateTimeIcon from '@hugeicons/core-free-icons/DateTimeIcon'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import FieldGroup from '@/components/shared/FieldGroup.svelte'
  import {
    formatDateTimeMs,
    loadRecentTimeRanges,
    removeRecentTimeRange,
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
    recentTimeRanges = loadRecentTimeRanges()
  })

  function applyRecentTimeRange(index: number) {
    const entry = recentTimeRanges[index]
    if (!entry) return
    ctx.setSelection({ type: 'recent', start: entry.start, end: entry.end })
  }

  async function removeRecentRange(index: number, target: HTMLButtonElement) {
    const entry = recentTimeRanges[index]
    if (!entry) return
    const section = target.closest('details')
    recentTimeRanges = removeRecentTimeRange(entry.start, entry.end)
    await tick()

    const removeButtons = section?.querySelectorAll<HTMLButtonElement>(
      '.recent-range-remove'
    )
    const nextRemove =
      removeButtons?.[Math.min(index, removeButtons.length - 1)]
    const focusTarget =
      nextRemove ?? section?.querySelector<HTMLElement>(':scope > summary')
    focusTarget?.focus()
  }
</script>

<FieldGroup
  label="Recent"
  count={recentTimeRanges.length > 0 ? recentTimeRanges.length : undefined}
>
  {#snippet icon()}
    <HugeiconsIcon
      icon={DateTimeIcon}
      size={14}
      strokeWidth={1.5}
      class="shrink-0 text-base-content/55"
      aria-hidden="true"
    />
  {/snippet}
  {#if recentTimeRanges.length === 0}
    <div class="recent-range-empty">No recent time ranges</div>
  {:else}
    <ul class="recent-range-list" aria-label="Recent time ranges">
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
            <span class="recent-range-label">Start</span>
            <span class="recent-range-value">{startFmt.dateTime}</span>
            <span class="recent-range-label">End</span>
            <span class="recent-range-value">{endFmt.dateTime}</span>
          </button>
          <button
            type="button"
            class="recent-range-remove tooltip tooltip-left"
            aria-label="Remove recent range from {startFmt.dateTime} to {endFmt.dateTime}"
            data-tip="Remove recent time range"
            onclick={event => removeRecentRange(index, event.currentTarget)}
          >
            <HugeiconsIcon
              icon={Cancel01Icon}
              size={12}
              strokeWidth={1.5}
              aria-hidden="true"
            />
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

  .recent-range-item {
    @apply relative;
  }

  .recent-range-button {
    box-sizing: border-box;
    padding-inline-start: var(--fg-inline);
    padding-inline-end: calc(var(--fg-inline) + 1.5rem);
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

  .recent-range-remove {
    /* Center the 24px hit target beneath the 14px caret. */
    right: calc(var(--fg-inline) - 0.3125rem);
    top: 50%;
    transform: translateY(-50%);
    @apply absolute z-10 flex h-6 w-6 items-center justify-center rounded-full border-0 bg-transparent p-0 text-base-content/70;
    @apply hover:bg-base-300/60 hover:text-error focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/35;
  }

  .recent-range-label {
    @apply text-[0.625rem] font-medium text-base-content/45;
  }

  .recent-range-value {
    @apply min-w-0 truncate text-center font-mono text-xs tracking-tight tabular-nums;
  }
</style>
