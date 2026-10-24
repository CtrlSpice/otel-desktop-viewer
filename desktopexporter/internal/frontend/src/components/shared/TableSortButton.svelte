<script lang="ts">
  import type { Snippet } from 'svelte'

  let {
    label,
    direction,
    align = 'left',
    title,
    onclick,
    children,
    inset = true,
    inlineDirection = false,
  }: {
    label: string
    direction?: 'asc' | 'desc'
    align?: 'left' | 'right'
    title?: string
    onclick: () => void
    children: Snippet
    inset?: boolean
    inlineDirection?: boolean
  } = $props()
</script>

<button
  type="button"
  class="table-sort"
  class:table-sort--right={align === 'right'}
  class:table-sort--flush={!inset}
  class:table-sort--inline-direction={inlineDirection}
  aria-label={`Sort by ${label}`}
  {title}
  {onclick}
>
  {@render children()}
  {#if direction}<span class="table-sort__direction" aria-hidden="true"
      >{direction === 'asc' ? '↑' : '↓'}</span
    >{/if}
</button>

<style lang="postcss">
  @reference "../../app.css";

  .table-sort {
    @apply relative block min-w-0 w-full flex-1 cursor-pointer rounded-sm pr-3 text-left text-sm font-normal focus-visible:outline-2 focus-visible:outline-primary;
    min-height: var(--signal-card-row-h);
  }
  .table-sort--right {
    @apply pl-3 pr-0 text-right;
  }
  .table-sort--flush {
    padding-inline: 0;
  }
  .table-sort__direction {
    @apply absolute right-0 top-1/2 -translate-y-1/2 text-sm;
  }

  .table-sort--right .table-sort__direction {
    @apply left-0 right-auto;
  }
  .table-sort--inline-direction {
    @apply flex items-center gap-1;
  }
  .table-sort--inline-direction.table-sort--right {
    @apply justify-end;
  }
  .table-sort--inline-direction .table-sort__direction {
    position: static;
    translate: none;
  }
</style>
