<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatDateTimeParts } from '@/utils/time'

  type Props = {
    startMs: number
    endMs: number
    class?: string
  }

  let { startMs, endMs, class: className = '' }: Props = $props()

  const timeContext = getTimeContext()

  let start = $derived(formatDateTimeParts(startMs, timeContext.tz))
  let end = $derived(formatDateTimeParts(endMs, timeContext.tz))
  let sharedDate = $derived(start.date === end.date)
  let sharedTimezone = $derived(start.timezone === end.timezone)
</script>

{#snippet endpoint(
  label: string,
  ms: number,
  parts: ReturnType<typeof formatDateTimeParts>
)}
  <span class="time-range__endpoint">
    <span class="time-range__label" aria-hidden="true">{label}</span>
    <time
      datetime={new Date(ms).toISOString()}
      title={`${parts.date} ${parts.time} ${parts.timezone}`}
      aria-label={`${label}: ${parts.date} ${parts.time} ${parts.timezone}`}
    >
      {#if !sharedDate}<span class="time-range__date">{parts.date}</span>{/if}
      <span class="time-range__clock"
        >{parts.time.slice(0, -4)}<span class="time-range__fraction"
          >{parts.time.slice(-4)}</span
        ></span
      >
      {#if !sharedTimezone}<span class="time-range__zone">{parts.timezone}</span
        >{/if}
    </time>
  </span>
{/snippet}

<div class="time-range {className}" aria-label="Chart time range">
  <div class="time-range__bounds">
    {@render endpoint('Start', startMs, start)}
    <span class="time-range__arrow" aria-hidden="true">→</span>
    {@render endpoint('End', endMs, end)}
  </div>
  {#if sharedDate || sharedTimezone}
    <span class="time-range__context">
      {#if sharedDate}<span>{start.date}</span>{/if}
      {#if sharedDate && sharedTimezone}<span aria-hidden="true">·</span>{/if}
      {#if sharedTimezone}<span>{start.timezone}</span>{/if}
    </span>
  {/if}
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .time-range {
    @apply flex min-w-0 flex-wrap items-baseline gap-x-4 gap-y-1 py-1.5 text-xs;
  }

  .time-range__bounds {
    @apply flex min-w-0 flex-wrap items-baseline gap-x-2 gap-y-1;
  }

  .time-range__endpoint {
    @apply inline-flex items-baseline gap-1.5;
  }

  .time-range__label {
    color: var(--color-subtle);
    font-size: 0.625rem;
  }

  time {
    @apply inline-flex flex-wrap items-baseline gap-x-1.5 gap-y-0.5 tabular-nums;
  }

  .time-range__clock {
    @apply whitespace-nowrap font-medium text-base-content;
  }

  .time-range__fraction {
    @apply font-normal;
    color: var(--color-subtle);
  }

  .time-range__context {
    @apply inline-flex items-baseline gap-1.5 whitespace-nowrap tabular-nums;
    color: var(--color-subtle);
  }

  .time-range__date,
  .time-range__zone,
  .time-range__arrow {
    color: var(--color-subtle);
  }
</style>
