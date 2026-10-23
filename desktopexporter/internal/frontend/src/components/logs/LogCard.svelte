<script lang="ts">
  import type { LogSummary } from '@/types/api-types'
  import SignalCard from '@/components/shared/SignalCard.svelte'
  import SignalBadges from '@/components/shared/SignalBadges.svelte'
  import SignalDate from '@/components/shared/SignalDate.svelte'
  import ServiceDot from '@/components/shared/ServiceDot.svelte'

  type Props = {
    log: LogSummary
    selected?: boolean
    showLogBadge?: boolean
    offset?: string
    onclick?: (id: string) => void
    href?: string
  }

  let {
    log,
    selected = false,
    showLogBadge = false,
    offset,
    onclick,
    href,
  }: Props = $props()

  let serviceTitle = $derived(log.serviceName?.trim() || '(unknown service)')
  let bodyPreview = $derived((log.bodyPreview ?? '').trim())
</script>

<SignalCard
  id={log.logRef}
  {selected}
  title={serviceTitle}
  description={bodyPreview || undefined}
  {onclick}
  {href}
>
  {#snippet date()}<SignalDate
      label="Recorded"
      timestamp={log.timestamp}
      suffix={offset ? `(${offset})` : undefined}
    />{/snippet}
  {#snippet titleContent()}
    <ServiceDot service={log.serviceName ?? ''} />
    {serviceTitle}{#if showLogBadge}<span class="text-xs text-rp-subtle"
        >{` · Log`}</span
      >{/if}
  {/snippet}
  {#snippet badge()}
    <SignalBadges
      signal="log"
      severityNumber={log.severityNumber}
      severityText={log.severityText}
    />
  {/snippet}
</SignalCard>
