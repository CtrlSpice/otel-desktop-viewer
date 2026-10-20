<script lang="ts">
  import type { WaterfallGridLayout } from './WaterfallTimeAxisHeader.svelte'

  let { layout }: { layout: WaterfallGridLayout } = $props()
</script>

<div
  class="waterfall-grid"
  style:width="calc({layout.widthPx}px + var(--guide-dot-radius))"
  aria-hidden="true"
>
  {#each layout.guides as guide}
    <span
      class="waterfall-grid__guide dot-guide-vertical"
      class:dot-guide-vertical--minor={!guide.major}
      style:left="calc({guide.offsetPx}px + var(--guide-dot-radius))"
    ></span>
  {/each}
</div>

<style lang="postcss">
  @reference "../../../app.css";

  .waterfall-grid {
    @apply pointer-events-none absolute top-0 bottom-0 z-0 overflow-hidden;
    left: calc(0px - var(--guide-dot-radius));
  }

  .waterfall-grid__guide {
    @apply absolute top-0 bottom-0 -translate-x-1/2;
  }
</style>
