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
  {title}>{count}</span
>

<style lang="postcss">
  @reference "../../app.css";
  .tree-child-count {
    @apply text-center font-normal;
    color: var(--color-subtle);
    width: var(--waterfall-gutter-col);
    margin-left: calc(0px - var(--waterfall-gutter-tail) - 0.25rem);
    margin-right: 0.25rem;
  }
</style>
