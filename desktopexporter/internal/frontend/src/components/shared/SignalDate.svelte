<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatTimestampParts } from '@/utils/time'

  let {
    label,
    timestamp,
    unavailable = 'Unavailable',
    suffix,
  }: {
    label: string
    timestamp?: bigint
    unavailable?: string
    suffix?: string
  } = $props()
  const timeContext = getTimeContext()
  const parts = $derived(
    timestamp === undefined
      ? undefined
      : formatTimestampParts(timestamp, timeContext.tz, 'milliseconds')
  )
</script>

<div
  class="signal-date"
  title={parts
    ? `${label}: ${parts.value} ${parts.unit}`
    : `${label}: ${unavailable}`}
>
  <span class="signal-date__label">{label}:</span>
  <span class="min-w-0 truncate text-base-content"
    >{parts?.value ?? '—'}{#if suffix}
      <span class="text-rp-subtle">{' '}{suffix}</span>{/if}</span
  >
</div>

<style lang="postcss">
  @reference "../../app.css";
  .signal-date {
    @apply grid min-w-0 items-center gap-x-2 text-sm tabular-nums;
    grid-template-columns: max-content minmax(0, 1fr);
    min-height: var(--signal-card-row-h);
  }
  .signal-date__label {
    color: var(--color-subtle);
  }
</style>
