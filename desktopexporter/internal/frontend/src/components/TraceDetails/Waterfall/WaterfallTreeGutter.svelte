<script module lang="ts">
  export type TreeConnectorMeta = {
    childrenCount: number
    ancestorHasNextSibling: boolean[]
    isLastChild: boolean
  }
</script>

<script lang="ts">
  import type { WaterfallRowData } from './WaterfallView.svelte'

  /** Same as `row.colorToken` — derived so palette changes never desync from the row type. */
  type BarColorToken = WaterfallRowData['colorToken']

  type SegmentKind = 'none' | 'passthrough' | 'tee' | 'elbow'

  type Props = {
    depth: number
    tree: TreeConnectorMeta
    colorToken: BarColorToken
    subtreeCollapsed: boolean
    onToggleExpand: () => void
  }

  let { depth, tree, colorToken, subtreeCollapsed, onToggleExpand }: Props =
    $props()

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
        <div
          class="seg__arm"
          style:width="calc(var(--waterfall-gutter-col) - var(--waterfall-gutter-hub) / 2)"
        ></div>
      {:else if kind === 'elbow'}
        <div class="seg seg--elbow"></div>
        <div
          class="seg__arm seg__arm--elbow"
          style:width="calc(var(--waterfall-gutter-col) - var(--waterfall-gutter-hub) / 2)"
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
        class="gutter__btn gutter__btn--{colorToken} gutter__btn--hub shrink-0"
        class:gutter__btn--expanded={!subtreeCollapsed}
        class:gutter__btn--collapsed={subtreeCollapsed}
        aria-expanded={!subtreeCollapsed}
        aria-label={subtreeCollapsed
          ? `Expand ${childCount} child spans`
          : `Collapse ${childCount} child spans`}
        onclick={e => {
          e.stopPropagation()
          onToggleExpand()
        }}
      >
        <span class="gutter__btn-label tabular-nums">{childCount}</span>
        <svg
          class="gutter__btn-caret"
          class:gutter__btn-caret--down={!subtreeCollapsed}
          viewBox="0 0 24 24"
          fill="none"
          aria-hidden="true"
        >
          <circle cx="12" cy="12" r="10" stroke="currentColor" stroke-width="1.5" />
          <path d="M10 8l4 4-4 4" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round" />
        </svg>
      </button>
    {:else}
      <button
        type="button"
        class="gutter__btn gutter__btn--{colorToken} gutter__btn--leaf shrink-0"
        tabindex="-1"
        aria-disabled="true"
        aria-label="No child spans"
      >
        <span class="tabular-nums">0</span>
      </button>
    {/if}
  </div>
</div>

<style lang="postcss">
  @reference "../../../app.css";
  .gutter {
    @apply relative flex-shrink-0;
    height: 28px;
  }

  .gutter__col {
    @apply absolute top-0 bottom-0;
  }

  /* ───── Segment shapes ───── */

  .seg {
    @apply pointer-events-none absolute w-px bg-base-content/15;
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

  /* └  top-half vertical (arm added separately) */
  .seg--elbow {
    top: 0;
    bottom: 50%;
  }

  /* ── horizontal arm from column center rightward */
  .seg__arm {
    @apply pointer-events-none absolute border-b border-base-content/15;
    top: 50%;
    left: 50%;
    height: 0;
  }

  .seg__arm--elbow {
    border-bottom-left-radius: 4px;
  }

  /* ───── Node slot (hub button column) ───── */

  .gutter__node {
    --hub-px: var(--waterfall-gutter-hub);
    @apply absolute top-0 bottom-0 z-[1] flex items-center justify-center;
  }

  .gutter__stem {
    @apply pointer-events-none absolute z-0 w-px -translate-x-1/2 bg-base-content/15;
    left: 50%;
    top: calc(50% + (var(--hub-px) / 2));
    bottom: 0;
  }

  .gutter__btn--hub {
    @apply relative z-[1];
  }

  .gutter__btn-label,
  .gutter__btn-caret {
    @apply absolute;
    transition: opacity 0.1s;
  }

  .gutter__btn-label {
    opacity: 1;
  }

  .gutter__btn-caret {
    width: var(--waterfall-gutter-hub);
    height: var(--waterfall-gutter-hub);
    opacity: 0;
    transition: opacity 0.1s, transform 0.15s;
  }

  .gutter__btn-caret--down {
    transform: rotate(90deg);
  }

  .gutter__btn--hub:hover .gutter__btn-label {
    opacity: 0;
  }

  .gutter__btn--hub:hover .gutter__btn-caret {
    opacity: 1;
  }

  /* ───── Button base ───── */

  .gutter__btn {
    @apply inline-flex cursor-pointer items-center justify-center rounded-full border-0 p-0 font-semibold leading-none outline-none transition-[background-color,color] duration-100;
    @apply focus-visible:ring-2 focus-visible:ring-primary/40 focus-visible:ring-offset-1 focus-visible:ring-offset-base-100;
    min-width: var(--waterfall-gutter-hub);
    min-height: var(--waterfall-gutter-hub);
    width: var(--waterfall-gutter-hub);
    height: var(--waterfall-gutter-hub);
    font-size: 8px;
  }

  /* ───── Color tokens ───── */

  .gutter__btn--gold {
    --tree-accent: var(--color-warning);
  }
  .gutter__btn--pine {
    --tree-accent: var(--color-secondary);
  }
  .gutter__btn--foam {
    --tree-accent: var(--color-accent);
  }
  .gutter__btn--iris {
    --tree-accent: var(--color-primary);
  }
  .gutter__btn--rose {
    --tree-accent: var(--color-rose);
  }
  .gutter__btn--error {
    --tree-accent: var(--color-error);
  }

  /* ───── Button states ───── */

  .gutter__btn--expanded,
  .gutter__btn--collapsed {
    background-color: color-mix(in srgb, var(--tree-accent) 18%, transparent);
    color: var(--tree-accent);
  }

  .gutter__btn--expanded:hover,
  .gutter__btn--collapsed:hover {
    background-color: color-mix(in srgb, var(--tree-accent) 28%, transparent);
  }

  .gutter__btn--leaf {
    @apply cursor-default bg-base-300/35 opacity-80 ring-1 ring-inset ring-base-content/10;
    pointer-events: none;
    color: var(--tree-accent);
  }
</style>
