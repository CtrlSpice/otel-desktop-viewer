<script lang="ts">
  import type { SpanTiming, TraceSummary } from '@/types/api-types'
  import TraceCard from '@/components/traces/TraceCard.svelte'
  import SpanCard from '@/components/traces/SpanCard.svelte'
  import Identifier from '@/components/shared/Identifier.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import { itemHref, navigateToItem, SPAN_PARAM } from '@/route'
  import { detailPanel } from '@/state/detail-panel.svelte'

  let { traceID, spanID }: { traceID: string; spanID: string | null } = $props()
  let trace = $state<TraceSummary | null>(null)
  let span = $state<SpanTiming | null>(null)
  let loading = $state(true)
  let error = $state('')
  let retry = $state(0)
  let patch = $derived(spanID ? { [SPAN_PARAM]: spanID } : undefined)

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

  function openTrace() {
    detailPanel.reveal()
    navigateToItem('traces', traceID, 'push', patch)
  }
</script>

<section aria-label="Associated trace" aria-busy={loading}>
  {#if loading}
    <p class="px-3 py-2 text-sm text-base-content/60" role="status">
      Loading associated trace…
    </p>
  {:else if error}
    <div class="flex items-center gap-2 px-3 py-2 text-sm" role="alert">
      <span>{error}</span>
      <button type="button" class="btn btn-ghost btn-xs" onclick={() => retry++}
        >Retry</button
      >
    </div>
  {:else if span}
    <SpanCard {span} onclick={openTrace} />
  {:else if trace}
    <TraceCard {trace} onclick={openTrace} />
  {:else}
    <div class="px-3 py-2 text-sm text-base-content/60">
      <p>Associated trace is not stored.</p>
      <a
        class="link link-primary"
        href={itemHref('traces', traceID, patch)}
        onclick={event => {
          event.preventDefault()
          openTrace()
        }}>Open trace <Identifier value={traceID} /></a
      >
    </div>
  {/if}
</section>
