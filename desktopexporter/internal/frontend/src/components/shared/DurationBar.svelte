<script lang="ts">
  import { formatDuration } from '@/utils/time'
  import SpanBar from './SpanBar.svelte'

  let {
    duration,
    maximum,
    offset = 0n,
  }: {
    duration: bigint
    maximum: bigint
    offset?: bigint
  } = $props()
  const position = $derived(
    maximum > 0n ? Number((offset * 10000n) / maximum) / 100 : 0
  )
  const width = $derived(
    maximum > 0n ? Number((duration * 10000n) / maximum) / 100 : 0
  )
</script>

<span class="duration-bar">
  <span class="duration-bar__track">
    <SpanBar offsetPercent={position} widthPercent={width} />
    <span
      class="duration-bar__label"
      style:left={`calc(${position + width}% + 1rem)`}
      >{formatDuration(duration)}</span
    >
  </span>
</span>

<style lang="postcss">
  @reference "../../app.css";
  .duration-bar {
    @apply relative block min-h-[var(--signal-card-row-h)] min-w-0;
  }
  .duration-bar__track {
    @apply absolute inset-y-0 left-0 right-24;
  }
  .duration-bar__label {
    @apply absolute top-1/2 -translate-y-1/2 whitespace-nowrap tabular-nums;
  }
</style>
