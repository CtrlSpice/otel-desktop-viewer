<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { getChartTimeRangeLabels } from '@/utils/chart-time-axis'

  type Props = {
    startMs: number
    endMs: number
  }

  let { startMs, endMs }: Props = $props()

  const timeContext = getTimeContext()

  let labels = $derived(
    getChartTimeRangeLabels(startMs, endMs, timeContext.timezone),
  )
</script>

<div
  class="flex items-baseline gap-2 border-b border-base-300/40 px-2 py-1.5 text-xs text-rp-subtle"
  aria-label="Chart time range"
>
  <span class="tabular-nums">{labels.start}</span>
  {#if labels.end}
    <span aria-hidden="true">–</span>
    <span class="tabular-nums">{labels.end}</span>
  {/if}
</div>
