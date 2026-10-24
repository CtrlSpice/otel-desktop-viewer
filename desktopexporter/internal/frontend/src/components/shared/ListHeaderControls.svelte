<script lang="ts">
  import type { Snippet } from 'svelte'

  let {
    label = 'List controls',
    sort,
    filters,
  }: {
    label?: string
    sort?: Snippet
    filters?: Snippet
  } = $props()
</script>

<div class="list-header-controls" role="group" aria-label={label}>
  {#if sort}<div class="list-header-controls__sort">{@render sort()}</div>{/if}
  {#if filters}<div class="list-header-controls__filters">
      {@render filters()}
    </div>{/if}
</div>

<style lang="postcss">
  @reference "../../app.css";
  .list-header-controls {
    @apply flex min-w-0 w-full items-center gap-2 text-sm font-normal;
  }
  .list-header-controls__sort {
    @apply flex min-w-0 items-center gap-0;
  }
  .list-header-controls__filters {
    @apply ml-auto flex shrink-0 items-center gap-2;
  }
</style>
