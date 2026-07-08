<script lang="ts">
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { formatDateTime } from '@/utils/time'

  type Props = {
    timestampMs: number
    class?: string
  }

  let { timestampMs, class: className = '' }: Props = $props()

  const timeContext = getTimeContext()

  let timestampLabel = $derived(
    formatDateTime(timestampMs, timeContext.tz, 'milliseconds')
  )
</script>

<div
  class="flex items-baseline gap-1.5 py-1.5 text-xs text-rp-subtle {className}"
  aria-label="Log timestamp"
>
  <span class="text-xs text-rp-subtle">timestamp:</span>
  <span class="tabular-nums text-base-content">{timestampLabel}</span>
</div>
