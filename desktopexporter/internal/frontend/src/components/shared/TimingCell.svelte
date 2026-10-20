<script lang="ts">
  import type { Snippet } from 'svelte'
  import WaterfallGrid from '@/components/traces/Waterfall/WaterfallGrid.svelte'
  import WaterfallTimeRuler from '@/components/traces/Waterfall/WaterfallTimeRuler.svelte'
  import {
    waterfallTimeAxis,
    waterfallGrid,
  } from '@/components/traces/Waterfall/WaterfallTimeAxisHeader.svelte'
  let {
    maximum,
    header = false,
    children,
  }: { maximum: bigint; header?: boolean; children?: Snippet } = $props()
  let width = $state(0)
  const axis = $derived(
    waterfallTimeAxis(maximum, Math.max(1, Math.floor(width / 80)))
  )
  const layout = $derived(waterfallGrid(axis, width))
</script>

<div class="timing-cell">
  <div class="timing-cell__plot" bind:clientWidth={width}>
    {#if header}<WaterfallTimeRuler {axis} {layout} />{:else}<WaterfallGrid
        {layout}
      />{/if}
  </div>
  <div class="timing-cell__value">{@render children?.()}</div>
</div>

<style lang="postcss">
  @reference "../../app.css";
  .timing-cell {
    @apply relative flex h-full w-full min-w-0 items-center;
    min-height: var(--table-row-h);
  }
  .timing-cell__plot {
    @apply absolute inset-y-0 left-0 right-24;
  }
  .timing-cell__value {
    @apply relative z-[1] w-full min-w-0;
  }
</style>
