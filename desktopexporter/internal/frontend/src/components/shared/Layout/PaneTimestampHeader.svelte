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
  class="flex items-center gap-2 text-xs leading-normal text-subtle {className}"
  aria-label="Log timestamp"
>
  <span class="text-xs text-subtle">timestamp:</span>
  <span class="tabular-nums text-base-content">{timestampLabel}</span>
</div>
