<script lang="ts">
  import type { WaterfallAxis } from './WaterfallTimeAxisHeader.svelte'

  let { axis }: { axis: WaterfallAxis } = $props()
</script>

<div class="waterfall-time-guides" aria-hidden="true">
  {#each axis.ticks as tick}
    <span style:left={`${tick.offsetPercent}%`}></span>
  {/each}
</div>

<style>
  .waterfall-time-guides {
    position: absolute;
    inset: 0;
    pointer-events: none;
    overflow: hidden;
  }

  .waterfall-time-guides span {
    position: absolute;
    top: 0;
    bottom: 0;
    width: 1px;
    background: var(--color-muted);
    opacity: 0.35;
  }
</style>
