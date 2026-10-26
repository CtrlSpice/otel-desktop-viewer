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
    defaultWeights = {},
  }: {
    label: string
    sections: { id: string; content: Snippet }[]
    storageKey: string
    defaultWeights?: Readonly<Record<string, number>>
  } = $props()
  const stack = untrack(() =>
    createDetailStackState(storageKey, defaultWeights)
  )
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
  {#each sections as section (section.id)}
    <DetailStackItem id={section.id} content={section.content} />
  {/each}
</div>

<style>
  .detail-stack {
    display: flex;
    flex-direction: column;
    gap: calc(var(--header-gap) / 2);
  }

  .detail-stack > :global(:last-child:not(:first-child)) {
    margin-block-start: auto;
  }

  .detail-stack--dragging :global(.detail-bubble__heading--resizable) {
    cursor: grabbing;
  }

  /* Disclosure and resizing are immediate; section order never changes. */
  .detail-stack :global(.detail-bubble--resizable),
  .detail-stack :global(details),
  .detail-stack :global(details)::details-content,
  .detail-stack :global(.collapse-content) {
    transition: none;
  }
</style>
