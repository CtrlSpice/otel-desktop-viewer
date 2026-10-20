<script lang="ts">
  import type { Snippet } from 'svelte'
  import WaterfallTimeRuler from '@/components/traces/Waterfall/WaterfallTimeRuler.svelte'
  import WaterfallTimeGuides from '@/components/traces/Waterfall/WaterfallTimeGuides.svelte'
  import { waterfallTimeAxis } from '@/components/traces/Waterfall/WaterfallTimeAxisHeader.svelte'
  let {
    maximum,
    header = false,
    unavailable = false,
    children,
  }: {
    maximum: bigint
    header?: boolean
    unavailable?: boolean
    children?: Snippet
  } = $props()
  let width = $state(0)
  const axis = $derived(
    waterfallTimeAxis(maximum, Math.max(1, Math.floor(width / 80)))
  )
</script>

<div class="timing-cell">
  <div class="timing-cell__plot" bind:clientWidth={width}>
    {#if header}<WaterfallTimeRuler {axis} />{:else}<WaterfallTimeGuides
        {axis}
      />{/if}
    {#if unavailable}<span class="timing-cell__unavailable" aria-hidden="true"
      ></span>{/if}
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
  .timing-cell__unavailable {
    position: absolute;
    left: 0;
    right: 0;
    top: 50%;
    height: 1px;
    opacity: 0.35;
    pointer-events: none;
    background: linear-gradient(
      to right,
      var(--color-muted) calc(50% - 4px),
      transparent calc(50% - 4px),
      transparent calc(50% + 4px),
      var(--color-muted) calc(50% + 4px)
    );
  }
</style>
