<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatDateTime } from '@/utils/time'

  type Props = {
    startMs: number
    endMs: number
    class?: string
    /** Card styled like the chart selection overlay. */
    variant?: 'inline' | 'legend'
    /**
     * The range shown is the data's own, not the one selected.
     *
     * Says so rather than cropping quietly: the reader is looking at a
     * narrower window than they asked for, and that is worth one word. Only
     * ever true when no window was asked for -- an explicit selection is
     * always drawn as given, because the emptiness in it is part of the
     * answer.
     */
    fitToData?: boolean
  }

  let {
    startMs,
    endMs,
    class: className = '',
    variant = 'inline',
    fitToData = false,
  }: Props = $props()

  const timeContext = getTimeContext()

  let startLabel = $derived(
    formatDateTime(startMs, timeContext.tz, 'milliseconds')
  )
  let endLabel = $derived(formatDateTime(endMs, timeContext.tz, 'milliseconds'))
</script>

{#if variant === 'legend'}
  <div
    class="chart-selection-legend chart-selection-legend--stat chart-time-range-legend {className}"
    aria-label="Chart time range"
  >
    <div class="chart-time-range-legend__row">
      <span class="chart-time-range-legend__prefix">start:</span>
      <span class="chart-time-range-legend__value">{startLabel}</span>
    </div>
    <div class="chart-time-range-legend__row">
      <span class="chart-time-range-legend__prefix">end:</span>
      <span class="chart-time-range-legend__value">{endLabel}</span>
    </div>
    {#if fitToData}
      <div class="chart-time-range-legend__row">
        <span class="chart-time-range-legend__prefix">fitted to data</span>
      </div>
    {/if}
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
    {#if fitToData}
      <span
        class="text-xs text-rp-subtle italic"
        title="No time range selected, so the chart is fitted to this metric's own data."
        >fitted to data</span
      >
    {/if}
  </div>
{/if}
