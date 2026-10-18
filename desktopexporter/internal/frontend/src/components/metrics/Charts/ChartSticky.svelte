<script lang="ts">
  import type { Snippet } from 'svelte'
  import {
    chartStickyPosition,
    type ChartAnchor,
  } from '@/components/metrics/utils/chart-sticky-position'

  let { anchor, children }: { anchor: ChartAnchor | null; children: Snippet } =
    $props()
  let width = $state(0)
  let height = $state(0)
  let cardWidth = $state(0)
  let cardHeight = $state(0)
  let position = $derived(
    chartStickyPosition(anchor, width, height, cardWidth, cardHeight)
  )
</script>

<div class="chart-sticky" bind:clientWidth={width} bind:clientHeight={height}>
  <div
    class="chart-sticky__card"
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
    @apply absolute flex flex-col items-end gap-1 overflow-auto;
    width: max-content;
    max-width: calc(100% - 1rem);
    max-height: calc(100% - 1rem);
  }
  .chart-sticky__card :global(.chart-selection-legend) {
    max-width: 100%;
    min-width: 0;
  }
</style>
