<script lang="ts">
  import type { TraceSummary } from '@/types/api-types'
  import {
    formatDurationParts,
    formatTimestampParts,
    traceSummaryDurationNs,
  } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import SignalBadges from '@/components/shared/SignalBadges.svelte'
  import SummaryBreakdown from '@/components/shared/SummaryBreakdown.svelte'

  type Props = {
    trace: TraceSummary
    selected?: boolean
    onclick?: (id: string) => void
  }

  let { trace, selected = false, onclick }: Props = $props()

  const timeContext = getTimeContext()

  let startParts = $derived(
    formatTimestampParts(trace.startTime, timeContext.tz, 'milliseconds')
  )

  // Not "yet": a promoted-orphan or salvaged-cycle trace has no null-parent
  // span and is not waiting for one.
  let titleLabel = $derived(trace.rootSpan?.name ?? 'No root span')
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
  descriptionIsID
  {titleMuted}
  {onclick}
>
  {#snippet badge()}
    <SignalBadges
      signal="trace"
      spanCount={trace.spanCount}
      errorCount={trace.errorCount}
    />
  {/snippet}
  {#snippet meta()}
    <div class="flex w-full min-w-0 flex-col gap-2 pt-1">
      <div
        class="flex min-w-0 items-center justify-between gap-3 text-xs tabular-nums"
      >
        <span
          class="flex min-w-0 items-center gap-1"
          title={`Start: ${startParts.value} ${startParts.unit}`}
        >
          <span class="shrink-0">Start:</span>
          <span class="truncate text-base-content">{startParts.value}</span>
          {#if startParts.unit}<span class="shrink-0">{startParts.unit}</span
            >{/if}
        </span>
        {#if durationParts}
          <span class="flex shrink-0 items-center gap-1">
            <span>Duration:</span>
            <span class="text-primary">{durationParts.value}</span>
            <span class="text-primary">{durationParts.unit}</span>
          </span>
        {/if}
      </div>
      <SummaryBreakdown signal="trace" parts={trace.serviceBreakdown ?? []} />
    </div>
  {/snippet}
</SignalCard>
