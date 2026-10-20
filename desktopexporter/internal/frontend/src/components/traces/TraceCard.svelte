<script lang="ts">
  import type { TraceSummary } from '@/types/api-types'
  import { formatDurationParts, traceSummaryDurationNs } from '@/utils/time'
  import SignalDate from '@/components/shared/SignalDate.svelte'
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import SignalBadges from '@/components/shared/SignalBadges.svelte'
  import SummaryBreakdown from '@/components/shared/SummaryBreakdown.svelte'

  type Props = {
    trace: TraceSummary
    selected?: boolean
    onclick?: (id: string) => void
  }

  let { trace, selected = false, onclick }: Props = $props()

  // Not "yet": a promoted-orphan or salvaged-cycle trace has no null-parent
  // span and is not waiting for one.
  let titleLabel = $derived(trace.rootSpan?.name ?? 'No root span')
  let serviceLabel = $derived(trace.rootSpan?.serviceName)
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
  <SummaryBreakdown parts={services} rootService={serviceLabel} tooltipOnly />
{/snippet}

<SignalCard
  id={trace.traceID}
  {selected}
  title={titleLabel}
  subtitle={serviceLabel}
  interactiveSubtitle={serviceBreakdown}
  titleContent={!trace.hasRootSpan ? missingRootTitle : undefined}
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
    <SignalDate label="Started" timestamp={trace.startTime} />
  {/snippet}
  {#snippet meta()}
    {#if durationParts}
      <span class="flex shrink-0 items-center gap-1">
        <span>Duration:</span>
        <span class="text-base-content">{durationParts.value}</span>
        <span class="text-base-content">{durationParts.unit}</span>
      </span>
    {/if}
  {/snippet}
</SignalCard>

{#snippet missingRootTitle()}
  <span
    class="inline-flex items-center gap-2 text-sm font-normal text-rp-subtle"
  >
    [<span class="size-2 shrink-0 rounded-full bg-info" aria-hidden="true"
    ></span>
    <span>No root span</span>]
  </span>
{/snippet}
