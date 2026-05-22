<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatDateTime } from '@/utils/time'

  type Props = {
    startMs: number
    endMs: number
    class?: string
    /** Card styled like the chart selection overlay. */
    variant?: 'inline' | 'legend'
  }

  let {
    startMs,
    endMs,
    class: className = '',
    variant = 'inline',
  }: Props = $props()

  const timeContext = getTimeContext()

  let startLabel = $derived(
    formatDateTime(startMs, timeContext.timezone, 'milliseconds')
  )
  let endLabel = $derived(
    formatDateTime(endMs, timeContext.timezone, 'milliseconds')
  )
</script>

{#if variant === 'legend'}
  <div
    class="chart-selection-legend chart-selection-legend--stat chart-time-range-legend {className}"
    aria-label="Chart time range"
  >
    <span class="chart-time-range-legend__prefix">start:</span>
    <span class="chart-time-range-legend__value">{startLabel}</span>
    <span class="chart-time-range-legend__prefix">end:</span>
    <span class="chart-time-range-legend__value">{endLabel}</span>
  </div>
{:else}
  <div
    class="flex items-baseline gap-1.5 py-1.5 text-xs text-rp-subtle {className}"
    aria-label="Chart time range"
  >
    <span class="text-xs text-rp-subtle">start:</span>
    <span class="tabular-nums text-base-content">{startLabel}</span>
    <span class="text-xs text-rp-subtle">end:</span>
    <span class="tabular-nums text-base-content">{endLabel}</span>
  </div>
{/if}
