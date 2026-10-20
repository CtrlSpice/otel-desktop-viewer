<script lang="ts">
  import type { Snippet } from 'svelte'
  import {
    chartStickyPosition,
    type ChartAnchor,
    type ChartStickySize,
  } from '@/components/metrics/utils/chart-sticky-position'

  let {
    anchor,
    children,
    positioner,
  }: {
    anchor: ChartAnchor | null
    children: Snippet
    positioner?: (size: ChartStickySize) => { left: number; top: number }
  } = $props()
  let width = $state(0)
  let height = $state(0)
  let cardWidth = $state(0)
  let cardHeight = $state(0)
  let position = $derived(
    positioner
      ? positioner({ width, height, cardWidth, cardHeight })
      : chartStickyPosition(anchor, width, height, cardWidth, cardHeight)
  )
</script>

<div class="chart-sticky" bind:clientWidth={width} bind:clientHeight={height}>
  <!-- Keyboard focus enables native scrolling when the pinned readout overflows. -->
  <!-- svelte-ignore a11y_no_noninteractive_tabindex -->
  <div
    class="chart-sticky__card"
    role="region"
    aria-label="Pinned chart selection"
    tabindex="0"
    bind:offsetWidth={cardWidth}
    bind:offsetHeight={cardHeight}
    style:left="{position.left}px"
    style:top="{position.top}px"
  >
    {@render children()}
  </div>
</div>

<style lang="postcss">
  @reference "../../../app.css";
  .chart-sticky {
    @apply pointer-events-none absolute inset-0 z-[3];
  }
  .chart-sticky__card {
    @apply pointer-events-auto absolute flex flex-col items-end gap-1 overflow-auto;
    width: max-content;
    max-width: calc(100% - 1rem);
    max-height: calc(100% - 1rem);
    overscroll-behavior: contain;
  }
  .chart-sticky__card:focus-visible {
    outline: 2px solid var(--color-primary);
    outline-offset: -2px;
  }
  .chart-sticky__card :global(.chart-selection-legend) {
    pointer-events: auto;
    flex-shrink: 0;
    max-width: 100%;
    min-width: 0;
  }
</style>
