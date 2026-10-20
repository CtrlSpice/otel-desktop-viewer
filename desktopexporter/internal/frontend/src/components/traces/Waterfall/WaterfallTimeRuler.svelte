<script lang="ts">
  import type {
    WaterfallAxis,
    WaterfallGridLayout,
  } from './WaterfallTimeAxisHeader.svelte'
  import WaterfallGrid from './WaterfallGrid.svelte'
  let { axis, layout }: { axis: WaterfallAxis; layout: WaterfallGridLayout } =
    $props()
</script>

<div class="waterfall-time-ruler">
  <WaterfallGrid {layout} />
  {#each axis.ticks as tick}
    <div
      class="waterfall-time-ruler__tick"
      style:left={`${tick.offsetPercent}%`}
    >
      <span class="waterfall-time-ruler__label">{tick.label}</span>
    </div>
  {/each}
</div>

<style lang="postcss">
  @reference "../../../app.css";
  .waterfall-time-ruler {
    @apply absolute inset-0 min-w-0;
  }
  .waterfall-time-ruler :global(.waterfall-grid) {
    top: calc(var(--primary-label-center, 50%) + 0.5lh);
  }
  .waterfall-time-ruler__tick {
    @apply absolute bottom-0 h-full;
  }
  .waterfall-time-ruler__label {
    @apply absolute text-sm font-normal tracking-normal text-center;
    top: var(--primary-label-center, 50%);
    width: var(--tick-label-w, 80px);
    margin-left: calc(var(--tick-label-w, 80px) / -2);
    transform: translateY(-50%);
  }
</style>
