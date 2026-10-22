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
  }: { label: string; children: Snippet; storageKey?: string } = $props()
  const stack = untrack(() =>
    storageKey ? createDetailStackState(storageKey) : undefined
  )
  setContext(DETAIL_STACK, stack)
  onDestroy(() => stack?.destroy())
</script>

<div
  class="detail-stack grid min-h-0 flex-1 auto-rows-[minmax(0,auto)] content-start gap-2 overflow-hidden"
  class:detail-stack--resizable={Boolean(stack)}
  class:detail-stack--dragging={stack?.dragging != null}
  role="region"
  aria-label={label}
>
  {@render children()}
</div>

<style>
  .detail-stack {
    --detail-motion-duration: var(--motion-compact-duration);
    --detail-motion-easing: var(--motion-settle-easing);
  }

  .detail-stack--resizable {
    display: flex;
    flex-direction: column;
    gap: calc(var(--header-gap) / 2);
  }

  .detail-stack--dragging :global(.detail-bubble--resizable) {
    transition: none;
  }

  .detail-stack--dragging :global(.detail-bubble__heading--resizable) {
    cursor: grabbing;
  }

  .detail-stack--dragging :global(details),
  .detail-stack--dragging :global(details)::details-content,
  .detail-stack--dragging :global(.collapse-content) {
    transition: none;
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
</style>
