<script lang="ts">
  import type { TraceSummary } from '@/types/api-types'
  import TraceCard from '@/components/traces/TraceCard.svelte'
  import Identifier from '@/components/shared/Identifier.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import { itemHref, navigateToItem, SPAN_PARAM } from '@/route'
  import { detailPanel } from '@/state/detail-panel.svelte'

  let { traceID, spanID }: { traceID: string; spanID: string | null } = $props()
  let trace = $state<TraceSummary | null>(null)
  let loading = $state(true)
  let error = $state('')
  let retry = $state(0)
  let patch = $derived(spanID ? { [SPAN_PARAM]: spanID } : undefined)

  $effect(() => {
    void retry
    const id = traceID
    const controller = new AbortController()
    trace = null
    loading = true
    error = ''
    void telemetryAPI.getTraceSummary(id, controller.signal).then(
      result => {
        if (!controller.signal.aborted) {
          trace = result
          loading = false
        }
      },
      () => {
        if (!controller.signal.aborted) {
          error = 'Unable to load the associated trace.'
          loading = false
        }
      }
    )
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
