<script lang="ts">
  import type { SpanTiming, TraceSummary } from '@/types/api-types'
  import Identifier from '@/components/shared/Identifier.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
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

  let { traceID, spanID }: { traceID: string; spanID: string | null } = $props()
  let trace = $state<TraceSummary | null>(null)
  let span = $state<SpanTiming | null>(null)
  let loading = $state(true)
  let error = $state('')
  let retry = $state(0)
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
    const id = traceID
    const linkedSpanID = spanID
    const controller = new AbortController()
    trace = null
    span = null
    loading = true
    error = ''
    async function load() {
      try {
        if (linkedSpanID) {
          // A missing or unavailable span lookup still permits the trace fallback.
          const spans = await telemetryAPI
            .getSpanTimings(
              [{ traceID: id, spanID: linkedSpanID }],
              controller.signal
            )
            .catch(() => [])
          if (controller.signal.aborted) return
          span = spans[0] ?? null
        }
        if (!span) {
          const result = await telemetryAPI.getTraceSummary(
            id,
            controller.signal
          )
          if (controller.signal.aborted) return
          trace = result
        }
      } catch {
        if (!controller.signal.aborted) {
          error = 'Unable to load the associated trace.'
        }
      } finally {
        if (!controller.signal.aborted) loading = false
      }
    }
    void load()
    return () => controller.abort()
  })

  function openTrace(event: MouseEvent, selectSpan = false) {
    if (!isPlainLeftClick(event)) return
    event.preventDefault()
    detailPanel.reveal()
    navigateToItem('traces', traceID, 'push', selectSpan ? patch : undefined)
  }
</script>

<section
  class="trace-context px-3 py-2 text-sm"
  aria-label="Trace context"
  aria-busy={loading}
>
  <h3 class="text-lg font-normal text-base-content">Trace context</h3>
  <dl class="my-2">
    <div
      class="trace-context__field"
      data-field="traceId"
      data-field-kind="field"
    >
      <dt class="text-rp-subtle">Trace ID</dt>
      <dd class="min-w-0">
        <a
          class="link link-primary inline-flex max-w-full"
          aria-label={`Open trace ${traceID}`}
          href={itemHref('traces', traceID)}
          onclick={event => openTrace(event)}><Identifier value={traceID} /></a
        >
      </dd>
    </div>
    {#if spanID}
      <div
        class="trace-context__field"
        data-field="spanId"
        data-field-kind="field"
      >
        <dt class="text-rp-subtle">Span ID</dt>
        <dd class="min-w-0">
          <a
            class="link link-primary inline-flex max-w-full"
            aria-label={`Open span ${spanID} in trace ${traceID}`}
            href={itemHref('traces', traceID, patch)}
            onclick={event => openTrace(event, true)}
            ><Identifier value={spanID} /></a
          >
        </dd>
      </div>
    {/if}
  </dl>
  {#if loading}
    <p class="text-rp-subtle" role="status">Loading trace context…</p>
  {:else if error}
    <div class="flex items-center gap-2" role="alert">
      <span>{error}</span>
      <button type="button" class="btn btn-ghost btn-xs" onclick={() => retry++}
        >Retry</button
      >
    </div>
  {:else if span || trace}
    {#if spanID && !span}
      <p class="text-rp-subtle">
        Span details unavailable. Showing trace summary.
      </p>
    {/if}
    <div aria-label={span ? 'Span summary' : 'Trace summary'}>
      <div class="flex min-w-0 flex-wrap items-baseline gap-x-2">
        {#if title !== undefined}
          <span class="break-words text-lg font-normal text-base-content"
            >{title}</span
          >
        {:else}
          <span class="inline-flex items-center gap-2 text-rp-subtle">
            [<span
              class="size-2 shrink-0 rounded-full bg-info"
              aria-hidden="true"
            ></span>
            <span>No root span</span>]
          </span>
        {/if}
        {#if service}<span class="text-rp-subtle">· ({service})</span>{/if}
      </div>
      <div class="flex flex-wrap items-baseline gap-x-2 text-base-content">
        {#if startParts}<span
            title={`Start: ${startParts.value} ${startParts.unit}`}
            >{startParts.value}</span
          >{/if}
        {#if durationParts}
          {#if startParts}<span class="text-rp-subtle" aria-hidden="true"
              >·</span
            >{/if}
          <span
            ><span class="text-rp-subtle">Duration:</span>
            {durationParts.value}
            {durationParts.unit}</span
          >
        {/if}
      </div>
    </div>
  {:else}
    <p class="text-rp-subtle">Associated trace is not stored.</p>
  {/if}
</section>

<style lang="postcss">
  @reference "../../app.css";
  .trace-context__field {
    @apply grid min-h-6 grid-cols-[5rem_minmax(0,1fr)] items-baseline gap-x-2 rounded;
  }
  .trace-context__field:global(.detail-property--revealed) {
    background-color: var(--color-highlight-low);
  }
</style>
