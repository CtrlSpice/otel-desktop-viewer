<script lang="ts">
  import type { Snippet } from 'svelte'
  let {
    label,
    expanded,
    onclick,
    children,
    class: className = '',
  }: {
    label: string
    expanded: boolean
    onclick: () => void
    children: Snippet
    class?: string
  } = $props()
</script>

<button
  type="button"
  class="tree-toggle {className}"
  aria-label={label}
  aria-expanded={expanded}
  onclick={event => {
    event.stopPropagation()
    onclick()
  }}
>
  {@render children()}
</button>

<style lang="postcss">
  @reference "../../app.css";
  .tree-toggle {
    @apply flex shrink-0 cursor-pointer items-center gap-1 rounded-none border-0 bg-transparent p-0 text-sm font-normal focus-visible:outline-2 focus-visible:outline-primary;
    height: var(--table-row-h);
  }
  .tree-toggle:hover {
    background-color: var(--color-highlight-low);
  }
</style>
