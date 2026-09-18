<script lang="ts">
  /*
   * One stacked field of a detail panel, laid out so it survives arbitrary
   * attribute names.
   *
   * The key/type header and value are deliberately separate blocks. This keeps
   * a long key legible while reserving the full detail-pane width for its value.
   *
   * The alternative was a fixed key column, and real data rules it out: a k8s
   * annotation key like
   * `k8s.pod.annotations.kubectl.kubernetes.io/last-applied-configuration`
   * measures 473px, so a column sized to hold it leaves negative room for
   * values in a 390px pane. Its only escape is truncating keys, and a key you
   * cannot read is a field you cannot identify. This layout degrades the other
   * way: the longer the content, the more rows stack, until every row is
   * stacked -- which is just the safe layout, reached gradually.
   *
   * Keys wrap and are never clamped, for the same reason: identifying the
   * field is the precondition for reading it.
   *
   * Values clamp to three lines and offer the rest on a button. Values used to
   * be cut to one line with an ellipsis, which cut mid-token -- a trace id
   * sliced through its hex -- at a point that moved with the pane width.
   * Wrapping without a limit is worse: one stack trace buries every field
   * under it, and this panel exists to be scanned.
   *
   * The control is a button rather than the text itself, because clicking the
   * text fights selecting and copying, which is the other thing people do with
   * these values.
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
