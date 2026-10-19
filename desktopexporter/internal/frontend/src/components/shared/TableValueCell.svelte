<script lang="ts">
  import type { Snippet } from 'svelte'

  let {
    label,
    text,
    numeric = false,
    inset = true,
    onInspect,
    children,
  }: {
    label: string
    text: string
    numeric?: boolean
    inset?: boolean
    onInspect: () => void
    children?: Snippet
  } = $props()
</script>

<div class="table-value-cell" class:table-value-cell--inset={inset}>
  <button
    type="button"
    class="table-value-cell__inspect"
    class:text-right={numeric}
    title={text}
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
  .table-value-cell__inspect {
    @apply min-w-0 flex-1 cursor-pointer select-text truncate rounded-sm border-0 bg-transparent p-0 text-left;
    font: inherit;
    color: inherit;
  }
  .table-value-cell__inspect.text-right {
    text-align: right;
  }
</style>
