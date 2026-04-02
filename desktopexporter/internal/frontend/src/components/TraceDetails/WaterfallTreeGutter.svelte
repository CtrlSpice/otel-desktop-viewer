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

  /** Column width (px) for each tree level. */
  const COL = 22
  /** Must match btn width/height — horizontal arms stop at the circle edge. */
  const HUB_PX = 16

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
  let gutterWidthPx = $derived((depth + 1) * COL + 4)
  let armWidthPx = $derived(COL - HUB_PX / 2)

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

<div class="gutter" style:width="{gutterWidthPx}px">
  {#each segments as kind, d}
    <div class="gutter__col" style:left="{d * COL}px" style:width="{COL}px">
      {#if kind === 'passthrough'}
        <div class="seg seg--passthrough"></div>
      {:else if kind === 'tee'}
        <div class="seg seg--tee"></div>
        <div class="seg__arm" style:width="{armWidthPx}px"></div>
      {:else if kind === 'elbow'}
        <div class="seg seg--elbow"></div>
        <div
          class="seg__arm seg__arm--elbow"
          style:width="{armWidthPx}px"
        ></div>
      {/if}
    </div>
  {/each}

  <div class="gutter__node" style:left="{depth * COL}px" style:width="{COL}px">
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
        <span class="tabular-nums">{childCount}</span>
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
    --hub-px: 16px;
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

  /* ───── Button base ───── */

  .gutter__btn {
    @apply inline-flex cursor-pointer items-center justify-center rounded-full border-0 p-0 font-semibold leading-none outline-none transition-[background-color,color] duration-100;
    @apply focus-visible:ring-2 focus-visible:ring-primary/40 focus-visible:ring-offset-1 focus-visible:ring-offset-base-100;
    min-width: 16px;
    min-height: 16px;
    width: 16px;
    height: 16px;
    font-size: 8px;
  }

  /* ───── Color tokens ───── */

  .gutter__btn--gold {
    --tree-accent: var(--rp-gold);
  }
  .gutter__btn--pine {
    --tree-accent: var(--rp-pine);
  }
  .gutter__btn--foam {
    --tree-accent: var(--rp-foam);
  }
  .gutter__btn--iris {
    --tree-accent: var(--rp-iris);
  }
  .gutter__btn--rose {
    --tree-accent: var(--rp-rose);
  }
  .gutter__btn--error {
    --tree-accent: var(--rp-love);
  }

  /* ───── Button states ───── */

  .gutter__btn--expanded {
    background-color: color-mix(in srgb, var(--tree-accent) 24%, transparent);
    color: var(--tree-accent);
  }

  .gutter__btn--expanded:hover {
    background-color: color-mix(in srgb, var(--tree-accent) 34%, transparent);
  }

  .gutter__btn--collapsed {
    background-color: color-mix(in srgb, var(--tree-accent) 12%, transparent);
    color: color-mix(in srgb, var(--tree-accent) 42%, var(--rp-muted));
  }

  .gutter__btn--collapsed:hover {
    background-color: color-mix(in srgb, var(--tree-accent) 20%, transparent);
    color: color-mix(in srgb, var(--tree-accent) 58%, var(--rp-muted));
  }

  .gutter__btn--leaf {
    @apply cursor-default bg-base-300/35 opacity-80 ring-1 ring-inset ring-base-content/10;
    pointer-events: none;
    color: var(--tree-accent);
  }
</style>
