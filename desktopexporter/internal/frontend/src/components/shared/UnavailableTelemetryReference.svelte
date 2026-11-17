<script lang="ts">
  import Identifier from './Identifier.svelte'

  let {
    traceID,
    spanID,
    traceHref,
    spanHref,
    onopen,
    revealFields = false,
  }: {
    traceID: string
    spanID?: string | null
    traceHref?: string
    spanHref?: string
    onopen?: (event: MouseEvent, selectSpan: boolean) => void
    revealFields?: boolean
  } = $props()
</script>

<span
  class="unavailable-reference inline-flex min-w-0 flex-wrap items-center gap-2 text-sm"
>
  <span class="size-2 shrink-0 rounded-full bg-muted" aria-hidden="true"></span>
  <span class="text-subtle">Traces:</span>
  <span
    class="min-w-0"
    data-field={revealFields ? 'traceId' : undefined}
    data-field-kind={revealFields ? 'field' : undefined}
  >
    {#if traceHref}
      <a
        class="inline-flex max-w-full hover:underline focus-visible:underline"
        href={traceHref}
        aria-label={`Open trace ${traceID}`}
        onclick={event => onopen?.(event, false)}
        ><Identifier value={traceID} /></a
      >
    {:else}<span class="sr-only">Trace ID</span>{' '}<Identifier
        value={traceID}
      />{/if}
  </span>
  {#if spanID}
    <span
      class="min-w-0"
      data-field={revealFields ? 'spanId' : undefined}
      data-field-kind={revealFields ? 'field' : undefined}
    >
      {#if spanHref}
        <a
          class="inline-flex max-w-full hover:underline focus-visible:underline"
          href={spanHref}
          aria-label={`Open span ${spanID} in trace ${traceID}`}
          onclick={event => onopen?.(event, true)}
          ><Identifier value={spanID} /></a
        >
      {:else}<span class="sr-only">Span ID</span>{' '}<Identifier
          value={spanID}
        />{/if}
    </span>
  {/if}
  <span class="text-subtle">(unavailable)</span>
</span>

<style>
  .unavailable-reference :global(.identifier) {
    color: var(--color-base-content);
  }
</style>
