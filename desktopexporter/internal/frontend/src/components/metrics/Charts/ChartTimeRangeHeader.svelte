<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatDateTime } from '@/utils/time'

  type Props = {
    startMs: number
    endMs: number
    class?: string
  }

  let { startMs, endMs, class: className = '' }: Props = $props()

  const timeContext = getTimeContext()

  let startLabel = $derived(
    formatDateTime(startMs, timeContext.tz, 'milliseconds')
  )
  let endLabel = $derived(formatDateTime(endMs, timeContext.tz, 'milliseconds'))
</script>

<div
  class="flex flex-wrap items-baseline gap-x-3 py-1.5 text-xs text-rp-subtle {className}"
  aria-label="Chart time range"
>
  <span class="flex items-baseline gap-1.5">
    <span class="text-xs text-rp-subtle">start:</span>
    <span class="tabular-nums text-base-content">{startLabel}</span>
  </span>
  <span class="flex items-baseline gap-1.5">
    <span class="text-xs text-rp-subtle">end:</span>
    <span class="tabular-nums text-base-content">{endLabel}</span>
  </span>
</div>
