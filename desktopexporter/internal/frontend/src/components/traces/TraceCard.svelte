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
  import Identifier from '@/components/shared/Identifier.svelte'
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
  const services = $derived(trace.serviceBreakdown ?? [])
  const hasServiceGrid = $derived(
    services.filter(part => part.count > 0).length > 1
  )

  let durationParts = $derived.by(() => {
    const ns = traceSummaryDurationNs(trace)
    return ns !== undefined ? formatDurationParts(ns) : null
  })
</script>

{#snippet serviceGrid()}
  <SummaryBreakdown parts={services} rootService={serviceLabel} />
{/snippet}

<SignalCard
  id={trace.traceID}
  {selected}
  title={titleLabel}
  subtitle={serviceLabel}
  subtitleContent={hasServiceGrid ? serviceGrid : undefined}
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
  {#snippet date()}
    <span
      class="flex min-w-0 items-center gap-1"
      title={`Start: ${startParts.value} ${startParts.unit}`}
    >
      <span class="shrink-0">Start:</span>
      <span class="truncate text-base-content">{startParts.value}</span>
      {#if startParts.unit}<span class="shrink-0">{startParts.unit}</span>{/if}
    </span>
  {/snippet}

  {#snippet meta()}
    <div class="flex w-full min-w-0 flex-col gap-2 pt-1">
      <div
        class="grid min-w-0 grid-cols-2 gap-x-4 gap-y-1.5 text-xs tabular-nums"
      >
        {#if durationParts}
          <span class="flex min-w-0 items-center gap-1">
            <span class="shrink-0">Duration:</span>
            <span class="truncate text-base-content">{durationParts.value}</span
            >
            <span class="shrink-0 text-base-content">{durationParts.unit}</span>
          </span>
        {/if}
        <span class="flex min-w-0 items-center gap-1">
          <span class="shrink-0" title="Trace ID">ID:</span>
          <span
            class="truncate font-mono text-base-content"
            title={trace.traceID}
          >
            <Identifier value={trace.traceID} />
          </span>
        </span>
      </div>
    </div>
  {/snippet}
</SignalCard>
