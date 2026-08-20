<script lang="ts">
  /*
   * A detail row that wraps to a few lines and offers the rest on request.
   *
   * Wraps the whole "key: value" pair rather than the value alone. Clamping
   * needs `display: -webkit-box`, which makes its box block-level -- put that
   * on the value and the key is pushed onto its own line, doubling the height
   * of every row in a panel whose job is to be scanned. Around the pair, the
   * two spans flow as one run of text and the clamp counts lines of that.
   *
   * Values used to be cut to one line with an ellipsis, which fails twice on
   * what people read here. It cuts mid-token -- a trace id sliced through its
   * hex, a URL losing its path -- and the cut lands wherever the pane happens
   * to end, so the same field shows a different amount at different widths.
   *
   * Wrapping everything without limit is worse: one stack trace buries every
   * field beneath it. Three lines covers the overwhelming majority with no
   * interaction at all, and only genuinely long values clip.
   *
   * The control is a button, not the text. Clicking the text is what the
   * request asked for, but it fights selecting and copying -- the other thing
   * people do with these values -- and truncated text is not a discoverable
   * affordance. It appears only when something is actually hidden.
   */
  import type { Snippet } from 'svelte'

  type Props = {
    /** Key and value, rendered inline inside the clamp. */
    children: Snippet
    /** Changing this resets the expansion: a different span asks a different question. */
    resetKey?: unknown
  }

  let { children, resetKey }: Props = $props()

  let expanded = $state(false)
  let clipped = $state(false)
  let el = $state<HTMLElement | null>(null)

  $effect(() => {
    void resetKey
    expanded = false
  })

  // Measured, not guessed from length: whether three lines is enough depends on
  // the pane's width, which the string cannot know. Re-measured on resize, so
  // dragging the sidebar keeps the control honest.
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

<span
  bind:this={el}
  class="expandable-value"
  class:expandable-value--clamped={!expanded}>{@render children()}</span
>{#if clipped}
  <button
    type="button"
    class="expandable-value__toggle"
    onclick={() => (expanded = !expanded)}
    aria-expanded={expanded}>{expanded ? 'Show less' : 'Show more'}</button
  >
{/if}

<style lang="postcss">
  @reference "../../app.css";

  .expandable-value {
    /* Breaks inside a long unbroken token, which is what these values are:
       ids, URLs, base64. Without it a single 200-character word overflows the
       pane rather than wrapping inside it. */
    overflow-wrap: anywhere;
  }

  .expandable-value--clamped {
    display: -webkit-box;
    -webkit-box-orient: vertical;
    -webkit-line-clamp: 3;
    overflow: hidden;
  }

  .expandable-value__toggle {
    @apply cursor-pointer text-xs underline decoration-dotted underline-offset-2;
    color: var(--color-subtle);
  }

  .expandable-value__toggle:hover {
    @apply text-base-content;
  }
</style>
