<script lang="ts">
  import type { Snippet } from 'svelte'

  let {
    label,
    text,
    numeric = false,
    inset = true,
    onInspect,
    loadText,
    children,
  }: {
    label: string
    text: string
    numeric?: boolean
    inset?: boolean
    onInspect?: () => void
    loadText?: (signal: AbortSignal) => Promise<string>
    children?: Snippet
  } = $props()
  let hoverText = $state<string>()
  let pending = false
  let controller: AbortController | undefined
  $effect(() => {
    void text
    void loadText
    hoverText = undefined
    pending = false
    return () => controller?.abort()
  })
  function loadHover() {
    if (!loadText || pending || hoverText !== undefined) return
    controller = new AbortController()
    const request = controller
    pending = true
    void loadText(request.signal)
      .then(full => {
        if (!request.signal.aborted) hoverText = full
      })
      .catch(error => {
        if (!request.signal.aborted) hoverText = String(error)
      })
      .finally(() => {
        if (!request.signal.aborted) pending = false
      })
  }
</script>

<div class="table-value-cell" class:table-value-cell--inset={inset}>
  {#if onInspect}
    <button
      type="button"
      class="table-value-cell__inspect"
      class:text-right={numeric}
      title={hoverText ?? text}
      onpointerenter={loadHover}
      onfocus={loadHover}
      aria-label={`Show ${label} in details`}
      onclick={event => {
        event.stopPropagation()
        if (event.detail !== 0 && window.getSelection()?.isCollapsed === false)
          return
        onInspect()
      }}
    >
      {#if children}{@render children()}{:else}{text}{/if}
    </button>
  {:else}
    <span
      class="table-value-cell__text"
      class:text-right={numeric}
      title={text}
    >
      {#if children}{@render children()}{:else}{text}{/if}
    </span>
  {/if}
</div>

<style lang="postcss">
  @reference "../../app.css";
  .table-value-cell {
    @apply flex min-w-0 items-center;
  }
  .table-value-cell--inset {
    padding-inline: var(--table-cell-inline-padding, 0.5rem);
    padding-inline-end: var(
      --table-cell-inline-end,
      var(--table-cell-inline-padding, 0.5rem)
    );
  }
  .table-value-cell__inspect,
  .table-value-cell__text {
    @apply min-w-0 flex-1 select-text truncate rounded-sm border-0 bg-transparent p-0 text-left;
    font: inherit;
    color: inherit;
  }
  .table-value-cell__inspect {
    cursor: pointer;
  }
  .table-value-cell__inspect.text-right,
  .table-value-cell__text.text-right {
    text-align: right;
  }
</style>
