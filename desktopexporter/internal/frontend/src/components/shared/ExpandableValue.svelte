<script lang="ts">
  /*
   * Keys remain fully readable. Values clamp to three measured lines and use a
   * separate button so text selection and copying still work.
   */
  import type { Snippet } from 'svelte'

  type Props = {
    /** The key, rendered by the caller so each signal keeps its own markup. */
    keyLabel: Snippet
    value: string
    /** Extra classes for the value (tabular-nums, font-mono, ...). */
    valueClass?: string
  }

  let { keyLabel, value, valueClass = '' }: Props = $props()

  let expanded = $state(false)
  let clipped = $state(false)
  let el = $state<HTMLElement | null>(null)

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
    const node = el
    if (!node) return
    const measure = () => {
      if (expanded) return
      clipped = node.scrollHeight > node.clientHeight + 1
    }
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(node)
    return () => ro.disconnect()
  })
</script>

<span class="detail-pair">
  {@render keyLabel()}
  <span
    bind:this={el}
    class="detail-pair__value {valueClass}"
    class:detail-pair__value--clamped={!expanded}>{value}</span
  >
  {#if clipped}
    <button
      type="button"
      class="detail-pair__toggle"
      onclick={() => (expanded = !expanded)}
      aria-expanded={expanded}>{expanded ? 'Show less' : 'Show more'}</button
    >
  {/if}
</span>

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
    -webkit-line-clamp: 3;
    overflow: hidden;
  }

  .detail-pair__toggle {
    @apply shrink-0 cursor-pointer text-xs underline decoration-dotted underline-offset-2;
    color: var(--color-subtle);
  }

  .detail-pair__toggle:hover {
    @apply text-base-content;
  }
</style>
