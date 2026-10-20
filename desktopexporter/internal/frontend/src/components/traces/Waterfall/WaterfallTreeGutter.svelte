<script module lang="ts">
  export type TreeConnectorMeta = {
    childrenCount: number
    ancestorHasNextSibling: boolean[]
    isLastChild: boolean
  }
</script>

<script lang="ts">
  type SegmentKind = 'none' | 'passthrough' | 'tee' | 'elbow'

  type Props = {
    depth: number
    tree: TreeConnectorMeta
    /** Concrete colour (palette HCL or `--color-*` var) for the node dot.
     *  Error spans already pass
     *  `--color-error` here, so no separate `isError` branch is needed. */
    color: string
    subtreeCollapsed: boolean
    onToggleExpand: () => void
  }

  let {
    depth,
    tree,
    color,
    subtreeCollapsed,
    onToggleExpand,
  }: Props = $props()

  let hasChildren = $derived(tree.childrenCount > 0)
  let childCount = $derived(tree.childrenCount)

  /**
   * Picks the line shape for one gutter column.
   *
   * The last column (the parent) gets a connector with a horizontal arm:
   *   └ elbow  if this span is the last child
   *   ├ tee    if more siblings follow
   *
   * Every earlier column is just an ancestor's trunk:
   *   │ passthrough  if that ancestor still has siblings below
   *     none         if that ancestor's group is finished
   */
  const segmentAt = (hasNext: boolean, d: number): SegmentKind =>
    d === depth - 1
      ? tree.isLastChild
        ? 'elbow'
        : 'tee'
      : hasNext
        ? 'passthrough'
        : 'none'

  let segments = $derived(
    tree.ancestorHasNextSibling.slice(0, depth).map(segmentAt)
  )
</script>

<div
  class="gutter"
  style:width="calc({depth + 1} * var(--waterfall-gutter-col) + var(--waterfall-gutter-tail))"
  style:--tree-accent={color}
>
  {#each segments as kind, d}
    <div
      class="gutter__col"
      style:left="calc({d} * var(--waterfall-gutter-col))"
      style:width="var(--waterfall-gutter-col)"
    >
      {#if kind === 'passthrough'}
        <div class="seg seg--passthrough"></div>
      {:else if kind === 'tee'}
        <div class="seg seg--tee"></div>
        <div class="seg__arm" class:seg__arm--leaf={!hasChildren}></div>
      {:else if kind === 'elbow'}
        <div
          class="seg__arm seg__arm--elbow"
          class:seg__arm--leaf={!hasChildren}
        ></div>
      {/if}
    </div>
  {/each}

  <div
    class="gutter__node"
    style:left="calc({depth} * var(--waterfall-gutter-col))"
    style:width="var(--waterfall-gutter-col)"
  >
    {#if hasChildren}
      {#if !subtreeCollapsed}
        <span class="gutter__stem" aria-hidden="true"></span>
      {/if}
      <button
        type="button"
        class="gutter__btn gutter__btn--hub shrink-0"
        aria-expanded={!subtreeCollapsed}
        aria-label={subtreeCollapsed
          ? `Expand ${childCount} child spans`
          : `Collapse ${childCount} child spans`}
        onclick={e => {
          e.stopPropagation()
          onToggleExpand()
        }}
      >
        <span class="gutter__btn-dot" aria-hidden="true"></span>
      </button>
    {:else}
      <button
        type="button"
        class="gutter__btn gutter__btn--leaf shrink-0"
        tabindex="-1"
        aria-disabled="true"
        aria-label="No child spans"
      ></button>
    {/if}
  </div>
</div>

<style lang="postcss">
  @reference "../../../app.css";
  .gutter {
    --leaf-dot-size: var(--waterfall-gutter-leaf);
    --tree-line-color: color-mix(in srgb, var(--color-muted) 50%, transparent);
    @apply relative flex-shrink-0;
    height: var(--table-row-h);
  }

  .gutter__col {
    @apply absolute top-0 bottom-0;
  }

  /* ───── Segment shapes ───── */

  .seg {
    @apply pointer-events-none absolute w-px;
    background-color: var(--tree-line-color);
    left: 50%;
    transform: translateX(-50%);
  }

  /* │  full-height vertical */
  .seg--passthrough {
    top: 0;
    bottom: 0;
  }

  /* ├  full-height vertical (arm added separately) */
  .seg--tee {
    top: 0;
    bottom: 0;
  }

  /* ── horizontal arm from column center rightward */
  .seg__arm {
    --target-dot-size: var(--waterfall-gutter-hub);
    @apply pointer-events-none absolute border-b;
    border-color: var(--tree-line-color);
    top: calc(50% - 0.5px);
    left: 50%;
    height: 0;
    /* Static, so it lives here rather than as a style: directive. Inline, the
       calc exceeded printWidth and prettier-plugin-svelte wrapped it inside
       the quotes, which svelte-check reads as an unterminated string. */
    width: calc(var(--waterfall-gutter-col) - var(--target-dot-size) / 2);
  }

  .seg__arm--elbow {
    top: 0;
    left: calc(50% - 0.5px);
    height: calc(50% + 0.5px);
    width: calc(var(--waterfall-gutter-col) - var(--target-dot-size) / 2 + 0.5px);
    border-left: 1px solid var(--tree-line-color);
    border-bottom-left-radius: var(--radius-xl);
  }

  .seg__arm--leaf {
    --target-dot-size: var(--leaf-dot-size);
  }

  /* ───── Node slot (hub button column) ───── */

  .gutter__node {
    --hub-px: var(--waterfall-gutter-hub);
    @apply absolute top-0 bottom-0 z-[1] flex items-center justify-center;
  }

  .gutter__stem {
    @apply pointer-events-none absolute z-0 w-px -translate-x-1/2;
    background-color: var(--tree-line-color);
    left: 50%;
    top: calc(50% + (var(--hub-px) / 2));
    bottom: 0;
  }

  .gutter__btn--hub {
    @apply relative z-[1] inline-flex items-center justify-center rounded-full border-0 bg-transparent p-0 text-base-content;
    width: var(--waterfall-gutter-hit);
    height: var(--waterfall-gutter-hit);
  }

  .gutter__btn-dot {
    @apply absolute;
    width: var(--waterfall-gutter-hub);
    height: var(--waterfall-gutter-hub);
    border-radius: 50%;
    background-color: var(--tree-accent);
  }

  /* ───── Button base ───── */

  .gutter__btn {
    @apply cursor-pointer outline-none transition-[background-color,color] duration-100;
    @apply focus-visible:ring-2 focus-visible:ring-primary/40 focus-visible:ring-offset-1 focus-visible:ring-offset-base-100;
  }

  .gutter__btn--leaf {
    @apply cursor-default rounded-full p-0;
    width: var(--leaf-dot-size);
    height: var(--leaf-dot-size);
    min-width: var(--leaf-dot-size);
    min-height: var(--leaf-dot-size);
    border: 0;
    background-color: var(--tree-accent);
    pointer-events: none;
  }
</style>
