<script lang="ts">
  let {
    count,
    title,
    class: className = '',
  }: { count: number; title?: string; class?: string } = $props()
</script>

<span
  class="tree-child-count shrink-0 text-sm tabular-nums {className}"
  aria-hidden="true"
  {title}>({count})</span
>

<style lang="postcss">
  @reference "../../app.css";
  .tree-child-count {
    @apply text-center font-normal tracking-normal whitespace-nowrap;
    color: var(--color-subtle);
    font-size: var(--text-sm);
    width: var(--tree-count-width);
  }
</style>
