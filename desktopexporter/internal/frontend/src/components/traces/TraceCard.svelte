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
  const services = $derived(
    trace.serviceBreakdown?.some(part => part.count > 0)
      ? trace.serviceBreakdown
      : serviceLabel === undefined
        ? []
        : [
            {
              key: `service:${serviceLabel}`,
              label: serviceLabel,
              count: trace.spanCount,
            },
          ]
  )

  let durationParts = $derived.by(() => {
    const ns = traceSummaryDurationNs(trace)
    return ns !== undefined ? formatDurationParts(ns) : null
  })
</script>

{#snippet serviceBreakdown()}
  <SummaryBreakdown parts={services} rootService={serviceLabel} />
{/snippet}

<SignalCard
  id={trace.traceID}
  {selected}
  title={titleLabel}
  subtitle={serviceLabel}
  interactiveSubtitle={services.length ? serviceBreakdown : undefined}
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
    <div
      class="flex min-w-0 flex-wrap items-center justify-between gap-x-4 gap-y-1"
    >
      <span
        class="flex min-w-0 max-w-full items-center gap-1"
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
          <span class="text-base-content">{durationParts.value}</span>
          <span class="text-base-content">{durationParts.unit}</span>
        </span>
      {/if}
    </div>
  {/snippet}

  {#snippet meta()}
    <span class="flex w-full min-w-0 items-center gap-1 pt-1 text-xs">
      <span class="shrink-0 font-mono">trace_id:</span>
      <span class="truncate font-mono text-base-content" title={trace.traceID}>
        <Identifier value={trace.traceID} />
      </span>
    </span>
  {/snippet}
</SignalCard>
