<script lang="ts">
  import type { TraceSummary } from '@/types/api-types'
  import {
    formatDurationParts,
    formatTimestampParts,
    traceSummaryDurationNs,
  } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import SignalCard from '@/components/SignalCard.svelte'

  type Props = {
    trace: TraceSummary
    selected?: boolean
    onclick?: (id: string) => void
  }

  let { trace, selected = false, onclick }: Props = $props()

  const timeContext = getTimeContext()

  let startParts = $derived(
    formatTimestampParts(
      trace.startTime,
      timeContext.timezone,
      'milliseconds'
    )
  )

  let titleLabel = $derived(trace.rootSpan?.name ?? 'No root span yet')
  let serviceLabel = $derived(trace.rootSpan?.serviceName)
  let titleMuted = $derived(!trace.hasRootSpan)

  let durationParts = $derived.by(() => {
    const ns = traceSummaryDurationNs(trace)
    return ns !== undefined ? formatDurationParts(ns) : null
  })
</script>

<SignalCard
  id={trace.traceID}
  {selected}
  title={titleLabel}
  subtitle={serviceLabel}
  description={trace.traceID}
  {titleMuted}
  timeLayout="interval"
  timestamp={startParts.value}
  timestampUnit={startParts.unit || undefined}
  duration={durationParts?.value}
  durationUnit={durationParts?.unit}
  {onclick}
>
  {#snippet badge()}
    <span class="badge badge-xs badge-soft badge-neutral tabular-nums">
      {trace.spanCount} span{trace.spanCount !== 1 ? 's' : ''}
    </span>
    {#if trace.errorCount > 0}
      <span class="badge badge-xs badge-soft badge-error tabular-nums">
        {trace.errorCount} err
      </span>
    {/if}
  {/snippet}
</SignalCard>
