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
    formatDateTime(startMs, timeContext.timezone, 'milliseconds')
  )
  let endLabel = $derived(
    formatDateTime(endMs, timeContext.timezone, 'milliseconds')
  )
</script>

<div
  class="flex items-baseline gap-1.5 py-1.5 text-xs text-rp-subtle {className}"
  aria-label="Chart time range"
>
  <span class="text-xs text-rp-subtle">start:</span>
  <span class="tabular-nums text-base-content">{startLabel}</span>
  <span class="text-xs text-rp-subtle">end:</span>
  <span class="tabular-nums text-base-content">{endLabel}</span>
</div>
