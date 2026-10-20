<script lang="ts">
  import type { SpanTiming } from '@/types/api-types'
  import { formatDurationParts, formatTimestampParts } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import SignalBadges from '@/components/shared/SignalBadges.svelte'

  let {
    span,
    onclick,
  }: {
    span: SpanTiming
    onclick?: (id: string) => void
  } = $props()

  const timeContext = getTimeContext()
  const startParts = $derived(
    formatTimestampParts(span.startTime, timeContext.tz, 'milliseconds')
  )
  const durationParts = $derived(
    span.endTime >= span.startTime
      ? formatDurationParts(span.endTime - span.startTime)
      : null
  )
</script>

<SignalCard
  id={span.spanID}
  title={span.name}
  subtitle={span.serviceName ? `(${span.serviceName})` : undefined}
  idLine={span.spanID}
  {onclick}
>
  {#snippet badge()}
    <SignalBadges
      signal="trace"
      spanCount={1}
      errorCount={span.statusCodeValue === 2 ? 1 : 0}
    />
  {/snippet}
  {#snippet date()}
    <div
      class="flex min-w-0 flex-wrap items-center justify-between gap-x-4 gap-y-1"
    >
      <span
        class="flex min-w-0 max-w-full items-center gap-1"
        title={`Start: ${startParts.value} ${startParts.unit}`}
      >
        <span class="truncate text-base-content">{startParts.value}</span>
      </span>
      {#if durationParts}
        <span class="flex shrink-0 items-center gap-1">
          <span>Duration:</span>
          <span class="text-base-content">{durationParts.value}</span>
          <span class="text-base-content">{durationParts.unit}</span>
        </span>
      {/if}
    </div>
  {/snippet}
</SignalCard>
