<script lang="ts">
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import SignalDate from '@/components/shared/SignalDate.svelte'
  import Identifier from '@/components/shared/Identifier.svelte'
  import ValueCopyButton from '@/components/shared/ValueCopyButton.svelte'
  import type { SpanNode } from '@/types/api-types'
  import { getServiceName } from '@/utils/resource'
  import { formatDuration } from '@/utils/time'
  import { traceHref } from './model'
  import { referenceKey } from './eval-traces'
  import type { EvalTelemetrySummary } from './eval-telemetry-summaries'
  import { traceServiceColor } from '@/utils/trace-service-color'
  import { themeSignal } from '@/state/theme.svelte'

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
    {@const service = span ? getServiceName(span.resource) : undefined}
    <SignalCard
      id={referenceKey(entry.reference)}
      href={traceHref(entry.reference)}
      title={span?.name ?? entry.label}
    >
      {#snippet titleContent()}
        <span class="inline-flex min-w-0 items-center gap-2" title={service}>
          <span
            class="h-2 w-2 shrink-0 rounded-full"
            style:background-color={service
              ? traceServiceColor(service, themeSignal.value)
              : 'var(--color-muted)'}
            aria-hidden="true"
          ></span>
          <span class="truncate">{span?.name ?? entry.label}</span>
        </span>
      {/snippet}
      {#snippet interactiveSubtitle()}
        <div
          class="min-h-[var(--signal-card-row-h)] truncate text-sm text-secondary"
          title={entry.label}
        >
          {span
            ? entry.label
            : !entry.reference.spanID
              ? 'Trace reference · no span ID recorded'
              : loading
                ? 'Loading span…'
                : 'Span unavailable'}
        </div>
        <div
          class="eval-telemetry__identity"
          role="group"
          aria-label={`${entry.label} identifiers`}
        >
          <span class="text-rp-subtle" title="Trace ID">TID</span>
          <Identifier value={entry.reference.traceID} />
          <ValueCopyButton text={entry.reference.traceID} label="trace ID" />
          {#if entry.reference.spanID !== undefined}
            <span class="text-rp-subtle" title="Span ID">SID</span>
            <Identifier value={entry.reference.spanID} />
            <ValueCopyButton text={entry.reference.spanID} label="span ID" />
          {/if}
        </div>
      {/snippet}
      {#snippet badge()}
        {#if span}<span
            class="inline-flex items-center gap-2 text-sm text-base-content"
          >
            <span
              class="h-2 w-2 shrink-0 rounded-full"
              style:background-color={span.statusCodeValue === 2
                ? 'var(--color-error)'
                : span.statusCodeValue === 1
                  ? 'var(--color-accent)'
                  : 'var(--color-muted)'}
              aria-hidden="true"
            ></span>
            {span.statusCode}
          </span>{/if}
      {/snippet}
      {#snippet meta()}
        {#if span && span.endTime >= span.startTime}<span
            >Duration: <span class="text-base-content tabular-nums"
              >{formatDuration(span.endTime - span.startTime)}</span
            ></span
          >{/if}
      {/snippet}
      {#snippet date()}<SignalDate
          label="Started"
          timestamp={span?.startTime}
        />{/snippet}
    </SignalCard>
  {/each}
</div>

<style lang="postcss">
  @reference "../../app.css";
  .eval-telemetry__identity {
    @apply grid min-w-0 grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-2 text-sm;
    grid-auto-rows: var(--signal-card-row-h);
  }
  .eval-telemetry__identity :global(.identifier) {
    @apply min-w-0 truncate;
  }
  .eval-telemetry__identity :global(.value-copy) {
    width: var(--signal-card-row-h);
    height: var(--signal-card-row-h);
  }
</style>
