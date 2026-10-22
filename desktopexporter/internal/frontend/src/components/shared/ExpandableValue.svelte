<script lang="ts">
  /*
   * Keys remain fully readable. Values clamp to three measured lines and use a
   * separate button so text selection and copying still work.
   */
  import type { Snippet } from 'svelte'

  type Props = {
    /** The key, rendered by the caller so each signal keeps its own markup. */
    keyLabel?: Snippet
    value: string
    /** Rich field content. The value remains its identity for expansion reset. */
    children?: Snippet
    /** Extra classes for the value (tabular-nums, font-mono, ...). */
    valueClass?: string
  }

  let { keyLabel, value, children, valueClass = '' }: Props = $props()
  const contentID = $props.id()

  let expanded = $state(false)
  let clipped = $state(false)
  let el = $state<HTMLElement | null>(null)
  let content = $state<HTMLElement | null>(null)

  // Clicking between spans should not leave a field open from the previous
  // one: the same field on the next span is a different question.
  $effect(() => {
    void value
    expanded = false
  })

  // Measured, not guessed from length: whether three lines is enough depends
  // on the width the value ends up with, which the string cannot know. The
  // observer keeps it honest as the pane is dragged.
  $effect(() => {
    void value
    const node = el
    if (!node) return
    const measure = () => {
      if (expanded) return
      clipped = node.scrollHeight > node.clientHeight + 1
    }
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(node)
    if (content) ro.observe(content)
    return () => ro.disconnect()
  })
</script>

<div class="detail-pair">
  {@render keyLabel?.()}
  <div
    id={contentID}
    bind:this={el}
    class="detail-pair__value {valueClass}"
    class:detail-pair__value--rich={!!children}
    class:detail-pair__value--clamped={!expanded}
    onfocusin={() => {
      if (clipped) expanded = true
    }}
  >
    {#if children}<div class="detail-pair__content" bind:this={content}>
        {@render children()}
      </div>{:else}{value}{/if}
    {#if children && clipped && !expanded}
      <span class="detail-pair__ellipsis" aria-hidden="true">…</span>
    {/if}
  </div>
  {#if clipped}
    <div class="detail-pair__disclosure">
      <button
        type="button"
        class="detail-pair__toggle"
        onclick={() => (expanded = !expanded)}
        aria-controls={contentID}
        aria-expanded={expanded}>{expanded ? 'Show less' : 'Show more'}</button
      >
    </div>
  {/if}
</div>

<style lang="postcss">
  @reference "../../app.css";

  .detail-pair {
    @apply flex min-w-0 flex-col items-stretch gap-0.5;
  }

  .detail-pair__value {
    @apply relative min-w-0 w-full text-base-content;
    overflow-wrap: anywhere;
  }

  .detail-pair__value--clamped {
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 3;
    overflow: hidden;
  }

  .detail-pair__value--rich.detail-pair__value--clamped {
    display: block;
    max-height: 3lh;
  }

  .detail-pair__content {
    display: flow-root;
  }

  .detail-pair__ellipsis {
    position: absolute;
    inset-block-end: 0;
    inset-inline-end: 0;
    padding-inline-start: 0.25em;
    background-color: var(--item-state-bg, var(--color-base-100));
    pointer-events: none;
    user-select: none;
  }

  .detail-pair__disclosure {
    @apply flex items-baseline gap-1;
    white-space: normal;
    color: var(--color-subtle);
  }

  .detail-pair__toggle {
    @apply shrink-0 cursor-pointer text-xs underline decoration-dotted underline-offset-2;
    color: var(--color-subtle);
  }

  .detail-pair__toggle:hover {
    @apply text-base-content;
  }
</style>
