<script lang="ts">
  import type { Snippet } from 'svelte'
  import { onDestroy, setContext, untrack } from 'svelte'
  import DetailStackItem from './DetailStackItem.svelte'
  import {
    createDetailStackState,
    DETAIL_STACK,
  } from './utils/detail-stack-state.svelte'

  let {
    label,
    sections,
    storageKey,
  }: {
    label: string
    sections: { id: string; content: Snippet }[]
    storageKey: string
  } = $props()
  const stack = untrack(() => createDetailStackState(storageKey))
  setContext(DETAIL_STACK, stack)
  $effect(() => {
    const order = sections.map(section => section.id)
    untrack(() => stack.setOrder(order))
  })
  $effect(() => {
    const key = storageKey
    untrack(() => stack.setStorageKey(key))
  })
  onDestroy(() => stack.destroy())
</script>

<div
  class="detail-stack detail-stack--resizable min-h-0 flex-1 overflow-hidden"
  class:detail-stack--dragging={stack.dragging != null}
  role="region"
  aria-label={label}
>
  {#each stack.ordered(sections) as section (section.id)}
    <DetailStackItem id={section.id} content={section.content} />
  {/each}
</div>

<style>
  .detail-stack {
    display: flex;
    flex-direction: column;
    gap: calc(var(--header-gap) / 2);
  }

  .detail-stack--dragging :global(.detail-bubble__heading--resizable) {
    cursor: grabbing;
  }

  /* Grouping and resizing are immediate; DaisyUI must not delay native disclosure. */
  .detail-stack :global(.detail-bubble--resizable),
  .detail-stack :global(details),
  .detail-stack :global(details)::details-content,
  .detail-stack :global(.collapse-content) {
    transition: none;
  }
</style>
