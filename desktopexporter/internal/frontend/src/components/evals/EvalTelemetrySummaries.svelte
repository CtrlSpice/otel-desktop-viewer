<script lang="ts">
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import SignalDate from '@/components/shared/SignalDate.svelte'
  import Identifier from '@/components/shared/Identifier.svelte'
  import type { SpanNode } from '@/types/api-types'
  import { getServiceName } from '@/utils/resource'
  import { formatDurationParts } from '@/utils/time'
  import { formatMetricValuePlain } from '@/components/metrics/utils/format-metric-value'
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

  function formatTelemetryDuration(nanoseconds: bigint): string {
    const { value, unit } = formatDurationParts(nanoseconds)
    return formatMetricValuePlain(Number(value), {
      unit,
      maxFractionDigits: 3,
    })
  }
</script>

<div aria-label="Referenced telemetry" class="eval-telemetry min-w-0">
  {#each entries as entry (referenceKey(entry.reference))}
    {@const span = nodes.get(referenceKey(entry.reference))?.spanData}
    {@const service = span ? getServiceName(span.resource) : undefined}
    {@const title =
      span?.name ?? (entry.reference.spanID ? entry.label : 'Trace')}
    <SignalCard
      id={referenceKey(entry.reference)}
      href={traceHref(entry.reference)}
      {title}
    >
      {#snippet beforeTitle()}
        {#if entry.reference.spanID}
          <div
            class="min-h-[var(--signal-card-row-h)] truncate text-sm text-secondary"
            title={entry.label}
          >
            {span
              ? entry.label
              : loading
                ? 'Loading span…'
                : 'Span unavailable'}
          </div>
        {/if}
      {/snippet}
      {#snippet titleContent()}
        <span class="inline-flex min-w-0 items-center gap-2" title={service}>
          <span
            class="h-2 w-2 shrink-0 rounded-full"
            style:background-color={service
              ? traceServiceColor(service, themeSignal.value)
              : 'var(--color-muted)'}
            aria-hidden="true"
          ></span>
          <span class="truncate">{title}</span>
        </span>
      {/snippet}
      {#snippet subtitleContent()}
        <div
          class="eval-telemetry__identity"
          role="group"
          aria-label={`${entry.label} identifiers`}
        >
          <span class="text-rp-subtle">Traces:</span>
          <span>
            <span class="sr-only">Trace ID </span>
            <Identifier value={entry.reference.traceID} />
          </span>
          {#if entry.reference.spanID !== undefined}
            <span>
              <span class="sr-only">Span ID </span>
              <Identifier value={entry.reference.spanID} />
            </span>
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
              >{formatTelemetryDuration(span.endTime - span.startTime)}</span
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
    @apply flex min-h-[var(--signal-card-row-h)] min-w-0 flex-wrap items-center gap-x-2 text-sm;
  }
  .eval-telemetry .eval-telemetry__identity :global(.identifier) {
    text-decoration-line: var(--eval-id-decoration, none);
    text-underline-offset: 0.2em;
    color: var(--color-base-content);
  }
  .eval-telemetry :global(.signal-row:focus-visible) {
    --eval-id-decoration: underline;
  }
  @media (hover: hover) {
    .eval-telemetry :global(.signal-row:hover) {
      --eval-id-decoration: underline;
    }
  }
</style>
