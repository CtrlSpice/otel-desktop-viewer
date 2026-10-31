<script lang="ts">
  /*
   * Keep values up to three lines whole. Longer values use two native-clamped
   * lines and a separate disclosure so text selection and copying still work.
   */
  import type { Snippet } from 'svelte'

  type Props = {
    /** The key, rendered by the caller so each signal keeps its own markup. */
    keyLabel?: Snippet
    value: string
    /** Reset disclosure when rich content changes, without serializing it. */
    identity?: unknown
    /** Rich field content. The value remains its identity for expansion reset. */
    children?: Snippet
    /** Extra classes for the value (tabular-nums, font-mono, ...). */
    valueClass?: string
  }

  let { keyLabel, value, identity, children, valueClass = '' }: Props = $props()
  const contentID = $props.id()

  let expanded = $state(false)
  let clipped = $state(false)
  let el = $state<HTMLElement | null>(null)
  let content = $state<HTMLElement | null>(null)

  // Clicking between spans should not leave a field open from the previous
  // one: the same field on the next span is a different question.
  $effect(() => {
    void (identity ?? value)
    expanded = false
  })

  // Measured, not guessed from length: whether three lines is enough depends
  // on the width the value ends up with, which the string cannot know. The
  // observer keeps it honest as the pane is dragged.
  $effect(() => {
    void (identity ?? value)
    const node = el
    if (!node) return
    const measure = () => {
      if (expanded) return
      const lineHeight = Number.parseFloat(getComputedStyle(node).lineHeight)
      clipped = node.scrollHeight > lineHeight * 3 + 1
    }
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(node)
    const mutations = new MutationObserver(measure)
    if (content)
      mutations.observe(content, {
        childList: true,
        subtree: true,
        characterData: true,
      })
    return () => {
      ro.disconnect()
      mutations.disconnect()
    }
  })
</script>

<div class="detail-pair">
  {@render keyLabel?.()}
  <div
    id={contentID}
    bind:this={el}
    class="detail-pair__value {valueClass}"
    class:detail-pair__value--clamped={!expanded}
    style:--preview-lines={clipped ? 2 : 3}
    onfocusin={() => {
      if (clipped) expanded = true
    }}
  >
    {#if children}<div class="detail-pair__content" bind:this={content}>
        {@render children()}
      </div>{:else}{value}{/if}
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
    @apply min-w-0 w-full text-base-content;
    overflow-wrap: anywhere;
  }

  .detail-pair__value--clamped {
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: var(--preview-lines);
    overflow: hidden;
  }

  .detail-pair__content {
    display: contents;
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
