<script lang="ts">
  import type { LogData } from '@/types/api-types'
  import SignalCard from '@/components/SignalCard.svelte'
  import { formatTimestamp } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'
  import { getServiceName } from '@/utils/resource'

  type SeverityBand = 'trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal'

  type Props = {
    log: LogData
    selected?: boolean
    onclick?: (id: string) => void
  }

  let { log, selected = false, onclick }: Props = $props()

  const timeContext = getTimeContext()

  function severityBand(n: number): SeverityBand {
    if (n <= 4) return 'trace'
    if (n <= 8) return 'debug'
    if (n <= 12) return 'info'
    if (n <= 16) return 'warn'
    if (n <= 20) return 'error'
    return 'fatal'
  }

  const BADGE_CLASS: Record<SeverityBand, string> = {
    trace: 'badge badge-xs badge-soft badge-neutral',
    debug: 'badge badge-xs badge-soft badge-info',
    info: 'badge badge-xs badge-soft badge-success',
    warn: 'badge badge-xs badge-soft badge-warning',
    error: 'badge badge-xs badge-soft badge-error',
    fatal: 'badge badge-xs badge-soft badge-error',
  }

  let band = $derived(severityBand(log.severityNumber))
  let severityTitle = $derived(log.severityText || band.toUpperCase())

  let tsLabel = $derived(
    formatTimestamp(log.timestamp, timeContext.timezone, 'milliseconds')
  )

  let service = $derived(getServiceName(log.resource))

  let bodyForMeta = $derived.by(() => log.body.trim())
</script>

<SignalCard
  id={log.id}
  {selected}
  title={service || '(unknown service)'}
  timestamp={tsLabel || undefined}
  {onclick}
>{#snippet badge()}
    <span class={BADGE_CLASS[band]}>{severityTitle}</span>
  {/snippet}{#snippet meta()}
    {#if bodyForMeta}
      <span class="w-full min-w-0 whitespace-pre-wrap break-words text-base-content/50">
        {bodyForMeta}
      </span>
    {/if}
  {/snippet}
</SignalCard>
