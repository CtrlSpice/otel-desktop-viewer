<script lang="ts">
  import type { SpanTiming, TraceSummary } from '@/types/api-types'
  import Identifier from '@/components/shared/Identifier.svelte'
  import LoadingStatus from '@/components/shared/LoadingStatus.svelte'
  import UnavailableTelemetryReference from '@/components/shared/UnavailableTelemetryReference.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import { createRead } from '@/services/reads'
  import {
    isPlainLeftClick,
    itemHref,
    navigateToItem,
    SPAN_PARAM,
  } from '@/route'
  import { detailPanel } from '@/state/detail-panel.svelte'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import {
    formatDurationParts,
    formatTimestampParts,
    traceSummaryDurationNs,
  } from '@/utils/time'

  let {
    traceID: requestedTraceID,
    spanID: requestedSpanID,
  }: { traceID: string; spanID: string | null } = $props()
  let displayed = $state<{ traceID: string; spanID: string | null }>()
  const traceID = $derived(displayed?.traceID ?? requestedTraceID)
  const spanID = $derived(displayed ? displayed.spanID : requestedSpanID)
  let trace = $state<TraceSummary | null>(null)
  let span = $state<SpanTiming | null>(null)
  const reference = createRead<{
    span: SpanTiming | null
    trace: TraceSummary | null
    spanMissing: boolean
    traceMissing: boolean
  }>()
  const loading = $derived(reference.pending)
  const error = $derived(reference.error ?? '')
  let retry = $state(0)
  let spanMissing = $state(false)
  let traceMissing = $state(false)
  const unavailable = $derived(!loading && (spanMissing || traceMissing))
  let patch = $derived(spanID ? { [SPAN_PARAM]: spanID } : undefined)
  const timeContext = getTimeContext()
  const title = $derived(span?.name ?? trace?.rootSpan?.name)
  const service = $derived(span?.serviceName ?? trace?.rootSpan?.serviceName)
  const start = $derived(span?.startTime ?? trace?.startTime)
  const startParts = $derived(
    start === undefined
      ? null
      : formatTimestampParts(start, timeContext.tz, 'milliseconds')
  )
  const duration = $derived(
    span
      ? span.endTime >= span.startTime
        ? span.endTime - span.startTime
        : undefined
      : trace
        ? traceSummaryDurationNs(trace)
        : undefined
  )
  const durationParts = $derived(
    duration === undefined ? null : formatDurationParts(duration)
  )

  $effect(() => {
    void retry
    const id = requestedTraceID
    const linkedSpanID = requestedSpanID
    void reference.load(
      async signal => {
        let nextSpan: SpanTiming | null = null
        let nextTrace: TraceSummary | null = null
        let nextSpanMissing = false
        let nextTraceMissing = false
        if (linkedSpanID) {
          try {
            const spans = await telemetryAPI.getSpanTimings(
              [{ traceID: id, spanID: linkedSpanID }],
              signal
            )
            signal.throwIfAborted()
            nextSpan = spans[0] ?? null
            nextSpanMissing = !nextSpan
          } catch {
            throw new Error('Unable to load the associated span.')
          }
        }
        if (!nextSpan) {
          const result = await telemetryAPI
            .getTraceSummary(id, signal)
            .catch(() => {
              throw new Error('Unable to load the associated trace.')
            })
          nextTrace = result
          nextTraceMissing = result === null
        }
        return {
          span: nextSpan,
          trace: nextTrace,
          spanMissing: nextSpanMissing,
          traceMissing: nextTraceMissing,
        }
      },
      result => {
        span = result.span
        trace = result.trace
        spanMissing = result.spanMissing
        traceMissing = result.traceMissing
        displayed = { traceID: id, spanID: linkedSpanID }
      }
    )
    return () => reference.cancel()
  })

  function openTrace(event: MouseEvent) {
    if (!isPlainLeftClick(event)) return
    event.preventDefault()
    detailPanel.reveal()
    navigateToItem('traces', traceID, 'push', patch)
  }
</script>

<section
  class="trace-context px-3 py-2 text-sm"
  aria-label="Trace context"
  aria-busy={loading}
>
  <h3 class="text-sm font-normal text-subtle">Trace context</h3>
  {#if loading}
    <LoadingStatus
      pending={loading}
      label="Loading trace context…"
      requestKey={`${requestedTraceID}:${requestedSpanID ?? ''}:${retry}`}
    />
  {/if}
  {#if error}
    <div class="flex items-center gap-2" role="alert">
      <span>{error}</span>
      <button type="button" class="btn btn-ghost btn-xs" onclick={() => retry++}
        >Retry</button
      >
    </div>
  {/if}
  {#if span || trace}
    <div aria-label={span ? 'Span summary' : 'Trace summary'}>
      {#if spanID && !span}<div class="text-subtle">Trace summary</div>{/if}
      <div class="flex min-w-0 flex-wrap items-baseline gap-x-2">
        {#if title !== undefined}
          <span class="break-words text-lg font-normal text-base-content"
            >{title}</span
          >
        {:else}
          <span class="inline-flex items-center gap-2 text-subtle">
            [<span
              class="size-2 shrink-0 rounded-full bg-info"
              aria-hidden="true"
            ></span>
            <span>No root span</span>]
          </span>
        {/if}
        {#if service}<span class="text-subtle">· ({service})</span>{/if}
      </div>
      <div class="flex flex-wrap items-baseline gap-x-2 text-base-content">
        {#if startParts}<span
            title={`Start: ${startParts.value} ${startParts.unit}`}
            >{startParts.value}</span
          >{/if}
        {#if durationParts}
          {#if startParts}<span class="text-subtle" aria-hidden="true">·</span
            >{/if}
          <span
            ><span class="text-subtle">Duration:</span>
            {durationParts.value}
            {durationParts.unit}</span
          >
        {/if}
      </div>
    </div>
  {/if}
  <a
    class="inline-flex min-h-[var(--signal-card-row-h)] max-w-full flex-wrap items-center gap-2 text-base-content hover:underline focus-visible:underline"
    aria-label={`${spanID ? `Open span ${spanID} in trace ${traceID}` : `Open trace ${traceID}`}${unavailable ? ' (unavailable)' : ''}`}
    href={itemHref('traces', traceID, patch)}
    onclick={openTrace}
  >
    {#if unavailable}
      <UnavailableTelemetryReference {traceID} {spanID} revealFields />
    {:else}
      <span class="min-w-0 rounded" data-field="traceId" data-field-kind="field"
        ><Identifier value={traceID} /></span
      >
      {#if spanID}
        <span
          class="min-w-0 rounded"
          data-field="spanId"
          data-field-kind="field"><Identifier value={spanID} /></span
        >
      {/if}
    {/if}
  </a>
</section>

<style lang="postcss">
  @reference "../../app.css";
  .trace-context :global([data-field].detail-property--revealed) {
    background-color: var(--color-highlight-low);
  }
</style>
