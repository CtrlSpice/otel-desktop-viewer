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
    activity?: {
      name: string
      source?: string
      description?: string
      toolError?: boolean
    }
  }

  let {
    log,
    selected = false,
    showLogBadge = false,
    offset,
    onclick,
    href,
    activity,
  }: Props = $props()

  let serviceTitle = $derived(log.serviceName?.trim() || '(unknown service)')
  let bodyPreview = $derived((log.bodyPreview ?? '').trim())
  let displayTitle = $derived(activity?.name || serviceTitle)
</script>

<SignalCard
  id={log.logRef}
  {selected}
  title={activity ? `${displayTitle} · ${serviceTitle}` : serviceTitle}
  description={bodyPreview && bodyPreview !== activity?.name
    ? bodyPreview
    : undefined}
  singleLineDescription
  {onclick}
  {href}
>
  {#snippet date()}<SignalDate
      label="Recorded"
      timestamp={log.timestamp}
      suffix={offset ? `(${offset})` : undefined}
    />{/snippet}
  {#snippet titleContent()}
    <span
      class="inline-flex max-w-full flex-wrap items-center gap-x-2 text-base-content"
    >
      <ServiceDot service={log.serviceName ?? ''} />
      <span class="truncate">{displayTitle}</span>
      {#if showLogBadge}<span class="shrink-0 text-sm text-secondary"
          ><span class="text-subtle" aria-hidden="true">{'· '}</span><span
            >{activity && log.eventName ? 'Log event' : 'Log'}</span
          ></span
        >{/if}
      {#if activity?.source}
        <span class="shrink-0 text-sm text-subtle" aria-hidden="true">·</span>
        <span
          class="shrink-0 text-sm text-secondary"
          title={activity.description}>{activity.source}</span
        >
      {/if}
      {#if activity?.toolError}<span class="shrink-0 text-sm">· Tool error</span
        >{/if}
    </span>
  {/snippet}
  {#snippet badge()}
    <SignalBadges
      signal="log"
      severityNumber={log.severityNumber}
      severityText={log.severityText}
    />
  {/snippet}
</SignalCard>
