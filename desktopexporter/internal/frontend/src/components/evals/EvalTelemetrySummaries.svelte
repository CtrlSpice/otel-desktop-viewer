<script lang="ts">
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import Identifier from '@/components/shared/Identifier.svelte'
  import type { SpanNode } from '@/types/api-types'
  import { getServiceName } from '@/utils/resource'
  import { formatDuration } from '@/utils/time'
  import { traceHref } from './model'
  import { referenceKey } from './eval-traces'
  import type { EvalTelemetrySummary } from './eval-telemetry-summaries'

  let {
    entries,
    nodes = new Map<string, SpanNode>(),
    loading = false,
  }: {
    entries: EvalTelemetrySummary[]
    nodes?: Map<string, SpanNode>
    loading?: boolean
  } = $props()
</script>

<div aria-label="Referenced telemetry" class="min-w-0">
  {#each entries as entry (referenceKey(entry.reference))}
    {@const span = nodes.get(referenceKey(entry.reference))?.spanData}
    <SignalCard
      id={referenceKey(entry.reference)}
      href={traceHref(entry.reference)}
      title={span?.name ?? entry.label}
      subtitle={span
        ? [entry.label, getServiceName(span.resource)]
            .filter(Boolean)
            .join(' · ')
        : entry.reference.spanID
          ? loading
            ? 'Loading span…'
            : 'Span summary unavailable'
          : 'Trace reference · No span specified'}
      timeLayout="interval"
      duration={span && span.endTime >= span.startTime
        ? formatDuration(span.endTime - span.startTime)
        : undefined}
    >
      {#snippet badge()}
        {#if span}<span
            class="inline-flex items-center gap-2 text-sm text-base-content"
          >
            {#if span.statusCodeValue === 2}<span
                class="h-2 w-2 rounded-full bg-error"
                aria-hidden="true"
              ></span>{/if}
            {span.statusCode}
          </span>{/if}
      {/snippet}
      {#snippet meta()}
        <span>Open in Traces</span>
        <Identifier value={entry.reference.spanID ?? entry.reference.traceID} />
      {/snippet}
    </SignalCard>
  {/each}
</div>
