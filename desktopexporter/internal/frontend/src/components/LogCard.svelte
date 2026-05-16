<script lang="ts">
  import type { LogSummary } from '@/types/api-types'
  import SignalCard from '@/components/SignalCard.svelte'
  import { formatTimestampParts } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'

  type SeverityBand = 'trace' | 'debug' | 'info' | 'warn' | 'error' | 'fatal'

  type Props = {
    log: LogSummary
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

  let timestampParts = $derived(
    formatTimestampParts(log.timestamp, timeContext.timezone, 'milliseconds')
  )

  let serviceTitle = $derived(log.serviceName?.trim() || '(unknown service)')
  let bodyPreview = $derived((log.bodyPreview ?? '').trim())
</script>

<SignalCard
  id={log.id}
  {selected}
  title={serviceTitle}
  description={bodyPreview || undefined}
  timeLayout="labeled"
  timestamp={timestampParts.value}
  timestampUnit={timestampParts.unit || undefined}
  {onclick}
>
  {#snippet badge()}
    <span class="{BADGE_CLASS[band]} tabular-nums" title={severityTitle}>
      {severityTitle} ({log.severityNumber})
    </span>
  {/snippet}
</SignalCard>
