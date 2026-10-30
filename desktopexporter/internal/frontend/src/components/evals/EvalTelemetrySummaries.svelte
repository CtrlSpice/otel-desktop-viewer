<script lang="ts">
  import { untrack } from 'svelte'
  import LoadingStatus from '@/components/shared/LoadingStatus.svelte'
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import SignalDate from '@/components/shared/SignalDate.svelte'
  import Identifier from '@/components/shared/Identifier.svelte'
  import type { SpanNode, TraceSummary } from '@/types/api-types'
  import { telemetryAPI } from '@/services/telemetry-service'
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
    ontracecount,
  }: {
    entries: EvalTelemetrySummary[]
    nodes?: Map<string, SpanNode>
    loading?: boolean
    ontracecount?: (count: number | undefined) => void
  } = $props()

  let traces = $state<Map<string, TraceSummary | null>>(new Map())
  let pending = $state.raw(new Set<string>())
  let requestKey = $state.raw<AbortSignal>()
  $effect(() => {
    const ids = [
      ...new Set(entries.map(entry => entry.reference.traceID.toLowerCase())),
    ]
    const stored = new Set(
      [...nodes.values()].map(node => node.spanData.traceID.toLowerCase())
    )
    const pending = ids.some(id => !stored.has(id) && !traces.has(id))
    ontracecount?.(
      pending
        ? undefined
        : ids.filter(id => stored.has(id) || traces.get(id)).length
    )
  })
  const lookupIDs = $derived(
    [
      ...new Set(
        entries
          .filter(entry => !nodes.has(referenceKey(entry.reference)))
          .map(entry => entry.reference.traceID.toLowerCase())
      ),
    ]
      .sort()
      .join(',')
  )
  $effect(() => {
    const ids = lookupIDs ? lookupIDs.split(',') : []
    const controller = new AbortController()
    requestKey = controller.signal
    pending = new Set(ids)
    async function load() {
      for (let offset = 0; offset < ids.length; offset += 4) {
        if (controller.signal.aborted) return
        await Promise.all(
          ids.slice(offset, offset + 4).map(async id => {
            try {
              const trace = await telemetryAPI.getTraceSummary(
                id,
                controller.signal
              )
              if (!controller.signal.aborted)
                traces = new Map(untrack(() => traces)).set(id, trace)
            } catch {
              // A failed request does not establish that the trace is absent.
            } finally {
              if (!controller.signal.aborted) {
                const next = new Set(untrack(() => pending))
                next.delete(id)
                pending = next
              }
            }
          })
        )
      }
    }
    void load()
    return () => controller.abort()
  })
</script>

<div aria-label="Referenced telemetry" class="eval-telemetry min-w-0">
  {#each entries as entry (referenceKey(entry.reference))}
    {@const span = nodes.get(referenceKey(entry.reference))?.spanData}
    {@const trace = traces.get(entry.reference.traceID.toLowerCase())}
    {@const unavailable = !span && trace === null}
    {@const traceOnly = !entry.reference.spanID}
    {@const tracePending = pending.has(entry.reference.traceID.toLowerCase())}
    {@const startedAt =
      span?.startTime ?? (traceOnly ? trace?.startTime : undefined)}
    {@const service = span
      ? getServiceName(span.resource)
      : traceOnly
        ? trace?.rootSpan?.serviceName
        : undefined}
    {@const title =
      span?.name ??
      (traceOnly ? (trace?.rootSpan?.name ?? 'Trace') : entry.label)}
    {@const hasDuration = span !== undefined && span.endTime >= span.startTime}
    {#snippet startDate()}
      <SignalDate label="Start" timestamp={startedAt} />
    {/snippet}
    {#snippet durationSummary()}
      {#if span && span.endTime >= span.startTime}<span class="shrink-0"
          >Duration: <span class="text-base-content tabular-nums"
            >{formatDuration(span.endTime - span.startTime)}</span
          ></span
        >{/if}
    {/snippet}
    {#snippet identifiers()}
      <span
        class="eval-telemetry__identity"
        role="group"
        aria-label={`${entry.label} identifiers`}
      >
        <span class="text-subtle">Traces:</span>
        <span
          ><span class="sr-only">Trace ID </span><Identifier
            value={entry.reference.traceID}
          /></span
        >
        {#if entry.reference.spanID !== undefined}
          <span
            ><span class="sr-only">Span ID </span><Identifier
              value={entry.reference.spanID}
            /></span
          >
        {/if}
        {#if unavailable}<span class="text-subtle">(unavailable)</span>{/if}
        <LoadingStatus
          pending={tracePending}
          label="Loading trace…"
          {requestKey}
          class="text-sm text-secondary"
        />
      </span>
    {/snippet}
    <SignalCard
      id={referenceKey(entry.reference)}
      href={traceHref(entry.reference)}
      disabled={unavailable}
      date={startedAt !== undefined ? startDate : undefined}
      dateEnd={startedAt !== undefined && hasDuration
        ? durationSummary
        : undefined}
      meta={startedAt === undefined && hasDuration
        ? durationSummary
        : undefined}
      title={unavailable ? entry.label : title}
      subtitleContent={unavailable ? undefined : identifiers}
    >
      {#snippet beforeTitle()}
        {#if entry.reference.spanID && !unavailable && (!span || entry.label !== title)}
          <div
            class="min-h-[var(--signal-card-row-h)] truncate text-sm text-secondary"
            title={entry.label}
          >
            {#if span}{entry.label}
            {:else if loading}<LoadingStatus
                pending
                label="Loading span…"
                requestKey={referenceKey(entry.reference)}
                class="text-sm text-secondary"
              />
            {:else}Span unavailable{/if}
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
          {#if unavailable}{@render identifiers()}{:else}<span class="truncate"
              >{title}</span
            >{/if}
        </span>
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
  .eval-telemetry :global(a.signal-row[href]:focus-visible) {
    --eval-id-decoration: underline;
  }
  @media (hover: hover) {
    .eval-telemetry :global(a.signal-row[href]:hover) {
      --eval-id-decoration: underline;
    }
  }
</style>
