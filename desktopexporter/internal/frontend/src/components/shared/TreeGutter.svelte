<script module lang="ts">
  export type TreeConnectorMeta = {
    childrenCount: number
    ancestorHasNextSibling: boolean[]
    isLastChild: boolean
  }
</script>

<script lang="ts">
  import type { Snippet } from 'svelte'
  import TreeConnector from '@/components/shared/TreeConnector.svelte'
  type SegmentKind = 'none' | 'passthrough' | 'tee' | 'elbow'

  type Props = {
    depth: number
    tree: TreeConnectorMeta
    /** Concrete colour (palette HCL or `--color-*` var) for the node dot.
     *  Error spans already pass
     *  `--color-error` here, so no separate `isError` branch is needed. */
    color?: string
    node?: Snippet
    subtreeCollapsed: boolean
  }

  let { depth, tree, color, node, subtreeCollapsed }: Props = $props()

  let hasChildren = $derived(tree.childrenCount > 0)

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

<span
  class="gutter"
  aria-hidden={node ? undefined : true}
  style:width="calc({depth + 1} * var(--waterfall-gutter-col) + var(--waterfall-gutter-tail))"
  style:--tree-accent={color}
>
  {#each segments as kind, d}
    <span
      class="gutter__col"
      style:left="calc({d} * var(--waterfall-gutter-col))"
      style:width="var(--waterfall-gutter-col)"
    >
      {#if kind !== 'none'}
        <TreeConnector
          {kind}
          targetSize={hasChildren
            ? 'var(--waterfall-gutter-hub)'
            : 'var(--waterfall-gutter-leaf)'}
        />
      {/if}
    </span>
  {/each}

  <span
    class="gutter__node"
    style:left="calc({depth} * var(--waterfall-gutter-col))"
    style:width="var(--waterfall-gutter-col)"
  >
    {#if hasChildren && !subtreeCollapsed}
      <TreeConnector kind="stem" />
    {/if}
    {#if node}{@render node()}{:else}<span
        class="gutter__dot"
        class:gutter__dot--leaf={!hasChildren}
      ></span>{/if}
  </span>
</span>

<style lang="postcss">
  @reference "../../app.css";
  .gutter {
    --leaf-dot-size: var(--waterfall-gutter-leaf);
    @apply relative block flex-shrink-0 self-stretch;
    min-height: var(--table-row-h);
  }

  .gutter__col {
    @apply absolute top-0 bottom-0;
  }

  /* ───── Decorative node slot ───── */

  .gutter__node {
    --hub-px: var(--waterfall-gutter-hub);
    @apply absolute top-0 bottom-0 z-[1] flex items-center justify-center;
  }

  .gutter__dot {
    @apply relative z-[1] rounded-full;
    width: var(--waterfall-gutter-hub);
    height: var(--waterfall-gutter-hub);
    background-color: var(--tree-accent);
  }

  .gutter__dot--leaf {
    width: var(--leaf-dot-size);
    height: var(--leaf-dot-size);
  }
</style>
