<script lang="ts">
  import type { TraceSummary } from '@/types/api-types'
  import {
    formatDuration,
    formatTimestamp,
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

  let startLabel = $derived(
    trace.rootSpan
      ? formatTimestamp(
          trace.rootSpan.startTime,
          timeContext.timezone,
          'milliseconds'
        )
      : ''
  )

  let service = $derived(trace.rootSpan?.serviceName)

  let durationLabel = $derived.by(() => {
    const ns = traceSummaryDurationNs(trace)
    return ns !== undefined ? formatDuration(ns) : ''
  })
</script>

<SignalCard
  id={trace.traceID}
  {selected}
  title={trace.rootSpan?.name || trace.traceID}
  subtitle={service || undefined}
  timestamp={startLabel || undefined}
  duration={durationLabel || undefined}
  {onclick}
>
  {#snippet badge()}
    <span class="badge badge-xs badge-soft badge-neutral">
      {trace.spanCount} span{trace.spanCount !== 1 ? 's' : ''}
    </span>
    {#if trace.errorCount > 0}
      <span class="badge badge-xs badge-soft badge-error ml-1">
        {trace.errorCount} err
      </span>
    {/if}
  {/snippet}
  {#snippet meta()}
    <span class="font-mono tabular-nums text-base-content/45" title={trace.traceID}>
      {trace.traceID}
    </span>
  {/snippet}
</SignalCard>
