<script lang="ts">
  import type { Snippet } from 'svelte'
  import { onDestroy, setContext, untrack } from 'svelte'
  import {
    createDetailStackState,
    DETAIL_STACK,
  } from './utils/detail-stack-state.svelte'

  let {
    label,
    children,
    storageKey,
  }: { label: string; children: Snippet; storageKey: string } = $props()
  const stack = untrack(() => createDetailStackState(storageKey))
  setContext(DETAIL_STACK, stack)
  $effect(() => {
    const key = storageKey
    untrack(() => stack.setStorageKey(key))
  })
  onDestroy(() => stack.destroy())
</script>

<div
  class="detail-stack detail-stack--resizable min-h-0 flex-1 overflow-hidden"
  class:detail-stack--dragging={stack.dragging != null}
  class:detail-stack--immediate={stack.immediate}
  role="region"
  aria-label={label}
>
  {@render children()}
</div>

<style>
  .detail-stack {
    --detail-motion-duration: var(--motion-compact-duration);
    --detail-motion-easing: var(--motion-settle-easing);
    display: flex;
    flex-direction: column;
    gap: calc(var(--header-gap) / 2);
  }

  .detail-stack--dragging :global(.detail-bubble__heading--resizable) {
    cursor: grabbing;
  }

  @media (prefers-reduced-motion: no-preference) {
    .detail-stack :global(details) {
      transition: grid-template-rows var(--detail-motion-duration)
        var(--detail-motion-easing);
    }

    .detail-stack :global(details)::details-content {
      interpolate-size: allow-keywords;
      height: 0;
      overflow: clip;
      transition:
        height var(--detail-motion-duration) var(--detail-motion-easing),
        content-visibility var(--detail-motion-duration) allow-discrete,
        overflow 0s allow-discrete var(--detail-overflow-delay, 0s);
    }

    .detail-stack :global(details[open])::details-content {
      --detail-overflow-delay: var(--detail-motion-duration);
      height: auto;
      overflow: visible;
    }

    .detail-stack :global(.collapse-content) {
      transition:
        content-visibility var(--detail-motion-duration) allow-discrete,
        visibility var(--detail-motion-duration) allow-discrete,
        padding var(--detail-motion-duration) var(--detail-motion-easing);
    }
  }

  .detail-stack--immediate :global(.detail-bubble--resizable),
  .detail-stack--immediate :global(details),
  .detail-stack--immediate :global(details)::details-content,
  .detail-stack--immediate :global(.collapse-content) {
    transition: none;
  }
</style>
