<script lang="ts">
  import type {
    WaterfallAxis,
  } from './WaterfallTimeAxisHeader.svelte'
  let { axis }: { axis: WaterfallAxis } =
    $props()
</script>

<div class="waterfall-time-ruler">
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
  .waterfall-time-ruler__tick {
    @apply absolute bottom-0 h-full;
  }
  .waterfall-time-ruler__tick::after {
    content: '';
    position: absolute;
    bottom: 0;
    width: 1px;
    height: 4px;
    background: var(--color-muted);
    opacity: 0.35;
    pointer-events: none;
  }
  .waterfall-time-ruler__label {
    @apply absolute text-sm font-normal tracking-normal text-center;
    top: var(--primary-label-center, 50%);
    width: var(--tick-label-w, 80px);
    margin-left: calc(var(--tick-label-w, 80px) / -2);
    transform: translateY(-50%);
  }
</style>
