<script lang="ts">
  import type { LogSummary } from '@/types/api-types'
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import SignalBadges from '@/components/shared/SignalBadges.svelte'
  import { formatTimestampParts } from '@/utils/time'
  import { getTimeContext } from '@/contexts/time-context.svelte'

  type Props = {
    log: LogSummary
    selected?: boolean
    onclick?: (id: string) => void
  }

  let { log, selected = false, onclick }: Props = $props()

  const timeContext = getTimeContext()

  let timestampParts = $derived(
    formatTimestampParts(log.timestamp, timeContext.tz, 'milliseconds')
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
    <SignalBadges
      signal="log"
      severityNumber={log.severityNumber}
      severityText={log.severityText}
    />
  {/snippet}
</SignalCard>
