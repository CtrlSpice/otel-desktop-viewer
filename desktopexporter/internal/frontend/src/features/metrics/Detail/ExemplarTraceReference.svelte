<script lang="ts">
  import Identifier from '@/components/shared/Identifier.svelte'
  import UnavailableTelemetryReference from '@/components/shared/UnavailableTelemetryReference.svelte'
  import { telemetryAPI } from '@/services/telemetry-service'
  import {
    isPlainLeftClick,
    itemHref,
    navigateToItem,
    SPAN_PARAM,
  } from '@/route'
  import { detailPanel } from '@/state/detail-panel.svelte'
  import type { Exemplar } from '@/types/api-types'

  let { exemplar }: { exemplar: Exemplar } = $props()
  let unavailable = $state(false)
  const patch = $derived(
    exemplar.spanID ? { [SPAN_PARAM]: exemplar.spanID } : undefined
  )
  const href = $derived(
    exemplar.traceID ? itemHref('traces', exemplar.traceID, patch) : undefined
  )

  $effect(() => {
    const { traceID, spanID } = exemplar
    const controller = new AbortController()
    unavailable = false
    async function load() {
      if (!traceID) return
      try {
        const missing = spanID
          ? (
              await telemetryAPI.getSpanTimings(
                [{ traceID, spanID }],
                controller.signal
              )
            ).length === 0
          : (await telemetryAPI.getTraceSummary(traceID, controller.signal)) ===
            null
        if (!controller.signal.aborted) unavailable = missing
      } catch {
        // Failed lookups leave the reference unresolved, not confirmed absent.
      }
    }
    void load()
    return () => controller.abort()
  })

  function open(event: MouseEvent) {
    if (!exemplar.traceID || !isPlainLeftClick(event)) return
    event.preventDefault()
    detailPanel.reveal()
    navigateToItem('traces', exemplar.traceID, 'push', patch)
  }
</script>

{#if unavailable && exemplar.traceID}
  <UnavailableTelemetryReference
    traceID={exemplar.traceID}
    spanID={exemplar.spanID}
    traceHref={href}
    spanHref={exemplar.spanID ? href : undefined}
    onopen={open}
  />
{:else}
  {#if exemplar.traceID}
    <a class="link font-sans text-sm text-base-content" {href} onclick={open}
      >trace: <Identifier value={exemplar.traceID} /></a
    >
  {/if}
  {#if exemplar.spanID && exemplar.traceID}
    <a class="link font-sans text-sm text-base-content" {href} onclick={open}
      >span: <Identifier value={exemplar.spanID} /></a
    >
  {:else if exemplar.spanID}
    <span class="font-sans text-sm text-base-content"
      >span: <Identifier value={exemplar.spanID} /></span
    >
  {/if}
{/if}
